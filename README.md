# Archon

AI-powered standards auditor for Go projects.

Archon connects to an LLM provider and runs a configurable rule set to audit
whether a project adheres to defined standards. Standards are resolved in order:

1. **Project** — `.archon/standards.md` in the project root
2. **Organization** — `.archon/standards.md` at the org level (GitHub repo
   named after the project's owner, or a configured org-wide repo)
3. **Fallback** — a GitHub repo specified by the user as the canonical
   standards source

## Quickstart

```bash
go install github.com/elpic/archon/cmd/archon@latest
archon audit ./...
```

## Configuration

| Source | Path | Resolution priority |
|---|---|---|
| Project | `.archon/standards.md` | 1 (highest) |
| Org | `.github/.archon/standards.md` in the org repo | 2 |
| Fallback | GitHub repo specified via `--fallback <org/repo>` | 3 |

The LLM provider is configured via environment variables:

- `OPENAI_API_KEY` (default provider)
- `ANTHROPIC_API_KEY`
- `OPENROUTER_API_KEY`

## Status

Experimental. APIs and rule formats will change.

## License

MIT
