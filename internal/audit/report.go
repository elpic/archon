package audit

import (
	"fmt"
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
