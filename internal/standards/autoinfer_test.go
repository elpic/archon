package standards

import (
	"os"
	"strings"
	"testing"
)

func TestParseRemoteURL(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		// Accepted: GitHub SSH.
		{"ssh with .git", "git@github.com:octocat/repo.git", "octocat"},
		{"ssh without .git", "git@github.com:octocat/repo", "octocat"},
		// Accepted: GitHub HTTPS / HTTP.
		{"https with .git", "https://github.com/octocat/repo.git", "octocat"},
		{"https without .git", "https://github.com/octocat/repo", "octocat"},
		{"http without .git", "http://github.com/octocat/repo", "octocat"},
		// Owners with hyphens, dots, and underscores.
		{"ssh with hyphenated owner", "git@github.com:my-org/my-repo.git", "my-org"},
		{"ssh with dotted owner", "git@github.com:owner.name/repo.git", "owner.name"},
		{"https with underscored owner", "https://github.com/owner_name/repo.git", "owner_name"},

		// Accepted: HTTPS with embedded credentials (QA Gap 3). The token
		// in the userinfo is the most common credential-leak surface; we
		// must extract the owner without leaking the userinfo into any
		// return value.
		{"https with pat credentials", "https://x-access-token:ghp_secret@github.com/owner/repo.git", "owner"},

		// Rejected: non-GitHub hosts.
		{"ssh gitlab", "git@gitlab.com:octocat/repo.git", ""},
		{"https gitlab", "https://gitlab.com/octocat/repo.git", ""},
		{"ssh bitbucket", "git@bitbucket.org:octocat/repo.git", ""},

		// Rejected: malformed path (too few / too many segments).
		{"ssh missing repo", "git@github.com:octocat", ""},
		{"ssh missing owner", "git@github.com:/repo.git", ""},
		{"ssh extra path", "git@github.com:octocat/repo/extra.git", ""},
		{"https missing repo", "https://github.com/octocat", ""},
		{"https missing owner", "https://github.com:/repo.git", ""},
		{"https extra path", "https://github.com/octocat/repo/extra", ""},

		// Rejected: empty / garbage.
		{"empty", "", ""},
		{"plain string", "not a url", ""},
		{"https wrong host with path", "https://example.com/octocat/repo", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseRemoteURL(tc.in)
			if got != tc.want {
				t.Errorf("parseRemoteURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
			// Regression guard: a return value must never contain a
			// credential-like substring, no matter what the input was.
			// A future refactor of ownerFromHTTPS that re-includes the
			// raw URL or userinfo would fail this.
			if got != "" {
				if strings.Contains(got, "ghp_") || strings.Contains(got, "@") || strings.Contains(got, "://") {
					t.Errorf("parseRemoteURL(%q) = %q leaks credentials / URL syntax", tc.in, got)
				}
			}
		})
	}
}

// TestOwnerFromHTTPS_NoUserinfoLeak (QA Gap 3, part 2): a focused regression
// test against the most likely credential-leak surface. The returned
// string must be the owner only — never the full URL, never anything
// containing the userinfo or the token.
func TestOwnerFromHTTPS_NoUserinfoLeak(t *testing.T) {
	const in = "https://x-access-token:ghp_secret123@github.com/owner/repo.git"
	got := ownerFromHTTPS(in)
	if got != "owner" {
		t.Errorf("ownerFromHTTPS(...) = %q, want %q", got, "owner")
	}
	// Strong assertion: the returned string must be free of credential
	// markers regardless of how the implementation evolves.
	for _, marker := range []string{"ghp_", "x-access-token", "@", "://", "secret"} {
		if strings.Contains(got, marker) {
			t.Errorf("ownerFromHTTPS(...) = %q, must not contain %q", got, marker)
		}
	}
}

// TestInferOrgRepo_MalformedGHRepository (QA Gap 2): a malformed
// GITHUB_REPOSITORY value must not produce a non-empty org/repo at the
// public inferOrgRepo level. The previous coverage at 83.3% missed the
// malformed-env paths because ownerFromGHRepository was the only thing
// tested directly.
func TestInferOrgRepo_MalformedGHRepository(t *testing.T) {
	cases := []struct {
		name, env string
	}{
		{"empty", ""},
		{"missing slash", "foo"},
		{"empty owner", "/foo"},
		{"empty repo", "foo/"},
		{"too many slashes", "foo/bar/baz"},
		{"just a slash", "/"},
		{"whitespace only", "   "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Ensure no inherited state from the host environment.
			t.Setenv("GITHUB_REPOSITORY", tc.env)
			// A plain temp dir with no .git makes the git-remote
			// path a guaranteed miss, so the function can only
			// return a non-empty value via GITHUB_REPOSITORY.
			dir := t.TempDir()
			if got := inferOrgRepo(dir); got != "" {
				t.Errorf("inferOrgRepo(env=%q) = %q, want \"\"", tc.env, got)
			}
		})
	}
}

// TestInferOrgRepo_ValidGHRepository (sanity check, complements the
// malformed test): a well-formed GITHUB_REPOSITORY must produce
// "<owner>/.archon".
func TestInferOrgRepo_ValidGHRepository(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "elpic/testing")
	dir := t.TempDir()
	if got := inferOrgRepo(dir); got != "elpic/.archon" {
		t.Errorf("inferOrgRepo(valid env) = %q, want %q", got, "elpic/.archon")
	}
}

// TestInferOrgRepo_EnvUnset: when GITHUB_REPOSITORY is unset and the
// target is not a git repo, inferOrgRepo must return "". This exercises
// the other branch of the OR-fall-through in the public function.
func TestInferOrgRepo_EnvUnset(t *testing.T) {
	oldVal, hadOld := os.LookupEnv("GITHUB_REPOSITORY")
	if err := os.Unsetenv("GITHUB_REPOSITORY"); err != nil {
		t.Fatalf("unsetenv: %v", err)
	}
	t.Cleanup(func() {
		if hadOld {
			os.Setenv("GITHUB_REPOSITORY", oldVal)
		}
	})
	dir := t.TempDir()
	if got := inferOrgRepo(dir); got != "" {
		t.Errorf("inferOrgRepo(env unset, no git) = %q, want \"\"", got)
	}
}
