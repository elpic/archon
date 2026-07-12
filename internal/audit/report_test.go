package audit

import (
	"regexp"
	"strings"
	"testing"

	"github.com/elpic/archon/internal/llm"
)

func TestReport_Format_NoViolations(t *testing.T) {
	r := &Report{Target: "./..."}
	got := r.Format()
	if !strings.Contains(got, "no violations") {
		t.Errorf("expected 'no violations' in output, got: %q", got)
	}
	if !strings.Contains(got, "./...") {
		t.Errorf("expected target in output, got: %q", got)
	}
	if strings.Contains(got, "Standards:") {
		t.Errorf("expected no 'Standards:' line when source is empty, got: %q", got)
	}
}

func TestReport_Format_WithViolations(t *testing.T) {
	r := &Report{
		Target: "./...",
		Violations: []llm.Violation{
			{Rule: "no-comments", Description: "Comments are forbidden", Severity: llm.SeverityError},
		},
	}
	got := r.Format()
	if !strings.Contains(got, "1 violation") {
		t.Errorf("expected '1 violation' in output, got: %q", got)
	}
	if !strings.Contains(got, "no-comments") {
		t.Errorf("expected rule name in output, got: %q", got)
	}
	if !strings.Contains(got, "error") {
		t.Errorf("expected severity in output, got: %q", got)
	}
}

func TestReport_Format_WithStandardsSource(t *testing.T) {
	cases := []struct {
		name    string
		report  Report
		wantSub string
	}{
		{
			name: "source only, no violations",
			report: Report{
				Target:          "./...",
				StandardsSource: "github.com/elpic/go-standards@abc123",
			},
			wantSub: "Standards: github.com/elpic/go-standards@abc123",
		},
		{
			name: "source and violations",
			report: Report{
				Target:          "./...",
				StandardsSource: "github.com/elpic/go-standards@abc123",
				Violations: []llm.Violation{
					{Rule: "r1", Description: "d1", Severity: llm.SeverityWarn},
				},
			},
			wantSub: "Standards: github.com/elpic/go-standards@abc123",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.report.Format()
			if !strings.Contains(got, tc.wantSub) {
				t.Errorf("expected %q in output, got: %q", tc.wantSub, got)
			}
			// The Standards line must come before any violation list.
			idx := strings.Index(got, "Standards:")
			if idx != 0 {
				t.Errorf("expected 'Standards:' at start of output, got idx=%d: %q", idx, got)
			}
		})
	}
}

// TestReport_FormatDiagnostic_FullCoords: with a Violation that
// has a file and line/column, FormatDiagnostic must produce a single
// line in the problem-matcher format:
//
//	path:line:col: [severity] message
//
// The regex pins the exact shape so editor problem-matchers
// (Neovim quickfix, Helix, VS Code "Error Lens" via the
// problem-matcher action) can rely on it.
func TestReport_FormatDiagnostic_FullCoords(t *testing.T) {
	r := &Report{
		Target: "./...",
		Violations: []llm.Violation{
			{
				Rule:        "no-comments",
				Description: "Comments are forbidden",
				Severity:    llm.SeverityError,
				File:        "internal/foo/foo.go",
				Line:        42,
				Column:      7,
			},
		},
	}
	got := r.FormatDiagnostic()
	want := "internal/foo/foo.go:42:7: [error] Comments are forbidden\n"
	if got != want {
		t.Errorf("FormatDiagnostic() = %q, want %q", got, want)
	}
	pattern := regexp.MustCompile(`^.+:\d+:\d+: \[[a-z]+\] .+\n$`)
	if !pattern.MatchString(got) {
		t.Errorf("FormatDiagnostic() = %q does not match problem-matcher pattern %q", got, pattern)
	}
}

// TestReport_FormatDiagnostic_MissingLine: a Violation with a
// file but no line/column renders as "path:?:?" so editors that
// jump to a coordinate get a clear "unknown" signal rather than
// ":0:0" which is a real (if odd) coordinate.
func TestReport_FormatDiagnostic_MissingLine(t *testing.T) {
	r := &Report{
		Target: "./...",
		Violations: []llm.Violation{
			{
				Rule:        "r1",
				Description: "d1",
				Severity:    llm.SeverityWarn,
				File:        "x.go",
			},
		},
	}
	got := r.FormatDiagnostic()
	want := "x.go:?:?: [warn] d1\n"
	if got != want {
		t.Errorf("FormatDiagnostic() = %q, want %q", got, want)
	}
}

// TestReport_FormatDiagnostic_NoCoords: a Violation with no source
// coordinates at all renders as "?:?:?" so the line still parses
// and the reader is not misled into thinking line 0 column 0 is
// meaningful.
func TestReport_FormatDiagnostic_NoCoords(t *testing.T) {
	r := &Report{
		Target: "./...",
		Violations: []llm.Violation{
			{Rule: "r1", Description: "d1", Severity: llm.SeverityInfo},
		},
	}
	got := r.FormatDiagnostic()
	want := "?:?:?: [info] d1\n"
	if got != want {
		t.Errorf("FormatDiagnostic() = %q, want %q", got, want)
	}
	pattern := regexp.MustCompile(`^.+:\?:\?: \[[a-z]+\] .+\n$`)
	if !pattern.MatchString(got) {
		t.Errorf("FormatDiagnostic() = %q does not match ?-coords pattern %q", got, pattern)
	}
}

