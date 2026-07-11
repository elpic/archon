---
title: "Standards as Code"
status: selected
priority: medium
effort: L
impact: medium
sprint: 1
labels: [feature, ideate]
source: 10x-features.md
---

# Standards as Code

Diff, log, validate, share.

## Problem

Today standards.md is a markdown blob the LLM interprets. There's no
way to see *what* changed, *when* it changed, or whether two projects in
the same org are inheriting different things. Once `archon init` exists,
the next question is "where do the standards come from?" — and the
answer should be a discoverable ecosystem.

## Why now

This is the longest-term moat and the network-effect play. Once teams
can `archon standards add acme/go-strict` or `archon standards share .`,
the standards themselves become shareable artifacts. The LLM-judge
design *requires* high-quality standards docs to be useful, and a
marketplace solves that. It's also the feature that makes standards
drift *visible* — when the org changes a rule, every inheriting project
sees the diff in `archon standards log`, the same way engineers see
`git log` for code.

## Acceptance criteria

- [ ] `archon standards resolve` prints the resolved standards
      document, the resolution chain (project → org → fallback), and
      the source commit SHA
- [ ] `archon standards diff <ref>` shows the unified diff between
      the currently-resolved standards and the version at `<ref>` (a
      git ref, a tag, or "last-week")
- [ ] `archon standards log` prints a chronological list of changes
      to the resolved standards source (uses GitHub's commits API)
- [ ] `archon standards validate` runs a meta-audit: checks the
      standards doc for ambiguity, contradictions, and unfalsifiable
      rules. Catches the "rule that says 'write good code'" failure mode.
- [ ] `archon standards add <org/repo>` adds a remote fallback to
      `.archon/config.yaml`; the file format is human-editable TOML
- [ ] A `archon-stdlib` org repo (curated by us) ships 3 starter
      standards: `strict`, `default`, `minimal` — same shape as
      `eslint-config-*` in the JS world

## Notes

Link back to the source: see `.brain/features/proposed/10x-features.md` for
full rationale.
