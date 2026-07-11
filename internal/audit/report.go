package audit

import (
	"fmt"
	"strings"

	"github.com/elpic/archon/internal/llm"
)

// Report aggregates violations from a single audit Run.
type Report struct {
	Target     string
	Violations []llm.Violation
}

// Format renders the report for terminal output.
func (r *Report) Format() string {
	if len(r.Violations) == 0 {
		return fmt.Sprintf("✓ %s: no violations\n", r.Target)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Archon audit: %s\n", r.Target)
	fmt.Fprintf(&b, "%d violation(s)\n\n", len(r.Violations))
	for i, v := range r.Violations {
		fmt.Fprintf(&b, "%d. [%s] %s\n   %s\n", i+1, v.Severity, v.Rule, v.Description)
	}
	return b.String()
}
