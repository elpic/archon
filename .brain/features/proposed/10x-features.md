# archon — 10x Feature Proposals

> Strategic feature proposals to turn archon from "yet another linter" into
> "the way teams align on standards." Sorted by priority.

## Framing

The skeleton is well-shaped but the *product* doesn't exist yet. The
differentiator archon has that nobody else does is **tiered standards
resolution** — org-level inheritance is archon's structural moat. Every
proposal below leans on that moat. Table-stakes (working LLM client, CI
formats, flags) are bundled into one foundation feature rather than spread
across seven.

---

## P1 — The Wedge

### 1. "Adopt in 10 Seconds" — `archon init` + org inheritance

**Problem.** Every new Go repo at a company re-litigates the same
"what's our error-handling style / module layout / naming" debate. There's
no ceremony to inherit team standards, so they get copied-pasted into
README and drift within a quarter.

**Why now / why it matters.** In 2026, "team alignment" is a
multi-million-hour/year tax. The org-inheritance pattern is archon's
unique structural advantage — no other tool has it. `archon init` is the
conversion mechanism: a staff engineer writes standards once in the org
repo, every new service inherits on day one. This is the feature that
turns archon from a per-project tool into a *company* tool. The
flywheel: more orgs use it → more standards ecosystems → more adoption.

**Effort:** M (1-3 days) **Impact:** HIGH

**Acceptance criteria**
- [ ] `archon init` scaffolds `.archon/standards.md` in the target project,
      pre-populated with a comment header pointing to the org-level source
- [ ] Resolver correctly implements all three tiers: project →
      `gh:<owner>/<repo>/.archon/standards.md` → `--fallback`
- [ ] `archon init --from elpic/go-standards` creates a project that
      inherits elpic/go-standards via GitHub fetch (with a real
      network call, mocked in tests)
