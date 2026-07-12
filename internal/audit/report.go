package audit

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/elpic/archon/internal/llm"
)

// Report aggregates violations from a single audit Run.
//
// StandardsSource records where the resolved standards came from. For
// project-local files this is the on-disk path; for org-level or
// fallback sources it is "github.com/<owner>/<repo>@<sha>". Empty when
// the resolver returned without a source (currently unreachable).
type Report struct {
	Target          string
	Violations      []llm.Violation
	StandardsSource string
}

// Format renders the report for terminal output.
//
// When StandardsSource is non-empty, a "Standards: <source>" line is
// emitted at the top so the reader can see which tier was used to
// resolve the standards.
func (r *Report) Format() string {
	var b strings.Builder
	if r.StandardsSource != "" {
		fmt.Fprintf(&b, "Standards: %s\n", r.StandardsSource)
	}
	if len(r.Violations) == 0 {
		if r.StandardsSource == "" {
			return fmt.Sprintf("✓ %s: no violations\n", r.Target)
		}
		fmt.Fprintf(&b, "✓ %s: no violations\n", r.Target)
		return b.String()
	}
	fmt.Fprintf(&b, "Archon audit: %s\n", r.Target)
	fmt.Fprintf(&b, "%d violation(s)\n\n", len(r.Violations))
	for i, v := range r.Violations {
		fmt.Fprintf(&b, "%d. [%s] %s\n   %s\n", i+1, v.Severity, v.Rule, v.Description)
	}
	return b.String()
}

// FormatDiagnostic renders each violation in the "problem-matcher"
// format consumed by editor quickfix / GCC / clippy style pickers:
//
//	path:line:col: [severity] message
//
// It is the output contract for the `archon watch` subcommand: one
// violation per line, with no header or summary, so editors can
// stream the lines and jump directly to the offending code. A
// violation with a missing file or zero line/column renders as
// "?:line:col" / "path:?:col" / "path:line:?" so the line always
// parses as a problem matcher entry.
func (r *Report) FormatDiagnostic() string {
	if len(r.Violations) == 0 {
		return ""
	}
	var b strings.Builder
	for _, v := range r.Violations {
		fmt.Fprintln(&b, v.String())
	}
	return b.String()
}

// FormatFix renders each violation with a suggested fix in unified diff
// format. Only violations that have a Suggestion field are included.
// The output is a human-readable suggestion; it is NOT a machine-consumable
// patch (the "-" line is the violation description, not actual file content).
func (r *Report) FormatFix() string {
	if len(r.Violations) == 0 {
		return ""
	}
	var b strings.Builder
	for _, v := range r.Violations {
		if v.Suggestion == "" {
			continue
		}
		if v.File == "" {
			continue
		}
		// Validate file path to prevent path traversal
		if !isSafePath(v.File) {
			continue
		}
		// Generate a minimal unified diff for the suggestion
		fmt.Fprintf(&b, "--- a/%s\n", v.File)
		fmt.Fprintf(&b, "+++ b/%s\n", v.File)
		if v.Line > 0 {
			fmt.Fprintf(&b, "@@ -%d,1 +%d,1 @@\n", v.Line, v.Line)
		} else {
			fmt.Fprintf(&b, "@@ -0,0 +1,1 @@\n")
		}
		fmt.Fprintf(&b, "-%s\n", v.Description)
		fmt.Fprintf(&b, "+%s\n", v.Suggestion)
	}
	return b.String()
}

// isSafePath reports whether path is safe to use in a diff.
// It rejects absolute paths, paths with directory traversal, and empty strings.
func isSafePath(path string) bool {
	if path == "" {
		return false
	}
	if filepath.IsAbs(path) {
		return false
	}
	// Check for directory traversal attempts
	if strings.Contains(path, "..") {
		return false
	}
	// Reject empty or single-dot paths
	clean := filepath.Clean(path)
	if clean == "." || clean == "" {
		return false
	}
	return true
}
