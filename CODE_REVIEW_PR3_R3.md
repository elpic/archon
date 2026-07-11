## Code Review: PR #3 — The Inner Loop (archon watch + clickable diagnostics)
**Reviewer:** @code-reviewer  
**Date:** 2026-07-11  
**Status:** **APPROVED WITH NITS** — All previous approvals hold; only minor nits in latest commit

---

### Summary
PR #3 implements ticket #001 "The Inner Loop" — `archon watch` subcommand with fsnotify-based file watching (500ms debounce), Violation coordinates (File/Line/Column), and problem-matcher format output for editor integration.

**Commits reviewed:**
- `4b89c2d` — Original feature implementation
- `a10ee74` — Fixes data races in tests and watcher channel close (round 3 fix)

All tests pass with `-race -count=3`. The watch command runs correctly and shows expected stub behavior.

---

### Files Reviewed

| File | Status | Notes |
|------|--------|-------|
| `internal/watch/watch.go` | ✅ Approved | Core watcher logic; race fix in `a10ee74` |
| `internal/watch/watch_test.go` | ✅ Approved | Unit tests for debouncing, standards change, SIGINT |
| `cmd/archon/main.go` | ✅ Approved | `watch` subcommand, signal handling, stub provider |
| `cmd/archon/watch_test.go` | ⚠️ Nits | Integration tests; flaky sleeps/polling |
| `internal/llm/llm.go` | ✅ Approved | Violation with File/Line/Column + problem-matcher `String()` |
| `internal/audit/report.go` | ✅ Approved | `FormatDiagnostic()` for problem-matcher output |
| `internal/audit/report_test.go` | ✅ Approved | Tests for diagnostic formatting |
| `internal/llm/llm_test.go` | ✅ Approved | Tests for Violation.String() |

---

### Findings

#### 🟡 Important (Should Fix)

**1. Duplicate comment block in `internal/watch/watch.go:282-292`**
```go
// isStandardsPath reports whether p is the project's standards file,
// (DUPLICATE COMMENT BLOCK HERE)

// isStandardsPath reports whether p is the project's standards file,
```
**Suggestion:** Remove lines 282-284 (the first duplicate comment block).

---

**2. Flaky integration tests in `cmd/archon/watch_test.go`**
Tests use polling loops with `time.Sleep(50*ms)` and 1.5s deadlines:
```go
// Lines 113-119, 175-181, 232-239
deadline := time.Now().Add(1500 * time.Millisecond)
for time.Now().Before(deadline) {
    if strings.Contains(stdout.String(), "[error]") { break }
    time.Sleep(50 * time.Millisecond)
}
```
**Problem:** Flaky in slow CI (heavy load, macOS runners).  
**Suggestion:** Replace with channel-based synchronization or `sync.WaitGroup` like the unit tests do. Or at minimum, increase deadline to 5s and sleep to 100ms for CI stability.

---

**3. Unit tests use short sleeps in `internal/watch/watch_test.go`**
```go
// Line 41, 80
w := &FSNotifyWatcher{DebounceWindow: 50 * time.Millisecond}
```
**Problem:** 50ms debounce + fsnotify delivery latency can exceed 1s timeout on loaded CI.  
**Suggestion:** Increase `DebounceWindow` to 100ms in tests, or use a `sync.Cond`/`channel` to signal when debounce fires (like the production code does with `time.AfterFunc`).

---

#### 🔵 Minor (Nice to Have)

**4. Missing test: `StandardsChanged` debouncing**
`TestWatcher_Debounce` (line 88) only tests `Changed` events. Add a test verifying `.archon/standards.md` rapid writes coalesce into a single `StandardsChanged` event.

**5. Missing test: Error channel close handling**
The watcher handles `fw.Errors` channel close (line 260-264) but no test covers this path. Add a test that triggers an fsnotify error and verifies clean shutdown.

**6. Magic number: `defaultDebounce = 500 * time.Millisecond`**
Add a comment explaining the rationale (editor save bursts, fsnotify event coalescing on macOS/Linux).

**7. `debounce()` method could be inlined**
Called only once in `loop()`. Inlining improves readability:
```go
debounce := w.DebounceWindow
if debounce == 0 { debounce = defaultDebounce }
```

---

### Security Check
- ✅ No hardcoded secrets
- ✅ Input validation on `target` path (absolute, directory check)
- ✅ No obvious injection vulnerabilities
- ✅ fsnotify is a well-maintained stdlib-adjacent dependency
- ❌ **Refer to @security-reviewer:** No — no new attack surface beyond fsnotify (already approved)

---

### Test Coverage
| Area | Status |
|------|--------|
| Unit tests (watch.go) | ✅ Comprehensive: debounce, standards change, SIGINT, error handling |
| Integration tests (watch_test.go) | ⚠️ Flaky but functional; see nits above |
| Violation formatting | ✅ Unit tests for `String()` and `FormatDiagnostic()` |
| Race detection | ✅ All tests pass `-race -count=3` |

---

### Previous Round Status
| Round | Code Review | QA | Security | DevOps |
|-------|-------------|-----|----------|--------|
| 1 | ✅ Approved | ❌ Changes Requested | ❌ Changes Requested | ✅ Approved |
| 2 | ✅ Approved | ✅ Approved | ✅ Approved | ✅ Approved |
| 3 (this) | ✅ **Approved with Nits** | — | — | — |

**All previous approvals hold.** Commit `a10ee74` fixed the data races that would have blocked QA/Security.

---

### Decision
- [ ] **Approved** — Ready to merge
- [x] **Approved with nits** — Minor issues, can merge (nits are non-blocking)
- [ ] **Changes requested** — Address findings before merge
- [ ] **Needs discussion** — Requires conversation

---

### Next Steps
1. Merge when ready (nits are non-blocking)
2. Notify @integration-verifier to verify GitHub CI checks pass
3. Consider addressing nits in follow-up commits for CI stability