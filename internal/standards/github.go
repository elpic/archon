package standards

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// GitHubContentsPath is the default path fetched from a remote org's
// standards repository.
const GitHubContentsPath = ".archon/standards.md"

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
func NewHTTPFetcher() Fetcher {
	return &httpFetcher{client: http.DefaultClient, base: "https://api.github.com"}
}

// NewHTTPFetcherWithClient returns a Fetcher using the given HTTP client.
// Useful for tests that want custom timeouts or transports.
func NewHTTPFetcherWithClient(client *http.Client) Fetcher {
	if client == nil {
		client = http.DefaultClient
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
func (f *httpFetcher) Fetch(ctx context.Context, owner, repo, path string) ([]byte, string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/contents/%s", f.base, owner, repo, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", &ErrFetch{URL: url, Err: err}
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, "", &ErrFetch{URL: url, Err: err}
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
			URL: url,
			Err: fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body))),
		}
	}

	var cr contentsResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return nil, "", &ErrFetch{URL: url, Err: fmt.Errorf("decode: %w", err)}
	}

	// GitHub returns content as base64 with embedded newlines every 60 chars.
	cleaned := strings.Join(strings.Fields(cr.Content), "")
	body, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		return nil, "", &ErrFetch{URL: url, Err: fmt.Errorf("base64: %w", err)}
	}
	return body, cr.SHA, nil
}
