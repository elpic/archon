package standards

import (
	"context"
	"errors"
	"os"
	"os/exec"
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

// selectiveFetcher is like fakeFetcher but returns a fixed error for
// requests not in the responses map. This lets tests assert that the
// resolver swallows *ErrFetch (e.g. 403), not just *ErrNotFound.
type selectiveFetcher struct {
	responses map[string]fakeResponse
	fetchErr  error
	calls     []fakeCall
}

func (f *selectiveFetcher) Fetch(_ context.Context, owner, repo, path string) ([]byte, string, error) {
	f.calls = append(f.calls, fakeCall{owner, repo, path})
	if r, ok := f.responses[owner+"/"+repo+"/"+path]; ok {
		return r.body, r.sha, nil
	}
	if f.fetchErr != nil {
		return nil, "", f.fetchErr
	}
	return nil, "", &ErrNotFound{Owner: owner, Repo: repo, Path: path}
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

// unsetEnv removes key from the process environment for the duration of
// the test. Go's t.Setenv can only set, not unset, so this helper handles
// the restore-or-unset cleanup.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	oldVal, hadOld := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unsetenv %s: %v", key, err)
	}
	t.Cleanup(func() {
		if hadOld {
			os.Setenv(key, oldVal)
		} else {
			os.Unsetenv(key)
		}
	})
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
		// Accepted.
		{"a/b", "a", "b", false},
		{"elpic/go-standards", "elpic", "go-standards", false},
		{"my-org/my.repo", "my-org", "my.repo", false},
		{"owner_name/repo_name", "owner_name", "repo_name", false},

		// Rejected: shape.
		{"a/b/c", "", "", true},  // GitHub repo names cannot contain slashes
		{"", "", "", true},       // empty input
		{"/b", "", "", true},     // empty owner
		{"a/", "", "", true},     // empty repo
		{"a", "", "", true},      // no slash

		// Rejected: SEC-002 — reserved URL characters that could be
		// smuggled past the parser and reinterpreted by the URL layer.
		// (The old fmt.Sprintf-based URL builder would let a '?' here
		// become the start of the query string.)
		{"a?x=1/b", "", "", true},    // '?' in owner
		{"a/b?x=1", "", "", true},    // '?' in repo
		{"a/b#frag", "", "", true},   // '#' in repo
		{"a b/c", "", "", true},      // space in owner
		{"a/b c", "", "", true},      // space in repo
		{"a&b/c", "", "", true},      // '&' in owner
		{"a/b&c", "", "", true},      // '&' in repo
		{"a;b/c", "", "", true},      // ';' in owner
		{"a/b;c", "", "", true},      // ';' in repo
		{"a%2Fb/c", "", "", true},    // pre-encoded slash
		{"a/b%2Fc", "", "", true},    // pre-encoded slash
		{"a:b/c", "", "", true},      // ':' in owner (URL authority delimiter)
		{"a/b@host", "", "", true},   // '@' in repo (URL userinfo delimiter)

		// The exact SEC-002 regression: a 'from:' header that looks like
		// a clean owner/repo but is actually a query injection.
		{"elpic/archon?next=evil", "", "", true},
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

// TestValidateOrgRepo: the public validator must agree with parseOrgRepo
// and reject the same inputs.
func TestValidateOrgRepo(t *testing.T) {
	for _, in := range []string{
		"a/b", "elpic/go-standards", "my-org/my.repo", "owner_name/repo_name",
		"", "/b", "a/", "a", "a/b/c",
		"a?x=1/b", "a/b?x=1", "a/b#frag", "a b/c", "a/b c",
		"a&b/c", "a/b&c", "a;b/c", "a/b;c",
		"elpic/archon?next=evil",
	} {
		t.Run(in, func(t *testing.T) {
			err := ValidateOrgRepo(in)
			switch in {
			case "a/b", "elpic/go-standards", "my-org/my.repo", "owner_name/repo_name":
				if err != nil {
					t.Errorf("ValidateOrgRepo(%q) = %v, want nil", in, err)
				}
			default:
				if err == nil {
					t.Errorf("ValidateOrgRepo(%q) = nil, want error", in)
				}
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

// TestResolver_AutoInferFromGITHUB_REPOSITORY: when GITHUB_REPOSITORY is set
// and no project file / no `from:` header exists, the resolver fetches
// <owner>/.archon/standards.md via auto-infer.
func TestResolver_AutoInferFromGITHUB_REPOSITORY(t *testing.T) {
	unsetEnv(t, "GITHUB_REPOSITORY")
	t.Setenv("GITHUB_REPOSITORY", "elpic/testing")

	dir := t.TempDir()
	fetcher := &fakeFetcher{
		responses: map[string]fakeResponse{
			"elpic/.archon/.archon/standards.md": {body: []byte("AUTO BODY"), sha: "autosha"},
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
	if doc.Source != "github.com/elpic/.archon@autosha" {
		t.Errorf("Source = %q, want github.com/elpic/.archon@autosha", doc.Source)
	}
	if doc.Body != "AUTO BODY" {
		t.Errorf("Body = %q, want AUTO BODY", doc.Body)
	}
	if len(fetcher.calls) != 1 {
		t.Fatalf("expected exactly 1 fetcher call, got %d: %+v", len(fetcher.calls), fetcher.calls)
	}
	got := fetcher.calls[0]
	if got.owner != "elpic" || got.repo != ".archon" || got.path != ".archon/standards.md" {
		t.Errorf("call = %+v, want elpic/.archon/.archon/standards.md", got)
	}
}

// TestResolver_AutoInferFromGitRemote: when GITHUB_REPOSITORY is unset and
// the target is a git repo whose `origin` remote is a GitHub URL, the
// resolver auto-infers the owner from the remote and fetches
// <owner>/.archon/standards.md.
func TestResolver_AutoInferFromGitRemote(t *testing.T) {
	unsetEnv(t, "GITHUB_REPOSITORY")

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"remote", "add", "origin", "git@github.com:AvantFinCo/card-ledger.git"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	fetcher := &fakeFetcher{
		responses: map[string]fakeResponse{
			"AvantFinCo/.archon/.archon/standards.md": {body: []byte("AUTO"), sha: "infersha"},
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
	if doc.Source != "github.com/AvantFinCo/.archon@infersha" {
		t.Errorf("Source = %q, want github.com/AvantFinCo/.archon@infersha", doc.Source)
	}
	if doc.Body != "AUTO" {
		t.Errorf("Body = %q, want AUTO", doc.Body)
	}
}

// TestResolver_AutoInferFallbackToWithFallback: when auto-infer points at an
// org with no `.archon` repo (fetcher returns ErrNotFound), resolution must
// fall through to the configured WithFallback rather than surfacing the
// auto-infer miss as an error.
func TestResolver_AutoInferFallbackToWithFallback(t *testing.T) {
	unsetEnv(t, "GITHUB_REPOSITORY")
	t.Setenv("GITHUB_REPOSITORY", "elpic/testing")

	dir := t.TempDir()
	fetcher := &fakeFetcher{
		responses: map[string]fakeResponse{
			"elpic/standards/.archon/standards.md": {body: []byte("FB"), sha: "fbsha"},
			// No entry for "elpic/.archon/.archon/standards.md" → fakeFetcher
			// returns ErrNotFound, which tier 3 must swallow.
		},
	}
	r, err := NewResolver(".", WithFetcher(fetcher), WithFallback("elpic/standards"))
	if err != nil {
		t.Fatal(err)
	}

	doc, err := r.Resolve(context.Background(), dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if doc.Source != "github.com/elpic/standards@fbsha" {
		t.Errorf("Source = %q, want github.com/elpic/standards@fbsha", doc.Source)
	}
	if doc.Body != "FB" {
		t.Errorf("Body = %q, want FB", doc.Body)
	}
	// Expect two fetcher calls: one for the auto-infer target (miss), one
	// for the configured fallback (hit).
	if len(fetcher.calls) != 2 {
		t.Fatalf("expected 2 fetcher calls, got %d: %+v", len(fetcher.calls), fetcher.calls)
	}
	if got := fetcher.calls[0]; got.owner != "elpic" || got.repo != ".archon" {
		t.Errorf("first call = %+v, want auto-infer elpic/.archon", got)
	}
	if got := fetcher.calls[1]; got.owner != "elpic" || got.repo != "standards" {
		t.Errorf("second call = %+v, want fallback elpic/standards", got)
	}
}

// TestResolver_AutoInferFallbackToWithFallback_403 (QA Gap 1): a regression
// in tier-3 fall-through that special-cased ErrNotFound would let a 403
// (private repo) bubble up. The real httpFetcher returns ErrFetch for 403,
// and the resolver must swallow that too. This pins the behavior: any
// non-nil error from the auto-infer fetch is silently dropped and the
// resolver moves on to the next tier.
func TestResolver_AutoInferFallbackToWithFallback_403(t *testing.T) {
	unsetEnv(t, "GITHUB_REPOSITORY")
	t.Setenv("GITHUB_REPOSITORY", "elpic/testing")

	dir := t.TempDir()
	// 403 → ErrFetch (the way the real httpFetcher surfaces it). We cannot
	// use the default fakeFetcher here because its miss path returns
	// ErrNotFound. Build a fetcher that returns ErrFetch for the auto-infer
	// target and a successful response for the fallback.
	fetcher := &selectiveFetcher{
		responses: map[string]fakeResponse{
			"elpic/standards/.archon/standards.md": {body: []byte("FB"), sha: "fbsha"},
		},
		fetchErr: &ErrFetch{
			URL: "https://api.github.com/repos/elpic/.archon/contents/.archon/standards.md",
			Err: errors.New("status 403: Not Found"),
		},
	}
	r, err := NewResolver(".", WithFetcher(fetcher), WithFallback("elpic/standards"))
	if err != nil {
		t.Fatal(err)
	}

	doc, err := r.Resolve(context.Background(), dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if doc.Source != "github.com/elpic/standards@fbsha" {
		t.Errorf("Source = %q, want github.com/elpic/standards@fbsha", doc.Source)
	}
	if doc.Body != "FB" {
		t.Errorf("Body = %q, want FB", doc.Body)
	}
	// Tier 3 was attempted, then tier 4.
	if got := fetcher.calls[0]; got.owner != "elpic" || got.repo != ".archon" {
		t.Errorf("first call = %+v, want auto-infer elpic/.archon", got)
	}
	if got := fetcher.calls[1]; got.owner != "elpic" || got.repo != "standards" {
		t.Errorf("second call = %+v, want fallback elpic/standards", got)
	}
	if len(fetcher.calls) != 2 {
		t.Errorf("expected exactly 2 fetcher calls, got %d: %+v", len(fetcher.calls), fetcher.calls)
	}
}

// TestResolver_NoGitNoEnvNoFallback: when neither GITHUB_REPOSITORY is set
// nor the target is a git repo (or git is missing) nor a WithFallback is
// configured, Resolve returns the "no standards found" error. Auto-infer
// must fail silently in this case.
func TestResolver_NoGitNoEnvNoFallback(t *testing.T) {
	unsetEnv(t, "GITHUB_REPOSITORY")

	dir := t.TempDir() // plain directory, no .git
	r, err := NewResolver(".", WithFetcher(&fakeFetcher{}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Resolve(context.Background(), dir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no standards found") {
		t.Errorf("expected 'no standards found' in error, got %v", err)
	}
}

// TestResolver_HeaderBeatsAutoInfer: an explicit `from: acme/specific` in
// the project file wins over the auto-infer target derived from
// GITHUB_REPOSITORY. Explicit beats convention.
func TestResolver_HeaderBeatsAutoInfer(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "elpic/testing")

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".archon", "standards.md"), "<!-- from: acme/specific -->\n")

	fetcher := &fakeFetcher{
		responses: map[string]fakeResponse{
			"acme/specific/.archon/standards.md": {body: []byte("EXPLICIT"), sha: "expsha"},
			"elpic/.archon/.archon/standards.md":  {body: []byte("AUTO"), sha: "autosha"},
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
	if doc.Source != "github.com/acme/specific@expsha" {
		t.Errorf("Source = %q, want github.com/acme/specific@expsha", doc.Source)
	}
	if doc.Body != "EXPLICIT" {
		t.Errorf("Body = %q, want EXPLICIT", doc.Body)
	}
	// Auto-infer must not have been consulted.
	for _, call := range fetcher.calls {
		if call.owner == "elpic" && call.repo == ".archon" {
			t.Errorf("auto-infer was consulted: %+v", call)
		}
	}
	if len(fetcher.calls) != 1 {
		t.Errorf("expected exactly 1 fetcher call (the from-header), got %d: %+v", len(fetcher.calls), fetcher.calls)
	}
}
