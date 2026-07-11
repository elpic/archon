// Package standards resolves the applicable standards documents for a
// project. Resolution order: project-local > org-level > fallback GitHub repo.
package standards

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Resolver locates and loads the standards markdown that governs a project.
type Resolver struct {
	fallbackOrgRepo string
}

// NewResolver constructs a Resolver rooted at the given working directory.
func NewResolver(_ string) (*Resolver, error) {
	return &Resolver{}, nil
}

// Document represents a resolved standards file and where it came from.
type Document struct {
	Source string
	Body   string
}

// Resolve returns the effective standards Document for the target project.
// Resolution order: project (`.archon/standards.md`) > org-level > fallback.
func (r *Resolver) Resolve(ctx context.Context, target string) (*Document, error) {
	if doc, ok := r.fromProject(target); ok {
		return doc, nil
	}
	if r.fallbackOrgRepo != "" {
		// TODO: fetch from GitHub via the org/repo fallback
	}
	return nil, fmt.Errorf("no standards found for %s", target)
}

func (r *Resolver) fromProject(target string) (*Document, bool) {
	path := filepath.Join(target, ".archon", "standards.md")
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return &Document{Source: path, Body: string(body)}, true
}
