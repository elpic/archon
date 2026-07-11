package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/elpic/archon/internal/audit"
	"github.com/elpic/archon/internal/llm"
	"github.com/elpic/archon/internal/standards"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: archon <command> [args]")
		fmt.Println("commands:")
		fmt.Println("  audit <path>   audit a project for standards compliance")
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	switch os.Args[1] {
	case "audit":
		if len(os.Args) < 3 {
			log.Fatal("audit requires a path argument")
		}
		target := os.Args[2]

		resolver, err := standards.NewResolver(".")
		if err != nil {
			log.Fatalf("standards resolver: %v", err)
		}

		provider, err := llm.New(ctx)
		if err != nil {
			log.Fatalf("llm provider: %v", err)
		}

		runner := audit.NewRunner(resolver, provider)
		report, err := runner.Run(ctx, target)
		if err != nil {
			log.Fatalf("audit failed: %v", err)
		}

		fmt.Print(report.Format())
	default:
		log.Fatalf("unknown command: %s", os.Args[1])
	}
}
