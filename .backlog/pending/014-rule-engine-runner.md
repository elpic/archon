---
title: "Rule Engine Runner — Orchestrate Checks"
status: pending
priority: high
sprint: 1
labels: [feature, rules]
created: "2026-07-12"
---

# Rule Engine Runner — Orchestrate Checks

Wire rule loading + checkers into a cohesive engine.

## Problem

Individual checkers exist but nothing orchestrates them. The engine needs
to load rules, match files, run checks, and aggregate results.

## Acceptance criteria

- [ ] `Engine` struct holds loaded rules and checker instances
- [ ] `NewEngine(rulesDir string) (*Engine, error)` loads all rules
- [ ] `Run(target string) (*Report, error)` executes all rules against target
- [ ] File targeting: each rule only runs against files matching its `target` glob
- [ ] Excludes: files matching `exclude` patterns are skipped
- [ ] Weighted scoring: `score = round(passing / applicable * 100)`
- [ ] Report includes per-rule verdict (pass/violation/N/A) and violations
- [ ] Findings written to cache dir (`$XDG_CACHE_HOME/archon/...`)

## Technical notes

- Engine composes PatternChecker and FileChecker
- Rules are grouped by category for output
- N/A rules excluded from scoring (like service-auditor)
- Cache dir: `$XDG_CACHE_HOME/archon/{service}/{date}/findings.md`

## Depends on

- #011 (Rule Loader)
- #012 (Pattern Checker)
- #013 (File Checker)
