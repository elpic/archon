package audit

import (
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