- [ ] Resolved source is visible in `archon audit` output ("Standards:
      github.com/elpic/go-standards@abc123") — inheritance is observable,
      not invisible
- [ ] `archon init` is idempotent: running twice does not clobber an
      existing project standards file

---

## P2 — The Daily Tool

### 2. "The Inner Loop" — `archon watch` + clickable diagnostics

**Problem.** A linter that only runs in CI is forgotten. A linter that
runs on every save becomes a habit. golangci-lint won the Go world partly
because `golangci-lint run` is fast enough to wire into file watchers.
Archon's first release can't do that — it shells out to an LLM. We need
a watch mode that's smart about *when* to call the LLM.

**Why now / why it matters.** "Daily tool vs one-time install" is the
single biggest retention predictor. Watch mode also creates a unique
product signature: archon watches *the standards document itself* and
re-audits when org rules change — something no static analyzer can do.
This is the "from CI job to daily companion" pivot.

**Effort:** M **Impact:** HIGH

**Acceptance criteria**
- [ ] `archon watch` uses fsnotify to watch the target project (debounced,
      500ms)
- [ ] On file change: incrementally re-audit only the changed file (no
      full project re-scan) and print clickable `path:line:col: [severity] message`
- [ ] On `.archon/standards.md` change: re-resolve + re-audit everything
      and print "Standards updated from <source>; re-running audit"
- [ ] Output format follows the LSP/diagnostic convention so editors
      (Neovim, VS Code, Helix) parse it natively
- [ ] Graceful shutdown on SIGINT (in-flight LLM call cancelled via
      the existing `signal.NotifyContext`)

---

## P3 — The Trust Builder

### 3. "Audit That Teaches" — source locations, quick-fixes, `archon explain`

**Problem.** Today `Violation` is `{Rule, Description, Severity}` — no
file, no line, no fix suggestion. An engineer who sees "violates error
wrapping rule" has no way to act on it. LLM-as-judge tools fail when
they're inscrutable: teams stop trusting them within a sprint.

**Why now / why it matters.** The judge-vs-teacher split is the
difference between a tool teams *use* and a tool teams *argue about in
retro*. "Your code does X on line 42; here's a one-line fix; here's the
org's reasoning" turns a scolding AI into a teaching AI. Also unlocks
`archon explain` as a *standalone learning surface* — a developer can
ask "what does our org say about context propagation?" and get a
markdown explanation grounded in the actual standards doc.

**Effort:** M-L (touches the `Provider` interface, requires a prompt
redesign) **Impact:** HIGH

**Acceptance criteria**
- [ ] `llm.Violation` grows `File`, `Line`, `Column`, `Suggestion string`,
      and `RuleDoc string` (anchor into the standards doc). Backwards-compat
      shim: missing fields render as "?"
- [ ] Prompt is structured (JSON schema) so the LLM *must* return
      `file:line:col` or the violation is rejected as malformed
- [ ] `archon audit --fix` (off by default) prints the suggested fix in
      unified diff format
- [ ] `archon explain <rule-id>` prints: the rule text, the LLM's
      reasoning, 2-3 examples from the codebase, and a one-line "fix it
      with:" suggestion
- [ ] `explain` is usable standalone (no audit required) — it just
      resolves standards and answers the question

---

## P4 — The 2026 Distribution

### 4. "Archon Speaks MCP" — first-class MCP server

**Problem.** In 2026, every developer has an AI agent in their editor
(Claude Code, Cursor, Continue, Zed). If archon is not in that agent's
toolbelt, it doesn't exist for the largest growing segment of users.
`npm install` lost to `pnpm dlx` and the like for the same reason: meet
developers where they are.

**Why now / why it matters.** MCP is the lingua franca of agentic tools
in 2026. An `archon mcp` subcommand that exposes `audit_file`,
`resolve_standards`, `explain_rule`, and `get_violations` as MCP tools
means every agent session can run archon audits conversationally. "Hey
Cursor, run archon on the file I just edited" becomes one prompt. This
is the land-and-expand distribution play that defines the next 18 months
of devtools.

**Effort:** M (use `modelcontextprotocol/go-sdk`; bends the "stdlib
only" rule once, and it's worth it) **Impact:** HIGH

**Acceptance criteria**
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

---

## P5 — The CI Economics

### 5. "Diff-Aware Audits" — `--changed` and friends

**Problem.** LLM calls cost money and take seconds. Auditing an entire
microservice on every PR is a 10x cost vs. auditing only the files that
changed. No team will wire archon into CI if a single PR burns $2 and
adds 90s to the queue.

**Why now / why it matters.** This is the feature that decides whether
archon lives in CI or gets shelved after the first invoice. It's also
the feature that makes archon a *green* tool — being faster than the
human review it augments is non-negotiable for adoption.

**Effort:** S (auto-detect from `GITHUB_BASE_REF` / `CI_MERGE_REQUEST_TARGET_BRANCH_NAME`) **Impact:** HIGH

**Acceptance criteria**
- [ ] `archon audit --changed` audits only files in `git diff --name-only HEAD~1`
- [ ] `archon audit --since <ref>` audits files changed between `<ref>` and HEAD
- [ ] Auto-detection: in GitHub Actions, defaults to
      `${{ github.event.pull_request.base.sha }}` if no flag given
- [ ] `--changed` reports a cost summary at the end: "Audited 4 files
      (skipped 238); estimated cost $0.03"
- [ ] When `--changed` is combined with `--watch`, only the changed file
      is re-audited (no project-wide blast radius)

---

## P6 — The Distribution Plumbing

### 6. "Ship-Ready CI" — SARIF, exit codes, GitHub Action, pre-commit

**Problem.** No flags, no JSON, no SARIF, no exit code, no GitHub
Action. The binary can't actually be used in any real CI system. The
README advertises `--fallback` which doesn't exist. This is the table
stakes for any code-scanning tool in 2026.

**Why now / why it matters.** Distribution *is* the product for
devtools. A tool that can't be wired into GitHub code scanning, GitLab
CI, pre-commit, or lefthook is a tool that doesn't exist. Bundling this
as a single P6 feature (rather than a chore) frames it correctly: the
*output surface* is the product as much as the audit logic is.

**Effort:** M **Impact:** HIGH

**Acceptance criteria**
- [ ] `--format terminal|json|sarif` flag on `archon audit`; default
      is `terminal`, `--format sarif` writes SARIF 2.1.0 to stdout
- [ ] Exit codes: `0` clean, `1` violations found, `2` error (config,
      LLM failure, network). Documented in `archon audit --help`
- [ ] `--fallback <org/repo>` flag actually plumbs through to
      `standards.NewResolver` (fixes the dead field today)
- [ ] `.github/action.yml` for `archon-action@v0`: inputs are `path`,
      `fallback`, `openai-api-key`; output is `report.sarif` uploaded
      via `github/codeql-action/upload-sarif`
- [ ] `archon install pre-commit` writes a `.pre-commit-config.yaml`
      fragment; `archon install lefthook` does the same
- [ ] `--explain` flag on `archon audit` prints a one-line "Run
      `archon explain <rule>` to learn more" footer under each violation

---

## P7 — The Moat

### 7. "Standards as Code" — diff, log, validate, share

**Problem.** Today standards.md is a markdown blob the LLM interprets.
There's no way to see *what* changed, *when* it changed, or whether two
projects in the same org are inheriting different things. Once `archon
init` exists, the next question is "where do the standards come from?"
— and the answer should be a discoverable ecosystem.

**Why now / why it matters.** This is the longest-term moat and the
network-effect play. Once teams can `archon standards add acme/go-strict`
or `archon standards share .`, the standards themselves become
shareable artifacts. The LLM-judge design *requires* high-quality
standards docs to be useful, and a marketplace solves that. It's also
the feature that makes standards drift *visible* — when the org
changes a rule, every inheriting project sees the diff in `archon
standards log`, the same way engineers see `git log` for code.

**Effort:** L (registry API + CLI + GitHub-backed cache) **Impact:** MEDIUM (high long-term)

**Acceptance criteria**
- [ ] `archon standards resolve` prints the resolved standards
      document, the resolution chain (project → org → fallback), and
      the source commit SHA
- [ ] `archon standards diff <ref>` shows the unified diff between
      the currently-resolved standards and the version at `<ref>` (a
      git ref, a tag, or "last-week")
- [ ] `archon standards log` prints a chronological list of changes
      to the resolved standards source (uses GitHub's commits API)
- [ ] `archon standards validate` runs a meta-audit: checks the
      standards doc for ambiguity, contradictions, and unfalsifiable
      rules. Catches the "rule that says 'write good code'" failure mode.
- [ ] `archon standards add <org/repo>` adds a remote fallback to
      `.archon/config.yaml`; the file format is human-editable TOML
- [ ] A `archon-stdlib` org repo (curated by us) ships 3 starter
      standards: `strict`, `default`, `minimal` — same shape as
      `eslint-config-*` in the JS world

---

## What I deliberately cut

- **"Implement the LLM provider"** — chore, not a feature. Bundled into
  the foundation of feature #6 ("Ship-Ready CI") because the moment you
  ship SARIF you need a working LLM client behind it.
- **"Add caching of resolved standards"** — implicit in #1 and #7; not
  a user-visible feature.
- **"Multi-language / polyglot standards"** — distraction. Go is the
  beachhead. JS/TS/Python come in v2 once the Go version has network
  effects.
- **"Slack/Discord notifications"** — the teams that need this will
  build it on top of SARIF + GitHub Action outputs. We shouldn't.
- **"Custom rule DSL"** — anti-feature. The whole point of archon is
  that *the LLM reads prose standards*. A DSL is a step backward.

## Sequencing rationale

P1 (Adopt) and P6 (Ship-Ready CI) are co-blocking for any real adoption.
But Adopt is the strategic wedge; CI plumbing is table stakes. We
sequence Adopt first because the moment a team runs `archon init`, they
discover they can't yet use it in CI — which is the strongest possible
signal to ship #6 next.

P2 (Watch) and P3 (Teach) are co-blocking for the daily-tool positioning.
Watch is bigger for retention; Teach is bigger for trust. Ship Watch
first (it's the visible "I use this every day" signal), Teach second
(the deepening).

P4 (MCP) is a parallelizable distribution bet — it can land whenever
the audit pipeline stabilizes. P5 (Diff-Aware) is the unlock for CI
adoption, ship as soon as #6 lands. P7 (Standards as Code) is the
long-term moat; it compounds but isn't blocking adoption.
