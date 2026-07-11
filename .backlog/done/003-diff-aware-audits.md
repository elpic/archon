---
title: "Diff-Aware Audits"
status: done
priority: high
sprint: 1
labels: [feature, ideate]
created: "2026-07-11"
completed: "2026-07-11"
pr: "https://github.com/elpic/archon/pull/4"
---

# Diff-Aware Audits

`--changed` and friends.

## Problem

LLM calls cost money and take seconds. Auditing an entire microservice
on every PR is a 10x cost vs. auditing only the files that changed. No
team will wire archon into CI if a single PR burns $2 and adds 90s to
the queue.

## Acceptance criteria

- [ ] `archon audit --changed` audits only files in
      `git diff --name-only HEAD~1`
- [ ] `archon audit --since <ref>` audits files changed between `<ref>` and
      HEAD
- [ ] Auto-detection: in GitHub Actions, defaults to
      `${{ github.event.pull_request.base.sha }}` if no flag given
- [ ] `--changed` reports a cost summary at the end: "Audited 4 files
      (skipped 238); estimated cost $0.03"
- [ ] When `--changed` is combined with `--watch`, only the changed file
      is re-audited (no project-wide blast radius)

## Notes

This is the feature that decides whether archon lives in CI or gets
shelved after the first invoice. It's also the feature that makes archon
a *green* tool — being faster than the human review it augments is
non-negotiable for adoption.

## Source

Full rationale: `.brain/features/proposed/005-diff-aware-audits.md` (and
the original strategic context in
`.brain/features/proposed/10x-features.md`).
