---
title: "Archon Speaks MCP"
status: selected
priority: high
effort: M
impact: high
sprint: 1
labels: [feature, ideate]
source: 10x-features.md
---

# Archon Speaks MCP

First-class MCP server.

## Problem

In 2026, every developer has an AI agent in their editor (Claude Code,
Cursor, Continue, Zed). If archon is not in that agent's toolbelt, it
doesn't exist for the largest growing segment of users. `npm install` lost
to `pnpm dlx` and the like for the same reason: meet developers where
they are.

## Why now

MCP is the lingua franca of agentic tools in 2026. An `archon mcp`
subcommand that exposes `audit_file`, `resolve_standards`, `explain_rule`,
and `get_violations` as MCP tools means every agent session can run archon
audits conversationally. "Hey Cursor, run archon on the file I just
edited" becomes one prompt. This is the land-and-expand distribution
play that defines the next 18 months of devtools.

## Acceptance criteria

- [ ] `archon mcp` subcommand starts a stdio MCP server exposing tools:
      `audit_file`, `audit_project`, `resolve_standards`, `explain_rule`
- [ ] Tool schemas are rich enough that an agent can ask "audit only
      files changed since main" or "explain rule error-wrap" without
      prose-guessing
- [ ] Server streams progress events for long audits (so Cursor's UI
      can show "Auditing 3/12 files...")
- [ ] Published in the MCP registry under `devtools/archon`
- [ ] `AGENTS.md` gets a one-paragraph "Using archon from your agent"
      section with copy-paste Claude/Cursor config

## Notes

Link back to the source: see `.brain/features/proposed/10x-features.md` for
full rationale.
