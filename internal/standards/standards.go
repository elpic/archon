// Package standards resolves the applicable standards documents for a
// project. Resolution order: project-local > org-level > fallback GitHub repo.
package standards

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Resolver locates and loads the standards markdown that governs a project.
type Resolver struct {
	workdir         string
	fallbackOrgRepo string
	fetcher         Fetcher
}

// Option configures a Resolver.
type Option func(*Resolver)

// WithFallback sets the org/repo used when neither the project file nor
// a `from:` header in the project file resolves to a usable source.
//
// The value must be in the form "owner/repo" (e.g. "elpic/go-standards").
// An invalid value is rejected by NewResolver.
func WithFallback(orgRepo string) Option {
	return func(r *Resolver) { r.fallbackOrgRepo = orgRepo }
}

// WithFetcher overrides the Fetcher used for remote standards resolution.
// Tests pass a fake; production code uses NewHTTPFetcher.
func WithFetcher(f Fetcher) Option {
	return func(r *Resolver) { r.fetcher = f }
}

// NewResolver constructs a Resolver rooted at the given working directory.
//
// The workdir argument is currently unused but reserved for future relative
// path resolution; pass "." for the current process working directory.
func NewResolver(_ string, opts ...Option) (*Resolver, error) {
	r := &Resolver{}
	for _, opt := range opts {
		opt(r)
	}
	if r.fallbackOrgRepo != "" {
		if _, _, err := parseOrgRepo(r.fallbackOrgRepo); err != nil {
			return nil, fmt.Errorf("fallback: %w", err)
		}
	}
	if r.fetcher == nil {
		r.fetcher = NewHTTPFetcher()
	}
	return r, nil
}

// Document represents a resolved standards file and where it came from.
type Document struct {
	Source string
	Body   string
}

// Resolve returns the effective standards Document for the target project.
//
// Resolution order:
//  1. project: `target/.archon/standards.md` if it has substantive body content
//     (more than a `from:` redirect comment).
//  2. org-header: the `from: owner/repo` line in the project file, if present.
//     The org repo's `.archon/standards.md` is fetched via the configured Fetcher.
//  3. fallback: the org/repo passed to WithFallback, if configured.
//
// A project file that is *only* a `from:` redirect comment is treated as a
// tier-1 miss; resolution falls through to tier 2.
func (r *Resolver) Resolve(ctx context.Context, target string) (*Document, error) {
	projectPath := projectStandardsPath(target)

	// Tier 1: project file with substantive body.
	if doc, ok := r.fromProject(target); ok && !isRedirectOnly(doc.Body) {
		return doc, nil
	}

	// Tier 2: `from:` header in the project file → fetch from the org.
	if from, ok := r.fromOrgHeader(target); ok {
		return r.fetchSource(ctx, from)
	}

	// Tier 3: configured fallback → fetch from the fallback org.
	if r.fallbackOrgRepo != "" {
		return r.fetchSource(ctx, r.fallbackOrgRepo)
	}

	// Differentiate the "file exists but is empty" case from "no file at all"
	// so the user gets a precise error.
	if _, err := os.Stat(projectPath); err == nil {
		return nil, fmt.Errorf("standards file %s is empty and has no `from:` header", projectPath)
	}
	return nil, fmt.Errorf("no standards found for %s", target)
}

// fromProject reads target/.archon/standards.md. It is the same function as
// before this change; Resolve decides whether the body is "substantive".
func (r *Resolver) fromProject(target string) (*Document, bool) {
	path := projectStandardsPath(target)
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return &Document{Source: path, Body: string(body)}, true
}

// fromOrgHeader reads the project file and returns the value of the
// `from: owner/repo` header if present. It returns ok=false if the file
// is missing or has no `from:` header.
func (r *Resolver) fromOrgHeader(target string) (string, bool) {
	path := projectStandardsPath(target)
	body, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return parseFromHeader(string(body))
}

