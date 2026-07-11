---
title: "Audit That Teaches"
status: selected
priority: high
effort: M
impact: high
sprint: 1
labels: [feature, ideate]
source: 10x-features.md
---

# Audit That Teaches

Source locations, quick-fixes, `archon explain`.

## Problem

Today `Violation` is `{Rule, Description, Severity}` — no file, no line,
no fix suggestion. An engineer who sees "violates error wrapping rule" has
no way to act on it. LLM-as-judge tools fail when they're inscrutable:
teams stop trusting them within a sprint.

## Why now

The judge-vs-teacher split is the difference between a tool teams *use*
and a tool teams *argue about in retro*. "Your code does X on line 42;
here's a one-line fix; here's the org's reasoning" turns a scolding AI
into a teaching AI. Also unlocks `archon explain` as a *standalone
learning surface* — a developer can ask "what does our org say about
context propagation?" and get a markdown explanation grounded in the
actual standards doc.

## Acceptance criteria

- [ ] `llm.Violation` grows `File`, `Line`, `Column`, `Suggestion string`,
      and `RuleDoc string` (anchor into the standards doc). Backwards-compat
      shim: missing fields render as "?"
- [ ] Prompt is structured (JSON schema) so the LLM *must* return
      `file:line:col` or the violation is rejected as malformed
- [ ] `archon audit --fix` (off by default) prints the suggested fix in
      unified diff format
- [ ] `archon explain <rule-id>` prints: the rule text, the LLM's
      reasoning, 2-3 examples from the codebase, and a one-line "fix it
      with:" suggestion
- [ ] `explain` is usable standalone (no audit required) — it just
      resolves standards and answers the question

## Notes

Link back to the source: see `.brain/features/proposed/10x-features.md` for
full rationale.
