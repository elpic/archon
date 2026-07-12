package rules

import (
	"os"
	"path/filepath"
	"testing"
)

// helper: create a temp dir structure with rule files.
func setupTestDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoad_ValidRule(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"docker/no-latest.md": `---
name: no-latest-tag
severity: error
weight: 2
target: "**/Dockerfile*"
exclude: "vendor/**"
---
# No latest tag

Do not use the :latest tag in production Dockerfiles.`,
	})

	rules, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}

	r := rules[0]
	if r.Name != "no-latest-tag" {
		t.Errorf("Name = %q, want %q", r.Name, "no-latest-tag")
	}
	if r.Severity != "error" {
		t.Errorf("Severity = %q, want %q", r.Severity, "error")
	}
	if r.Weight != 2 {
		t.Errorf("Weight = %d, want 2", r.Weight)
	}
	if r.Target != "**/Dockerfile*" {
		t.Errorf("Target = %q, want %q", r.Target, "**/Dockerfile*")
	}
	if len(r.Exclude) != 1 || r.Exclude[0] != "vendor/**" {
		t.Errorf("Exclude = %v, want [vendor/**]", r.Exclude)
	}
	if r.Category != "docker" {
		t.Errorf("Category = %q, want %q", r.Category, "docker")
	}
	if r.Body == "" {
		t.Error("Body should not be empty")
	}
}

func TestLoad_Defaults(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"general/minimal.md": `---
name: minimal-rule
---
Some body text.`,
	})

	rules, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}

	r := rules[0]
	if r.Severity != "warn" {
		t.Errorf("Severity = %q, want %q", r.Severity, "warn")
	}
	if r.Weight != 1 {
		t.Errorf("Weight = %d, want 1", r.Weight)
	}
	if r.Target != "**/*" {
		t.Errorf("Target = %q, want %q", r.Target, "**/*")
	}
	if r.Exclude != nil {
		t.Errorf("Exclude = %v, want nil", r.Exclude)
	}
}

func TestLoad_RootDirRule(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"root-rule.md": `---
name: root-rule
---
Body.`,
	})

	rules, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	if rules[0].Category != "general" {
		t.Errorf("Category = %q, want %q", rules[0].Category, "general")
	}
}

func TestLoad_SkipsNonMarkdown(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"docker/notes.txt": "not a rule",
		"docker/readme":    "---\nname: bad\n---\nBody.",
		"docker/good.md":   "---\nname: good\n---\nBody.",
	})

	rules, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	if rules[0].Name != "good" {
		t.Errorf("Name = %q, want %q", rules[0].Name, "good")
	}
}

func TestLoad_SkipsInvalidFrontmatter(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"docker/valid.md":   "---\nname: valid\n---\nBody.",
		"docker/bad.md":     "---\nseverity: error\n---\nNo name.",
		"docker/unclosed.md": "---\nname: unclosed\n\nNo closing delimiters.",
	})

	rules, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1 (invalid/unclosed should be skipped)", len(rules))
	}
	if rules[0].Name != "valid" {
		t.Errorf("Name = %q, want %q", rules[0].Name, "valid")
	}
}

func TestLoad_SortedByCategoryThenName(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"docker/b-rule.md":  "---\nname: b-rule\n---\nBody.",
		"docker/a-rule.md":  "---\nname: a-rule\n---\nBody.",
		"ci/b-rule.md":      "---\nname: b-rule\n---\nBody.",
		"ci/a-rule.md":      "---\nname: a-rule\n---\nBody.",
	})

	rules, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(rules) != 4 {
		t.Fatalf("got %d rules, want 4", len(rules))
	}

	// Expect: ci/a, ci/b, docker/a, docker/b
	want := []struct{ cat, name string }{
		{"ci", "a-rule"},
		{"ci", "b-rule"},
		{"docker", "a-rule"},
		{"docker", "b-rule"},
	}
	for i, w := range want {
		if rules[i].Category != w.cat || rules[i].Name != w.name {
			t.Errorf("rules[%d] = (%s, %s), want (%s, %s)",
				i, rules[i].Category, rules[i].Name, w.cat, w.name)
		}
	}
}

