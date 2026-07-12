package rules

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/elpic/archon/internal/llm"
)

// FileChecker checks file existence, absence, and basic content patterns
// in a directory. It parses File, NoFile, and Content directives from
// the rule's markdown body.
type FileChecker struct {
	Rule Rule
}

// NewFileChecker creates a FileChecker for the given rule.
func NewFileChecker(rule Rule) *FileChecker {
	return &FileChecker{Rule: rule}
}

// CheckDirectory walks the directory and evaluates all file directives
// from the rule body. Returns violations for:
//   - File: <glob> — no match found (file should exist)
//   - NoFile: <glob> — match found (file should not exist)
//   - Content: <glob> contains <pattern> — pattern not found in matched files
func (fc *FileChecker) CheckDirectory(dir string) []llm.Violation {
	directives := parseFileDirectives(fc.Rule.Body)
	if len(directives) == 0 {
		return nil
	}

	// Pre-resolve all glob patterns against the directory.
	globCache := make(map[string][]string)
	for _, d := range directives {
		if _, ok := globCache[d.Glob]; !ok {
			matches, _ := filepath.Glob(filepath.Join(dir, d.Glob))
			globCache[d.Glob] = matches
		}
	}

	var violations []llm.Violation
	for _, d := range directives {
		matches := globCache[d.Glob]

		switch d.Kind {
		case directiveFile:
			if len(matches) == 0 {
				violations = append(violations, fc.violation(
					d.Glob,
					"required file not found: "+d.Glob,
				))
			}

		case directiveNoFile:
			if len(matches) > 0 {
				violations = append(violations, fc.violation(
					relPath(dir, matches[0]),
					"forbidden file found: "+d.Glob,
				))
			}

		case directiveContent:
			if len(matches) == 0 {
				violations = append(violations, fc.violation(
					d.Glob,
					"content check: no files match "+d.Glob,
				))
				continue
			}
			for _, m := range matches {
				data, err := os.ReadFile(m)
				if err != nil {
					continue
				}
				if !strings.Contains(string(data), d.Pattern) {
					violations = append(violations, fc.violation(
						relPath(dir, m),
						"content check: "+m+" does not contain "+d.Pattern,
					))
				}
			}
		}
	}

	return violations
}

// violation builds a Violation for this rule with the given file and description.
func (fc *FileChecker) violation(file, description string) llm.Violation {
	return llm.Violation{
		Rule:        fc.Rule.Name,
		Description: description,
		Severity:    parseSeverity(fc.Rule.Severity),
		File:        file,
	}
}

// fileDirectiveKind identifies the type of file directive.
type fileDirectiveKind int

const (
	directiveFile    fileDirectiveKind = iota // File: <glob>
	directiveNoFile                           // NoFile: <glob>
	directiveContent                          // Content: <glob> contains <pattern>
)

// fileDirective is a parsed File/NoFile/Content line from a rule body.
type fileDirective struct {
	Kind    fileDirectiveKind
	Glob    string
	Pattern string // only set for directiveContent
}

// parseFileDirectives extracts file directives from rule body lines.
// Comment lines (starting with "--") are skipped.
func parseFileDirectives(body string) []fileDirective {
	var directives []fileDirective
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "--") {
			continue
		}

		switch {
		case strings.HasPrefix(line, "Content:"):
			rest := strings.TrimSpace(strings.TrimPrefix(line, "Content:"))
			idx := strings.Index(rest, "contains")
			if idx == -1 {
				continue
			}
			glob := strings.TrimSpace(rest[:idx])
			pat := strings.TrimSpace(rest[idx+len("contains"):])
			if glob == "" || pat == "" {
				continue
			}
			directives = append(directives, fileDirective{
				Kind:    directiveContent,
				Glob:    glob,
				Pattern: pat,
			})

		case strings.HasPrefix(line, "NoFile:"):
			glob := strings.TrimSpace(strings.TrimPrefix(line, "NoFile:"))
			if glob == "" {
				continue
			}
			directives = append(directives, fileDirective{
				Kind: directiveNoFile,
				Glob: glob,
			})

		case strings.HasPrefix(line, "File:"):
			glob := strings.TrimSpace(strings.TrimPrefix(line, "File:"))
			if glob == "" {
				continue
			}
			directives = append(directives, fileDirective{
				Kind: directiveFile,
				Glob: glob,
			})
		}
	}
	return directives
}

// relPath returns path relative to base, or the full path if Rel fails.
func relPath(base, path string) string {
	r, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return r
}
