# Architecture

## High-level diagram

```
                     +-------------------+
                     |   cmd/archon      |
                     | (subcommand       |
                     |  dispatch, flag   |
                     |  parsing, signal) |
                     +---------+---------+
                               |
        +----------------------+----------------------+----------------------+
        v                      v                      v                      v
+---------------+   +-----------------+   +-------------------+   +-------------------+
| internal/     |   | internal/llm    |   | internal/audit    |   | internal/watch    |
| standards     |   | (Provider iface |   | (Runner +         |   | (fsnotify-based   |
| (Resolver,    |   |  + stub New())  |   |  Report.Format,   |   |  debounced FS     |
|  Fetcher,     |   +-----------------+   |  FormatDiagnostic)|   |  watcher,         |
|  Document)    |            ^                    ^   ^       |   |  plumbing only)   |
+-------+-------+            |                    |   |       |   +-------------------+
        |                    |                    |   |       |            ^
        v                    |                    |   |       |            |
./.archon/standards.md       |                    |   |       |            |
        |                    |                    |   |       |            |
        |  audit.Runner.Run(ctx, target)          |   |       |            |
        |  1. resolver.Resolve() -> *Document     |   |       |            |
         |     (project | org-header |              |   |       |            |
         |      auto-infer | fallback)             |   |       |            |
        |  2. provider.Audit(ctx, doc.Body, t)     |   |       |            |
        |     -> []Violation                      |   |       |            |
        |  3. Report{Target, Violations,           |   |       |            |
        |         StandardsSource=doc.Source}     |   |       |            |
        |     -> Report.Format() |                |   |       |            |
        |        Report.FormatDiagnostic()        |   |       |            |
        +----------------------------------------+   |       |            |
                                |                       |       |            |
                                +-----------------------+-------+            |
                                                |                              |
                                                v                              |
                            `archon watch`: chan Event -> re-Run pipeline ->   |
                            FormatDiagnostic() on stdout (problem-matcher)      |
                                                                              |
                            `archon watch` subscribes to the watch.FSNotifyWatcher
                            and re-runs the same audit pipeline on every change
```

## Data flow

1. `main.go` dispatches on `os.Args[1]`: `audit`, `watch`, `init`, or `help`. Each
   subcommand parses its own flags with the stdlib `flag` package.
2. For `archon audit`:
   1. `standards.NewResolver(".", WithFallback(...))` is constructed.
   2. `llm.New(ctx)` is called (stub: returns "not yet implemented").
   3. `audit.NewRunner(resolver, provider)` runs the pipeline:
      a. `resolver.Resolve(ctx, target)` walks the four tiers
         (project → org-header → auto-infer → fallback) and returns a
         `*Document`. See "Resolver tiers" below.
      b. `provider.Audit(ctx, doc.Body, target)` returns `[]Violation`.
      c. A `Report{Target, Violations, StandardsSource=doc.Source}` is
         built and `Format()` renders it (with a `Standards: <source>`
         line when the source is non-empty).
3. For `archon watch`:
   1. A `Resolver` and a `Provider` (or a stub that re-emits the LLM
      construction error) are wired up the same way as `archon audit`.
   2. `watch.FSNotifyWatcher.Watch(ctx, target)` walks the target
      recursively, subscribes to fsnotify on every directory, and
      returns a channel of debounced `Event` values.
   3. The event loop dispatches:
      - `Changed` → `runner.Run` → `report.FormatDiagnostic()` (problem-matcher).
      - `StandardsChanged` → re-resolve + `runner.Run` (the resolver
        re-reads the project file from disk on every call, so the
        standards change is observed).
      - `Error` → log to stderr, continue.
   4. Audit errors (most commonly the LLM stub today) are logged to
      stderr; the watch loop does not exit on them.
4. For `archon init`:
   1. `archon init` (no flag) writes a `.archon/standards.md` scaffold
      with a comment header and a `# Project Standards` body — the
      file is tier 1 (project-local source).
   2. `archon init --from owner/repo` writes a redirect-only comment
      block containing the `from:` directive. The resolver sees a
      redirect-only file, misses tier 1, and fetches the org source at
      audit time.
   3. Both forms are idempotent: if `.archon/standards.md` already
      exists, the call is a no-op.

## Component responsibilities

