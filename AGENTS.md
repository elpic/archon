# AGENTS.md

## Project: archon

AI-powered standards auditor for Go projects. Connects to an LLM provider,
resolves applicable standards (project > org > fallback GitHub repo), and runs
a configurable rule set against the target project.

## Stack

- Go 1.23+
- LLM providers via env vars (OpenRouter / OpenAI / Anthropic)

## Layout

```
cmd/archon/        CLI entry point
internal/audit/    audit runner + report formatting
internal/llm/      LLM provider abstraction
internal/rules/    rule definitions (future)
internal/standards/ standards resolver (project > org > fallback)
.archon/           archon's own dogfooded standards
```

## Git workflow

Trunk-based. See `~/.config/opencode/skills/trunk-based-git/SKILL.md`.
- Short-lived branches off `main`, rebase never merge.
- Branch prefixes: `feat/`, `fix/`, `chore/`, `sesh/`.
- Squash-merge PRs to `main`. Never push to `main` directly.

## Commands

```bash
go build ./...    # compile
go test ./...     # run tests
go run ./cmd/archon audit ./...  # run locally
```
