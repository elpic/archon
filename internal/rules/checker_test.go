package rules

import (
	"testing"

	"github.com/elpic/archon/internal/llm"
)

func TestPatternChecker_SinglePattern(t *testing.T) {
	rule := Rule{
		Name:     "no-latest-tag",
		Severity: "error",
		Body:     "Pattern: FROM.*:latest",
	}
	pc := NewPatternChecker(rule)

	content := []byte("FROM ubuntu:latest\nRUN apt-get update\n")
	violations := pc.Check("Dockerfile", content)

	if len(violations) != 1 {
		t.Fatalf("got %d violations, want 1", len(violations))
	}
	v := violations[0]
	if v.Rule != "no-latest-tag" {
		t.Errorf("Rule = %q, want %q", v.Rule, "no-latest-tag")
	}
	if v.File != "Dockerfile" {
		t.Errorf("File = %q, want %q", v.File, "Dockerfile")
	}
	if v.Line != 1 {
		t.Errorf("Line = %d, want 1", v.Line)
	}
	if v.Column != 1 {
		t.Errorf("Column = %d, want 1", v.Column)
	}
	if v.Severity != llm.SeverityError {
		t.Errorf("Severity = %v, want SeverityError", v.Severity)
	}
	if v.Description != "pattern match: FROM ubuntu:latest" {
		t.Errorf("Description = %q, want %q", v.Description, "pattern match: FROM ubuntu:latest")
	}
}

func TestPatternChecker_MultipleMatches(t *testing.T) {
	rule := Rule{
		Name:     "no-printf",
		Severity: "warn",
		Body:     "Pattern: fmt\\.Printf",
	}
	pc := NewPatternChecker(rule)

	content := []byte("package main\n\nfunc main() {\n\tfmt.Printf(\"hello\")\n\tfmt.Printf(\"world\")\n}\n")
	violations := pc.Check("main.go", content)

	if len(violations) != 2 {
		t.Fatalf("got %d violations, want 2", len(violations))
	}
	if violations[0].Line != 4 {
		t.Errorf("first violation Line = %d, want 4", violations[0].Line)
	}
	if violations[1].Line != 5 {
		t.Errorf("second violation Line = %d, want 5", violations[1].Line)
	}
}

func TestPatternChecker_MultiplePatterns(t *testing.T) {
	rule := Rule{
		Name:     "docker-checks",
		Severity: "error",
		Body: `Pattern: FROM.*:latest
Pattern: ADD.*http`,
	}
	pc := NewPatternChecker(rule)

	content := []byte("FROM node:latest\nADD https://example.com/file.tar.gz .\n")
	violations := pc.Check("Dockerfile", content)

	if len(violations) != 2 {
		t.Fatalf("got %d violations, want 2", len(violations))
	}
	if violations[0].Line != 1 {
		t.Errorf("first violation Line = %d, want 1", violations[0].Line)
	}
	if violations[1].Line != 2 {
		t.Errorf("second violation Line = %d, want 2", violations[1].Line)
	}
}

func TestPatternChecker_SkipsCommentLines(t *testing.T) {
	rule := Rule{
		Name:     "mixed",
		Severity: "warn",
		Body: `-- This is a comment
Pattern: TODO
-- Another comment
Pattern: FIXME`,
	}
	pc := NewPatternChecker(rule)

	content := []byte("TODO: fix this\nFIXME: broken\n")
	violations := pc.Check("file.go", content)

	if len(violations) != 2 {
		t.Fatalf("got %d violations, want 2", len(violations))
	}
}

func TestPatternChecker_NoPatterns(t *testing.T) {
	rule := Rule{
		Name:     "empty",
		Severity: "warn",
		Body:     "This is just body text with no patterns.",
	}
	pc := NewPatternChecker(rule)

	violations := pc.Check("file.go", []byte("content"))
	if violations != nil {
		t.Errorf("got %d violations, want nil", len(violations))
	}
}

func TestPatternChecker_NoMatches(t *testing.T) {
	rule := Rule{
		Name:     "no-match",
		Severity: "error",
		Body:     "Pattern: xyzzy_nonexistent",
	}
	pc := NewPatternChecker(rule)

	violations := pc.Check("file.go", []byte("hello world"))
	if len(violations) != 0 {
		t.Errorf("got %d violations, want 0", len(violations))
	}
}

