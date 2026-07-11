package standards

import (
	"context"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

// gitRemoteTimeout caps the time we'll wait for `git remote get-url`.
// A hung or misbehaving git invocation must never block an audit.
const gitRemoteTimeout = 2 * time.Second

// inferOrgRepo returns "<owner>/.archon" derived from the environment, or
// "" if it cannot be determined. Checks, in order:
//  1. GITHUB_REPOSITORY env var (set in GitHub Actions, format "owner/repo")
//  2. `git -C target remote get-url origin` (parses SSH and HTTPS formats)
//
// Returns "" on any failure (no git, no remote, unparseable URL, etc.) —
// the caller treats "" as a miss and falls through.
func inferOrgRepo(target string) string {
	if owner, ok := ownerFromGHRepository(os.Getenv("GITHUB_REPOSITORY")); ok {
		return owner + "/.archon"
	}
	if owner := ownerFromGitRemote(target); owner != "" {
		return owner + "/.archon"
	}
	return ""
}

// ownerFromGHRepository extracts the owner from a "owner/repo" string
// (the value of the GITHUB_REPOSITORY env var, set by GitHub Actions).
// Returns ok=false for empty input, missing slash, too many slashes,
// or empty parts — i.e. for anything that is not a clean two-segment
// "owner/repo". We intentionally do not apply the URL-character
// allow-list here (the GITHUB_REPOSITORY value is environment-controlled,
// not user-controlled); we only require the shape.
func ownerFromGHRepository(s string) (string, bool) {
	if s == "" {
		return "", false
	}
	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	return parts[0], true
}

// ownerFromGitRemote shells out to `git remote get-url origin` in target
// and returns the parsed owner, or "" on any failure (git missing, no
// remote, unparseable URL, timeout). The exec is bounded by gitRemoteTimeout.
func ownerFromGitRemote(target string) string {
	ctx, cancel := context.WithTimeout(context.Background(), gitRemoteTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", "origin")
	cmd.Dir = target
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return parseRemoteURL(strings.TrimSpace(string(out)))
}

// parseRemoteURL extracts the owner from a GitHub remote URL.
//
// Accepted forms (owner "octocat" in all examples):
//
//	git@github.com:octocat/repo.git
//	git@github.com:octocat/repo
//	https://github.com/octocat/repo.git
//	https://github.com/octocat/repo
//	http://github.com/octocat/repo
//
// Anything else (non-GitHub host, malformed path, empty input) returns "".
func parseRemoteURL(remote string) string {
	if remote == "" {
		return ""
	}
	switch {
	case strings.HasPrefix(remote, "git@github.com:"):
		return ownerFromSSH(remote)
	case strings.HasPrefix(remote, "http://") || strings.HasPrefix(remote, "https://"):
		return ownerFromHTTPS(remote)
	}
	return ""
}

// ownerFromSSH parses a GitHub SSH remote. The path is conventionally
// "owner/repo" (optionally ".git"-suffixed). Anything outside that
// shape is rejected.
func ownerFromSSH(remote string) string {
	const prefix = "git@github.com:"
	rest := strings.TrimPrefix(remote, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	// parts[1] is the repo; .git suffix is allowed but not required.
	// We only need owner, so we don't bother stripping it.
	return parts[0]
}

// ownerFromHTTPS parses a GitHub HTTPS remote. The host must be exactly
// "github.com" and the path must have exactly two non-empty segments
// (owner and repo, the latter optionally ".git"-suffixed).
func ownerFromHTTPS(remote string) string {
	u, err := url.Parse(remote)
	if err != nil {
		return ""
	}
	if u.Host != "github.com" {
		return ""
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return parts[0]
}
