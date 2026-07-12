package rules

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elpic/archon/internal/llm"
)

func TestNewEngine_LoadsRules(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"docker/no-latest.md": `---
name: no-latest-tag
severity: error
weight: 2
target: "**/Dockerfile*"
---
Pattern: FROM.*:latest`,
		"general/check-readme.md": `---
name: require-readme
severity: warn
weight: 1
target: "**/*"
---
File: README.md`,
	})

	engine, err := NewEngine(dir)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	if len(engine.rules) != 2 {
		t.Fatalf("got %d rules, want 2", len(engine.rules))
	}
}

func TestNewEngine_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	engine, err := NewEngine(dir)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	if len(engine.rules) != 0 {
		t.Fatalf("got %d rules, want 0", len(engine.rules))
	}
}

func TestNewEngine_NonexistentDir(t *testing.T) {
	_, err := NewEngine("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}

func TestEngine_Run_PatternCheck_Pass(t *testing.T) {
	rulesDir := setupTestDir(t, map[string]string{
		"docker/no-latest.md": `---
name: no-latest-tag
severity: error
weight: 2
target: "**/Dockerfile*"
---
Pattern: FROM.*:latest`,
	})
	target := setupTestDir(t, map[string]string{
		"Dockerfile": "FROM ubuntu:22.04\nRUN apt-get update\n",
	})

	engine, err := NewEngine(rulesDir)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	report, err := engine.Run(target)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(report.Results) != 1 {
		t.Fatalf("got %d results, want 1", len(report.Results))
	}
	rr := report.Results[0]
	if rr.Verdict != VerdictPass {
		t.Errorf("Verdict = %v, want VerdictPass", rr.Verdict)
	}
	if len(rr.Violations) != 0 {
		t.Errorf("got %d violations, want 0", len(rr.Violations))
	}
	if report.Score != 100 {
		t.Errorf("Score = %d, want 100", report.Score)
	}
}

func TestEngine_Run_PatternCheck_Violation(t *testing.T) {
	rulesDir := setupTestDir(t, map[string]string{
		"docker/no-latest.md": `---
name: no-latest-tag
severity: error
weight: 2
target: "**/Dockerfile*"
---
Pattern: FROM.*:latest`,
	})
	target := setupTestDir(t, map[string]string{
		"Dockerfile": "FROM ubuntu:latest\n",
	})

	engine, err := NewEngine(rulesDir)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	report, err := engine.Run(target)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	rr := report.Results[0]
	if rr.Verdict != VerdictViolation {
		t.Errorf("Verdict = %v, want VerdictViolation", rr.Verdict)
	}
	if len(rr.Violations) != 1 {
		t.Errorf("got %d violations, want 1", len(rr.Violations))
	}
	if report.Score != 0 {
		t.Errorf("Score = %d, want 0", report.Score)
	}
}

func TestEngine_Run_PatternCheck_NA(t *testing.T) {
	rulesDir := setupTestDir(t, map[string]string{
		"docker/no-latest.md": `---
name: no-latest-tag
severity: error
weight: 2
target: "**/Dockerfile*"
---
Pattern: FROM.*:latest`,
	})
	// Target has no Dockerfiles — rule should be N/A.
	target := setupTestDir(t, map[string]string{
		"main.go": "package main\n",
	})

	engine, err := NewEngine(rulesDir)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	report, err := engine.Run(target)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	rr := report.Results[0]
	if rr.Verdict != VerdictNA {
		t.Errorf("Verdict = %v, want VerdictNA", rr.Verdict)
	}
	if report.Score != 0 {
		t.Errorf("Score = %d, want 0 (N/A rules excluded from score)", report.Score)
	}
}

func TestEngine_Run_FileDirective_Pass(t *testing.T) {
	rulesDir := setupTestDir(t, map[string]string{
		"general/require-readme.md": `---
name: require-readme
severity: warn
weight: 1
target: "**/*"
---
File: README.md`,
	})
	target := setupTestDir(t, map[string]string{
		"README.md": "# Hello\n",
	})

	engine, err := NewEngine(rulesDir)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	report, err := engine.Run(target)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	rr := report.Results[0]
	if rr.Verdict != VerdictPass {
		t.Errorf("Verdict = %v, want VerdictPass", rr.Verdict)
	}
	if report.Score != 100 {
		t.Errorf("Score = %d, want 100", report.Score)
	}
}

func TestEngine_Run_FileDirective_Violation(t *testing.T) {
	rulesDir := setupTestDir(t, map[string]string{
		"general/require-readme.md": `---
name: require-readme
severity: warn
weight: 1
target: "**/*"
---
File: README.md`,
	})
	target := setupTestDir(t, map[string]string{
		"main.go": "package main\n",
	})

	engine, err := NewEngine(rulesDir)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	report, err := engine.Run(target)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	rr := report.Results[0]
	if rr.Verdict != VerdictViolation {
		t.Errorf("Verdict = %v, want VerdictViolation", rr.Verdict)
	}
	if len(rr.Violations) != 1 {
		t.Errorf("got %d violations, want 1", len(rr.Violations))
	}
	if report.Score != 0 {
		t.Errorf("Score = %d, want 0", report.Score)
	}
}

func TestEngine_Run_Excludes(t *testing.T) {
	rulesDir := setupTestDir(t, map[string]string{
		"docker/no-latest.md": `---
name: no-latest-tag
severity: error
weight: 2
target: "**/Dockerfile*"
exclude: "vendor/**"
---
Pattern: FROM.*:latest`,
	})
	target := setupTestDir(t, map[string]string{
		"vendor/app/Dockerfile": "FROM ubuntu:latest\n",
		"Dockerfile":            "FROM ubuntu:22.04\n",
	})

	engine, err := NewEngine(rulesDir)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	report, err := engine.Run(target)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	rr := report.Results[0]
	// vendor/ excluded, main Dockerfile is clean → pass
	if rr.Verdict != VerdictPass {
		t.Errorf("Verdict = %v, want VerdictPass (vendor excluded)", rr.Verdict)
	}
}

func TestEngine_Run_WeightedScoring(t *testing.T) {
	rulesDir := setupTestDir(t, map[string]string{
		"docker/no-latest.md": `---
name: no-latest-tag
severity: error
weight: 3
target: "**/Dockerfile*"
---
Pattern: FROM.*:latest`,
		"general/require-readme.md": `---
name: require-readme
severity: warn
weight: 1
target: "**/*"
---
File: README.md`,
	})
	// Dockerfile violates, README exists → 1 pass (w=1), 1 violation (w=3)
	// score = round(1 / 4 * 100) = 25
	target := setupTestDir(t, map[string]string{
		"Dockerfile": "FROM ubuntu:latest\n",
		"README.md":  "# Hello\n",
	})

	engine, err := NewEngine(rulesDir)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	report, err := engine.Run(target)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	wantScore := int(math.Round(1.0 / 4.0 * 100)) // 25
	if report.Score != wantScore {
		t.Errorf("Score = %d, want %d", report.Score, wantScore)
	}
}

func TestEngine_Run_MultipleRules(t *testing.T) {
	rulesDir := setupTestDir(t, map[string]string{
		"docker/no-latest.md": `---
name: no-latest-tag
severity: error
weight: 2
target: "**/Dockerfile*"
---
Pattern: FROM.*:latest`,
		"general/require-readme.md": `---
name: require-readme
severity: warn
weight: 1
target: "**/*"
---
File: README.md`,
		"general/no-todo.md": `---
name: no-todo
severity: warn
weight: 1
target: "**/*.go"
---
Pattern: TODO`,
	})
	target := setupTestDir(t, map[string]string{
		"Dockerfile": "FROM ubuntu:latest\n",
		"README.md":  "# Hello\n",
		"main.go":    "// TODO: fix this\npackage main\n",
	})

	engine, err := NewEngine(rulesDir)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	report, err := engine.Run(target)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(report.Results) != 3 {
		t.Fatalf("got %d results, want 3", len(report.Results))
	}

	// no-latest-tag: violation (FROM ubuntu:latest)
	// require-readme: pass (README.md exists)
	// no-todo: violation (TODO in main.go)
	violations := 0
	passes := 0
	for _, rr := range report.Results {
		switch rr.Verdict {
		case VerdictViolation:
			violations++
		case VerdictPass:
			passes++
		}
	}
	if violations != 2 {
		t.Errorf("got %d violations, want 2", violations)
	}
	if passes != 1 {
		t.Errorf("got %d passes, want 1", passes)
	}
}

func TestEngine_Run_NoRules(t *testing.T) {
	dir := t.TempDir()
	target := setupTestDir(t, map[string]string{
		"main.go": "package main\n",
	})

	engine, err := NewEngine(dir)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	report, err := engine.Run(target)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(report.Results) != 0 {
		t.Errorf("got %d results, want 0", len(report.Results))
	}
	if report.Score != 0 {
		t.Errorf("Score = %d, want 0", report.Score)
	}
}

func TestEngine_Run_NoPatternsNoDirectives(t *testing.T) {
	rulesDir := setupTestDir(t, map[string]string{
		"general/empty-rule.md": `---
name: empty-rule
severity: warn
weight: 1
target: "**/*"
---
This rule has no patterns or file directives.`,
	})
	target := setupTestDir(t, map[string]string{
		"main.go": "package main\n",
	})

	engine, err := NewEngine(rulesDir)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	report, err := engine.Run(target)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	rr := report.Results[0]
	if rr.Verdict != VerdictNA {
		t.Errorf("Verdict = %v, want VerdictNA (no patterns or directives)", rr.Verdict)
	}
}

func TestEngine_Run_CombinedPatternAndFileDirective(t *testing.T) {
	rulesDir := setupTestDir(t, map[string]string{
		"docker/docker-checks.md": `---
name: docker-checks
severity: error
weight: 2
target: "**/Dockerfile*"
---
Pattern: FROM.*:latest
File: Dockerfile`,
	})
	target := setupTestDir(t, map[string]string{
		"Dockerfile": "FROM ubuntu:22.04\n",
	})

	engine, err := NewEngine(rulesDir)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	report, err := engine.Run(target)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	rr := report.Results[0]
	// No pattern violation, Dockerfile exists → pass
	if rr.Verdict != VerdictPass {
		t.Errorf("Verdict = %v, want VerdictPass", rr.Verdict)
	}
}

func TestEngine_Run_CombinedPatternViolationAndFilePass(t *testing.T) {
	rulesDir := setupTestDir(t, map[string]string{
		"docker/docker-checks.md": `---
name: docker-checks
severity: error
weight: 2
target: "**/Dockerfile*"
---
Pattern: FROM.*:latest
File: Dockerfile`,
	})
	target := setupTestDir(t, map[string]string{
		"Dockerfile": "FROM ubuntu:latest\n",
	})

	engine, err := NewEngine(rulesDir)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	report, err := engine.Run(target)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	rr := report.Results[0]
	if rr.Verdict != VerdictViolation {
		t.Errorf("Verdict = %v, want VerdictViolation", rr.Verdict)
	}
	if len(rr.Violations) != 1 {
		t.Errorf("got %d violations, want 1", len(rr.Violations))
	}
}

func TestGlobMatch(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"**/*", "foo/bar.go", true},
		{"**/*.go", "foo/bar.go", true},
		{"**/*.go", "foo/bar/baz.go", true},
		{"**/*.go", "foo/bar.txt", false},
		{"*.go", "main.go", true},
		{"*.go", "sub/main.go", false},
		{"**/Dockerfile*", "Dockerfile", true},
		{"**/Dockerfile*", "foo/Dockerfile", true},
		{"**/Dockerfile*", "foo/Dockerfile.dev", true},
		{"**/Dockerfile*", "foo/main.go", false},
		{"vendor/**", "vendor/foo/bar.go", true},
		{"vendor/**", "src/main.go", false},
		{"docker/*.md", "docker/file.md", true},
		{"docker/*.md", "docker/sub/file.md", false},
	}
	for _, tc := range cases {
		t.Run(tc.pattern+"_"+tc.path, func(t *testing.T) {
			got := globMatch(tc.pattern, tc.path)
			if got != tc.want {
				t.Errorf("globMatch(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
			}
		})
	}
}

func TestFilterByTarget(t *testing.T) {
	rule := Rule{Target: "**/Dockerfile*"}
	files := []string{
		"Dockerfile",
		"foo/Dockerfile.dev",
		"main.go",
		"sub/Dockerfile",
	}
	got := filterByTarget(rule, files)
	if len(got) != 3 {
		t.Fatalf("got %d files, want 3", len(got))
	}
	for _, f := range got {
		if !strings.Contains(filepath.Base(f), "Dockerfile") {
			t.Errorf("unexpected file %s", f)
		}
	}
}

func TestFilterByTarget_Wildcard(t *testing.T) {
	rule := Rule{Target: "**/*"}
	files := []string{"main.go", "README.md"}
	got := filterByTarget(rule, files)
	if len(got) != 2 {
		t.Fatalf("got %d files, want 2", len(got))
	}
}

func TestFilterByExclude(t *testing.T) {
	rule := Rule{Exclude: []string{"vendor/**", "test/**"}}
	files := []string{
		"main.go",
		"vendor/foo/bar.go",
		"test/foo_test.go",
		"pkg/util.go",
	}
	got := filterByExclude(rule, files)
	if len(got) != 2 {
		t.Fatalf("got %d files, want 2", len(got))
	}
	for _, f := range got {
		if strings.HasPrefix(f, "vendor") || strings.HasPrefix(f, "test") {
			t.Errorf("unexpected file %s", f)
		}
	}
}

func TestFilterByExclude_Empty(t *testing.T) {
	rule := Rule{}
	files := []string{"/project/main.go", "/project/README.md"}
	got := filterByExclude(rule, files)
	if len(got) != 2 {
		t.Fatalf("got %d files, want 2", len(got))
	}
}

// --- Report tests ---

func TestNewReport_PerfectScore(t *testing.T) {
	results := []RuleResult{
		{Rule: Rule{Name: "a", Weight: 2}, Verdict: VerdictPass},
		{Rule: Rule{Name: "b", Weight: 1}, Verdict: VerdictPass},
	}
	r := NewReport("/project", results)
	if r.Score != 100 {
		t.Errorf("Score = %d, want 100", r.Score)
	}
	if r.Violated != 0 {
		t.Errorf("Violated = %d, want 0", r.Violated)
	}
}

func TestNewReport_ZeroScore(t *testing.T) {
	results := []RuleResult{
		{Rule: Rule{Name: "a", Weight: 2}, Verdict: VerdictViolation, Violations: []llm.Violation{{Rule: "a", Description: "bad"}}},
		{Rule: Rule{Name: "b", Weight: 1}, Verdict: VerdictViolation, Violations: []llm.Violation{{Rule: "b", Description: "bad"}}},
	}
	r := NewReport("/project", results)
	if r.Score != 0 {
		t.Errorf("Score = %d, want 0", r.Score)
	}
	if r.Violated != 2 {
		t.Errorf("Violated = %d, want 2", r.Violated)
	}
}

func TestNewReport_NAExcluded(t *testing.T) {
	// Only N/A rules → score should be 0 (no applicable rules).
	results := []RuleResult{
		{Rule: Rule{Name: "a", Weight: 5}, Verdict: VerdictNA},
	}
	r := NewReport("/project", results)
	if r.Score != 0 {
		t.Errorf("Score = %d, want 0 (N/A excluded)", r.Score)
	}
}

func TestNewReport_WeightedScore(t *testing.T) {
	results := []RuleResult{
		{Rule: Rule{Name: "a", Weight: 3}, Verdict: VerdictViolation, Violations: []llm.Violation{{Rule: "a"}}},
		{Rule: Rule{Name: "b", Weight: 1}, Verdict: VerdictPass},
	}
	r := NewReport("/project", results)
	// pass weight = 1, total = 4, score = round(1/4 * 100) = 25
	wantScore := int(math.Round(1.0 / 4.0 * 100))
	if r.Score != wantScore {
		t.Errorf("Score = %d, want %d", r.Score, wantScore)
	}
}

func TestNewReport_ScoreRounding(t *testing.T) {
	// Test that rounding is correct: 2/3 * 100 = 66.666... → 67
	results := []RuleResult{
		{Rule: Rule{Name: "a", Weight: 1}, Verdict: VerdictPass},
		{Rule: Rule{Name: "b", Weight: 1}, Verdict: VerdictPass},
		{Rule: Rule{Name: "c", Weight: 1}, Verdict: VerdictViolation, Violations: []llm.Violation{{Rule: "c"}}},
	}
	r := NewReport("/project", results)
	wantScore := int(math.Round(2.0 / 3.0 * 100))
	if r.Score != wantScore {
		t.Errorf("Score = %d, want %d", r.Score, wantScore)
	}
}

func TestReport_Format(t *testing.T) {
	results := []RuleResult{
		{Rule: Rule{Name: "no-latest", Weight: 1, Category: "docker"}, Verdict: VerdictViolation, Violations: []llm.Violation{
			{Rule: "no-latest", Description: "latest tag used", File: "Dockerfile", Line: 1},
		}},
		{Rule: Rule{Name: "require-readme", Weight: 1, Category: "general"}, Verdict: VerdictPass},
	}
	r := NewReport("/project", results)
	output := r.Format()
	if !strings.Contains(output, "Score: 50/100") {
		t.Errorf("Format() missing score, got:\n%s", output)
	}
	if !strings.Contains(output, "1 violation(s)") {
		t.Errorf("Format() missing violation count, got:\n%s", output)
	}
	if !strings.Contains(output, "## docker") {
		t.Errorf("Format() missing docker category, got:\n%s", output)
	}
	if !strings.Contains(output, "## general") {
		t.Errorf("Format() missing general category, got:\n%s", output)
	}
	if !strings.Contains(output, "✗ no-latest") {
		t.Errorf("Format() missing violation icon, got:\n%s", output)
	}
	if !strings.Contains(output, "✓ require-readme") {
		t.Errorf("Format() missing pass icon, got:\n%s", output)
	}
}

func TestReport_FormatDiagnostic(t *testing.T) {
	results := []RuleResult{
		{Rule: Rule{Name: "no-latest", Weight: 1}, Verdict: VerdictViolation, Violations: []llm.Violation{
			{Rule: "no-latest", Description: "latest tag", Severity: llm.SeverityError, File: "Dockerfile", Line: 1, Column: 1},
		}},
	}
	r := NewReport("/project", results)
	output := r.FormatDiagnostic()
	if !strings.Contains(output, "Dockerfile:1:1:") {
		t.Errorf("FormatDiagnostic() missing problem-matcher format, got:\n%s", output)
	}
}

func TestReport_FormatDiagnostic_NoViolations(t *testing.T) {
	results := []RuleResult{
		{Rule: Rule{Name: "a", Weight: 1}, Verdict: VerdictPass},
	}
	r := NewReport("/project", results)
	output := r.FormatDiagnostic()
	if output != "" {
		t.Errorf("FormatDiagnostic() = %q, want empty", output)
	}
}

func TestReport_Findings(t *testing.T) {
	results := []RuleResult{
		{Rule: Rule{Name: "a", Weight: 1}, Verdict: VerdictViolation, Violations: []llm.Violation{{Rule: "a"}, {Rule: "a"}}},
		{Rule: Rule{Name: "b", Weight: 1}, Verdict: VerdictPass},
		{Rule: Rule{Name: "c", Weight: 1}, Verdict: VerdictViolation, Violations: []llm.Violation{{Rule: "c"}}},
	}
	r := NewReport("/project", results)
	findings := r.Findings()
	if len(findings) != 3 {
		t.Errorf("Findings() = %d, want 3", len(findings))
	}
}

func TestVerdictString(t *testing.T) {
	cases := []struct {
		v    Verdict
		want string
	}{
		{VerdictPass, "pass"},
		{VerdictViolation, "violation"},
		{VerdictNA, "N/A"},
		{Verdict(99), "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.v.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestVerdictIcon(t *testing.T) {
	cases := []struct {
		v    Verdict
		want string
	}{
		{VerdictPass, "✓"},
		{VerdictViolation, "✗"},
		{VerdictNA, "–"},
		{Verdict(99), "?"},
	}
	for _, tc := range cases {
		if got := verdictIcon(tc.v); got != tc.want {
			t.Errorf("verdictIcon(%v) = %q, want %q", tc.v, got, tc.want)
		}
	}
}

func TestWriteFindings(t *testing.T) {
	results := []RuleResult{
		{Rule: Rule{Name: "a", Weight: 1, Category: "general"}, Verdict: VerdictPass},
	}
	r := NewReport("/tmp/myproject", results)

	cacheDir := t.TempDir()
	outPath, err := r.WriteFindings(cacheDir)
	if err != nil {
		t.Fatalf("WriteFindings() error = %v", err)
	}
	wantPath := filepath.Join(cacheDir, "archon", "findings", "myproject", "findings.md")
	if outPath != wantPath {
		t.Errorf("outPath = %q, want %q", outPath, wantPath)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), "Score:") {
		t.Errorf("findings.md missing score, got:\n%s", string(data))
	}
}

func TestWriteFindings_EmptyCacheDir(t *testing.T) {
	results := []RuleResult{
		{Rule: Rule{Name: "a", Weight: 1}, Verdict: VerdictPass},
	}
	r := NewReport("/project", results)
	_, err := r.WriteFindings("")
	if err == nil {
		t.Fatal("expected error for empty cache dir")
	}
}

func TestReport_Score_AllNA(t *testing.T) {
	results := []RuleResult{
		{Rule: Rule{Name: "a", Weight: 5}, Verdict: VerdictNA},
		{Rule: Rule{Name: "b", Weight: 3}, Verdict: VerdictNA},
	}
	r := NewReport("/project", results)
	if r.Score != 0 {
		t.Errorf("Score = %d, want 0 (all N/A)", r.Score)
	}
}

func TestReport_Score_MixedNAAndApplicable(t *testing.T) {
	results := []RuleResult{
		{Rule: Rule{Name: "a", Weight: 5}, Verdict: VerdictNA},
		{Rule: Rule{Name: "b", Weight: 5}, Verdict: VerdictPass},
	}
	r := NewReport("/project", results)
	// Only b is applicable → score = 100
	if r.Score != 100 {
		t.Errorf("Score = %d, want 100", r.Score)
	}
}

func TestEngine_Run_Integration(t *testing.T) {
	// Full integration test: load rules from disk, run against a target.
	rulesDir := setupTestDir(t, map[string]string{
		"docker/no-latest.md": `---
name: no-latest-tag
severity: error
weight: 2
target: "**/Dockerfile*"
---
Pattern: FROM.*:latest`,
		"general/require-readme.md": `---
name: require-readme
severity: warn
weight: 1
target: "**/*"
---
File: README.md`,
		"general/no-todo.md": `---
name: no-todo
severity: warn
weight: 1
target: "**/*.go"
---
Pattern: TODO`,
		"general/no-env.md": `---
name: no-env
severity: error
weight: 2
target: "**/*"
---
NoFile: .env`,
	})
	target := setupTestDir(t, map[string]string{
		"Dockerfile": "FROM ubuntu:latest\n",
		"README.md":  "# My Project\n",
		"main.go":    "package main\n\n// TODO: clean up\n",
		".env":       "SECRET=abc\n",
	})

	engine, err := NewEngine(rulesDir)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	report, err := engine.Run(target)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(report.Results) != 4 {
		t.Fatalf("got %d results, want 4", len(report.Results))
	}

	// Verify all verdicts.
	wantVerdicts := map[string]Verdict{
		"no-latest-tag":   VerdictViolation,
		"require-readme":  VerdictPass,
		"no-todo":         VerdictViolation,
		"no-env":          VerdictViolation,
	}
	for _, rr := range report.Results {
		want, ok := wantVerdicts[rr.Rule.Name]
		if !ok {
			t.Errorf("unexpected rule %q", rr.Rule.Name)
			continue
		}
		if rr.Verdict != want {
			t.Errorf("rule %q: Verdict = %v, want %v", rr.Rule.Name, rr.Verdict, want)
		}
	}

	// Score: only require-readme passes (w=1), total applicable weight = 2+1+1+2=6
	wantScore := int(math.Round(1.0 / 6.0 * 100))
	if report.Score != wantScore {
		t.Errorf("Score = %d, want %d", report.Score, wantScore)
	}

	// Violations: no-latest (1) + no-todo (1) + no-env (1) = 3
	if report.Violated != 3 {
		t.Errorf("Violated = %d, want 3", report.Violated)
	}

	// Format should produce readable output.
	output := report.Format()
	if !strings.Contains(output, "Archon audit:") {
		t.Error("Format() missing header")
	}
	if !strings.Contains(output, "Score:") {
		t.Error("Format() missing score")
	}

	// Write findings.
	cacheDir := t.TempDir()
	outPath, err := report.WriteFindings(cacheDir)
	if err != nil {
		t.Fatalf("WriteFindings() error = %v", err)
	}
	if _, err := os.Stat(outPath); os.IsNotExist(err) {
		t.Errorf("findings file not created at %s", outPath)
	}
}
