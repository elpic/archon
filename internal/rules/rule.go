// Package rules defines the Rule type and loads rule files from disk.
package rules

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Rule represents a single audit rule loaded from a markdown file.
type Rule struct {
	Name     string   // rule identifier from frontmatter
	Severity string   // info | warn | error | critical
	Weight   int      // relative importance (higher = more important)
	Target   string   // glob pattern for files this rule applies to
	Exclude  []string // glob patterns for files to skip
	Body     string   // markdown body after frontmatter
	Category string   // derived from parent folder name
	Path     string   // absolute path to the rule file
}

// Load walks dir recursively and returns all valid rules found.
// Non-markdown files are skipped. Invalid frontmatter produces a
// warning log and the file is skipped.
func Load(dir string) ([]Rule, error) {
	var rules []Rule

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !isMarkdown(path) {
			return nil
		}

		rule, parseErr := parseFile(path, dir)
		if parseErr != nil {
			slog.Warn("skipping invalid rule file",
				"path", path,
				"error", parseErr,
			)
			return nil
		}
		rules = append(rules, rule)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk rules dir: %w", err)
	}

	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Category != rules[j].Category {
			return rules[i].Category < rules[j].Category
		}
		return rules[i].Name < rules[j].Name
	})

	return rules, nil
}

// isMarkdown returns true if the file extension indicates markdown.
func isMarkdown(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".markdown"
}

// parseFile reads a rule file, splits frontmatter from body, and
// populates a Rule with defaults for missing fields.
func parseFile(path, rootDir string) (Rule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Rule{}, fmt.Errorf("read file: %w", err)
	}

	frontmatter, body, err := splitFrontmatter(string(data))
	if err != nil {
		return Rule{}, err
	}

	meta := parseFrontmatter(frontmatter)

	name := meta["name"]
	if name == "" {
		return Rule{}, fmt.Errorf("missing required field: name")
	}

	category := deriveCategory(path, rootDir)

	return Rule{
		Name:     name,
		Severity: defaultStr(meta["severity"], "warn"),
		Weight:   defaultInt(meta["weight"], 1),
		Target:   defaultStr(meta["target"], "**/*"),
		Exclude:  parseList(meta["exclude"]),
		Body:     strings.TrimSpace(body),
		Category: category,
		Path:     path,
	}, nil
}

// splitFrontmatter separates YAML frontmatter (between --- delimiters)
// from the markdown body. If no frontmatter is present, it returns an
// empty frontmatter and the full content as body.
func splitFrontmatter(content string) (frontmatter, body string, err error) {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "---") {
		return "", trimmed, nil
	}

	// Find closing ---
	rest := trimmed[3:]
	endIdx := strings.Index(rest, "---")
	if endIdx == -1 {
		return "", "", fmt.Errorf("unclosed frontmatter")
	}

	frontmatter = strings.TrimSpace(rest[:endIdx])
	body = strings.TrimSpace(rest[endIdx+3:])
	return frontmatter, body, nil
}

// parseFrontmatter parses simple YAML key: value lines into a map.
// Supports quoted and unquoted string values. Does not handle nested
// structures — frontmatter for rules is flat key-value pairs only.
func parseFrontmatter(yaml string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(yaml, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, "\"'")
		result[key] = val
	}
	return result
}

// deriveCategory extracts the category from the rule file's path
// relative to the root directory. For example, if root is ".rules"
// and the file is ".rules/docker/best-practices.md", the category
// is "docker". Files directly in the root get "general".
func deriveCategory(path, rootDir string) string {
	rel, err := filepath.Rel(rootDir, path)
	if err != nil {
		return "general"
	}
	dir := filepath.Dir(rel)
	if dir == "." {
		return "general"
	}
	// Use only the first path component as category.
	parts := strings.SplitN(dir, string(os.PathSeparator), 2)
	return parts[0]
}

func defaultStr(val, fallback string) string {
	if val == "" {
		return fallback
	}
	return val
}

func defaultInt(val string, fallback int) int {
	if val == "" {
		return fallback
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return fallback
	}
	return n
}

// parseList splits a comma-separated string into a trimmed slice.
// Returns nil for empty input.
func parseList(val string) []string {
	if val == "" {
		return nil
	}
	parts := strings.Split(val, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
