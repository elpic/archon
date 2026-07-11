---
title: "Standards as Code"
status: pending
priority: medium
sprint: 1
labels: [feature, ideate]
created: "2026-07-11"
---

# Standards as Code

Diff, log, validate, share.

## Problem

Today standards.md is a markdown blob the LLM interprets. There's no
way to see *what* changed, *when* it changed, or whether two projects in
the same org are inheriting different things. Once `archon init` exists,
the next question is "where do the standards come from?" — and the
answer should be a discoverable ecosystem.

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

This is the longest-term moat and the network-effect play. Once teams
can `archon standards add acme/go-strict` or `archon standards share .`,
the standards themselves become shareable artifacts. The LLM-judge
design *requires* high-quality standards docs to be useful, and a
marketplace solves that.

## Source

Full rationale: `.brain/features/proposed/007-standards-as-code.md` (and
the original strategic context in
`.brain/features/proposed/10x-features.md`).
