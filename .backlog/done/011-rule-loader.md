---
title: "Rule Loader — Parse Markdown Rules"
status: done
priority: high
sprint: 1
labels: [feature, rules]
created: "2026-07-12"
completed: "2026-07-12"
pr: "https://github.com/elpic/archon/pull/7"
---

# Rule Loader — Parse Markdown Rules

Load and parse markdown rule files from `.rules/` directory.

## Problem

No mechanism exists to load rules from disk. The engine needs to discover
rule files, parse YAML frontmatter, and extract check criteria.

## Acceptance criteria

- [ ] `internal/rules` package with `Rule` struct: `{Name, Severity, Weight, Target, Exclude, Body, Category, Path}`
- [ ] `Load(dir string) ([]Rule, error)` walks `.rules/` recursively
- [ ] Parses YAML frontmatter (name, severity, weight, target, exclude)
- [ ] Extracts markdown body (everything after frontmatter)
- [ ] Category derived from parent folder name (e.g., `.rules/docker/` → category "docker")
- [ ] Skips non-markdown files, logs warnings for invalid frontmatter
- [ ] Returns rules sorted by category then name

## Technical notes

- Use `gopkg.in/yaml.v3` for frontmatter parsing (or a simple regex parser to avoid deps)
- Target defaults to `"**/*"` when missing
- Exclude defaults to `[]` when missing
- Weight defaults to `1` when missing
- Severity defaults to `"warn"` when missing

## Depends on

Nothing — pure I/O + parsing.
