---
title: "Deterministic Rule Engine"
status: pending
priority: high
sprint: 1
labels: [feature, architecture]
created: "2026-07-12"
updated: "2026-07-12"
---

# Deterministic Rule Engine

Replace LLM-as-judge with a deterministic, markdown-based rule engine.

## Problem

The current architecture relies on an LLM to interpret standards and find
violations. This is non-deterministic, expensive, and opaque. Teams need
a tool that runs instantly, produces consistent results, and doesn't require
an API key to function.

The service-auditor plugin (AvantFinCo/avant-agent-plugins) proves this
model works: shell scripts execute checks, violations are emitted to stdout,
and a weighted score is computed deterministically.

## Vision

Archon becomes a **deterministic linter for any project** that uses
markdown files as rule definitions. Rules are organized in folders by
category. The engine parses markdown, extracts check criteria, and applies
them to the codebase using pattern matching, file checks, or other
deterministic validations.

**Language-agnostic**: rules work on Go, Python, Node, Ruby, or any project.
No Go-specific AST analysis — just regex patterns, file existence checks,
and content inspection.

### Rule format (markdown-based, no scripts)

```
.rules/
  error-handling/
    wrap-errors.md
    no-error-swallow.md
  style/
    no-printf.md
    no-console-log.md
  testing/
    has-tests.md
    no-skip-in-ci.md
  docker/
    has-healthcheck.md
    no-latest-tag.md
  github-actions/
    has-ci-workflow.md
```

Each rule file is markdown with frontmatter:

```markdown
---
name: no-latest-tag
severity: warn
weight: 10
target: "Dockerfile"
---

# Don't use :latest tag in Dockerfiles

Using `:latest` makes builds non-reproducible.

## Check

- Pattern: `FROM.*:latest` → VIOLATION
- Pattern: `FROM\s+\S+:(?!latest)\S+` → PASS
```

### Folder organization

- `.rules/` at project root (configurable via `--rules-dir`)
- Folders = categories (error-handling, style, docker, etc.)
- Rules are markdown files with YAML frontmatter
- No scripts — the engine interprets the markdown patterns

## Acceptance criteria

- [ ] `internal/rules` package implements rule loading from `.rules/` directory
- [ ] Rules are markdown files with YAML frontmatter (name, severity, weight, target, exclude)
- [ ] Rules support file targeting via glob patterns (target/exclude)
- [ ] Rules support pattern matching (regex on file content)
- [ ] Rules support file existence checks (has Dockerfile, has CI workflow, etc.)
- [ ] Folder names become categories in the output
- [ ] `archon audit` runs all rules deterministically (no LLM required)
- [ ] Weighted scoring: `score = round(passing / applicable * 100)`
- [ ] Findings written to cache dir (not in repo)
- [ ] Language-agnostic: works on Go, Python, Node, Ruby, Docker, GitHub Actions, etc.
- [ ] LLM integration becomes optional (for explain/reasoning features only)

## Technical notes

### Rule engine architecture

```
internal/rules/
  engine.go       # RuleEngine: load rules, run checks, collect violations
  loader.go       # Parse markdown + frontmatter from .rules/ directory
  checker.go      # Interface for different check types
  checkers/
    pattern.go    # Regex-based pattern matching on file content
    filecheck.go  # File existence/size checks
```

### Rule file contract

```yaml
---
name: kebab-case-id
severity: info|warn|error|critical
weight: integer (for scoring)
target: glob pattern (default: "**/*")
exclude: [glob patterns]
---

# Rule title

Description of what this rule checks.

## Check

Check criteria in markdown (interpreted by the engine).
Lines starting with `Pattern:` are regex checks.
Lines starting with `File:` are file existence checks.
```

### Migration path

1. Implement rule engine in `internal/rules`
2. Keep LLM provider as optional (for `archon explain` reasoning)
3. Update `internal/audit` to use rule engine instead of LLM
4. Migrate existing `.archon/standards.md` rules to `.rules/` folder
5. Deprecate LLM-based audit (keep for explain/reasoning)

## Source

Inspired by:
- service-auditor (AvantFinCo) — deterministic shell-based audit
- eslint, golangci-lint — rule-based linters
- The user's explicit request: "100% deterministic, no scripts, markdown rules, any project type"
