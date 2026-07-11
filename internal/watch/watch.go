// Package watch provides filesystem watching for the `archon watch`
// subcommand. It is plumbing-only: the watcher emits a stream of
// Event values over a channel and never invokes the audit/llm
// pipeline itself. The cmd/archon event loop subscribes and decides
// what to do with each event.
//
// fsnotify is the only external dependency. The "stdlib-only"
// invariant documented in .brain/architecture.md is intentionally
// preserved for everything except file-watching, which has no
// stdlib equivalent.
package watch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// EventKind tags the semantics of a single watch Event.
type EventKind int

const (
	// Changed is emitted for any file change other than the
	// project's standards file. Callers should re-run the audit
	// pipeline in response.
	Changed EventKind = iota
	// StandardsChanged is emitted when .archon/standards.md
	// changes (project-local file). Callers should re-resolve
	// standards from the project file and re-run the audit.
	StandardsChanged
	// Error is emitted when the watcher cannot observe the
	// filesystem (file removed mid-watch, etc.). The Path field
	// carries the offending path; the Err field carries the
	// underlying error.
	Error
)

// String returns a stable, lowercase label for each EventKind. The
// value is intended for log lines, not for parsing.
func (k EventKind) String() string {
	switch k {
	case Changed:
		return "changed"
	case StandardsChanged:
		return "standards-changed"
	case Error:
		return "error"
	default:
		return "unknown"
	}
}

// Event is a single watch notification. The interpretation of the
// fields depends on Kind:
//
//   - Changed: Path is the file that changed.
//   - StandardsChanged: Path is the standards file; StandardsSource
//     is the new source label (when the watcher can derive it, e.g.
//     "local:<path>").
//   - Error: Path is the offending path; Err is the underlying error.
type Event struct {
	Path            string
	Kind            EventKind
	StandardsSource string
	Err             error
}

// Watcher is the contract cmd/archon depends on. The real
// implementation is FSNotifyWatcher. Tests may substitute a fake.
type Watcher interface {
	// Watch returns a channel of events and a cleanup function. The
	// returned channel is closed when ctx is cancelled or the
	// underlying watcher can no longer operate. The returned
	// error is non-nil only when the initial setup fails.
	Watch(ctx context.Context, target string) (<-chan Event, error)
}

// FSNotifyWatcher is the production Watcher. It walks target
// recursively, subscribes to fsnotify on every directory, and emits
// debounced events on the returned channel.
type FSNotifyWatcher struct {
	// DebounceWindow collapses bursts of fsnotify events for the
	// same path into a single downstream event. A zero value
	// falls back to defaultDebounce.
	DebounceWindow time.Duration
}

const defaultDebounce = 500 * time.Millisecond

func (w *FSNotifyWatcher) debounce() time.Duration {
	if w.DebounceWindow > 0 {
		return w.DebounceWindow
	}
	return defaultDebounce
}

// Watch implements the Watcher interface. The returned channel emits
// exactly one event per debounced file change; cancellation via ctx
// closes both the channel and the underlying fsnotify watcher.
func (w *FSNotifyWatcher) Watch(ctx context.Context, target string) (<-chan Event, error) {
	if target == "" {
		return nil, errors.New("watch: target is required")
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return nil, fmt.Errorf("watch: resolve target: %w", err)
	}
	stat, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("watch: stat target: %w", err)
	}
	if !stat.IsDir() {
		return nil, fmt.Errorf("watch: target %q is not a directory", abs)
	}

	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("watch: create fsnotify watcher: %w", err)
	}

	// Walk the tree once and subscribe to every directory. fsnotify
	// is not recursive; we extend the watch set on Create events
	// to pick up new subdirectories at runtime.
	if err := addTree(fw, abs); err != nil {
		_ = fw.Close()
		return nil, fmt.Errorf("watch: walk target: %w", err)
	}

	out := make(chan Event, 32)
	go w.loop(ctx, fw, abs, out)
	return out, nil
}

func addTree(w *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// A directory we cannot read is skipped, not fatal.
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		// Skip vendor/.git/node_modules-style noise: not strictly
		// required for the demo, but it prevents the watcher's
		// buffer from being spammed in a real Go project.
		base := filepath.Base(path)
		if base == ".git" {
			return filepath.SkipDir
		}
		return w.Add(path)
	})
}

// loop reads from the underlying fsnotify watcher, debounces events
// per path, classifies the event (Changed vs StandardsChanged vs
// Error), and forwards to out. It exits when ctx is cancelled, when
// the underlying watcher is closed, or when the fsnotify error
// channel delivers a non-fsnotify "watcher closed" sentinel.
func (w *FSNotifyWatcher) loop(ctx context.Context, fw *fsnotify.Watcher, target string, out chan<- Event) {
	defer close(out)
	defer fw.Close()

	debounce := w.debounce()
	var (
		mu    sync.Mutex
		dirty = map[string]struct{}{}
	)

	// flush emits one Changed (or StandardsChanged) per dirty path
	// in the order they were first seen.
	flush := func() {
		mu.Lock()
		paths := make([]string, 0, len(dirty))
		for p := range dirty {
			paths = append(paths, p)
		}
		dirty = map[string]struct{}{}
		mu.Unlock()
		for _, p := range paths {
			kind := Changed
			if isStandardsPath(target, p) {
				kind = StandardsChanged
			}
			ev := Event{Kind: kind, Path: p}
			if kind == StandardsChanged {
				ev.StandardsSource = "local:" + p
			}
			send(ctx, out, ev)
		}
	}

	// pendingTimer is reset on every fsnotify event and fires
	// after `debounce` of quiet. The flush sends every accumulated
	// path. We use a single timer (not per-path) because editor
	// saves touch multiple files in quick succession and a per-path
	// timer would fragment the coalescing window.
	var (
		timerMu sync.Mutex
		timer   *time.Timer
	)
	resetTimer := func() {
		timerMu.Lock()
		defer timerMu.Unlock()
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(debounce, flush)
	}

	stop := func() {
		timerMu.Lock()
		if timer != nil {
			timer.Stop()
		}
		timerMu.Unlock()
	}

	for {
		select {
		case <-ctx.Done():
			stop()
			return
		case ev, ok := <-fw.Events:
			if !ok {
				stop()
				return
			}
			// A new directory appeared under the target — extend
			// the watch set so we see events inside it.
			if ev.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					_ = fw.Add(ev.Name)
				}
			}
			// Remove and Rename can fire on a directory; the
			// watcher is best-effort about these. We treat the
			// remaining event kinds as "something changed here".
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename|fsnotify.Chmod) != 0 {
				mu.Lock()
				dirty[ev.Name] = struct{}{}
				mu.Unlock()
				resetTimer()
			}
		case err, ok := <-fw.Errors:
			if !ok {
				stop()
				return
			}
			send(ctx, out, Event{Kind: Error, Err: err})
		}
	}
}

// send forwards ev to ch, respecting ctx cancellation. If the
// receiver is slow and the context is cancelled, the event is
// dropped rather than blocking the loop.
func send(ctx context.Context, ch chan<- Event, ev Event) {
	select {
	case <-ctx.Done():
	case ch <- ev:
	}
}

// isStandardsPath reports whether p is the project's standards file,
// relative to target. The standards file is conventionally
// target/.archon/standards.md; we match on the cleaned, absolute path
// so symlinks and trailing slashes don't break the comparison.
func isStandardsPath(target, p string) bool {
	want, _ := filepath.Abs(filepath.Join(target, ".archon", "standards.md"))
	got, _ := filepath.Abs(filepath.Clean(p))
	return strings.EqualFold(want, got)
}
