// Package llm abstracts over the LLM provider used to drive the audit.
// The default implementation talks to OpenRouter; swapping providers is
// a matter of implementing the Provider interface.
package llm

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// Provider drives an audit pass against a project directory using the
// resolved standards Document as the rubric.
//
// If changedFiles is non-empty, the provider should only audit those
// files (relative to the target directory).
type Provider interface {
	Audit(ctx context.Context, standardsBody []byte, target string, changedFiles []string) ([]Violation, error)
}

// Violation is a single standards deviation found by the LLM provider.
//
// Source coordinates (File / Line / Column) are populated when the
// provider can locate the offending code; they are zero-valued
// otherwise. The string representations rendered by String() use
// "? for any unset coordinate so the output stays a single line
// consumable by editor problem-matchers.
//
// Suggestion is a concrete fix the user can apply (unified diff hunk).
// RuleDoc is a markdown anchor or URL pointing to the rule's documentation.
type Violation struct {
	Rule        string
	Description string
	Severity    Severity
	File        string
	Line        int
	Column      int
	Suggestion  string
	RuleDoc     string
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

// New constructs a Provider by inspecting environment variables.
// Priority order:
//  1. OPENAI_API_KEY  → OpenAI (api.openai.com)
//  2. OPENROUTER_API_KEY → OpenRouter (openrouter.ai/api/v1, or OPENROUTER_BASE_URL)
//  3. ANTHROPIC_API_KEY → not yet implemented
//
// Returns an error if no provider can be selected.
func New(_ context.Context) (Provider, error) {
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return newOpenAIProvider(key, "https://api.openai.com/v1", ""), nil
	}

	if key := os.Getenv("OPENROUTER_API_KEY"); key != "" {
		baseURL := os.Getenv("OPENROUTER_BASE_URL")
		if baseURL == "" {
			baseURL = "https://openrouter.ai/api/v1"
		}
		return newOpenAIProvider(key, baseURL, ""), nil
	}

	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return nil, fmt.Errorf("anthropic provider not yet implemented; set OPENAI_API_KEY or OPENROUTER_API_KEY instead")
	}

	return nil, fmt.Errorf("no LLM API key found; set OPENAI_API_KEY, OPENROUTER_API_KEY, or ANTHROPIC_API_KEY")
}
