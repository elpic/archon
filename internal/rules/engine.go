package rules

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Engine orchestrates loading rules and running checks against a target.
type Engine struct {
	rulesDir string
	rules    []Rule
}

// NewEngine loads all rules from rulesDir and returns an Engine.
func NewEngine(rulesDir string) (*Engine, error) {
	rules, err := Load(rulesDir)
	if err != nil {
		return nil, fmt.Errorf("load rules: %w", err)
	}
	return &Engine{rulesDir: rulesDir, rules: rules}, nil
}

// Run executes all loaded rules against target and returns a Report.
func (e *Engine) Run(target string) (*Report, error) {
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return nil, fmt.Errorf("resolve target: %w", err)
	}

	// Collect all files under target, stored as relative paths for glob matching.
	relFiles, err := collectRelFiles(absTarget)
	if err != nil {
		return nil, fmt.Errorf("walk target: %w", err)
	}

	results := make([]RuleResult, 0, len(e.rules))
	for _, rule := range e.rules {
		rr := e.runRule(rule, absTarget, relFiles)
		results = append(results, rr)
	}

	return NewReport(absTarget, results), nil
}

// runRule evaluates a single rule against the target. It decides whether
// to run PatternChecker (per-file) and/or FileChecker (directory-level).
func (e *Engine) runRule(rule Rule, target string, relFiles []string) RuleResult {
	rr := RuleResult{Rule: rule}

	hasPatterns := len(parsePatterns(rule.Body)) > 0
	hasFileDirectives := len(parseFileDirectives(rule.Body)) > 0

	// Pattern-based check: run against each matching file.
	if hasPatterns {
		matching := filterByTarget(rule, relFiles)
		matching = filterByExclude(rule, matching)

		if len(matching) == 0 {
			// No applicable files — rule is N/A for pattern checks.
			// But if it also has file directives, we still run those.
			if !hasFileDirectives {
				rr.Verdict = VerdictNA
				return rr
			}
		} else {
			pc := NewPatternChecker(rule)
			for _, f := range matching {
				absPath := filepath.Join(target, f)
				content, err := os.ReadFile(absPath)
				if err != nil {
					continue
				}
				violations := pc.Check(f, content)
				rr.Violations = append(rr.Violations, violations...)
			}
		}
	}

	// Directory-level check (File: / NoFile: / Content:).
	if hasFileDirectives {
		fc := NewFileChecker(rule)
		violations := fc.CheckDirectory(target)
		rr.Violations = append(rr.Violations, violations...)
	}

	// Determine verdict.
	if len(rr.Violations) > 0 {
		rr.Verdict = VerdictViolation
	} else if !hasPatterns && !hasFileDirectives {
		// Rule with neither patterns nor file directives — N/A.
		rr.Verdict = VerdictNA
	} else {
		rr.Verdict = VerdictPass
	}

	return rr
}

// filterByTarget returns files matching the rule's target glob.
func filterByTarget(rule Rule, files []string) []string {
	if rule.Target == "" || rule.Target == "**/*" {
		return files
	}
	var matched []string
	for _, f := range files {
		if globMatch(rule.Target, f) {
			matched = append(matched, f)
		}
	}
	return matched
}

// filterByExclude removes files matching any of the rule's exclude patterns.
func filterByExclude(rule Rule, files []string) []string {
	if len(rule.Exclude) == 0 {
		return files
	}
	var kept []string
	for _, f := range files {
		excluded := false
		for _, pat := range rule.Exclude {
			if globMatch(pat, f) {
				excluded = true
				break
			}
		}
		if !excluded {
			kept = append(kept, f)
		}
	}
	return kept
}

// globMatch checks if path matches the glob pattern. It handles ** for
// recursive directory matching and * for single-path-component matching.
func globMatch(pattern, path string) bool {
	// Normalize to forward slashes for consistent matching.
	pattern = filepath.ToSlash(pattern)
	path = filepath.ToSlash(path)

	// Try filepath.Match first for simple patterns.
	matched, err := filepath.Match(pattern, path)
	if err == nil && matched {
		return true
	}

	// For ** patterns, walk down the path segments.
	if strings.Contains(pattern, "**") {
		return globMatchDeep(pattern, path)
	}

	return false
}

// globMatchDeep handles ** glob patterns by matching against path segments.
func globMatchDeep(pattern, path string) bool {
	patternParts := strings.Split(pattern, "/")
	pathParts := strings.Split(path, "/")
	return matchParts(patternParts, pathParts)
}

// matchParts recursively matches pattern segments against path segments.
func matchParts(pattern, path []string) bool {
	// Base case: both consumed.
	if len(pattern) == 0 && len(path) == 0 {
		return true
	}
	if len(pattern) == 0 {
		return false
	}

	if pattern[0] == "**" {
		// ** matches zero or more path segments.
		// Try consuming 0, 1, 2, ... segments.
		for i := 0; i <= len(path); i++ {
			if matchParts(pattern[1:], path[i:]) {
				return true
			}
		}
		return false
	}

	// Non-** segment: must have a path part and they must match.
	if len(path) == 0 {
		return false
	}

	matched, err := filepath.Match(pattern[0], path[0])
	if err != nil || !matched {
		return false
	}

	return matchParts(pattern[1:], path[1:])
}

// collectRelFiles walks the target directory and returns all file paths
// relative to target.
func collectRelFiles(target string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(target, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(target, path)
		if err != nil {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	return files, err
}
