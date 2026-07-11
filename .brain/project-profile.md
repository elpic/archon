---
name: "archon"
domain: "devtools"
type: "cli"
primary-language: "go"
frameworks: []
database: "none"
deployment: "single-static-binary"
analyzed: "2026-07-11"
---

# What it does

Archon is an AI-powered standards auditor for Go projects. It is intended to read a markdown standards document (e.g. `.archon/standards.md`), resolve the applicable standards across a project > org > GitHub fallback chain, send the standards and the target project to an LLM provider, and print a list of violations. As of this snapshot the orchestration shell, type system, resolution order, and the `archon watch` inner loop are real; the LLM client is a stub and the watch loop is therefore plumbing-only.

# Key components

- `cmd/archon/` — CLI entry point (`main.go`), `init` subcommand (`init.go`), `watch` subcommand (in `main.go`). Wires the internal packages, parses `os.Args`, installs a `signal.NotifyContext` for Ctrl-C handling, and runs the watch event loop.
- `internal/audit/` — Audit runner (`audit.go`) and report formatting (`report.go`). The runner resolves standards, calls the LLM provider, and assembles a `Report`. `Report.Format()` renders a terminal-friendly string; `Report.FormatDiagnostic()` renders the `path:line:col: [severity] message` problem-matcher stream consumed by `archon watch` and editor quickfix.
- `internal/llm/` — Provider interface (`Audit(ctx, standardsBody, target) ([]Violation, error)`) plus `Violation` (now with `File`/`Line`/`Column` source coordinates), `Severity` (Info/Warn/Error/Critical, iota), `Violation.String()` in problem-matcher format, and a stub `New()` that returns "not yet implemented".
- `internal/standards/` — Four-tier resolver (`project > org-header > auto-infer > fallback`). `Document` is `{Source, Body}`. Fetcher is injected (defaulting to the GitHub Contents API HTTP client) so tests can stub it.
- `internal/watch/` — fsnotify-backed filesystem watcher. `Watcher` interface, `FSNotifyWatcher` implementation, debounced event stream with `Changed` / `StandardsChanged` / `Error` kinds. Plumbing-only: it never calls into `audit` or `llm`.
- `internal/rules/` — Empty. Reserved for future rule definitions; not yet wired in.

# Data model

- `llm.Violation` — `{Rule string, Description string, Severity Severity, File string, Line int, Column int}` plus `String()` for problem-matcher format
- `llm.Severity` — enum (Info, Warn, Error, Critical) via iota
- `standards.Document` — `{Source string, Body []byte}`
- `audit.Report` — `{Target string, Violations []llm.Violation, StandardsSource string}`; methods `Format() string` and `FormatDiagnostic() string`
- `watch.Event` — `{Path string, Kind EventKind, StandardsSource string, Err error}`

There is no persistence layer, no configuration file format, and no on-disk state. Everything is in-memory and per-invocation.

# External dependencies

- Runtime: stdlib only, **plus** `github.com/fsnotify/fsnotify` for the watch subcommand. fsnotify is the project's one and only deliberate external dep, added because file-watching has no stdlib equivalent. `go.sum` exists.
- LLM providers: declared as a future dependency (OpenRouter / OpenAI / Anthropic) but no HTTP client is wired in yet. No environment variables are read at runtime today.
- GitHub fallback: the `standards.Fetcher` HTTP client is in place; the watch loop does not need it but `archon audit` does when no project file is present.
- CI tooling: mise, govulncheck, go vet, Blueprint-rendered GitHub Actions, elpic/actions/github/drift-check@v2.
