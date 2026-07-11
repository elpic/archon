// Package audit runs the rule set against a project and produces a report.
package audit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/elpic/archon/internal/llm"
	"github.com/elpic/archon/internal/standards"
)

// Runner orchestrates a single audit pass over a target project.
type Runner struct {
	resolver     *standards.Resolver
	llmProvider  llm.Provider
	changedFiles []string // if non-empty, only audit these files
}

// NewRunner constructs a Runner wired with the given standards resolver
// and LLM provider.
func NewRunner(resolver *standards.Resolver, llmProvider llm.Provider) *Runner {
	return &Runner{resolver: resolver, llmProvider: llmProvider}
}

// WithChangedFiles restricts the audit to only the given files (relative to target).
// If empty, the entire project is audited.
func (r *Runner) WithChangedFiles(files []string) *Runner {
	r.changedFiles = files
	return r
}

// Run executes the audit and returns a Report. The Report's Format method
// renders it to a human-readable string; callers may also inspect the
// structured violations before formatting.
//
// The returned Report carries the resolved StandardsSource so downstream
// tooling (and the human reader) can see whether the project file was
// used directly or whether standards were inherited from an org or
// fallback repo.
func (r *Runner) Run(ctx context.Context, target string) (*Report, error) {
	doc, err := r.resolver.Resolve(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("resolve standards: %w", err)
	}

	var changedFiles []string
	if len(r.changedFiles) > 0 {
		// Filter the target to only changed files for the LLM
		var paths []string
		for _, f := range r.changedFiles {
			full := filepath.Join(target, f)
			if _, err := os.Stat(full); err == nil {
				paths = append(paths, f)
			}
		}
		if len(paths) == 0 {
			// No changed files exist (they might have been deleted)
			return &Report{
				Target:          target,
				Violations:      nil,
				StandardsSource: "no changed files",
			}, nil
		}
		changedFiles = paths
	}

	violations, err := r.llmProvider.Audit(ctx, []byte(doc.Body), target, changedFiles)
	if err != nil {
		return nil, fmt.Errorf("llm audit: %w", err)
	}

	return &Report{
		Target:          target,
		Violations:      violations,
		StandardsSource: doc.Source,
	}, nil
}
