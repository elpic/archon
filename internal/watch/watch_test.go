package watch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// waitForEvent reads events from ch until predicate returns true or
// the timeout expires. Returns the matching event and true, or
// (zero, false) on timeout. It is the test-side counterpart to the
// production-side debounce: tests use a shorter debounce (see the
// FSNotifyWatcher{DebounceWindow: ...} calls below) so the same
// 1-second wall-clock budget gives a comfortable margin for the
// 500ms debounce to fire.
func waitForEvent(t *testing.T, ch <-chan Event, timeout time.Duration, predicate func(Event) bool) (Event, bool) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return Event{}, false
			}
			if predicate(ev) {
				return ev, true
			}
		case <-deadline.C:
			return Event{}, false
		}
	}
}

// TestWatcher_FileChange_EmitsChanged: writing a file in the
// watched target produces a Changed event within 1s.
func TestWatcher_FileChange_EmitsChanged(t *testing.T) {
	dir := t.TempDir()
	w := &FSNotifyWatcher{DebounceWindow: 50 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := w.Watch(ctx, dir)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	target := filepath.Join(dir, "hello.go")
	if err := os.WriteFile(target, []byte("package hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ev, ok := waitForEvent(t, ch, time.Second, func(e Event) bool {
		return e.Kind == Changed && e.Path == target
	})
	if !ok {
		t.Fatal("did not see Changed event for hello.go within 1s")
	}
	if ev.Err != nil {
		t.Errorf("event had Err: %v", ev.Err)
	}
}

// TestWatcher_StandardsFileChange_EmitsStandardsChanged: writing
// to .archon/standards.md produces a StandardsChanged event with a
// non-empty StandardsSource.
func TestWatcher_StandardsFileChange_EmitsStandardsChanged(t *testing.T) {
	dir := t.TempDir()
	archonDir := filepath.Join(dir, ".archon")
	if err := os.Mkdir(archonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	standards := filepath.Join(archonDir, "standards.md")
	if err := os.WriteFile(standards, []byte("# placeholder\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w := &FSNotifyWatcher{DebounceWindow: 50 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := w.Watch(ctx, dir)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Edit the standards file. We do not want this event
	// confused with a Create (the file already exists), so we
	// append rather than write from scratch.
	f, err := os.OpenFile(standards, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n## added later\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	ev, ok := waitForEvent(t, ch, time.Second, func(e Event) bool {
		return e.Kind == StandardsChanged
	})
	if !ok {
		t.Fatal("did not see StandardsChanged event within 1s")
	}
	if ev.Path != standards {
		t.Errorf("Path = %q, want %q", ev.Path, standards)
	}
	if ev.StandardsSource == "" {
		t.Errorf("StandardsSource empty on StandardsChanged event")
	}
}

// TestWatcher_Debounce: a burst of writes to the SAME path within
// the debounce window collapses to a single downstream event.
// The spec ("many fsnotify events for one user save should
// collapse to one channel send") describes the typical editor-save
// case where 5-10 fsnotify events fire for one logical save. We
// model that by overwriting the same file 10 times in quick
// succession.
func TestWatcher_Debounce(t *testing.T) {
	dir := t.TempDir()
	debounce := 200 * time.Millisecond
	w := &FSNotifyWatcher{DebounceWindow: debounce}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := w.Watch(ctx, dir)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Overwrite the same file 10 times inside the debounce window.
	target := filepath.Join(dir, "f.go")
	for i := 0; i < 10; i++ {
		if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Wait until the debounce window has clearly elapsed, then
	// drain whatever is in the channel.
	time.Sleep(debounce + 300*time.Millisecond)
	seen := 0
drain:
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				break drain
			}
			seen++
		case <-time.After(100 * time.Millisecond):
			break drain
		}
	}
	if seen >= 5 {
		t.Errorf("debounce did not coalesce: saw %d events for 10 overwrites of one file", seen)
	}
	if seen < 1 {
		t.Errorf("expected at least 1 event, saw %d", seen)
	}
	t.Logf("debounce collapsed 10 overwrites into %d events", seen)
}

// TestWatcher_RecursiveAdd: a subdirectory created AFTER the
// watcher starts must be observed. We create the dir, wait a beat
// for the watcher's goroutine to add it to the fsnotify watch set,
// then create a file inside and assert the inner file's event
// arrives.
func TestWatcher_RecursiveAdd(t *testing.T) {
	dir := t.TempDir()
	w := &FSNotifyWatcher{DebounceWindow: 50 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := w.Watch(ctx, dir)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	subdir := filepath.Join(dir, "sub")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Give the watcher's event loop a chance to receive the
	// Create event for the new subdir and call fw.Add on it.
	// Without this sleep the inner WriteFile can race the Add and
	// be missed. 100ms is plenty on every supported OS.
	time.Sleep(100 * time.Millisecond)
	inner := filepath.Join(subdir, "inner.go")
	if err := os.WriteFile(inner, []byte("package sub\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ev, ok := waitForEvent(t, ch, time.Second, func(e Event) bool {
		return e.Kind == Changed && e.Path == inner
	})
	if !ok {
		t.Fatal("did not see Changed event for inner.go in newly-created subdir within 1s")
	}
	if ev.Path != inner {
		t.Errorf("Path = %q, want %q", ev.Path, inner)
	}
}

// TestWatcher_CtxCancel: cancelling the context must close the
// event channel within 100ms, even with a long debounce window
// (so we know the cancel is not racing the debounce timer).
func TestWatcher_CtxCancel(t *testing.T) {
	dir := t.TempDir()
	w := &FSNotifyWatcher{DebounceWindow: 5 * time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := w.Watch(ctx, dir)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			// An event after cancel is acceptable; what matters
			// is that the channel eventually closes.
			_, ok = <-ch
			if ok {
				t.Fatal("channel did not close after cancel")
			}
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("channel did not close within 100ms of cancel")
	}
}

// TestWatcher_DirRemove: removing a watched directory mid-flight
// must not panic; the watcher should either close the channel or
// emit an Error event and continue. We do not assert a specific
// outcome because the exact behaviour depends on the OS, but we
// do require no panic and a deterministic loop exit.
func TestWatcher_DirRemove(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	w := &FSNotifyWatcher{DebounceWindow: 50 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := w.Watch(ctx, dir)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Remove the watched dir. fsnotify typically emits a
	// Remove/Rename event for the dir itself and any errors
	// surface through fw.Errors; either is acceptable here.
	if err := os.RemoveAll(sub); err != nil {
		t.Fatal(err)
	}

	// Drain for a short window to allow any pending events to
	// arrive. We do not assert the kind — only that nothing
	// panics and the channel reaches a stable state.
	deadline := time.NewTimer(500 * time.Millisecond)
	defer deadline.Stop()
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // channel closed cleanly
			}
		case <-deadline.C:
			// Not closed within 500ms is fine on some platforms
			// (the watcher may have transitioned to a steady
			// state). Cancel the context to force shutdown so the
			// test does not leak the goroutine.
			cancel()
			// Drain again briefly to ensure the goroutine exits.
			drainDeadline := time.NewTimer(200 * time.Millisecond)
			defer drainDeadline.Stop()
			for {
				select {
				case _, ok := <-ch:
					if !ok {
						return
					}
				case <-drainDeadline.C:
					return
				}
			}
		}
	}
}

// TestWatcher_NonExistentTarget: pointing Watch at a missing path
// surfaces a wrapped error rather than a panic.
func TestWatcher_NonExistentTarget(t *testing.T) {
	w := &FSNotifyWatcher{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := w.Watch(ctx, "/this/does/not/exist/anywhere-xyz")
	if err == nil {
		t.Fatal("expected error for non-existent target, got nil")
	}
}

// TestWatcher_FileTargetRejected: Watch requires a directory;
// passing a file path must return an error.
func TestWatcher_FileTargetRejected(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := &FSNotifyWatcher{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := w.Watch(ctx, file)
	if err == nil {
		t.Fatal("expected error when target is a file, got nil")
	}
}
