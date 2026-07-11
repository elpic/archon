---
title: "Adopt in 10 Seconds"
status: selected
priority: high
effort: M
impact: high
sprint: 1
labels: [feature, ideate]
source: 10x-features.md
---

# Adopt in 10 Seconds

`archon init` + org inheritance.

## Problem

Every new Go repo at a company re-litigates the same "what's our
error-handling style / module layout / naming" debate. There's no ceremony
to inherit team standards, so they get copied-pasted into README and drift
within a quarter.

## Why now

In 2026, "team alignment" is a multi-million-hour/year tax. The
org-inheritance pattern is archon's unique structural advantage — no other
tool has it. `archon init` is the conversion mechanism: a staff engineer
writes standards once in the org repo, every new service inherits on day
one. This is the feature that turns archon from a per-project tool into a
*company* tool. The flywheel: more orgs use it → more standards ecosystems
→ more adoption.

## Acceptance criteria

- [ ] `archon init` scaffolds `.archon/standards.md` in the target project,
      pre-populated with a comment header pointing to the org-level source
- [ ] Resolver correctly implements all three tiers: project →
      `gh:<owner>/<repo>/.archon/standards.md` → `--fallback`
- [ ] `archon init --from elpic/go-standards` creates a project that
      inherits elpic/go-standards via GitHub fetch (with a real network
      call, mocked in tests)
- [ ] Resolved source is visible in `archon audit` output ("Standards:
      github.com/elpic/go-standards@abc123") — inheritance is observable,
      not invisible
- [ ] `archon init` is idempotent: running twice does not clobber an
      existing project standards file

## Notes

Link back to the source: see `.brain/features/proposed/10x-features.md` for
full rationale.

**Extended with auto-infer (commit on `feat/archon-init`):** a fourth
resolver tier was added that derives the org from `GITHUB_REPOSITORY` or
`git remote get-url origin` and looks for `<owner>/.archon/standards.md`
— the GitHub `.github` convention applied to archon. With this in place,
"clone → `archon audit`" works with no config step beyond the org-level
repo. Auto-infer sits between the `from:` header (tier 2, explicit wins)
and `WithFallback` (tier 4); it is best-effort and never surfaces an
error to the caller.
