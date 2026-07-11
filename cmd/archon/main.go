package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/elpic/archon/internal/audit"
	"github.com/elpic/archon/internal/git"
	"github.com/elpic/archon/internal/llm"
	"github.com/elpic/archon/internal/standards"
	"github.com/elpic/archon/internal/watch"
)

const usage = `archon — AI-powered standards auditor

usage:
  archon audit  [--fallback owner/repo] [--target path]
  archon watch  [--fallback owner/repo] [--target path]
  archon init   [--from owner/repo]    [--target path]
  archon help
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var err error
	switch os.Args[1] {
	case "audit":
		err = runAudit(ctx, os.Args[2:])
	case "watch":
		err = runWatch(ctx, os.Args[2:])
	case "init":
		err = runInit(ctx, os.Args[2:])
	case "help", "-h", "--help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n%s", os.Args[1], usage)
		os.Exit(1)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func runAudit(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fallback := fs.String("fallback", "", "fallback org/repo for standards when no project or org source is found (e.g. elpic/go-standards)")
	target := fs.String("target", ".", "project path to audit")
	changed := fs.Bool("changed", false, "audit only files changed since HEAD~1 (uses git diff)")
	since := fs.String("since", "", "audit files changed since given ref (e.g. main, HEAD~3, commit SHA)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *target == "" {
		return fmt.Errorf("audit: --target must be non-empty")
	}

	resolver, err := newResolver(*fallback)
	if err != nil {
		return fmt.Errorf("standards resolver: %w", err)
	}
	provider, err := llm.New(ctx)
	if err != nil {
		return fmt.Errorf("llm provider: %w", err)
	}
	runner := audit.NewRunner(resolver, provider)

	var changedFiles []string
	if *changed {
		files, err := git.ChangedFiles(ctx, git.DiffOptions{
			Target:      *target,
			ChangedOnly: true,
		})
		if err != nil {
			return fmt.Errorf("get changed files: %w", err)
		}
		changedFiles = files
	} else if *since != "" {
		files, err := git.ChangedFiles(ctx, git.DiffOptions{
			Target: *target,
			Since:  *since,
		})
		if err != nil {
			return fmt.Errorf("get changed files: %w", err)
		}
		changedFiles = files
	}

	if len(changedFiles) > 0 {
		runner = runner.WithChangedFiles(changedFiles)
	}

	report, err := runner.Run(ctx, *target)
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	fmt.Print(report.Format())
	return nil
}

// runWatch subscribes to filesystem changes and re-runs the audit
// pipeline on each event. Output is the problem-matcher format
// (path:line:col: [severity] message), one violation per line, so
// editor quickfix / VS Code "Error Lens" can pick them up
// directly.
//
// The LLM provider is still a stub (llm.New returns
// "llm.New not yet implemented"). The watch loop logs that error
// and continues — this is the expected demo behaviour: every save
// surfaces the stub error, proving cancellation and event
// classification work end-to-end. When the real provider lands
// (separate ticket), the stubProvider below goes away.
func runWatch(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fallback := fs.String("fallback", "", "fallback org/repo for standards when no project or org source is found (e.g. elpic/go-standards)")
	target := fs.String("target", ".", "project path to watch")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *target == "" {
		return fmt.Errorf("watch: --target must be non-empty")
	}

	resolver, err := newResolver(*fallback)
	if err != nil {
		return fmt.Errorf("standards resolver: %w", err)
	}
	provider, err := llm.New(ctx)
	if err != nil {
		// The LLM client is still a stub. We log the error once
		// here (not exit) and wire a stubProvider that re-emits
		// the same error on every audit, so the watch loop can
		// keep running and the user can see the LLM plumbing
		// wake up on every change.
		fmt.Fprintf(os.Stderr, "archon: llm provider unavailable (%v); watch loop will continue and surface the error on each change\n", err)
		provider = &stubProvider{err: err}
	}
	return runWatchInner(ctx, resolver, provider, *target, os.Stdout, os.Stderr)
}

// runWatchInner is the testable core of runWatch: it takes a
// pre-wired Resolver and Provider so tests can substitute a fake
// LLM. The output writers are injectable so tests can capture the
// problem-matcher stream without touching the process stdout.
func runWatchInner(ctx context.Context, resolver *standards.Resolver, provider llm.Provider, target string, stdout, stderr io.Writer) error {
	runner := audit.NewRunner(resolver, provider)

	w := &watch.FSNotifyWatcher{}
	events, err := w.Watch(ctx, target)
	if err != nil {
		return fmt.Errorf("watch: %w", err)
	}
	fmt.Fprintf(stderr, "archon: watching %s (Ctrl-C to stop)\n", target)

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-events:
			if !ok {
				return nil
			}
			handleEvent(ctx, runner, ev, target, stdout, stderr)
		}
	}
}

// handleEvent is the per-event dispatch in the watch loop. It is
// split out so the loop body stays a clean select.
func handleEvent(ctx context.Context, runner *audit.Runner, ev watch.Event, target string, stdout, stderr io.Writer) {
	switch ev.Kind {
	case watch.StandardsChanged:
		fmt.Fprintf(stderr, "Standards updated from %s; re-running audit\n", ev.StandardsSource)
		runAndPrint(ctx, runner, target, stdout, stderr)
	case watch.Changed:
		fmt.Fprintf(stderr, "change detected: %s\n", ev.Path)
		runAndPrint(ctx, runner, target, stdout, stderr)
	case watch.Error:
		fmt.Fprintf(stderr, "archon: watch error on %s: %v\n", ev.Path, ev.Err)
	}
}

// runAndPrint executes a single audit pass and writes the
// diagnostic output to stdout. Errors (most commonly the LLM stub
// error today) are logged to stderr and the watch loop continues.
func runAndPrint(ctx context.Context, runner *audit.Runner, target string, stdout, stderr io.Writer) {
	report, err := runner.Run(ctx, target)
	if err != nil {
		// Treat context cancellation as a clean shutdown; surface
		// any other error as a stderr line so the loop keeps
		// going on the next event.
		if errors.Is(err, context.Canceled) {
			return
		}
		fmt.Fprintf(stderr, "archon: audit failed: %v\n", err)
		return
	}
	out := report.FormatDiagnostic()
	if out != "" {
		fmt.Fprint(stdout, out)
	}
}

// newResolver is the standards-side wiring shared by runAudit and
// runWatch. It does NOT touch the LLM provider; callers do that
// themselves so they can apply the policy that fits their
// subcommand (one-shot vs long-lived loop).
func newResolver(fallback string) (*standards.Resolver, error) {
	var opts []standards.Option
	if fallback != "" {
		opts = append(opts, standards.WithFallback(fallback))
	}
	return standards.NewResolver(".", opts...)
}

// stubProvider is the placeholder used by runWatch when llm.New
// fails (the LLM client is a stub today). It always reports the
// original construction error from Audit() so the caller's error
// path stays the same shape as it will be once the real provider
// lands. It is not a test fake and is not part of the llm package
// surface — it is plumbing so the watch loop can run today. When
// the real provider is implemented, this type goes away.
type stubProvider struct {
	err error
}

func (s *stubProvider) Audit(_ context.Context, _ []byte, _ string, _ []string) ([]llm.Violation, error) {
	return nil, s.err
}
