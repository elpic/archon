// Package llm abstracts over the LLM provider used to drive the audit.
// The default implementation talks to OpenRouter; swapping providers is
// a matter of implementing the Provider interface.
package llm

import (
	"context"
	"fmt"
)

// Provider drives an audit pass against a project directory using the
// resolved standards Document as the rubric.
type Provider interface {
	Audit(ctx context.Context, standardsBody []byte, target string) ([]Violation, error)
}

// Violation is a single standards deviation found by the LLM provider.
type Violation struct {
	Rule        string
	Description string
	Severity    Severity
}

// Severity ranks how strongly a rule was violated.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarn
	SeverityError
	SeverityCritical
)

func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarn:
		return "warn"
	case SeverityError:
		return "error"
	case SeverityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

func New(ctx context.Context) (Provider, error) {
	// Default implementation will dispatch on env vars (OPENROUTER_API_KEY,
	// OPENAI_API_KEY, ANTHROPIC_API_KEY) to pick the right backend.
	return nil, fmt.Errorf("llm.New not yet implemented")
}
