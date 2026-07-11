---
title: "Ship-Ready CI"
status: pending
priority: high
sprint: 1
labels: [feature, ideate]
created: "2026-07-11"
---

# Ship-Ready CI

SARIF, exit codes, GitHub Action, pre-commit.

## Problem

No flags, no JSON, no SARIF, no exit code, no GitHub Action. The binary
can't actually be used in any real CI system. The README advertises
`--fallback` which doesn't exist. This is the table stakes for any
code-scanning tool in 2026.

## Acceptance criteria

- [ ] `--format terminal|json|sarif` flag on `archon audit`; default
      is `terminal`, `--format sarif` writes SARIF 2.1.0 to stdout
- [ ] Exit codes: `0` clean, `1` violations found, `2` error (config,
      LLM failure, network). Documented in `archon audit --help`
- [ ] `--fallback <org/repo>` flag actually plumbs through to
      `standards.NewResolver` (fixes the dead field today)
- [ ] `.github/action.yml` for `archon-action@v0`: inputs are `path`,
      `fallback`, `openai-api-key`; output is `report.sarif` uploaded
      via `github/codeql-action/upload-sarif`
- [ ] `archon install pre-commit` writes a `.pre-commit-config.yaml`
      fragment; `archon install lefthook` does the same
- [ ] `--explain` flag on `archon audit` prints a one-line "Run
      `archon explain <rule>` to learn more" footer under each violation

## Notes

Distribution *is* the product for devtools. A tool that can't be wired
into GitHub code scanning, GitLab CI, pre-commit, or lefthook is a tool
that doesn't exist. Bundling this as a single P6 feature (rather than a
chore) frames it correctly: the *output surface* is the product as much
as the audit logic is.

## Source

Full rationale: `.brain/features/proposed/006-ship-ready-ci.md` (and the
original strategic context in
`.brain/features/proposed/10x-features.md`).
