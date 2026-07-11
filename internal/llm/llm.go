// Package llm abstracts over the LLM provider used to drive the audit.
// The default implementation talks to OpenRouter; swapping providers is
// a matter of implementing the Provider interface.
package llm

import (
	"context"
	"fmt"
	"strings"
)

// Provider drives an audit pass against a project directory using the
// resolved standards Document as the rubric.
type Provider interface {
	Audit(ctx context.Context, standardsBody []byte, target string) ([]Violation, error)
}

// Violation is a single standards deviation found by the LLM provider.
//
// Source coordinates (File / Line / Column) are populated when the
// provider can locate the offending code; they are zero-valued
// otherwise. The string representations rendered by String() use
// "? for any unset coordinate so the output stays a single line
// consumable by editor problem-matchers.
type Violation struct {
	Rule        string
	Description string
	Severity    Severity
	File        string
	Line        int
	Column      int
}

// String renders the violation in the "problem-matcher" format used by
// the watch subcommand:
//
//	path:line:col: [severity] message
//
// Missing coordinates are rendered as "?" rather than the zero value
// (":0:0" would mislead readers into thinking line 0 / column 0 is
// meaningful). Callers that want richer rendering (the audit
// subcommand's terminal layout) should use Report.Format() instead.
func (v Violation) String() string {
	path := v.File
	if path == "" {
		path = "?"
	}
	line := "?"
	if v.Line > 0 {
		line = fmt.Sprintf("%d", v.Line)
	}
	col := "?"
	if v.Column > 0 {
		col = fmt.Sprintf("%d", v.Column)
	}
	var b strings.Builder
	b.WriteString(path)
	b.WriteByte(':')
	b.WriteString(line)
	b.WriteByte(':')
	b.WriteString(col)
	b.WriteString(": [")
	b.WriteString(v.Severity.String())
	b.WriteString("] ")
	b.WriteString(v.Description)
	return b.String()
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
