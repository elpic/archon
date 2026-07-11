// Package audit runs the rule set against a project and produces a report.
package audit

import (
	"context"
	"fmt"

	"github.com/elpic/archon/internal/llm"
	"github.com/elpic/archon/internal/standards"
)

// Runner orchestrates a single audit pass over a target project.
type Runner struct {
	resolver     *standards.Resolver
	llmProvider  llm.Provider
}

// NewRunner constructs a Runner wired with the given standards resolver
// and LLM provider.
func NewRunner(resolver *standards.Resolver, llmProvider llm.Provider) *Runner {
	return &Runner{resolver: resolver, llmProvider: llmProvider}
}

// Run executes the audit and returns a Report. The Report's Format method
// renders it to a human-readable string; callers may also inspect the
// structured violations before formatting.
func (r *Runner) Run(ctx context.Context, target string) (*Report, error) {
	docs, err := r.resolver.Resolve(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("resolve standards: %w", err)
	}

	violations, err := r.llmProvider.Audit(ctx, []byte(docs.Body), target)
	if err != nil {
		return nil, fmt.Errorf("llm audit: %w", err)
	}

	return &Report{Target: target, Violations: violations}, nil
}
