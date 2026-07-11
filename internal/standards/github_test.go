package standards

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestHTTPFetcher_OK(t *testing.T) {
	want := []byte("hello world\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/contents/.archon/standards.md" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"content":"` + base64.StdEncoding.EncodeToString(want) + `","sha":"abc123"}`))
	}))
	defer srv.Close()

	fetcher := &httpFetcher{client: srv.Client(), base: srv.URL}
	body, sha, err := fetcher.Fetch(context.Background(), "o", "r", ".archon/standards.md")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(body) != string(want) {
		t.Errorf("body = %q, want %q", body, want)
	}
	if sha != "abc123" {
		t.Errorf("sha = %q, want abc123", sha)
	}
}

func TestHTTPFetcher_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	fetcher := &httpFetcher{client: srv.Client(), base: srv.URL}
	_, _, err := fetcher.Fetch(context.Background(), "o", "r", ".archon/standards.md")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var nf *ErrNotFound
	if !errors.As(err, &nf) {
		t.Errorf("expected ErrNotFound, got %T: %v", err, err)
	}
}

func TestHTTPFetcher_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()
	fetcher := &httpFetcher{client: srv.Client(), base: srv.URL}
	_, _, err := fetcher.Fetch(context.Background(), "o", "r", ".archon/standards.md")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var fe *ErrFetch
	if !errors.As(err, &fe) {
		t.Errorf("expected ErrFetch, got %T: %v", err, err)
	}
	if fe == nil || !strings.Contains(fe.URL, "/repos/o/r/contents/.archon/standards.md") {
		t.Errorf("URL = %q, expected to contain the path", fe.URL)
	}
}

func TestHTTPFetcher_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"content":"!!!not-base64!!!","sha":"abc"}`))
	}))
	defer srv.Close()
	fetcher := &httpFetcher{client: srv.Client(), base: srv.URL}
	_, _, err := fetcher.Fetch(context.Background(), "o", "r", ".archon/standards.md")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestNewHTTPFetcherWithClient_NilClient(t *testing.T) {
	// nil client should fall back to http.DefaultClient.
	f := NewHTTPFetcherWithClient(nil)
	if f == nil {
		t.Fatal("expected non-nil fetcher")
	}
}

func TestNewHTTPFetcher_Default(t *testing.T) {
	f := NewHTTPFetcher()
	if f == nil {
		t.Fatal("expected non-nil fetcher")
	}
}

// TestNewHTTPFetcher_HasTimeout: SEC-001. The default fetcher must build a
// dedicated *http.Client with a finite Timeout. A hung api.github.com must
// not DoS the audit indefinitely.
func TestNewHTTPFetcher_HasTimeout(t *testing.T) {
	f := NewHTTPFetcher()
	hf, ok := f.(*httpFetcher)
	if !ok {
		t.Fatalf("NewHTTPFetcher returned %T, want *httpFetcher", f)
	}
	if hf.client.Timeout != httpFetchTimeout {
		t.Errorf("Timeout = %v, want %v", hf.client.Timeout, httpFetchTimeout)
	}
	// Hard guarantee: the default client (zero timeout) must NOT be mutated.
	if hf.client == http.DefaultClient {
		t.Error("NewHTTPFetcher must not return http.DefaultClient (shared global)")
	}
}

// TestNewHTTPFetcherWithClient_NilClientHasTimeout: when nil is passed, we
// still build a dedicated client with a timeout — we never fall back to
// http.DefaultClient, which has zero timeout.
func TestNewHTTPFetcherWithClient_NilClientHasTimeout(t *testing.T) {
	f := NewHTTPFetcherWithClient(nil)
	hf := f.(*httpFetcher)
	if hf.client.Timeout != httpFetchTimeout {
		t.Errorf("Timeout = %v, want %v", hf.client.Timeout, httpFetchTimeout)
	}
	if hf.client == http.DefaultClient {
		t.Error("NewHTTPFetcherWithClient(nil) must not return http.DefaultClient")
	}
}

// TestNewHTTPFetcher_RejectsNonHTTPSRedirect: SEC-003. The default client
// must have a CheckRedirect policy that refuses non-https redirects, so
// enterprise installs or hostile networks cannot downgrade the connection
// and leak the standards body.
func TestNewHTTPFetcher_RejectsNonHTTPSRedirect(t *testing.T) {
	// Spin up an HTTPS server that issues a 301 redirect to a plain-HTTP URL.
	httpsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// We do not need the destination to actually exist — CheckRedirect
		// fires before the body is read.
		w.Header().Set("Location", "http://example.invalid/leak")
		w.WriteHeader(http.StatusMovedPermanently)
	}))
	defer httpsSrv.Close()

	// Build a client that (a) trusts the test server's self-signed cert
	// and (b) carries the default fetcher's CheckRedirect policy.
	defaultF := NewHTTPFetcher().(*httpFetcher)
	testClient := httpsSrv.Client()
	testClient.CheckRedirect = defaultF.client.CheckRedirect

	hf := &httpFetcher{client: testClient, base: httpsSrv.URL}
	_, _, err := hf.Fetch(context.Background(), "o", "r", ".archon/standards.md")
	if err == nil {
		t.Fatal("expected error on non-https redirect, got nil")
	}
	if !strings.Contains(err.Error(), "non-https") {
		t.Errorf("expected 'non-https' in error, got %v", err)
	}
}

