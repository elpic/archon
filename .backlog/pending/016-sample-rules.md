---
title: "Sample Rules — Starter Rule Set"
status: pending
priority: medium
sprint: 1
labels: [feature, rules]
created: "2026-07-12"
---

# Sample Rules — Starter Rule Set

Create initial rules covering common patterns.

## Problem

No rules exist yet. Need a starter set to validate the engine works and
provide value immediately.

## Acceptance criteria

- [ ] `.rules/` directory with 5-10 rules across categories
- [ ] Rules cover: error handling, style, Docker, GitHub Actions, testing
- [ ] Each rule has clear description and check criteria
- [ ] Rules are language-agnostic (work on Go, Python, Node, etc.)
- [ ] Example rules:
      - `error-handling/no-error-swallow.md` — bare `err` without check
      - `style/no-printf.md` — `fmt.Printf` in library code
      - `docker/no-latest-tag.md` — `FROM.*:latest` in Dockerfile
      - `github-actions/has-ci.md` — must have CI workflow
      - `testing/has-tests.md` — must have test files

## Technical notes

- Rules should be simple regex patterns (easy to understand and maintain)
- Each rule demonstrates the pattern format for users
- Include "Why this matters" section for teaching context

## Depends on

- #011 (Rule Loader) — needs the format defined