func TestPatternChecker_MultilinePattern(t *testing.T) {
	rule := Rule{
		Name:     "no-raw-http",
		Severity: "error",
		Body:     "Pattern: (?s)http://.*\\n",
	}
	pc := NewPatternChecker(rule)

	content := []byte("url := \"http://example.com\"\n")
	violations := pc.Check("main.go", content)

	if len(violations) != 1 {
		t.Fatalf("got %d violations, want 1", len(violations))
	}
}

func TestPatternChecker_LineCalculation(t *testing.T) {
	rule := Rule{
		Name:     "test-line",
		Severity: "warn",
		Body:     "Pattern: TARGET",
	}
	pc := NewPatternChecker(rule)

	// Target is on line 3, column 5
	content := []byte("line1\nline2\n    TARGET here\n")
	violations := pc.Check("file.txt", content)

	if len(violations) != 1 {
		t.Fatalf("got %d violations, want 1", len(violations))
	}
	if violations[0].Line != 3 {
		t.Errorf("Line = %d, want 3", violations[0].Line)
	}
	if violations[0].Column != 5 {
		t.Errorf("Column = %d, want 5", violations[0].Column)
	}
}

func TestPatternChecker_InvalidRegex(t *testing.T) {
	rule := Rule{
		Name:     "bad-regex",
		Severity: "warn",
		Body:     "Pattern: [invalid",
	}
	pc := NewPatternChecker(rule)

	// Should not panic, just return no violations.
	violations := pc.Check("file.go", []byte("content"))
	if len(violations) != 0 {
		t.Errorf("got %d violations for invalid regex, want 0", len(violations))
	}
}

func TestParsePatterns(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"single", "Pattern: foo", 1},
		{"multiple", "Pattern: foo\nPattern: bar", 2},
		{"with comments", "-- comment\nPattern: foo\n-- another\nPattern: bar", 2},
		{"empty pattern", "Pattern: ", 0},
		{"no patterns", "just body text", 0},
		{"mixed", "Pattern: foo\nother text\nPattern: bar", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parsePatterns(tc.body)
			if len(got) != tc.want {
				t.Errorf("parsePatterns() returned %d patterns, want %d", len(got), tc.want)
			}
		})
	}
}

func TestPosToLineCol(t *testing.T) {
	content := []byte("abc\ndef\nghi")

	cases := []struct {
		name     string
		offset   int
		wantLine int
		wantCol  int
	}{
		{"start", 0, 1, 1},
		{"mid first line", 2, 1, 3},
		{"after first newline", 4, 2, 1},
		{"second line char", 6, 2, 3},
		{"after second newline", 8, 3, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			line, col := posToLineCol(content, tc.offset)
			if line != tc.wantLine || col != tc.wantCol {
				t.Errorf("posToLineCol(%d) = (%d, %d), want (%d, %d)",
					tc.offset, line, col, tc.wantLine, tc.wantCol)
			}
		})
	}
}

func TestParseSeverity(t *testing.T) {
	cases := []struct {
		input string
		want  llm.Severity
	}{
		{"info", llm.SeverityInfo},
		{"error", llm.SeverityError},
		{"critical", llm.SeverityCritical},
		{"warn", llm.SeverityWarn},
		{"", llm.SeverityWarn},
		{"unknown", llm.SeverityWarn},
		{"ERROR", llm.SeverityError},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := parseSeverity(tc.input)
			if got != tc.want {
				t.Errorf("parseSeverity(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestPatternChecker_SeverityMapping(t *testing.T) {
	cases := []struct {
		severity string
		want     llm.Severity
	}{
		{"info", llm.SeverityInfo},
		{"warn", llm.SeverityWarn},
		{"error", llm.SeverityError},
		{"critical", llm.SeverityCritical},
	}
	for _, tc := range cases {
		t.Run(tc.severity, func(t *testing.T) {
			rule := Rule{
				Name:     "test",
				Severity: tc.severity,
				Body:     "Pattern: X",
			}
			pc := NewPatternChecker(rule)
			violations := pc.Check("f", []byte("X"))
			if len(violations) != 1 {
				t.Fatalf("got %d violations, want 1", len(violations))
			}
			if violations[0].Severity != tc.want {
				t.Errorf("Severity = %v, want %v", violations[0].Severity, tc.want)
			}
		})
	}
}

func TestPatternChecker_ImplementsChecker(t *testing.T) {
	var _ Checker = (*PatternChecker)(nil)
}
