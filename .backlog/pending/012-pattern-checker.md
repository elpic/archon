---
title: "Pattern Checker — Regex on File Content"
status: pending
priority: high
sprint: 1
labels: [feature, rules]
created: "2026-07-12"
---

# Pattern Checker — Regex on File Content

Implement regex-based pattern matching on file content.

## Problem

Rules need to check file contents for patterns (e.g., `FROM.*:latest` in
Dockerfiles, `fmt.Printf` in Go files). A regex checker is the simplest
and most portable check type.

## Acceptance criteria

- [ ] `Checker` interface: `Check(file string, content []byte) []Violation`
- [ ] `PatternChecker` implements Checker — runs regex patterns from rule body
- [ ] Parses `Pattern: <regex>` lines from rule markdown
- [ ] Returns violations with file, line number, and matched content
- [ ] Supports `--` comment syntax for PASS/VIOLATION markers in patterns
- [ ] Handles multi-line patterns (e.g., function definitions)

## Technical notes

- Pattern format: `Pattern: <regex>` — one pattern per line
- Each match = one violation
- Line numbers extracted from match position in content
- Use Go's `regexp` package (RE2 syntax, no backtracking)

## Depends on

- #011 (Rule Loader) — needs `Rule` struct with Body field
