# Archon

Archon is an AI-powered standards auditor for Go projects.

## Quickstart

```bash
go install github.com/elpic/archon/cmd/archon@latest
archon audit ./...
```

## Configuration

Standards resolution order:
1. `.archon/standards.md` in the project (highest priority)
2. Org-level `.archon/standards.md` in the org's standards repo
3. Fallback GitHub repo specified via `--fallback <org/repo>`

The LLM provider is configured via environment variables:
- `OPENROUTER_API_KEY` (default)
- `OPENAI_API_KEY`
- `ANTHROPIC_API_KEY`

## Development

```bash
go build ./...
go test ./...
```

## Status

Experimental. APIs and rule formats will change.

## License

MIT