// TestReport_FormatDiagnostic_Empty: an empty Report must produce
// empty output (no header, no summary) so `archon watch` can
// safely print the result on every change without polluting the
// editor quickfix list.
func TestReport_FormatDiagnostic_Empty(t *testing.T) {
	r := &Report{Target: "./..."}
	got := r.FormatDiagnostic()
	if got != "" {
		t.Errorf("FormatDiagnostic() on empty report = %q, want empty", got)
	}
}

// TestReport_FormatDiagnostic_Multiple: every violation in the
// report becomes its own line, in order, so the editor can jump
// through them sequentially.
func TestReport_FormatDiagnostic_Multiple(t *testing.T) {
	r := &Report{
		Target: "./...",
		Violations: []llm.Violation{
			{Rule: "r1", Description: "d1", Severity: llm.SeverityError, File: "a.go", Line: 1, Column: 1},
			{Rule: "r2", Description: "d2", Severity: llm.SeverityWarn, File: "b.go", Line: 2, Column: 2},
		},
	}
	got := r.FormatDiagnostic()
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %q", len(lines), got)
	}
	if lines[0] != "a.go:1:1: [error] d1" {
		t.Errorf("line 0 = %q", lines[0])
	}
	if lines[1] != "b.go:2:2: [warn] d2" {
		t.Errorf("line 1 = %q", lines[1])
	}
}

func TestIsSafePath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"", false},
		{"/etc/passwd", false},
		{"../etc/passwd", false},
		{".", false},
		{"internal/foo.go", true},
		{"a/b/c.go", true},
		{"my..config.go", false},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got := isSafePath(tc.path)
			if got != tc.want {
				t.Errorf("isSafePath(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestFormatFix_NoViolations(t *testing.T) {
	r := &Report{Target: "./..."}
	got := r.FormatFix()
	if got != "" {
		t.Errorf("FormatFix() on empty report = %q, want empty", got)
	}
}

func TestFormatFix_WithSuggestion(t *testing.T) {
	r := &Report{
		Target: "./...",
		Violations: []llm.Violation{
			{
				Rule:        "no-comments",
				Description: "found a comment",
				Severity:    llm.SeverityError,
				File:        "internal/foo/foo.go",
				Line:        42,
				Suggestion:  "remove the comment",
			},
		},
	}
	got := r.FormatFix()
	if !strings.Contains(got, "--- a/internal/foo/foo.go") {
		t.Errorf("expected '--- a/...' in output, got: %q", got)
	}
	if !strings.Contains(got, "+++ b/internal/foo/foo.go") {
		t.Errorf("expected '+++ b/...' in output, got: %q", got)
	}
	if !strings.Contains(got, "-found a comment") {
		t.Errorf("expected '-found a comment' in output, got: %q", got)
	}
	if !strings.Contains(got, "+remove the comment") {
		t.Errorf("expected '+remove the comment' in output, got: %q", got)
	}
}

func TestFormatFix_NoSuggestion(t *testing.T) {
	r := &Report{
		Target: "./...",
		Violations: []llm.Violation{
			{
				Rule:        "no-comments",
				Description: "found a comment",
				Severity:    llm.SeverityError,
				File:        "internal/foo/foo.go",
				Line:        42,
			},
		},
	}
	got := r.FormatFix()
	if got != "" {
		t.Errorf("FormatFix() without suggestion = %q, want empty", got)
	}
}

func TestFormatFix_UnsafePath(t *testing.T) {
	r := &Report{
		Target: "./...",
		Violations: []llm.Violation{
			{
				Rule:        "no-comments",
				Description: "found a comment",
				Severity:    llm.SeverityError,
				File:        "../etc/passwd",
				Line:        1,
				Suggestion:  "fix it",
			},
		},
	}
	got := r.FormatFix()
	if got != "" {
		t.Errorf("FormatFix() with unsafe path = %q, want empty", got)
	}
}

func TestFormatFix_MultipleViolations(t *testing.T) {
	r := &Report{
		Target: "./...",
		Violations: []llm.Violation{
			{
				Rule:        "r1",
				Description: "d1",
				Severity:    llm.SeverityError,
				File:        "a.go",
				Line:        1,
				Suggestion:  "fix1",
			},
			{
				Rule:        "r2",
				Description: "d2",
				Severity:    llm.SeverityWarn,
				File:        "b.go",
				Line:        2,
				Suggestion:  "fix2",
			},
			{
				Rule:        "r3",
				Description: "d3",
				Severity:    llm.SeverityInfo,
				File:        "c.go",
				// no suggestion — should be skipped
			},
		},
	}
	got := r.FormatFix()
	if !strings.Contains(got, "a.go") {
		t.Errorf("expected a.go in output, got: %q", got)
	}
	if !strings.Contains(got, "b.go") {
		t.Errorf("expected b.go in output, got: %q", got)
	}
	if strings.Contains(got, "c.go") {
		t.Errorf("c.go should not appear (no suggestion), got: %q", got)
	}
}

func TestFormatFix_ZeroLine(t *testing.T) {
	r := &Report{
		Target: "./...",
		Violations: []llm.Violation{
			{
				Rule:       "r1",
				Description: "d1",
				Severity:   llm.SeverityError,
				File:       "a.go",
				Line:       0,
				Suggestion: "fix",
			},
		},
	}
	got := r.FormatFix()
	if !strings.Contains(got, "@@ -0,0 +1,1 @@") {
		t.Errorf("expected '@@ -0,0 +1,1 @@' fallback for zero line, got: %q", got)
	}
}
