package rules

import (
	"testing"

	"github.com/elpic/archon/internal/llm"
)

func TestFileChecker_ImplementsDirectoryChecker(t *testing.T) {
	var _ DirectoryChecker = (*FileChecker)(nil)
}

// --- File directive ---

func TestFileChecker_FileExists(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"Dockerfile": "FROM ubuntu:22.04\n",
	})

	rule := Rule{
		Name:     "require-dockerfile",
		Severity: "error",
		Body:     "File: Dockerfile",
	}
	fc := NewFileChecker(rule)

	violations := fc.CheckDirectory(dir)
	if len(violations) != 0 {
		t.Errorf("got %d violations, want 0 (file exists)", len(violations))
	}
}

func TestFileChecker_FileMissing(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"README.md": "# Hello\n",
	})

	rule := Rule{
		Name:     "require-dockerfile",
		Severity: "error",
		Body:     "File: Dockerfile",
	}
	fc := NewFileChecker(rule)

	violations := fc.CheckDirectory(dir)
	if len(violations) != 1 {
		t.Fatalf("got %d violations, want 1", len(violations))
	}
	v := violations[0]
	if v.Rule != "require-dockerfile" {
		t.Errorf("Rule = %q, want %q", v.Rule, "require-dockerfile")
	}
	if v.Severity != llm.SeverityError {
		t.Errorf("Severity = %v, want SeverityError", v.Severity)
	}
	if v.File != "Dockerfile" {
		t.Errorf("File = %q, want %q", v.File, "Dockerfile")
	}
}

func TestFileChecker_FileGlob(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"docker-compose.yml": "version: '3'\n",
	})

	rule := Rule{
		Name:     "require-compose",
		Severity: "warn",
		Body:     "File: docker-compose.*",
	}
	fc := NewFileChecker(rule)

	violations := fc.CheckDirectory(dir)
	if len(violations) != 0 {
		t.Errorf("got %d violations, want 0 (glob matches)", len(violations))
	}
}

func TestFileChecker_FileGlobNoMatch(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"Makefile": "all:\n",
	})

	rule := Rule{
		Name:     "require-compose",
		Severity: "warn",
		Body:     "File: docker-compose.*",
	}
	fc := NewFileChecker(rule)

	violations := fc.CheckDirectory(dir)
	if len(violations) != 1 {
		t.Fatalf("got %d violations, want 1", len(violations))
	}
}

// --- NoFile directive ---

func TestFileChecker_NoFileAbsent(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"README.md": "# Hello\n",
	})

	rule := Rule{
		Name:     "no-env-file",
		Severity: "error",
		Body:     "NoFile: .env",
	}
	fc := NewFileChecker(rule)

	violations := fc.CheckDirectory(dir)
	if len(violations) != 0 {
		t.Errorf("got %d violations, want 0 (.env absent)", len(violations))
	}
}

func TestFileChecker_NoFilePresent(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		".env": "SECRET=abc\n",
	})

	rule := Rule{
		Name:     "no-env-file",
		Severity: "error",
		Body:     "NoFile: .env",
	}
	fc := NewFileChecker(rule)

	violations := fc.CheckDirectory(dir)
	if len(violations) != 1 {
		t.Fatalf("got %d violations, want 1", len(violations))
	}
	v := violations[0]
	if v.Rule != "no-env-file" {
		t.Errorf("Rule = %q, want %q", v.Rule, "no-env-file")
	}
	if v.Severity != llm.SeverityError {
		t.Errorf("Severity = %v, want SeverityError", v.Severity)
	}
}

func TestFileChecker_NoFileGlob(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"debug.log": "log data\n",
	})

	rule := Rule{
		Name:     "no-log-files",
		Severity: "warn",
		Body:     "NoFile: *.log",
	}
	fc := NewFileChecker(rule)

	violations := fc.CheckDirectory(dir)
	if len(violations) != 1 {
		t.Fatalf("got %d violations, want 1", len(violations))
	}
}

// --- Content directive ---

func TestFileChecker_ContentMatch(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"Dockerfile": "FROM ubuntu:22.04\nRUN apt-get update\n",
	})

	rule := Rule{
		Name:     "require-from",
		Severity: "error",
		Body:     "Content: Dockerfile contains FROM",
	}
	fc := NewFileChecker(rule)

	violations := fc.CheckDirectory(dir)
	if len(violations) != 0 {
		t.Errorf("got %d violations, want 0 (content contains pattern)", len(violations))
	}
}

func TestFileChecker_ContentMissing(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"Dockerfile": "RUN apt-get update\n",
	})

	rule := Rule{
		Name:     "require-from",
		Severity: "error",
		Body:     "Content: Dockerfile contains FROM",
	}
	fc := NewFileChecker(rule)

	violations := fc.CheckDirectory(dir)
	if len(violations) != 1 {
		t.Fatalf("got %d violations, want 1", len(violations))
	}
	v := violations[0]
	if v.Rule != "require-from" {
		t.Errorf("Rule = %q, want %q", v.Rule, "require-from")
	}
	if v.Severity != llm.SeverityError {
		t.Errorf("Severity = %v, want SeverityError", v.Severity)
	}
}

