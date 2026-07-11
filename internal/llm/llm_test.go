package llm

import (
	"regexp"
	"testing"
)

func TestSeverity_String(t *testing.T) {
	cases := []struct {
		name string
		sev  Severity
		want string
	}{
		{"info", SeverityInfo, "info"},
		{"warn", SeverityWarn, "warn"},
		{"error", SeverityError, "error"},
		{"critical", SeverityCritical, "critical"},
		{"unknown", Severity(99), "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.sev.String(); got != tc.want {
				t.Errorf("Severity(%d).String() = %q, want %q", tc.sev, got, tc.want)
			}
		})
	}
}

// TestViolation_ProblemMatcherFormat pins the problem-matcher
// format used by `archon watch` so editor quickfix parsers can
// rely on the exact shape. The regex is intentionally a copy of
// the convention used by gcc, clippy, and the GitHub Actions
// problem-matcher:
//
//	path:line:col: [severity] message
func TestViolation_ProblemMatcherFormat(t *testing.T) {
	v := Violation{
		Rule:        "no-comments",
		Description: "Comments are forbidden",
		Severity:    SeverityError,
		File:        "internal/foo/foo.go",
		Line:        42,
		Column:      7,
	}
	got := v.String()
	want := "internal/foo/foo.go:42:7: [error] Comments are forbidden"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	// Pin the exact shape so a refactor can't accidentally change
	// it without the test going red.
	pattern := regexp.MustCompile(`^.+:\d+:\d+: \[[a-z]+\] .+$`)
	if !pattern.MatchString(got) {
		t.Errorf("String() = %q does not match problem-matcher pattern %q", got, pattern)
	}
}

// TestViolation_ProblemMatcherFormat_Zero exercises the missing-coordinates
// case: an empty File and zero Line/Column must render as "?:?:?" so
// editors that try to jump to the location get a clear "unknown" signal
// rather than ":0:0" which could be mistaken for a real coordinate.
func TestViolation_ProblemMatcherFormat_Zero(t *testing.T) {
	v := Violation{
		Rule:        "no-comments",
		Description: "Comments are forbidden",
		Severity:    SeverityWarn,
	}
	got := v.String()
	want := "?:?:?: [warn] Comments are forbidden"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// TestViolation_ProblemMatcherFormat_Partial checks that the
// placeholder substitution is per-coordinate, not all-or-nothing:
// a file with a line but no column renders as "path:line:?".
func TestViolation_ProblemMatcherFormat_Partial(t *testing.T) {
	v := Violation{
		Rule:        "r",
		Description: "d",
		Severity:    SeverityInfo,
		File:        "x.go",
		Line:        5,
	}
	got := v.String()
	want := "x.go:5:?: [info] d"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
