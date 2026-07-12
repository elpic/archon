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

## Rules

### ErrorWrapping: Wrap errors with context

When returning errors, wrap them with context using `fmt.Errorf("...: %w", err)` or `errors.Join`. This preserves the error chain for debugging.

**Severity:** Error

### NoPrintf: Avoid fmt.Printf in library code

Avoid using fmt.Printf, fmt.Println, or similar in library code. Use structured logging (slog, zap, zerolog) instead so output can be controlled by the application.

**Severity:** Warning

## License

MIT
