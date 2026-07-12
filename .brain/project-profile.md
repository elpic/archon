---
name: "archon"
domain: "devtools"
type: "cli"
primary-language: "go"
frameworks: []
database: "none"
deployment: "single-static-binary"
analyzed: "2026-07-11"
updated: "2026-07-12"
---

# What it does

Archon is a **deterministic linter for Go projects** that uses markdown files as rule definitions. Rules are organized in folders by category (`.rules/error-handling/`, `.rules/style/`, etc.). The engine parses markdown, extracts check criteria, and applies them to the codebase using pattern matching and Go AST analysis.

The LLM integration is optional — used only for `archon explain` (reasoning, examples) but not required for `archon audit`. The core audit loop is 100% deterministic.

# Key components

- `cmd/archon/` — CLI entry point (`main.go`), `init` subcommand (`init.go`), `watch` subcommand (in `main.go`). Wires the internal packages, parses `os.Args`, installs a `signal.NotifyContext` for Ctrl-C handling, and runs the watch event loop.
- `internal/audit/` — Audit runner (`audit.go`) and report formatting (`report.go`). The runner loads rules, executes checks, and assembles a `Report`. `Report.Format()` renders a terminal-friendly string; `Report.FormatDiagnostic()` renders the `path:line:col: [severity] message` problem-matcher stream consumed by `archon watch` and editor quickfix.
- `internal/rules/` — **Core package** (ticket #010). Rule engine that loads markdown rules from `.rules/` directory, parses frontmatter, and executes checks via pattern matching and Go AST analysis.
- `internal/llm/` — Optional LLM provider for `archon explain` only. Not required for `archon audit`.
- `internal/standards/` — Four-tier resolver (`project > org-header > auto-infer > fallback`). `Document` is `{Source, Body}`. Fetcher is injected (defaulting to the GitHub Contents API HTTP client) so tests can stub it.
- `internal/watch/` — fsnotify-backed filesystem watcher. `Watcher` interface, `FSNotifyWatcher` implementation, debounced event stream with `Changed` / `StandardsChanged` / `Error` kinds. Plumbing-only: it never calls into `audit` or `llm`.

# Data model

- `rules.Rule` — `{Name, Severity, Weight, Target, Exclude, Fixable, Body string}`
- `rules.Violation` — `{Rule, Description, Severity, File, Line, Column, Suggestion, RuleDoc string}`
- `rules.Severity` — enum (Info, Warn, Error, Critical) via iota
- `standards.Document` — `{Source string, Body []byte}`
- `audit.Report` — `{Target string, Violations []rules.Violation, StandardsSource string}`; methods `Format() string` and `FormatDiagnostic() string`
- `watch.Event` — `{Path string, Kind EventKind, StandardsSource string, Err error}`

There is no persistence layer, no configuration file format, and no on-disk state. Everything is in-memory and per-invocation.

# External dependencies

- Runtime: stdlib only, **plus** `github.com/fsnotify/fsnotify` for the watch subcommand. fsnotify is the project's one and only deliberate external dep, added because file-watching has no stdlib equivalent. `go.sum` exists.
- LLM providers: optional, for `archon explain` only. Not required for core audit.
- GitHub fallback: the `standards.Fetcher` HTTP client is in place; the watch loop does not need it but `archon audit` does when no project file is present.
- CI tooling: mise, govulncheck, go vet, Blueprint-rendered GitHub Actions, elpic/actions/github/drift-check@v2.
