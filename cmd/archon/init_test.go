package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCmd_EmptyDirNoFlag(t *testing.T) {
	dir := t.TempDir()
	if err := runInitCmd(initCommand{target: dir}); err != nil {
		t.Fatalf("runInitCmd: %v", err)
	}
	file := filepath.Join(dir, ".archon", "standards.md")
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	s := string(body)
	if !strings.Contains(s, "# Project Standards") {
		t.Errorf("expected '# Project Standards' in body, got:\n%s", s)
	}
	// The body must include substantive content so the resolver
	// treats it as tier 1 (project) rather than a redirect.
	if hasFromDirective(s) {
		t.Errorf("expected no from: directive without --from, got one in:\n%s", s)
	}
}

// hasFromDirective returns true if body contains a `from: <value>`
// directive on its own (i.e. as a line the resolver would parse).
// It deliberately ignores prose that mentions the word "from".
func hasFromDirective(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		cleaned := strings.TrimPrefix(trimmed, "<!--")
		cleaned = strings.TrimSpace(strings.TrimSuffix(cleaned, "-->"))
		if strings.HasPrefix(cleaned, "from:") {
			val := strings.TrimSpace(strings.TrimPrefix(cleaned, "from:"))
			if val != "" {
				return true
			}
		}
	}
	return false
}

func TestInitCmd_WithFrom(t *testing.T) {
	dir := t.TempDir()
	if err := runInitCmd(initCommand{target: dir, from: "elpic/go-standards"}); err != nil {
		t.Fatalf("runInitCmd: %v", err)
	}
	file := filepath.Join(dir, ".archon", "standards.md")
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !hasFromDirective(string(body)) {
		t.Errorf("expected a 'from: elpic/go-standards' directive in body, got:\n%s", body)
	}
	// Without a substantive body, the resolver must treat this as a redirect.
	if hasBody(string(body)) {
		t.Errorf("expected empty body (redirect-only) with --from, got body content in:\n%s", body)
	}
}

// hasBody returns true if body has any non-comment, non-from-line content.
func hasBody(body string) bool {
	inComment := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if inComment {
			if strings.Contains(trimmed, "-->") {
				inComment = false
			}
			continue
		}
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "<!--") {
			if !strings.Contains(trimmed, "-->") {
				inComment = true
			}
			continue
		}
		if strings.HasSuffix(trimmed, "-->") {
			continue
		}
		if strings.HasPrefix(trimmed, "from:") {
			continue
		}
		return true
	}
	return false
}

func TestInitCmd_Idempotent(t *testing.T) {
	dir := t.TempDir()
	if err := runInitCmd(initCommand{target: dir, from: "owner/repo"}); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(dir, ".archon", "standards.md"))
	if err != nil {
		t.Fatal(err)
	}
	if err := runInitCmd(initCommand{target: dir, from: "owner/repo"}); err != nil {
		t.Fatalf("second run: %v", err)
	}
	second, err := os.ReadFile(filepath.Join(dir, ".archon", "standards.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("file changed between runs\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	// Also: changing the --from flag on a second run must NOT overwrite.
	if err := runInitCmd(initCommand{target: dir, from: "different/repo"}); err != nil {
		t.Fatalf("third run: %v", err)
	}
	third, err := os.ReadFile(filepath.Join(dir, ".archon", "standards.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(third) {
		t.Errorf("file clobbered by later run with different from\nfirst:\n%s\nthird:\n%s", first, third)
	}
}

func TestInitCmd_PreservesExistingContent(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, ".archon", "standards.md")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := "# My Custom Standards\n\nAlready authored content.\n"
	if err := os.WriteFile(file, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runInitCmd(initCommand{target: dir, from: "owner/repo"}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != existing {
		t.Errorf("existing content was overwritten:\nbefore:\n%s\nafter:\n%s", existing, got)
	}
}

func TestValidateFrom(t *testing.T) {
	cases := []struct {
		in     string
		wantOK bool
	}{
		{"", true},
		{"a/b", true},
		{"elpic/go-standards", true},
		{"a-b/c-d", true},
		{"not-a-repo", false},
		{"/b", false},
		{"a/", false},
		{"/", false},
		{"a/b/c", false}, // GitHub repo names cannot contain slashes
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			err := validateFrom(tc.in)
			if tc.wantOK && err != nil {
				t.Errorf("validateFrom(%q) = %v, want nil", tc.in, err)
			}
			if !tc.wantOK && err == nil {
				t.Errorf("validateFrom(%q) = nil, want error", tc.in)
			}
		})
	}
}

// TestRunInit_RejectsMalformedFrom: runInit surfaces the validation error
// instead of writing a file.
func TestRunInit_RejectsMalformedFrom(t *testing.T) {
	err := runInit(nil, []string{"--from", "not-a-repo"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "owner/repo") {
		t.Errorf("expected 'owner/repo' in error, got %v", err)
	}
	// File must not have been created.
	entries, _ := os.ReadDir(".")
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".archon") {
			t.Errorf("unexpected .archon path created: %s", e.Name())
		}
	}
}
