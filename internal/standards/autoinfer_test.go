package standards

import "testing"

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
			if got := parseRemoteURL(tc.in); got != tc.want {
				t.Errorf("parseRemoteURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
