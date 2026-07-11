package standards

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeFetcher is an in-memory Fetcher for resolver tests.
type fakeFetcher struct {
	// responses keys are "owner/repo/path".
	responses map[string]fakeResponse
	// err, if non-nil, is returned for every Fetch call.
	err error
	// calls records every Fetch invocation for inspection.
	calls []fakeCall
}

type fakeResponse struct {
	body []byte
	sha  string
}

type fakeCall struct {
	owner, repo, path string
}

func (f *fakeFetcher) Fetch(_ context.Context, owner, repo, path string) ([]byte, string, error) {
	f.calls = append(f.calls, fakeCall{owner, repo, path})
	if f.err != nil {
		return nil, "", f.err
	}
	r, ok := f.responses[owner+"/"+repo+"/"+path]
	if !ok {
		return nil, "", &ErrNotFound{Owner: owner, Repo: repo, Path: path}
	}
	return r.body, r.sha, nil
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestParseFromHeader(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{"inline comment", "<!-- from: a/b -->\n# Title\n", "a/b"},
		{"multiline comment", "<!--\n  from: a/b\n-->\n# Title\n", "a/b"},
		{"bare directive", "from: a/b\n# Title\n", "a/b"},
		{"absent", "# Title\n", ""},
		{"empty value", "from: \n# Title\n", ""},
		{"inside body text", "Some intro.\n<!-- from: a/b -->\n", "a/b"},
		{"leading whitespace", "   from: a/b\n# Title\n", "a/b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseFromHeader(tc.body)
			if tc.want == "" {
				if ok {
					t.Errorf("expected miss, got %q", got)
				}
				return
			}
			if !ok {
				t.Fatalf("expected hit for %q, got miss", tc.body)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsRedirectOnly(t *testing.T) {
	cases := []struct {
		name, body string
		want       bool
	}{
		{"empty", "", true},
		{"whitespace only", "   \n\n", true},
		{"single-line redirect", "<!-- from: a/b -->\n", true},
		{"multiline redirect", "<!--\n  from: a/b\n-->\n", true},
		{"redirect plus title", "<!-- from: a/b -->\n# Title\n", false},
		{"non-redirect body", "# Project Standards\n\nReal content.\n", false},
		{"bare from then body", "from: a/b\n# Title\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRedirectOnly(tc.body); got != tc.want {
				t.Errorf("isRedirectOnly(%q) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}

func TestParseOrgRepo(t *testing.T) {
	cases := []struct {
		in        string
		wantOwn   string
		wantRepo  string
		wantError bool
	}{
		{"a/b", "a", "b", false},
		{"elpic/go-standards", "elpic", "go-standards", false},
		{"a/b/c", "", "", true}, // GitHub repo names cannot contain slashes
		{"", "", "", true},
		{"/b", "", "", true},
		{"a/", "", "", true},
		{"a", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			owner, repo, err := parseOrgRepo(tc.in)
			if tc.wantError {
				if err == nil {
					t.Errorf("expected error for %q", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if owner != tc.wantOwn || repo != tc.wantRepo {
				t.Errorf("got (%q,%q), want (%q,%q)", owner, repo, tc.wantOwn, tc.wantRepo)
			}
		})
	}
}

// TestResolver_Tier1WinsOverTier2: a project file with substantive body wins
// over a `from:` redirect. The org is never fetched.
func TestResolver_Tier1WinsOverTier2(t *testing.T) {
	dir := t.TempDir()
	body := "# Project Standards\n\n<!-- from: owner/repo -->\nReal content.\n"
	writeFile(t, filepath.Join(dir, ".archon", "standards.md"), body)

	fetcher := &fakeFetcher{
		responses: map[string]fakeResponse{
			"owner/repo/.archon/standards.md": {body: []byte("ORG BODY"), sha: "abc123"},
		},
	}
	r, err := NewResolver(".", WithFetcher(fetcher))
	if err != nil {
		t.Fatal(err)
	}

	doc, err := r.Resolve(context.Background(), dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if doc.Source != filepath.Join(dir, ".archon", "standards.md") {
		t.Errorf("Source = %q, want local path", doc.Source)
	}
	if !strings.Contains(doc.Body, "Real content") {
		t.Errorf("Body = %q, expected local body preserved", doc.Body)
	}
	if len(fetcher.calls) != 0 {
		t.Errorf("expected no fetcher calls, got %d", len(fetcher.calls))
	}
}

// TestResolver_Tier2WinsOverTier3: a project file that is *only* a `from:`
// redirect triggers an org fetch, and the fallback is never consulted.
func TestResolver_Tier2WinsOverTier3(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".archon", "standards.md"), "<!-- from: owner/repo -->\n")

	fetcher := &fakeFetcher{
		responses: map[string]fakeResponse{
			"owner/repo/.archon/standards.md":    {body: []byte("ORG"), sha: "orgsha"},
			"fallback/repo/.archon/standards.md": {body: []byte("FALLBACK"), sha: "fbsha"},
		},
	}
	r, err := NewResolver(".", WithFetcher(fetcher), WithFallback("fallback/repo"))
	if err != nil {
		t.Fatal(err)
	}

	doc, err := r.Resolve(context.Background(), dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if doc.Source != "github.com/owner/repo@orgsha" {
		t.Errorf("Source = %q, want github.com/owner/repo@orgsha", doc.Source)
	}
	if doc.Body != "ORG" {
		t.Errorf("Body = %q, want ORG", doc.Body)
	}
	if len(fetcher.calls) != 1 {
		t.Fatalf("expected exactly 1 fetcher call, got %d: %+v", len(fetcher.calls), fetcher.calls)
	}
	if got := fetcher.calls[0]; got.owner != "owner" || got.repo != "repo" {
		t.Errorf("call = %+v, want owner/repo", got)
	}
}

// TestResolver_FallbackOnly: no project file at all → fallback is used.
func TestResolver_FallbackOnly(t *testing.T) {
	dir := t.TempDir()

	fetcher := &fakeFetcher{
		responses: map[string]fakeResponse{
			"fallback/repo/.archon/standards.md": {body: []byte("FB"), sha: "fbsha"},
		},
	}
	r, err := NewResolver(".", WithFetcher(fetcher), WithFallback("fallback/repo"))
	if err != nil {
		t.Fatal(err)
	}

	doc, err := r.Resolve(context.Background(), dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if doc.Source != "github.com/fallback/repo@fbsha" {
		t.Errorf("Source = %q, want github.com/fallback/repo@fbsha", doc.Source)
	}
	if doc.Body != "FB" {
		t.Errorf("Body = %q, want FB", doc.Body)
	}
}

// TestResolver_NoTiers: no project file, no fallback → error.
func TestResolver_NoTiers(t *testing.T) {
	dir := t.TempDir()
	r, err := NewResolver(".", WithFetcher(&fakeFetcher{}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Resolve(context.Background(), dir); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestResolver_EmptyProjectNoFrom: project file exists but is empty and has
// no `from:` header → error.
func TestResolver_EmptyProjectNoFrom(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".archon", "standards.md"), "")

	r, err := NewResolver(".", WithFetcher(&fakeFetcher{}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Resolve(context.Background(), dir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected 'empty' in error, got %v", err)
	}
}

// TestResolver_FetcherErrorPropagation: when the org fetch fails, the
// error surfaces to the caller; no fallback is attempted.
func TestResolver_FetcherErrorPropagation(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".archon", "standards.md"), "<!-- from: owner/repo -->\n")

	netErr := errors.New("network down")
	fetcher := &fakeFetcher{err: netErr}
	r, err := NewResolver(".", WithFetcher(fetcher), WithFallback("other/repo"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Resolve(context.Background(), dir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, netErr) {
		t.Errorf("expected wrapped network error, got %v", err)
	}
}

// TestResolver_NotFound: an org that does not exist returns ErrNotFound.
func TestResolver_NotFound(t *testing.T) {
	dir := t.TempDir()
	fetcher := &fakeFetcher{
		responses: map[string]fakeResponse{},
	}
	r, err := NewResolver(".", WithFetcher(fetcher), WithFallback("missing/repo"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Resolve(context.Background(), dir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var nf *ErrNotFound
	if !errors.As(err, &nf) {
		t.Errorf("expected ErrNotFound, got %T: %v", err, err)
	}
}

// TestNewResolver_RejectsInvalidFallback: malformed --fallback is rejected
// at construction time, not at first use.
func TestNewResolver_RejectsInvalidFallback(t *testing.T) {
	_, err := NewResolver(".", WithFallback("not-a-repo"))
	if err == nil {
		t.Fatal("expected error for invalid fallback, got nil")
	}
	if !strings.Contains(err.Error(), "fallback") {
		t.Errorf("expected 'fallback' in error, got %v", err)
	}
}
