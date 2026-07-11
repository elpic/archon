package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/elpic/archon/internal/audit"
	"github.com/elpic/archon/internal/llm"
	"github.com/elpic/archon/internal/standards"
)

const usage = `archon — AI-powered standards auditor

usage:
  archon audit [--fallback owner/repo] [--target path]
  archon init  [--from owner/repo]    [--target path]
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
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *target == "" {
		return fmt.Errorf("audit: --target must be non-empty")
	}

	var opts []standards.Option
	if *fallback != "" {
		opts = append(opts, standards.WithFallback(*fallback))
	}
	resolver, err := standards.NewResolver(".", opts...)
	if err != nil {
		return fmt.Errorf("standards resolver: %w", err)
	}
	provider, err := llm.New(ctx)
	if err != nil {
		return fmt.Errorf("llm provider: %w", err)
	}
	runner := audit.NewRunner(resolver, provider)
	report, err := runner.Run(ctx, *target)
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	fmt.Print(report.Format())
	return nil
}
