---
title: "The Inner Loop"
status: selected
priority: high
effort: M
impact: high
sprint: 1
labels: [feature, ideate]
source: 10x-features.md
---

# The Inner Loop

`archon watch` + clickable diagnostics.

## Problem

A linter that only runs in CI is forgotten. A linter that runs on every
save becomes a habit. golangci-lint won the Go world partly because
`golangci-lint run` is fast enough to wire into file watchers. Archon's
first release can't do that — it shells out to an LLM. We need a watch
mode that's smart about *when* to call the LLM.

## Why now

"Daily tool vs one-time install" is the single biggest retention
predictor. Watch mode also creates a unique product signature: archon
watches *the standards document itself* and re-audits when org rules
change — something no static analyzer can do. This is the "from CI job to
daily companion" pivot.

## Acceptance criteria

- [ ] `archon watch` uses fsnotify to watch the target project (debounced,
      500ms)
- [ ] On file change: re-audit the project (re-resolve standards + re-run
      the existing audit pipeline) and print clickable
      `path:line:col: [severity] message`. Per-file audit optimisation
      is a follow-up once `Provider.AuditFile` exists; the LLM
      round-trip is the bottleneck, not the FS scan.
- [ ] On `.archon/standards.md` change: re-resolve + re-audit everything
      and print "Standards updated from <source>; re-running audit"
- [ ] Output format follows the GCC/clippy/quickfix/problem-matcher
      convention (`path:line:col: [severity] message`) so editors
      (Neovim, VS Code, Helix) parse it natively. This is NOT the
      LSP JSON-RPC protocol — that's a separate, later ticket.
- [ ] Graceful shutdown on SIGINT (in-flight LLM call cancelled via
      the existing `signal.NotifyContext`)

## Notes

Link back to the source: see `.brain/features/proposed/10x-features.md` for
full rationale.

The `Violation` shape (File / Line / Column) is being added as part
of this ticket, ahead of the parallel "Audit That Teaches" ticket.
The watch output needs source coordinates to print problem-matcher
lines; the parallel ticket (when it lands) will build on top of
those fields.
