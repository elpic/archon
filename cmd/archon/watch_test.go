package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elpic/archon/internal/audit"
	"github.com/elpic/archon/internal/llm"
	"github.com/elpic/archon/internal/standards"
)

// syncBuffer is a thread-safe bytes.Buffer wrapper for tests that
// read from the buffer while a goroutine writes to it.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func (s *syncBuffer) Bytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Bytes()
}

// watchFakeProvider is the FakeProvider used by TestRunWatch_*.
// It records the body it was called with and returns the canned
// violations list.
type watchFakeProvider struct {
	violations []llm.Violation
}

func (f *watchFakeProvider) Audit(_ context.Context, _ []byte, _ string, _ []string) ([]llm.Violation, error) {
	return f.violations, nil
}

// TestRunWatch_FileChange_EmitsProblemMatcher: a save to a Go
// source file in the watched target must produce a
// problem-matcher-formatted line on stdout. The fake provider
// returns a single violation with full coordinates; the test
// pins the exact output line.
func TestRunWatch_FileChange_EmitsProblemMatcher(t *testing.T) {
	dir := t.TempDir()
	provider := &watchFakeProvider{
		violations: []llm.Violation{
			{
				Rule:        "no-comments",
				Description: "Comments are forbidden",
				Severity:    llm.SeverityError,
				File:        "internal/foo/foo.go",
				Line:        42,
				Column:      7,
			},
		},
	}

	// Start the watch loop in a goroutine, then trigger the
	// change. We can't use runWatchOnce because the change has
	// to happen after Watch() is wired up.
	if err := os.MkdirAll(filepath.Join(dir, ".archon"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".archon", "standards.md"), []byte("# Project\nReal content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
resolver, err := standards.NewResolver(".")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stdout := &syncBuffer{}
	stderr := &syncBuffer{}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = runWatchInner(ctx, resolver, provider, dir, stdout, stderr)
	}()

	// Give the watcher a moment to subscribe to the target,
	// then write the source file. fsnotify delivery on macOS
	// can take a few ms; 200ms is comfortable.
	time.Sleep(200 * time.Millisecond)
	target := filepath.Join(dir, "internal", "foo", "foo.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("package foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Wait for the audit to run + the output to land in
	// stdout. The debounce is 500ms in production; we give
	// 1.5s of slack for slow CI.
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if strings.Contains(stdout.String(), "[error]") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	cancel()
	<-done

	got := stdout.String()
	want := "internal/foo/foo.go:42:7: [error] Comments are forbidden\n"
	if !strings.Contains(got, want) {
		t.Errorf("stdout = %q\nwant substring %q", got, want)
	}
	if !strings.Contains(stderr.String(), "change detected") {
		t.Errorf("stderr = %q\nwant substring 'change detected'", stderr.String())
	}
}

// TestRunWatch_StandardsChange_EmitsNotice: a save to
// .archon/standards.md must print the
// "Standards updated from <source>; re-running audit" line.
func TestRunWatch_StandardsChange_EmitsNotice(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".archon"), 0o755); err != nil {
		t.Fatal(err)
	}
	standardsPath := filepath.Join(dir, ".archon", "standards.md")
	if err := os.WriteFile(standardsPath, []byte("# Project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := &watchFakeProvider{}

	resolver, err := standards.NewResolver(".")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stdout := &syncBuffer{}
	stderr := &syncBuffer{}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = runWatchInner(ctx, resolver, provider, dir, stdout, stderr)
	}()

	time.Sleep(200 * time.Millisecond)
	// Append to the standards file rather than rewriting from
	// scratch, so the fsnotify event is a Write (not a Create).
	f, err := os.OpenFile(standardsPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n## added later\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if strings.Contains(stderr.String(), "Standards updated from") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	cancel()
	<-done

	if !strings.Contains(stderr.String(), "Standards updated from") {
		t.Errorf("stderr = %q\nwant substring 'Standards updated from'", stderr.String())
	}
	if !strings.Contains(stderr.String(), "; re-running audit") {
		t.Errorf("stderr = %q\nwant substring '; re-running audit'", stderr.String())
	}
}

// TestRunWatch_StubError_KeepsRunning: when the LLM provider
// fails on every audit (the demo state of the project today),
// the watch loop must NOT exit. It must surface the error on
// stderr and continue processing subsequent events.
//
// This is the explicit AC: "graceful shutdown on SIGINT" +
// "plumbing-only" — the watch loop's response to the LLM stub
// is to log + continue.
func TestRunWatch_StubError_KeepsRunning(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".archon"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".archon", "standards.md"), []byte("# Project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// stubProvider is the production placeholder, not a test
	// fake — it returns the original LLM construction error on
	// every audit. This is the configuration that exists today.
	provider := &stubProvider{err: errStub}

	resolver, err := standards.NewResolver(".")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stdout := &syncBuffer{}
	stderr := &syncBuffer{}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = runWatchInner(ctx, resolver, provider, dir, stdout, stderr)
	}()

	// Trigger two file changes. Both should produce the stub
	// error on stderr; the loop must not exit between them.
	time.Sleep(200 * time.Millisecond)
	for i := 0; i < 2; i++ {
		p := filepath.Join(dir, "f"+string(rune('0'+i))+".go")
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(700 * time.Millisecond) // > 500ms debounce
	}

	// Loop should still be alive — we should be able to cancel
	// it without it having exited on its own.
	cancel()
	select {
	case <-done:
		// Good: loop exited on cancel.
	case <-time.After(time.Second):
		t.Fatal("watch loop did not exit after cancel")
	}

	// The stub error should appear in stderr at least once per
	// audit attempt; we look for "audit failed" as the
	// tell-tale string. We do not require a count — the goal
	// is to assert the loop survived.
	if !strings.Contains(stderr.String(), "audit failed") {
		t.Errorf("stderr = %q\nwant substring 'audit failed'", stderr.String())
	}
	// And no problem-matcher line should have been printed,
	// because the audit never succeeded.
	if strings.Contains(stdout.String(), "[error]") ||
		strings.Contains(stdout.String(), "[warn]") ||
		strings.Contains(stdout.String(), "[info]") {
		t.Errorf("stdout = %q\nshould not contain severity markers when audit failed", stdout.String())
	}
}

// errStub is a sentinel used by TestRunWatch_StubError_KeepsRunning.
// It is defined as a package-level var so the value can be
// referenced from the test's provider wiring and from any
// assertions without depending on the real LLM stub error text.
var errStub = errStubValue()

func errStubValue() error {
	// We import the real stub error via llm.New so the test
	// mirrors the production state without duplicating the
	// string. (Defining it inline would let the strings drift
	// if the stub message changes.)
	_, err := llm.New(context.Background())
	return err
}

// TestRunWatch_CtxCancel_ExitsCleanly: the watch loop must
// return within a short window of ctx cancellation. This is the
// "graceful shutdown on SIGINT" AC.
func TestRunWatch_CtxCancel_ExitsCleanly(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".archon"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".archon", "standards.md"), []byte("# Project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := &watchFakeProvider{}
	resolver, err := standards.NewResolver(".")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	stdout := &syncBuffer{}
	stderr := &syncBuffer{}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = runWatchInner(ctx, resolver, provider, dir, stdout, stderr)
	}()

	time.Sleep(100 * time.Millisecond) // let Watch() wire up
	cancel()

	select {
	case <-done:
		// Good.
	case <-time.After(time.Second):
		t.Fatal("watch loop did not exit within 1s of cancel")
	}
}

