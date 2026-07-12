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

**Severity:** error

### NoPrintf: Avoid fmt.Printf in library code

Avoid using fmt.Printf, fmt.Println, or similar in library code. Use structured logging (slog, zap, zerolog) instead so output can be controlled by the application.

**Severity:** warn

### ContextThreading: Pass context through call chains

Every function that performs I/O (network, filesystem, subprocess) must accept a `context.Context` as its first parameter and forward it to downstream calls. Never create a fresh `context.Background()` inside a function that already receives a context — this breaks cancellation and timeout propagation.

**Severity:** error

### NoErrorSwallow: Don't silently discard errors

Never discard an error with `_ = someCall()` or bare `someCall()` without checking the return value. If ignoring an error is intentional, document why with a comment explaining the reasoning. Use `errors.Is` or `errors.As` when comparing against sentinel or typed errors.

**Severity:** error

### SmallInterfaces: Keep interfaces small

Interfaces should have as few methods as possible — ideally one. Define interfaces at the consumer, not the producer. If a type only needs one method, declare an interface for that method rather than depending on the concrete type.

**Severity:** info

### TableDrivenTests: Use table-driven tests

Tests should use the table-driven pattern with `t.Run()` subtests. Each case should have a descriptive name. Avoid testify — use manual `if` / `t.Errorf` / `t.Fatal` assertions. No external test dependencies.

**Severity:** warn

### ErrorWrappingPrefix: Use terse lowercase prefixes in error messages

When wrapping errors with `fmt.Errorf`, use a short lowercase prefix describing the failing subsystem (e.g. `"standards resolver: %w"`, `"llm audit: %w"`). Don't start error messages with capitals or end them with periods.

**Severity:** info

### NilCheckBeforeDefer: Check errors before deferring close

When opening a resource that needs to be closed, check the error from the open call before deferring `Close()`. A failed open means the file handle is nil, and deferring `Close()` on nil will panic.

**Severity:** error

## Formatter

The formatter controls how archon renders violations to different output targets.
Each format serves a different consumer: human terminals, editor problem-matchers,
and diff-based fix suggestions.

### HumanReadable: Terminal output (Format)

Render each violation as a numbered block. Metadata (severity, rule, source location)
appears on dedicated lines so it stays scannable — do not embed it inside the
description text.

```
1. Comments are forbidden
   severity: error
   rule: no-comments
   file: internal/foo/foo.go
   line: 42
   suggestion: remove the comment
```

### Diagnostic: Editor problem-matchers (FormatDiagnostic)

One line per violation in the compact format editors expect:

```
path:line:col: [severity] message
```

This is a hard contract — editors and CI problem-matchers depend on the exact shape.
Do not add metadata fields to this format.

### FixSuggestion: Diff output (FormatFix)

Each violation produces a unified-diff-style block. The `-` line is the actual code
being replaced (read from the source file, not the violation description). The `+` line
is the suggested replacement. Metadata (rule, severity) appears as comments in the
diff header so tools can filter or group suggestions.

```
--- a/internal/foo/foo.go
+++ b/internal/foo/foo.go
@@ -42,1 +42,1 @@
-// TODO: implement this
+// removed unused comment
```

### MetadataFields: Structured violation properties

These properties are metadata and must never be baked into human-readable text.
They belong on their own lines in Format output, or as diff-header comments in
FormatFix output:

- **severity** — The violation level (info, warn, error, critical)
- **rule** — The rule ID that was violated
- **file** — Source file path (relative to target)
- **line** / **column** — Source coordinates
- **suggestion** — The suggested fix text
- **ruledoc** — Link or anchor to the rule's documentation

## License

MIT
