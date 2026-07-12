// Package rules defines the Rule type, loads rule files from disk, and
// provides checkers that evaluate file contents against rule patterns.
package rules

import (
	"regexp"
	"strings"

	"github.com/elpic/archon/internal/llm"
)

// Checker evaluates file content against a rule and returns any violations.
type Checker interface {
	Check(file string, content []byte) []llm.Violation
}

// DirectoryChecker evaluates a directory against a rule and returns any
// violations. Unlike Checker, it operates on the whole directory rather
// than a single file's content — useful for existence, absence, and
// cross-file checks.
type DirectoryChecker interface {
	CheckDirectory(dir string) []llm.Violation
}

// PatternChecker runs regex patterns extracted from a Rule's markdown body
// against file content. Each line matching "Pattern: <regex>" in the body
// is compiled and matched against the content. Every match becomes a
// Violation.
type PatternChecker struct {
	Rule Rule
}

// NewPatternChecker creates a PatternChecker for the given rule.
func NewPatternChecker(rule Rule) *PatternChecker {
	return &PatternChecker{Rule: rule}
}

// Check compiles all Pattern lines from the rule body and runs each
// against content. Matches are returned as Violations with file, line,
// and matched content.
func (pc *PatternChecker) Check(file string, content []byte) []llm.Violation {
	patterns := parsePatterns(pc.Rule.Body)
	if len(patterns) == 0 {
		return nil
	}

	var violations []llm.Violation
	for _, pat := range patterns {
		re, err := regexp.Compile(pat)
		if err != nil {
			continue // skip invalid patterns
		}
		matches := re.FindAllIndex(content, -1)
		for _, loc := range matches {
			line, col := posToLineCol(content, loc[0])
			matched := string(content[loc[0]:loc[1]])
			violations = append(violations, llm.Violation{
				Rule:        pc.Rule.Name,
				Description: "pattern match: " + matched,
				Severity:    parseSeverity(pc.Rule.Severity),
				File:        file,
				Line:        line,
				Column:      col,
			})
		}
	}
	return violations
}

// parsePatterns extracts regex patterns from lines matching "Pattern: <regex>"
// in the rule body. Lines starting with "--" are treated as comments and skipped.
func parsePatterns(body string) []string {
	var patterns []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "--") {
			continue
		}
		if strings.HasPrefix(line, "Pattern:") {
			pat := strings.TrimSpace(strings.TrimPrefix(line, "Pattern:"))
			if pat != "" {
				patterns = append(patterns, pat)
			}
		}
	}
	return patterns
}

// posToLineCol converts a byte offset in content to a 1-based line and column.
func posToLineCol(content []byte, offset int) (line, col int) {
	line = 1
	col = 1
	for i := 0; i < offset && i < len(content); i++ {
		if content[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}

// parseSeverity converts a severity string to the llm.Severity enum.
func parseSeverity(s string) llm.Severity {
	switch strings.ToLower(s) {
	case "info":
		return llm.SeverityInfo
	case "error":
		return llm.SeverityError
	case "critical":
		return llm.SeverityCritical
	default:
		return llm.SeverityWarn
	}
}