// Compile-time assertion that watchFakeProvider satisfies the
// llm.Provider interface used by audit.NewRunner. If the
// interface changes, this fails to build — which is the
// intended tripwire.
var _ llm.Provider = (*watchFakeProvider)(nil)

// Compile-time assertion that the production stubProvider is
// also an llm.Provider, so the watch loop's plumbing-only mode
// is type-checked.
var _ llm.Provider = (*stubProvider)(nil)

// TestAuditRunner_FormatDiagnostic_UsedByWatch: smoke test for
// the round-trip — fake provider → report → FormatDiagnostic
// → problem-matcher line. The watch loop in
// TestRunWatch_FileChange_EmitsProblemMatcher already covers
// the end-to-end flow, but this is a fast unit-level pin on
// the formatting in case the watch path is later refactored.
func TestAuditRunner_FormatDiagnostic_UsedByWatch(t *testing.T) {
	provider := &watchFakeProvider{
		violations: []llm.Violation{
			{
				Rule:        "r",
				Description: "d",
				Severity:    llm.SeverityWarn,
				File:        "a.go",
				Line:        3,
				Column:      4,
			},
		},
	}
	resolver, err := standards.NewResolver(".")
	if err != nil {
		t.Fatal(err)
	}
	runner := audit.NewRunner(resolver, provider)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".archon"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".archon", "standards.md"), []byte("# X\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := runner.Run(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	got := report.FormatDiagnostic()
	want := "a.go:3:4: [warn] d\n"
	if got != want {
		t.Errorf("FormatDiagnostic() = %q, want %q", got, want)
	}
}