- **cmd/archon** — process entry, subcommand dispatch (`audit`, `watch`, `init`, `help`), per-subcommand flag parsing via the stdlib `flag` package, signal handling, dependency wiring. The watch loop's dispatch and audit-error handling live here, not in `internal/watch`.
- **internal/standards** — find and load the standards document. `Resolver` holds a `fallbackOrgRepo` (now wired via `WithFallback`) and a `Fetcher` (now wired via `WithFetcher`, defaulting to `httpFetcher` hitting the GitHub Contents API). `fromProject` walks the target dir looking for `.archon/standards.md`. A new `fromOrgHeader` reads a `from: owner/repo` line from the project file; if the project file is *only* a redirect comment, tier 1 misses and the org source is fetched. Tier 3 (auto-infer) lives in `autoinfer.go` and derives the org from `GITHUB_REPOSITORY` or `git remote get-url origin`; the fetch is best-effort and any miss falls through.
- **internal/llm** — defines the contract between archon and any LLM provider. The `Provider` interface and data types are stable; only `New()` remains unimplemented. `Violation` carries `Rule`, `Description`, `Severity`, and now `File`/`Line`/`Column` source coordinates.
- **internal/audit** — orchestrates the pipeline and owns the report shape. `Report` carries `StandardsSource` (set from the resolved `Document.Source`) so the user can see which tier was used. `Report.Format()` prepends a `Standards: <source>` line when the source is non-empty; `Report.FormatDiagnostic()` renders each violation in the `path:line:col: [severity] message` problem-matcher format consumed by editor quickfix / VS Code "Error Lens". `Format()` is the terminal layout for one-shot `archon audit`; `FormatDiagnostic()` is the watch-loop stream.
- **internal/watch** — fsnotify-backed filesystem watcher. The `Watcher` interface exposes one method, `Watch(ctx, target) (<-chan Event, error)`, and returns a channel of debounced events classified as `Changed` / `StandardsChanged` / `Error`. The watcher is plumbing-only: it never calls into `audit` or `llm`. fsnotify is the project's one external dep (see "External dependencies" below).
- **internal/rules** — placeholder directory; nothing in it today.

## Resolver tiers

Resolution walks a fixed four-tier chain; the first hit wins.

1. **Project** — `target/.archon/standards.md` with substantive body content (more than a `from:` redirect comment). When the file is *only* a `from:` redirect, this tier is treated as a miss and resolution falls through.
2. **Header** — the `from: owner/repo` line inside the project file is the org's source of truth. The resolver fetches `github.com/<owner>/<repo>/.archon/standards.md`. An explicit `from:` header always wins over the auto-infer convention: when a user writes `from: elpic/go-strict`, that's deliberate.
3. **Auto-infer** (new) — if no project file or no `from:` header, the resolver derives the org from `GITHUB_REPOSITORY` (set in GitHub Actions) and falls back to `git -C <target> remote get-url origin`. The remote URL is parsed for a GitHub owner; resolution then looks for `github.com/<owner>/.archon/contents/.archon/standards.md`. This is the GitHub `.github` convention applied to archon: every org can publish a `.archon` repo whose `.archon/standards.md` all of its services inherit by default. Tier 3 is best-effort: any failure (no env, no git, non-GitHub remote, missing `.archon` repo) falls through silently — it never returns an error.
4. **Fallback** — the org/repo passed to `WithFallback(...)` / `--fallback`. The only tier the user must explicitly configure.

```
         project file         (substantive body)
                ↓ miss
         from: owner/repo     (org-header)
                ↓ miss
         GITHUB_REPOSITORY    ┐ auto-infer: best-effort,
         or git remote        ┘   silent on any failure
                ↓ miss
         WithFallback         (configured, explicit)
                ↓ miss
         "no standards found" error
```

## External dependencies

The project is **stdlib-only** for everything except filesystem
watching. `github.com/fsnotify/fsnotify` is the one and only external
dep, and is added deliberately: file-watching has no stdlib
equivalent in Go. The "stdlib only" invariant in
`.brain/project-profile.md` and the AGENTS.md stack note is preserved
in spirit — there is still no CLI framework, no HTTP client library,
no Viper/Cobra; fsnotify is the canonical Go file-watcher and
filling that gap with stdlib polling would be a half-measure, not
an honest implementation.

When the LLM client lands, it will use `net/http` (stdlib) and
`go.sum` will not need to grow. If a future feature genuinely
requires a new external dep, add it to this section and document
the why.

## Patterns used

- **Hexagonal-lite**: the `llm.Provider` interface isolates the LLM behind a port; standards loading is also behind the `Resolver` type; filesystem watching is behind the `watch.Watcher` interface. The audit runner is the application core.
- **Stdlib-only, single binary** (one exception: fsnotify). No Cobra, no Viper, no HTTP client dep. A deliberate "boring tools" choice.
- **Markdown as rubric**: standards have no schema; the LLM is trusted to interpret them. There is no separate rule registry on disk.
- **Tiered resolution with fall-through**: `project > org-header > auto-infer > fallback` is a fixed ordering. Each tier is implemented; auto-infer is best-effort and never surfaces errors.
- **GitHub `.github` convention, ported to archon**: every GitHub org can publish a `.archon` repo whose `.archon/standards.md` all of its services inherit automatically. Tier 3 makes "clone → `archon audit`" work with no config step beyond the org-level repo.
- **Pipeline runner**: `audit.Runner.Run` is a small three-step pipeline (Resolve → Audit → Report).
- **Watch as plumbing**: the `watch.Watcher` emits a stream of `Event` values; the cmd/archon event loop subscribes and decides what to do with each one. The watcher is testable in isolation (no audit/llm dependencies) and the cmd/archon dispatch is testable in isolation (no fsnotify dependency). The seam is the `Watcher` interface.
- **Problem-matcher output**: the `archon watch` output is the
  GCC/clippy/quickfix/problem-matcher format
  `path:line:col: [severity] message`, one violation per line, so
  editor quickfix / VS Code "Error Lens" can parse it natively.
  This is the contract; LSP JSON-RPC is a separate, later ticket.

