package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsValidRuleID(t *testing.T) {
	cases := []struct {
		ruleID string
		want   bool
	}{
		{"", false},
		{"ErrorWrapping", true},
		{"no-comments", true},
		{"rule_123", true},
		{"rule with spaces", false},
		{"../evil", false},
		{"rule/slash", false},
		{"rule;semicolon", false},
		{"rule.dot", false},
		{"A", true},
		{"0", true},
		{"a-b_c1", true},
	}
	for _, tc := range cases {
		t.Run(tc.ruleID, func(t *testing.T) {
			got := isValidRuleID(tc.ruleID)
			if got != tc.want {
				t.Errorf("isValidRuleID(%q) = %v, want %v", tc.ruleID, got, tc.want)
			}
		})
	}
}

func TestFindRuleInStandards(t *testing.T) {
	body := `# Standards

## ErrorWrapping
Wrap errors with context using fmt.Errorf and %w.

### NoPrintf
Do not use fmt.Printf in library code.

## NoComments
No comments in code.
`
	cases := []struct {
		name   string
		ruleID string
		want   string
	}{
		{"exact match", "ErrorWrapping", "Wrap errors with context using fmt.Errorf and %w."},
		{"no match", "NonExistent", ""},
		{"partial prefix does not match", "Error", ""},
		{"partial suffix does not match", "Wrapping", ""},
		{"rule at end of doc", "NoComments", "No comments in code."},
		{"rule with sub-heading", "NoPrintf", "Do not use fmt.Printf in library code."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := findRuleInStandards(body, tc.ruleID)
			if strings.TrimSpace(got) != strings.TrimSpace(tc.want) {
				t.Errorf("findRuleInStandards(body, %q) = %q, want %q", tc.ruleID, got, tc.want)
			}
		})
	}
}

func TestValidateTarget(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		target  string
		wantErr bool
	}{
		{"current dir", ".", false},
		{"relative path", "cmd/archon", false},
		{"absolute within cwd", cwd, false},
		{"absolute outside cwd", "/tmp", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := tc.target
			// For "absolute within cwd", use cwd itself
			if tc.name == "absolute within cwd" {
				target = cwd
			}
			// For relative paths, ensure they resolve within cwd
			if !filepath.IsAbs(target) && target != "." {
				target = filepath.Join(cwd, target)
			}
			err := validateTarget(target, "test")
			if (err != nil) != tc.wantErr {
				t.Errorf("validateTarget(%q) error = %v, wantErr %v", tc.target, err, tc.wantErr)
			}
		})
	}
}
