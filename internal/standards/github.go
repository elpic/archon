package standards

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GitHubContentsPath is the default path fetched from a remote org's
// standards repository.
const GitHubContentsPath = ".archon/standards.md"

// httpFetchTimeout bounds any single HTTP request the fetcher makes.
// A hung api.github.com response must never DoS an audit indefinitely.
// The git exec has its own 2s timeout (gitRemoteTimeout); this is for the
// network fetch that follows.
const httpFetchTimeout = 15 * time.Second

// rateLimitMarker is a substring the integration test searches for in
// GitHub's 403 body to detect unauthenticated rate-limit responses and
// skip cleanly. The unauthenticated limit is 60 req/hr/IP — shared CI
// egress IPs burn through it fast.
const rateLimitMarker = "rate limit"

// Fetcher abstracts the GitHub Contents API so the Resolver can be tested
// without network access.
type Fetcher interface {
	Fetch(ctx context.Context, owner, repo, path string) (body []byte, sha string, err error)
}

// ErrNotFound is returned when the remote file does not exist (HTTP 404).
type ErrNotFound struct {
	Owner string
	Repo  string
	Path  string
}

func (e *ErrNotFound) Error() string {
	return fmt.Sprintf("github: %s/%s/%s not found", e.Owner, e.Repo, e.Path)
}

// ErrFetch wraps a non-404 HTTP error or transport failure from the
// GitHub Contents API. The URL is preserved so the caller can surface it.
type ErrFetch struct {
	URL string
	Err error
}

func (e *ErrFetch) Error() string {
	return fmt.Sprintf("github: fetch %s: %v", e.URL, e.Err)
}

// Unwrap exposes the underlying error for errors.Is / errors.As.
func (e *ErrFetch) Unwrap() error { return e.Err }

// httpFetcher retrieves files via the GitHub Contents API.
//
// The unauthenticated rate limit is 60 requests per hour per IP.
// For CI and dogfooding this is fine; org-level usage should add a
// GitHub token via a future auth extension.
type httpFetcher struct {
	client *http.Client
	base   string
}

// NewHTTPFetcher returns a Fetcher backed by the public GitHub Contents API.
//
// It builds a dedicated *http.Client (it does NOT mutate http.DefaultClient,
// which is a shared global) with a request timeout and a CheckRedirect
// policy that refuses non-https redirects — defends against enterprise
// installs or hostile networks that try to downgrade the connection and
// leak the standards body to the LLM.
func NewHTTPFetcher() Fetcher {
	return NewHTTPFetcherWithClient(nil)
}

// NewHTTPFetcherWithClient returns a Fetcher using the given HTTP client.
// Useful for tests that want custom timeouts or transports. A nil client
// is replaced with a fresh dedicated client (with timeout + CheckRedirect),
// never with http.DefaultClient.
func NewHTTPFetcherWithClient(client *http.Client) Fetcher {
	if client == nil {
		client = &http.Client{
			Timeout: httpFetchTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if req.URL.Scheme != "https" {
					return fmt.Errorf("non-https redirect to %s", req.URL)
				}
				return nil
			},
		}
	}
	return &httpFetcher{client: client, base: "https://api.github.com"}
}

type contentsResponse struct {
	Content string `json:"content"`
	SHA     string `json:"sha"`
	Message string `json:"message"`
}

// Fetch retrieves the file at owner/repo/path from GitHub. The body is
// base64-decoded per the Contents API contract. The blob SHA is returned
// alongside the body so callers can record a content-addressed source.
//
// The URL is built via net/url (not fmt.Sprintf) so a future refactor
// cannot accidentally splice special characters into the path and have
// the stdlib reinterpret them as a query string. parseOrgRepo is the
// primary line of defense; this is defense in depth.
func (f *httpFetcher) Fetch(ctx context.Context, owner, repo, path string) ([]byte, string, error) {
	apiURL := buildContentsURL(f.base, owner, repo, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, "", &ErrFetch{URL: apiURL, Err: err}
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, "", &ErrFetch{URL: apiURL, Err: err}
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// fall through
	case http.StatusNotFound:
		return nil, "", &ErrNotFound{Owner: owner, Repo: repo, Path: path}
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, "", &ErrFetch{
			URL: apiURL,
			Err: fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body))),
		}
	}

	var cr contentsResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return nil, "", &ErrFetch{URL: apiURL, Err: fmt.Errorf("decode: %w", err)}
	}

	// GitHub returns content as base64 with embedded newlines every 60 chars.
	cleaned := strings.Join(strings.Fields(cr.Content), "")
	body, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		return nil, "", &ErrFetch{URL: apiURL, Err: fmt.Errorf("base64: %w", err)}
	}
	return body, cr.SHA, nil
}

// buildContentsURL constructs the GitHub Contents API URL for owner/repo/path.
//
// We percent-encode each path segment with url.PathEscape and set both
// Path (decoded) and RawPath (encoded) on the resulting url.URL so that:
//   - reserved characters in any segment (?, #, space, …) become %HH and
//     do NOT leak into the query string or fragment of the request;
//   - the on-the-wire URL keeps the literal '/' separators between segments.
//
// parseOrgRepo is the primary line of defense (it rejects these chars at
// the resolver layer); this is defense in depth.
func buildContentsURL(base, owner, repo, path string) string {
	u, err := url.Parse(base)
	if err != nil {
		// base is a hard-coded constant in this package; if it ever fails
		// to parse, fall back to Sprintf so the call still returns something
		// callable (the error will surface at http.NewRequestWithContext).
		return fmt.Sprintf("%s/repos/%s/%s/contents/%s", base, owner, repo, path)
	}
	raw := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/contents/" + escapePathSegments(path)
	// Build the decoded Path by round-tripping raw through url.PathUnescape.
	// (We deliberately set Path and RawPath to the matching decoded/encoded
	// pair so url.URL.String() returns raw verbatim instead of double-
	// escaping the percent characters in raw.)
	if decoded, derr := url.PathUnescape(raw); derr == nil {
		u.Path = decoded
	} else {
		u.Path = raw
	}
	u.RawPath = raw
	return u.String()
}

// escapePathSegments escapes each '/'-delimited segment of p individually,
// preserving the separator. This is needed because url.PathEscape also
// escapes '/', which would collapse a multi-segment path into a single
// segment of the resulting URL.
func escapePathSegments(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}
