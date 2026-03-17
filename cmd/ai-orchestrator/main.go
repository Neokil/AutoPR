package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"ai-ticket-worker/internal/application/orchestrator"
	"ai-ticket-worker/internal/config"
	"ai-ticket-worker/internal/gitutil"
	"ai-ticket-worker/internal/providers"
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
	serverURL := strings.TrimSpace(os.Getenv("AI_ORCHESTRATOR_SERVER_URL"))
	var svc orchestrator.Service
	if serverURL != "" {
		svc = orchestrator.NewRemoteService(serverURL, repoRoot)
	} else {
		fatalIf(workflow.EnsureStateIgnored(repoRoot, cfg.StateDirName))
		provider, err := providers.NewFromConfig(cfg)
		fatalIf(err)
		svc = orchestrator.NewWorkflowService(cfg, repoRoot, provider)
	}

	switch os.Args[1] {
	case "run":
		runCmd(ctx, svc, os.Args[2:])
	case "status":
		statusCmd(svc, os.Args[2:])
	case "approve":
		requireArgs("approve", os.Args[2:], 1)
		ticket := os.Args[2]
		fatalIf(svc.Approve(ctx, ticket))
		printNextSteps(svc, ticket)
	case "feedback":
		feedbackCmd(svc, os.Args[2:])
	case "reject":
		requireArgs("reject", os.Args[2:], 1)
		ticket := os.Args[2]
		fatalIf(svc.Reject(ticket))
		printNextSteps(svc, ticket)
	case "resume":
		requireArgs("resume", os.Args[2:], 1)
		ticket := os.Args[2]
		fatalIf(svc.ResumeTicket(ctx, ticket))
		printNextSteps(svc, ticket)
	case "pr":
		requireArgs("pr", os.Args[2:], 1)
		ticket := os.Args[2]
		fatalIf(svc.GeneratePR(ctx, ticket))
		printNextSteps(svc, ticket)
	case "cleanup":
		cleanupCmd(ctx, svc, os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}

func runCmd(ctx context.Context, svc orchestrator.Service, args []string) {
	requireArgs("run", args, 1)
	fatalIf(svc.RunTickets(ctx, args))
	for _, ticket := range args {
		printNextSteps(svc, ticket)
	}
}

func statusCmd(svc orchestrator.Service, args []string) {
	if len(args) > 1 {
		fatalIf(errors.New("usage: ai-orchestrator status [ticket-number]"))
	}
	ticket := ""
	if len(args) == 1 {
		ticket = args[0]
	}
	fatalIf(svc.Status(ticket))
	if ticket != "" {
		printNextSteps(svc, ticket)
	}
}

func feedbackCmd(svc orchestrator.Service, args []string) {
	requireArgs("feedback", args, 1)
	ticket := args[0]

	fs := flag.NewFlagSet("feedback", flag.ExitOnError)
	message := fs.String("message", "", "feedback message")
	_ = fs.Parse(args[1:])

	if strings.TrimSpace(*message) == "" {
		fatalIf(errors.New("feedback requires --message"))
	}
	fatalIf(svc.Feedback(ticket, *message))
	printNextSteps(svc, ticket)
}

func cleanupCmd(ctx context.Context, svc orchestrator.Service, args []string) {
	fs := flag.NewFlagSet("cleanup", flag.ExitOnError)
	doneOnly := fs.Bool("done", false, "cleanup only done tickets")
	all := fs.Bool("all", false, "cleanup all tickets")
	_ = fs.Parse(args)

	if *doneOnly && *all {
		fatalIf(errors.New("cleanup: use either --done or --all, not both"))
	}
	if *doneOnly {
		fatalIf(svc.CleanupDone(ctx))
		return
	}
	if *all {
		fatalIf(svc.CleanupAll(ctx))
		return
	}

	rest := fs.Args()
	if len(rest) != 1 {
		fatalIf(errors.New("usage: ai-orchestrator cleanup <ticket-number> | --done | --all"))
	}
	fatalIf(svc.CleanupTicket(ctx, rest[0]))
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
  ai-orchestrator pr <ticket-number>
  ai-orchestrator cleanup <ticket-number>
  ai-orchestrator cleanup --done
  ai-orchestrator cleanup --all`)
}

func printNextSteps(svc orchestrator.Service, ticket string) {
	msg, err := svc.NextSteps(ticket)
	if err != nil {
		return
	}
	fmt.Println()
	fmt.Println(msg)
}
