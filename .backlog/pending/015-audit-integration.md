---
title: "Audit Integration — Wire Engine to CLI"
status: pending
priority: high
sprint: 1
labels: [feature, rules]
created: "2026-07-12"
---

# Audit Integration — Wire Engine to CLI

Replace LLM-based audit with deterministic rule engine.

## Problem

The CLI currently calls the LLM provider for audit. It needs to call the
rule engine instead, producing the same output format.

## Acceptance criteria

- [ ] `archon audit` uses rule engine (not LLM) by default
- [ ] `--rules-dir` flag to override `.rules/` location
- [ ] Output format matches current: per-rule breakdown with verdicts
- [ ] Weighted score displayed at end
- [ ] `archon audit --fix` applies auto-fixable rules (future)
- [ ] LLM provider remains available for `archon explain`
- [ ] Backwards-compatible: works without `.rules/` (empty audit)

## Technical notes

- `internal/audit/Runner` gets new method or new runner type
- Keep `internal/llm` package for `archon explain` use case
- Exit code 0 always (operator decides what to do with low score)

## Depends on

- #014 (Rule Engine Runner)
