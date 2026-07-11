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
        +----------------------+----------------------+
        v                      v                      v
+---------------+   +-----------------+   +-------------------+
| internal/     |   | internal/llm    |   | internal/audit    |
| standards     |   | (Provider iface |   | (Runner +         |
| (Resolver,    |   |  + stub New())  |   |  Report.Format)   |
|  Fetcher,     |   +-----------------+   +-------------------+
|  Document)    |            ^                    ^   ^
+-------+-------+            |                    |   |
        |                    |                    |   |
        v                    |                    |   |
./.archon/standards.md       |                    |   |
        |                    |                    |   |
        |  audit.Runner.Run(ctx, target)          |   |
        |  1. resolver.Resolve() -> *Document     |   |
         |     (project | org-header |              |   |
         |      auto-infer | fallback)             |   |
        |  2. provider.Audit(ctx, doc.Body, t)     |   |
        |     -> []Violation                      |   |
        |  3. Report{Target, Violations,           |   |
        |         StandardsSource=doc.Source}     |   |
        |     -> Report.Format() -> string        |   |
        +----------------------------------------+   |
                                |                       |
                                +-----------------------+
```

## Data flow

1. `main.go` dispatches on `os.Args[1]`: `audit`, `init`, or `help`. Each
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
3. For `archon init`:
   1. `archon init` (no flag) writes a `.archon/standards.md` scaffold
      with a comment header and a `# Project Standards` body — the file
      is tier 1 (project-local source).
   2. `archon init --from owner/repo` writes a redirect-only comment
      block containing the `from:` directive. The resolver sees a
      redirect-only file, misses tier 1, and fetches the org source at
      audit time.
   3. Both forms are idempotent: if `.archon/standards.md` already
      exists, the call is a no-op.

## Component responsibilities

- **cmd/archon** — process entry, subcommand dispatch (`audit`, `init`, `help`), per-subcommand flag parsing via the stdlib `flag` package, signal handling, dependency wiring.
- **internal/standards** — find and load the standards document. `Resolver` holds a `fallbackOrgRepo` (now wired via `WithFallback`) and a `Fetcher` (now wired via `WithFetcher`, defaulting to `httpFetcher` hitting the GitHub Contents API). `fromProject` walks the target dir looking for `.archon/standards.md`. A new `fromOrgHeader` reads a `from: owner/repo` line from the project file; if the project file is *only* a redirect comment, tier 1 misses and the org source is fetched. Tier 3 (auto-infer) lives in `autoinfer.go` and derives the org from `GITHUB_REPOSITORY` or `git remote get-url origin`; the fetch is best-effort and any miss falls through.
- **internal/llm** — defines the contract between archon and any LLM provider. The `Provider` interface and data types are stable; only `New()` remains unimplemented.
- **internal/audit** — orchestrates the pipeline and owns the report shape. `Report` carries `StandardsSource` (set from the resolved `Document.Source`) so the user can see which tier was used. `Report.Format()` prepends a `Standards: <source>` line when the source is non-empty. Still plain string concatenation; no JSON, no SARIF, no file/line, no exit code.
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

## Patterns used

- **Hexagonal-lite**: the `llm.Provider` interface isolates the LLM behind a port; standards loading is also behind the `Resolver` type. The audit runner is the application core.
- **Stdlib-only, single binary**: no Cobra, no Viper, no HTTP client dep. A deliberate "boring tools" choice.
- **Markdown as rubric**: standards have no schema; the LLM is trusted to interpret them. There is no separate rule registry on disk.
- **Tiered resolution with fall-through**: `project > org-header > auto-infer > fallback` is a fixed ordering. Each tier is implemented; auto-infer is best-effort and never surfaces errors.
- **GitHub `.github` convention, ported to archon**: every GitHub org can publish a `.archon` repo whose `.archon/standards.md` all of its services inherit automatically. Tier 3 makes "clone → `archon audit`" work with no config step beyond the org-level repo.
- **Pipeline runner**: `audit.Runner.Run` is a small three-step pipeline (Resolve → Audit → Report).

## Key design decisions

1. **Single static binary, stdlib only.** No CLI framework, no HTTP client library. Forces everything to live in `net/http` once the LLM client lands.
2. **Markdown standards, LLM as judge.** No rule schema; the model interprets the document. This collapses what might have been a rule engine into a single LLM call.
3. **Four-tier resolution, locked-in order.** `project > org-header > auto-infer > fallback`. The `Resolver` API bakes in that order; replacing it would mean rewriting callers. Tier 3 is intentionally best-effort — a missing auto-infer target is a non-event, not an error.
4. **Flat violation list, no file:line.** `Violation` carries `Rule` and `Description` only. No source location, so downstream tooling cannot jump to the offending code.
5. **Context with signal handling in main.** `signal.NotifyContext` propagates cancellation to the runner and provider.
6. **Blueprint-managed CI + drift-check.** `setup.bp` renders the GitHub Actions matrix; `elpic/actions/github/drift-check@v2` is the only guard against hand-edits.
7. **mise tasks as the CI interface.** CI calls `mise run test:coverage`, `lint`, `build`, `security`, `test:integration` — not raw `go test`.
8. **Trunk-based git, documented not enforced.** No branch-protection automation; relies on convention.

## Locked-in interfaces

```go
// internal/llm
type Provider interface {
    Audit(ctx context.Context, standardsBody []byte, target string) ([]Violation, error)
}
type Violation struct { Rule, Description string; Severity Severity }
type Severity int // Info, Warn, Error, Critical

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
func (r Report) Format() string
```

## What's missing / incomplete

- `llm.New()` returns `"llm.New not yet implemented"` — the entire AI half is a stub.
- `internal/rules/` is empty; the rules/standards split is an unresolved architectural call.
- Report is terminal-only: no JSON, no SARIF, no file/line, no exit code.
- Docs disagree: `AGENTS.md` says Go 1.23+ but `go.mod` is 1.25; README/AGENTS.md/`.archon/standards.md` disagree on the default LLM provider and the org standards path.
- `go.sum` does not exist (no deps), so the security job is a no-op.
- Dogfooding is partial: `archon init` and the resolver work end-to-end, but `archon audit` still halts at the LLM stub.

The skeleton is now mostly fleshed out for the standards half. The remaining
work is the LLM client (one HTTP client + a Provider impl) and a decision
on whether `internal/rules/` survives.