func TestFileChecker_ContentGlobMultipleFiles(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go":    "package main\n\nfunc main() {}\n",
		"handler.go": "package main\n\nfunc handle() {}\n",
	})

	rule := Rule{
		Name:     "require-package",
		Severity: "error",
		Body:     "Content: *.go contains package main",
	}
	fc := NewFileChecker(rule)

	// All .go files contain "package main" — no violations.
	violations := fc.CheckDirectory(dir)
	if len(violations) != 0 {
		t.Errorf("got %d violations, want 0", len(violations))
	}
}

func TestFileChecker_ContentGlobNoMatch(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"main.go": "package main\n",
	})

	rule := Rule{
		Name:     "require-usage",
		Severity: "warn",
		Body:     "Content: *.txt contains hello",
	}
	fc := NewFileChecker(rule)

	// No .txt files exist — should report the glob has no matches.
	violations := fc.CheckDirectory(dir)
	if len(violations) != 1 {
		t.Fatalf("got %d violations, want 1", len(violations))
	}
}

func TestFileChecker_ContentGlobMixed(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"a.go": "package main\nimport fmt\n",
		"b.go": "package other\n",
	})

	rule := Rule{
		Name:     "require-fmt",
		Severity: "warn",
		Body:     "Content: *.go contains import fmt",
	}
	fc := NewFileChecker(rule)

	violations := fc.CheckDirectory(dir)
	if len(violations) != 1 {
		t.Fatalf("got %d violations, want 1 (b.go missing import)", len(violations))
	}
}

// --- Mixed directives ---

func TestFileChecker_MixedDirectives(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"Dockerfile": "FROM ubuntu:22.04\n",
		".env":       "SECRET=abc\n",
	})

	rule := Rule{
		Name:     "mixed-checks",
		Severity: "error",
		Body: `File: Dockerfile
NoFile: .env`,
	}
	fc := NewFileChecker(rule)

	violations := fc.CheckDirectory(dir)
	if len(violations) != 1 {
		t.Fatalf("got %d violations, want 1 (.env is forbidden)", len(violations))
	}
}

func TestFileChecker_CommentsSkipped(t *testing.T) {
	dir := setupTestDir(t, map[string]string{})

	rule := Rule{
		Name:     "commented",
		Severity: "warn",
		Body: `-- This is a comment
File: Dockerfile
-- Another comment`,
	}
	fc := NewFileChecker(rule)

	violations := fc.CheckDirectory(dir)
	if len(violations) != 1 {
		t.Fatalf("got %d violations, want 1 (only the File directive matters)", len(violations))
	}
}

func TestFileChecker_NoDirectives(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"Dockerfile": "FROM ubuntu:22.04\n",
	})

	rule := Rule{
		Name:     "no-ops",
		Severity: "warn",
		Body:     "This rule has no file directives.",
	}
	fc := NewFileChecker(rule)

	violations := fc.CheckDirectory(dir)
	if violations != nil {
		t.Errorf("got %d violations, want nil", len(violations))
	}
}

// --- Empty directives ---

func TestFileChecker_EmptyGlobIgnored(t *testing.T) {
	rule := Rule{
		Name:     "empty-globs",
		Severity: "warn",
		Body:     "File: \nNoFile: \nContent:  contains ",
	}
	fc := NewFileChecker(rule)

	dir := t.TempDir()
	violations := fc.CheckDirectory(dir)
	if violations != nil {
		t.Errorf("got %d violations, want nil (empty directives ignored)", len(violations))
	}
}

func TestFileChecker_ContentMissingContains(t *testing.T) {
	rule := Rule{
		Name:     "bad-content",
		Severity: "warn",
		Body:     "Content: Dockerfile",
	}
	fc := NewFileChecker(rule)

	dir := t.TempDir()
	violations := fc.CheckDirectory(dir)
	if violations != nil {
		t.Errorf("got %d violations, want nil (malformed Content line ignored)", len(violations))
	}
}

// --- parseFileDirectives ---

func TestParseFileDirectives(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		wantCount int
	}{
		{"single File", "File: Dockerfile", 1},
		{"single NoFile", "NoFile: .env", 1},
		{"single Content", "Content: Dockerfile contains FROM", 1},
		{"mixed", "File: Dockerfile\nNoFile: .env\nContent: README.md contains hello", 3},
		{"with comments", "-- comment\nFile: Dockerfile\n-- another", 1},
		{"empty body", "", 0},
		{"no directives", "Just some body text.", 0},
		{"empty File", "File: ", 0},
		{"empty NoFile", "NoFile: ", 0},
		{"Content without contains", "Content: Dockerfile", 0},
		{"Content empty glob", "Content:  contains pattern", 0},
		{"Content empty pattern", "Content: Dockerfile contains ", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseFileDirectives(tc.body)
			if len(got) != tc.wantCount {
				t.Errorf("parseFileDirectives() returned %d directives, want %d", len(got), tc.wantCount)
			}
		})
	}
}