// TestHTTPFetcher_URLEncodesSpecialChars: SEC-002. Even though parseOrgRepo
// rejects special characters in owner/repo at the resolver layer, the
// fetcher builds the URL via net/url (not fmt.Sprintf) so that a future
// refactor cannot accidentally reintroduce a query-string split.
func TestHTTPFetcher_URLEncodesSpecialChars(t *testing.T) {
	var gotRequestURI, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestURI = r.URL.RequestURI()
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusNotFound) // ignore body; we only care about the URL
	}))
	defer srv.Close()

	fetcher := &httpFetcher{client: srv.Client(), base: srv.URL}
	// Owner contains characters that would, under the old Sprintf, have
	// leaked into the query string. parseOrgRepo now rejects them, but
	// the fetcher is still defense-in-depth.
	_, _, _ = fetcher.Fetch(context.Background(), "o wn?er#1&x=2", "r", ".archon/standards.md")

	if gotQuery != "" {
		t.Errorf("RawQuery = %q, want empty (special chars must not create a query string)", gotQuery)
	}
	if gotRequestURI == "" {
		t.Fatal("server received no request")
	}
	// The on-the-wire request URI must have '?' percent-encoded as %3F.
	if !strings.Contains(gotRequestURI, "%3F") {
		t.Errorf("RequestURI = %q, expected '?' to be percent-encoded as %%3F", gotRequestURI)
	}
	// The fragment delimiter '#' is stripped by the client before sending.
	if strings.Contains(gotRequestURI, "#") {
		t.Errorf("RequestURI = %q, must not contain '#' (fragments are client-side)", gotRequestURI)
	}
}

// TestBuildContentsURL: unit test for the URL builder directly.
func TestBuildContentsURL(t *testing.T) {
	cases := []struct {
		name, base, owner, repo, path, wantRawPath string
	}{
		{
			name:        "github api, clean parts",
			base:        "https://api.github.com",
			owner:       "octocat",
			repo:        "hello",
			path:        ".archon/standards.md",
			wantRawPath: "/repos/octocat/hello/contents/.archon/standards.md",
		},
		{
			name:  "github api, special chars in owner are percent-encoded",
			base:  "https://api.github.com",
			owner: "o wn?er#1&x=2",
			repo:  "r",
			path:  ".archon/standards.md",
			// ? → %3F, space → %20, # → %23. & and = are not query separators
			// in a path segment, so they are left as-is.
			wantRawPath: "/repos/o%20wn%3Fer%231&x=2/r/contents/.archon/standards.md",
		},
		{
			name:        "test server base",
			base:        "http://127.0.0.1:8080",
			owner:       "o",
			repo:        "r",
			path:        ".archon/standards.md",
			wantRawPath: "/repos/o/r/contents/.archon/standards.md",
		},
		{
			name:        "multi-segment path with reserved chars",
			base:        "https://api.github.com",
			owner:       "owner",
			repo:        "repo",
			path:        ".archon/sub dir/file.md",
			// Per-segment escape: space inside a segment becomes %20,
			// but the literal '/' separators between segments are kept.
			wantRawPath: "/repos/owner/repo/contents/.archon/sub%20dir/file.md",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildContentsURL(tc.base, tc.owner, tc.repo, tc.path)
			u, err := url.Parse(got)
			if err != nil {
				t.Fatalf("parse %q: %v", got, err)
			}
			// EscapedPath() returns the on-the-wire path (RawPath if set and
			// valid, otherwise the path-mode-escaped form of Path).
			if got := u.EscapedPath(); got != tc.wantRawPath {
				t.Errorf("EscapedPath = %q, want %q (full URL: %s)", got, tc.wantRawPath, got)
			}
			// Critical: the URL must NOT have a query string. Special chars
			// must be percent-encoded into the path, not leaked into ?query=.
			if u.RawQuery != "" {
				t.Errorf("RawQuery = %q, want empty (full URL: %s)", u.RawQuery, got)
			}
		})
	}
}

func TestErrNotFound_Error(t *testing.T) {
	e := &ErrNotFound{Owner: "o", Repo: "r", Path: "p"}
	if !strings.Contains(e.Error(), "o/r/p") {
		t.Errorf("Error = %q, expected to contain o/r/p", e.Error())
	}
}

func TestErrFetch_ErrorUnwrap(t *testing.T) {
	inner := errors.New("inner")
	e := &ErrFetch{URL: "http://x", Err: inner}
	if !strings.Contains(e.Error(), "inner") {
		t.Errorf("Error = %q, expected to mention inner", e.Error())
	}
	if !errors.Is(e, inner) {
		t.Errorf("expected errors.Is to find inner via Unwrap")
	}
}
