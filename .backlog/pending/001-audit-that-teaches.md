---
title: "Audit That Teaches"
status: pending
priority: high
sprint: 1
labels: [feature, ideate]
created: "2026-07-11"
updated: "2026-07-12"
---

# Audit That Teaches

Source locations, quick-fixes, `archon explain`.

## Problem

Today `Violation` is `{Rule, Description, Severity}` — no file, no line,
no fix suggestion. An engineer who sees "violates error wrapping rule" has
no way to act on it.

With the pivot to a deterministic rule engine (ticket #010), this ticket
focuses on the **teaching** aspect: explaining *why* a rule exists and
*how* to fix it, using the rule's markdown documentation.

## Acceptance criteria

- [ ] `Violation` carries `File`, `Line`, `Column`, `Suggestion string`,
      and `RuleDoc string` (anchor into the rule file). Backwards-compat
      shim: missing fields render as "?"
- [ ] `archon audit --fix` (off by default) prints the suggested fix in
      unified diff format (for fixable rules)
- [ ] `archon explain <rule-id>` prints: the rule text from the markdown
      file, examples from the codebase, and a one-line "fix it with:" suggestion
- [ ] `explain` is usable standalone (no audit required) — it just
      loads the rule and answers the question
- [ ] Rule files include "Why this matters" section for teaching context

## Notes

The judge-vs-teacher split is the difference between a tool teams *use*
and a tool teams *argue about in retro*. "Your code does X on line 42;
here's a one-line fix; here's the org's reasoning" turns a scolding tool
into a teaching tool. Also unlocks `archon explain` as a *standalone
learning surface*.

With deterministic rules, the "teaching" comes from the rule's markdown
documentation, not from LLM reasoning. The rule file is the source of
truth for both checking and explaining.

## Source

Full rationale: `.brain/features/proposed/003-audit-that-teaches.md`
(and the original strategic context in
`.brain/features/proposed/10x-features.md`).
