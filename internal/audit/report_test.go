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