func TestLoad_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	rules, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("got %d rules, want 0", len(rules))
	}
}

func TestLoad_NonexistentDir(t *testing.T) {
	_, err := Load("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}

func TestLoad_NoFrontmatter(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"general/plain.md": `Just a markdown file with no frontmatter.`,
	})

	rules, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	// Without frontmatter, name is empty so it should be skipped.
	if len(rules) != 0 {
		t.Errorf("got %d rules, want 0 (no frontmatter means no name)", len(rules))
	}
}

func TestLoad_QuotedValues(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"docker/quoted.md": `---
name: "quoted-rule"
severity: "error"
target: "**/*.go"
---
Body.`,
	})

	rules, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	if rules[0].Name != "quoted-rule" {
		t.Errorf("Name = %q, want %q", rules[0].Name, "quoted-rule")
	}
	if rules[0].Severity != "error" {
		t.Errorf("Severity = %q, want %q", rules[0].Severity, "error")
	}
}

func TestLoad_CommaSeparatedExclude(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"docker/multi.md": `---
name: multi-exclude
exclude: "vendor/**, test/**, generated/**"
---
Body.`,
	})

	rules, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	want := []string{"vendor/**", "test/**", "generated/**"}
	got := rules[0].Exclude
	if len(got) != len(want) {
		t.Fatalf("Exclude len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Exclude[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSplitFrontmatter(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantFM  string
		wantBody string
		wantErr bool
	}{
		{
			name:    "normal",
			input:   "---\nname: test\n---\nBody here",
			wantFM:  "name: test",
			wantBody: "Body here",
		},
		{
			name:    "no frontmatter",
			input:   "Just body text",
			wantFM:  "",
			wantBody: "Just body text",
		},
		{
			name:    "unclosed",
			input:   "---\nname: test\n\nNo closing",
			wantErr: true,
		},
		{
			name:    "empty",
			input:   "",
			wantFM:  "",
			wantBody: "",
		},
		{
			name:    "body with leading newlines",
			input:   "---\nname: x\n---\n\n\nBody",
			wantFM:  "name: x",
			wantBody: "Body",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fm, body, err := splitFrontmatter(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if fm != tc.wantFM {
				t.Errorf("frontmatter = %q, want %q", fm, tc.wantFM)
			}
			if body != tc.wantBody {
				t.Errorf("body = %q, want %q", body, tc.wantBody)
			}
		})
	}
}

func TestDeriveCategory(t *testing.T) {
	cases := []struct {
		name string
		path string
		root string
		want string
	}{
		{"nested", "/project/.rules/docker/file.md", "/project/.rules", "docker"},
		{"root", "/project/.rules/file.md", "/project/.rules", "general"},
		{"deep", "/project/.rules/docker/compose/file.md", "/project/.rules", "docker"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveCategory(tc.path, tc.root)
			if got != tc.want {
				t.Errorf("deriveCategory(%q, %q) = %q, want %q", tc.path, tc.root, got, tc.want)
			}
		})
	}
}

func TestParseFrontmatter(t *testing.T) {
	yaml := `
name: test-rule
severity: error
weight: 5
target: "**/*.go"
exclude: "vendor/**"
`
	m := parseFrontmatter(yaml)
	if m["name"] != "test-rule" {
		t.Errorf("name = %q, want %q", m["name"], "test-rule")
	}
	if m["severity"] != "error" {
		t.Errorf("severity = %q, want %q", m["severity"], "error")
	}
	if m["weight"] != "5" {
		t.Errorf("weight = %q, want %q", m["weight"], "5")
	}
	if m["target"] != "**/*.go" {
		t.Errorf("target = %q, want %q", m["target"], "**/*.go")
	}
	if m["exclude"] != "vendor/**" {
		t.Errorf("exclude = %q, want %q", m["exclude"], "vendor/**")
	}
}

func TestParseList(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty", "", nil},
		{"single", "vendor/**", []string{"vendor/**"}},
		{"multi", "a, b, c", []string{"a", "b", "c"}},
		{"extra spaces", "  a ,  b  ,  c  ", []string{"a", "b", "c"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseList(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tc.want))
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}
