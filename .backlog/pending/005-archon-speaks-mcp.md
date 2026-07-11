---
title: "Archon Speaks MCP"
status: pending
priority: high
sprint: 1
labels: [feature, ideate]
created: "2026-07-11"
---

# Archon Speaks MCP

First-class MCP server.

## Problem

In 2026, every developer has an AI agent in their editor (Claude Code,
Cursor, Continue, Zed). If archon is not in that agent's toolbelt, it
doesn't exist for the largest growing segment of users. `npm install` lost
to `pnpm dlx` and the like for the same reason: meet developers where
they are.

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

MCP is the lingua franca of agentic tools in 2026. An `archon mcp`
subcommand that exposes audit + standards + explain as MCP tools means
every agent session can run archon audits conversationally. This is the
land-and-expand distribution play that defines the next 18 months of
devtools.

## Source

Full rationale: `.brain/features/proposed/004-archon-speaks-mcp.md` (and
the original strategic context in
`.brain/features/proposed/10x-features.md`).
