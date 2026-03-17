package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"ai-ticket-worker/internal/config"
	"ai-ticket-worker/internal/gitutil"
	"ai-ticket-worker/internal/providers"
	"ai-ticket-worker/internal/ticketsource"
	"ai-ticket-worker/internal/workflow"
)

func main() {
	ctx := context.Background()
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cfg, err := config.Load()
	fatalIf(err)

	cwd, err := os.Getwd()
	fatalIf(err)

	repoRoot, err := gitutil.RepoRoot(ctx, cwd)
	fatalIf(err)
	fatalIf(workflow.EnsureStateIgnored(repoRoot, cfg.StateDirName))

	source := ticketsource.NewShortcutMCPSource(cfg.ShortcutMCP, repoRoot)
	provider, err := providers.NewFromConfig(cfg)
	fatalIf(err)

	orch := workflow.New(cfg, repoRoot, source, provider)

	switch os.Args[1] {
	case "run":
		runCmd(ctx, orch, os.Args[2:])
	case "status":
		statusCmd(orch, os.Args[2:])
	case "approve":
		requireArgs("approve", os.Args[2:], 1)
		fatalIf(orch.Approve(ctx, os.Args[2]))
	case "feedback":
		feedbackCmd(orch, os.Args[2:])
	case "reject":
		requireArgs("reject", os.Args[2:], 1)
		fatalIf(orch.Reject(os.Args[2]))
	case "resume":
		requireArgs("resume", os.Args[2:], 1)
		fatalIf(orch.ResumeTicket(ctx, os.Args[2]))
	case "pr":
		requireArgs("pr", os.Args[2:], 1)
		fatalIf(orch.GeneratePR(ctx, os.Args[2]))
	default:
		usage()
		os.Exit(1)
	}
}

func runCmd(ctx context.Context, orch *workflow.Orchestrator, args []string) {
	requireArgs("run", args, 1)
	fatalIf(orch.RunTickets(ctx, args))
}

func statusCmd(orch *workflow.Orchestrator, args []string) {
	if len(args) > 1 {
		fatalIf(errors.New("usage: ai-orchestrator status [ticket-number]"))
	}
	ticket := ""
	if len(args) == 1 {
		ticket = args[0]
	}
	fatalIf(orch.Status(ticket))
}

func feedbackCmd(orch *workflow.Orchestrator, args []string) {
	requireArgs("feedback", args, 1)
	ticket := args[0]

	fs := flag.NewFlagSet("feedback", flag.ExitOnError)
	message := fs.String("message", "", "feedback message")
	_ = fs.Parse(args[1:])

	if strings.TrimSpace(*message) == "" {
		fatalIf(errors.New("feedback requires --message"))
	}
	fatalIf(orch.Feedback(ticket, *message))
}

func requireArgs(cmd string, args []string, min int) {
	if len(args) < min {
		fatalIf(fmt.Errorf("usage: ai-orchestrator %s ...", cmd))
	}
}

func fatalIf(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func usage() {
	fmt.Println(`ai-orchestrator

Commands:
  ai-orchestrator run <ticket-number> [<ticket-number>...]
  ai-orchestrator status [<ticket-number>]
  ai-orchestrator approve <ticket-number>
  ai-orchestrator feedback <ticket-number> --message "..."
  ai-orchestrator reject <ticket-number>
  ai-orchestrator resume <ticket-number>
  ai-orchestrator pr <ticket-number>`)
}