## Key design decisions

1. **Single static binary, one external dep.** `github.com/fsnotify/fsnotify` is the only non-stdlib dep, added because file-watching has no stdlib equivalent. Everything else (HTTP, CLI, config) remains stdlib.
2. **Deterministic rule engine, markdown-based rules.** No LLM dependency for core audit. Rules are markdown files with YAML frontmatter, organized in folders by category. The engine uses pattern matching and Go AST analysis to find violations deterministically.
3. **LLM as optional enhancement.** The LLM provider exists for `archon explain` (reasoning, examples) but is not required for `archon audit`. The core loop is deterministic.
4. **Four-tier resolution, locked-in order.** `project > org-header > auto-infer > fallback`. The `Resolver` API bakes in that order; replacing it would mean rewriting callers. Tier 3 is intentionally best-effort — a missing auto-infer target is a non-event, not an error.
5. **Violations carry source coordinates.** `Violation.File`, `Violation.Line`, `Violation.Column` are zero-valued when the provider can't locate the code; `String()` renders them as `?` for editor problem-matcher compatibility.
6. **Watch is plumbing-only until the LLM client lands.** The watch loop logs the LLM stub error on every change and continues. The loop is end-to-end real (fsnotify, debounce, classify, dispatch, format) so the moment the LLM client ships, `archon watch` produces real diagnostics with no further plumbing changes.
7. **Context with signal handling in main.** `signal.NotifyContext` propagates cancellation to the runner and provider; the watch loop's `ctx.Done()` triggers an `fsnotify.Watcher.Close()` and the loop returns.
8. **Blueprint-managed CI + drift-check.** `setup.bp` renders the GitHub Actions matrix; `elpic/actions/github/drift-check@v2` is the only guard against hand-edits.
9. **mise tasks as the CI interface.** CI calls `mise run test:coverage`, `lint`, `build`, `security`, `test:integration` — not raw `go test`.
10. **Trunk-based git, documented not enforced.** No branch-protection automation; relies on convention.

## Locked-in interfaces

```go
// internal/llm
type Provider interface {
    Audit(ctx context.Context, standardsBody []byte, target string) ([]Violation, error)
}
type Violation struct {
    Rule, Description string
    Severity          Severity
    File              string // empty when the provider can't locate the code
    Line              int    // 1-based; 0 when unknown
    Column            int    // 1-based; 0 when unknown
}
type Severity int // Info, Warn, Error, Critical
func (v Violation) String() string // problem-matcher format

// internal/standards
type Document struct { Source string; Body []byte }
type Resolver struct { fallbackOrgRepo string; fetcher Fetcher }
type Fetcher interface {
    Fetch(ctx context.Context, owner, repo, path string) (body []byte, sha string, err error)
}
// NewResolver(workdir, WithFallback("o/r"), WithFetcher(f)) (*Resolver, error)
// Resolve walks: project → org-header → auto-infer → fallback.
// Tier 3 (auto-infer) is best-effort: any failure falls through silently.

// internal/audit
type Report struct {
    Target          string
    Violations      []llm.Violation
    StandardsSource string // populated from resolved Document.Source
}
func (r *Report) Format() string            // terminal layout for `archon audit`
func (r *Report) FormatDiagnostic() string  // problem-matcher stream for `archon watch`

// internal/watch
type Watcher interface {
    Watch(ctx context.Context, target string) (<-chan Event, error)
}
type Event struct {
    Path            string
    Kind            EventKind // Changed, StandardsChanged, Error
    StandardsSource string    // populated on StandardsChanged
    Err             error     // populated on Error
}
```

## What's missing / incomplete

- `internal/rules/` is empty — this is now the **core** package (ticket #010)
- Rule engine needs: markdown parser, frontmatter extraction, pattern matchers, Go AST analysis
- No `.rules/` directory structure yet (categories, rule files)
- LLM provider exists but is optional (for `archon explain` only)
- Report is terminal-only: no JSON, no SARIF, no exit code (problem-matcher text is the only structured stream today)
- Docs disagree: `AGENTS.md` says Go 1.23+ but `go.mod` is 1.25; README/AGENTS.md/`.archon/standards.md` disagree on the default LLM provider and the org standards path.
- `go.sum` exists now (one external dep, fsnotify); govulncheck is no longer a no-op.
- Dogfooding is partial: `archon init`, the resolver, and `archon watch` work end-to-end, but `archon audit` and `archon watch`'s audit call still halt at the LLM stub.

The skeleton is now mostly fleshed out for the standards half AND the
inner loop. The remaining work is the rule engine (ticket #010) and
migrating from LLM-based audit to deterministic rules.