func (r *Resolver) fetchSource(ctx context.Context, orgRepo string) (*Document, error) {
	owner, repo, err := parseOrgRepo(orgRepo)
	if err != nil {
		return nil, err
	}
	body, sha, err := r.fetcher.Fetch(ctx, owner, repo, ".archon/standards.md")
	if err != nil {
		return nil, err
	}
	return &Document{
		Source: fmt.Sprintf("github.com/%s/%s@%s", owner, repo, sha),
		Body:   string(body),
	}, nil
}

func projectStandardsPath(target string) string {
	return filepath.Join(target, ".archon", "standards.md")
}

// parseOrgRepo splits "owner/repo" and validates both parts are non-empty.
// GitHub repo names cannot contain slashes, so we reject anything that
// splits into more than two parts.
func parseOrgRepo(s string) (owner, repo string, err error) {
	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid org/repo %q (expected owner/repo)", s)
	}
	return parts[0], parts[1], nil
}

// parseFromHeader scans the first lines of a markdown file for a
// `from: owner/repo` directive. The directive may appear bare or inside
// an HTML comment (`<!-- from: owner/repo -->`); single-line and multi-line
// comment forms are both accepted. The first match wins.
func parseFromHeader(body string) (string, bool) {
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	inComment := false
	for sc.Scan() {
		trimmed := strings.TrimSpace(sc.Text())
		if inComment {
			if idx := strings.Index(trimmed, "-->"); idx != -1 {
				inComment = false
				trimmed = strings.TrimSpace(trimmed[idx+3:])
				if trimmed == "" {
					continue
				}
			} else {
				if from, ok := matchFromDirective(trimmed); ok {
					return from, true
				}
				continue
			}
		}
		cleaned := trimmed
		if strings.HasPrefix(cleaned, "<!--") {
			cleaned = strings.TrimSpace(strings.TrimPrefix(cleaned, "<!--"))
			if strings.HasSuffix(cleaned, "-->") {
				cleaned = strings.TrimSpace(strings.TrimSuffix(cleaned, "-->"))
			} else {
				inComment = true
			}
		}
		if from, ok := matchFromDirective(cleaned); ok {
			return from, true
		}
	}
	return "", false
}

// matchFromDirective returns the value of a `from: value` prefix on s.
// Returns ok=false if the prefix is absent or the value is empty.
func matchFromDirective(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "from:") {
		return "", false
	}
	val := strings.TrimSpace(strings.TrimPrefix(s, "from:"))
	if val == "" {
		return "", false
	}
	return val, true
}

// isRedirectOnly reports whether body is empty or contains only an
// HTML comment block holding a `from:` directive. A file matching this
// pattern is treated as a redirect marker, not as the project standards.
func isRedirectOnly(body string) bool {
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	inComment := false
	var rest strings.Builder
	for sc.Scan() {
		trimmed := strings.TrimSpace(sc.Text())
		if inComment {
			if idx := strings.Index(trimmed, "-->"); idx != -1 {
				inComment = false
				trimmed = strings.TrimSpace(trimmed[idx+3:])
				if trimmed == "" {
					continue
				}
			} else {
				continue
			}
		}
		cleaned := trimmed
		if strings.HasPrefix(cleaned, "<!--") {
			cleaned = strings.TrimSpace(strings.TrimPrefix(cleaned, "<!--"))
			if strings.HasSuffix(cleaned, "-->") {
				cleaned = strings.TrimSpace(strings.TrimSuffix(cleaned, "-->"))
			} else {
				inComment = true
			}
			if strings.HasPrefix(cleaned, "from:") {
				continue
			}
			if cleaned == "" {
				continue
			}
			rest.WriteString(cleaned)
			rest.WriteString("\n")
			continue
		}
		if strings.HasPrefix(cleaned, "from:") {
			continue
		}
		rest.WriteString(sc.Text())
		rest.WriteString("\n")
	}
	return strings.TrimSpace(rest.String()) == ""
}
