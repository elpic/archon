package rules

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/elpic/archon/internal/llm"
)

// Verdict represents the outcome of evaluating a single rule.
type Verdict int

const (
	VerdictPass Verdict = iota
	VerdictViolation
	VerdictNA // rule did not apply (no matching files)
)

// String returns the human-readable label for a verdict.
func (v Verdict) String() string {
	switch v {
	case VerdictPass:
		return "pass"
	case VerdictViolation:
		return "violation"
	case VerdictNA:
		return "N/A"
	default:
		return "unknown"
	}
}

// RuleResult holds the outcome of a single rule after evaluation.
type RuleResult struct {
	Rule       Rule
	Verdict    Verdict
	Violations []llm.Violation
}

// Report aggregates the results of running all rules against a target.
type Report struct {
	Target   string
	Results  []RuleResult
	Score    int // 0-100, weighted, N/A rules excluded
	Violated int // total violations across all rules
}

// NewReport builds a Report from a set of rule results and the target path.
// Score = round(passing / applicable * 100). N/A rules are excluded.
func NewReport(target string, results []RuleResult) *Report {
	var totalWeight, passWeight int
	for _, r := range results {
		if r.Verdict == VerdictNA {
			continue
		}
		totalWeight += r.Rule.Weight
		if r.Verdict == VerdictPass {
			passWeight += r.Rule.Weight
		}
	}

	score := 0
	if totalWeight > 0 {
		score = int(float64(passWeight) / float64(totalWeight) * 100 + 0.5)
	}

	violated := 0
	for _, r := range results {
		violated += len(r.Violations)
	}

	return &Report{
		Target:   target,
		Results:  results,
		Score:    score,
		Violated: violated,
	}
}

// Format renders the report for terminal output.
func (r *Report) Format() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Archon audit: %s\n", r.Target)
	fmt.Fprintf(&b, "Score: %d/100 (%d violation(s))\n\n", r.Score, r.Violated)

	// Group results by category for readable output.
	byCategory := groupByCategory(r.Results)
	categories := make([]string, 0, len(byCategory))
	for cat := range byCategory {
		categories = append(categories, cat)
	}
	sort.Strings(categories)

	for _, cat := range categories {
		results := byCategory[cat]
		fmt.Fprintf(&b, "## %s\n", cat)
		for _, rr := range results {
			icon := verdictIcon(rr.Verdict)
			fmt.Fprintf(&b, "  %s %s (weight=%d)\n", icon, rr.Rule.Name, rr.Rule.Weight)
			for _, v := range rr.Violations {
				fmt.Fprintf(&b, "    - %s\n", v.Description)
				if v.File != "" {
					fmt.Fprintf(&b, "      file: %s\n", v.File)
				}
				if v.Line > 0 {
					fmt.Fprintf(&b, "      line: %d\n", v.Line)
				}
			}
		}
		fmt.Fprintf(&b, "\n")
	}

	return b.String()
}

// FormatDiagnostic renders each violation in problem-matcher format:
//
//	path:line:col: [severity] message
func (r *Report) FormatDiagnostic() string {
	if r.Violated == 0 {
		return ""
	}
	var b strings.Builder
	for _, rr := range r.Results {
		for _, v := range rr.Violations {
			fmt.Fprintln(&b, v.String())
		}
	}
	return b.String()
}

// Findings returns all violations across all rules.
func (r *Report) Findings() []llm.Violation {
	var all []llm.Violation
	for _, rr := range r.Results {
		all = append(all, rr.Violations...)
	}
	return all
}

// writeFile writes data to a file, creating parent dirs as needed.
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// groupByCategory groups RuleResults by their rule's category.
func groupByCategory(results []RuleResult) map[string][]RuleResult {
	m := make(map[string][]RuleResult)
	for _, rr := range results {
		cat := rr.Rule.Category
		if cat == "" {
			cat = "general"
		}
		m[cat] = append(m[cat], rr)
	}
	return m
}

// verdictIcon returns a terminal-friendly icon for the verdict.
func verdictIcon(v Verdict) string {
	switch v {
	case VerdictPass:
		return "✓"
	case VerdictViolation:
		return "✗"
	case VerdictNA:
		return "–"
	default:
		return "?"
	}
}

// WriteFindings writes the findings markdown to the cache dir.
// Path: $XDG_CACHE_HOME/archon/findings/<target-basename>/findings.md
// Falls back to $HOME/.cache if XDG_CACHE_HOME is unset.
func (r *Report) WriteFindings(cacheDir string) (string, error) {
	if cacheDir == "" {
		return "", fmt.Errorf("cache dir is empty")
	}
	targetName := filepath.Base(r.Target)
	outDir := filepath.Join(cacheDir, "archon", "findings", targetName)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}
	outPath := filepath.Join(outDir, "findings.md")
	content := r.Format()
	if err := writeFile(outPath, []byte(content)); err != nil {
		return "", fmt.Errorf("write findings: %w", err)
	}
	return outPath, nil
}
