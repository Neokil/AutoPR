package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/Neokil/AutoPR/internal/application/orchestrator"
	"github.com/Neokil/AutoPR/internal/gitutil"
)

const (
	defaultServerURL = "http://127.0.0.1:8080"
	minArgs          = 2
)

func resolveServerURL() string {
	serverURL := strings.TrimSpace(os.Getenv("AUTO_PR_SERVER_URL"))
	if serverURL != "" {
		return serverURL
	}

	return defaultServerURL
}

func main() {
	ctx := context.Background()
	if len(os.Args) < minArgs {
		usage()
		os.Exit(1)
	}

	cwd, err := os.Getwd()
	if err != nil {
		slog.Error("get working directory", "err", err)
		os.Exit(1)
	}

	repoRoot, err := gitutil.RepoRoot(ctx, cwd)
	if err != nil {
		slog.Error("find repository root", "err", err)
		os.Exit(1)
	}
	serverURL := resolveServerURL()
	svc := orchestrator.NewRemoteService(serverURL, repoRoot)

	switch os.Args[1] {
	case "run":
		runCmd(ctx, svc, os.Args[2:])
	case "wait-for-job":
		waitForJobCmd(ctx, svc, os.Args[2:])
	case "status":
		statusCmd(svc, os.Args[2:])
	case "action":
		actionCmd(ctx, svc, os.Args[2:])
	case "cleanup":
		cleanupCmd(ctx, svc, os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}

func runCmd(ctx context.Context, svc orchestrator.Service, args []string) {
	requireArgs("run", args, 1)
	for _, ticket := range args {
		err := svc.StartFlow(ctx, ticket)
		if err != nil {
			slog.Error("start flow", "err", err)
			os.Exit(1)
		}
	}
}

func statusCmd(svc orchestrator.Service, args []string) {
	if len(args) > 1 {
		slog.Error("invalid usage", "err", errUsageStatus)
		os.Exit(1)
	}
	ticket := ""
	if len(args) == 1 {
		ticket = args[0]
	}
	err := svc.Status(ticket)
	if err != nil {
		slog.Error("status", "err", err)
		os.Exit(1)
	}
	if ticket != "" {
		printNextSteps(svc, ticket)
	}
}

func actionCmd(ctx context.Context, svc orchestrator.Service, args []string) {
	requireArgs("action", args, 1)
	ticket := args[0]

	fs := flag.NewFlagSet("action", flag.ExitOnError)
	label := fs.String("label", "", "action label (required)")
	message := fs.String("message", "", "optional message (for provide_feedback actions)")
	_ = fs.Parse(args[1:])

	if strings.TrimSpace(*label) == "" {
		slog.Error("invalid usage", "err", errActionRequiresLabel)
		os.Exit(1)
	}
	err := svc.ApplyAction(ctx, ticket, *label, *message)
	if err != nil {
		slog.Error("apply action", "err", err)
		os.Exit(1)
	}
}

func waitForJobCmd(ctx context.Context, svc orchestrator.Service, args []string) {
	requireArgs("wait-for-job", args, 1)
	remote, ok := svc.(*orchestrator.RemoteService)
	if !ok {
		slog.Error("invalid usage", "err", errWaitForJobServerOnly)
		os.Exit(1)
	}
	job, err := remote.WaitForJob(ctx, args[0])
	if err != nil {
		slog.Error("wait for job", "err", err)
		os.Exit(1)
	}
	slog.Info("job completed", "job_id", job.ID, "status", job.Status)
}

func cleanupCmd(ctx context.Context, svc orchestrator.Service, args []string) {
	fs := flag.NewFlagSet("cleanup", flag.ExitOnError)
	doneOnly := fs.Bool("done", false, "cleanup only done tickets")
	all := fs.Bool("all", false, "cleanup all tickets")
	_ = fs.Parse(args)

	if *doneOnly && *all {
		slog.Error("invalid usage", "err", errCleanupFlags)
		os.Exit(1)
	}
	if *doneOnly {
		err := svc.CleanupDone(ctx)
		if err != nil {
			slog.Error("cleanup done", "err", err)
			os.Exit(1)
		}

		return
	}
	if *all {
		err := svc.CleanupAll(ctx)
		if err != nil {
			slog.Error("cleanup all", "err", err)
			os.Exit(1)
		}

		return
	}

	rest := fs.Args()
	if len(rest) != 1 {
		slog.Error("invalid usage", "err", errUsageCleanup)
		os.Exit(1)
	}
	err := svc.CleanupTicket(ctx, rest[0])
	if err != nil {
		slog.Error("cleanup ticket", "err", err)
		os.Exit(1)
	}
}

func requireArgs(cmd string, args []string, minArgs int) {
	if len(args) < minArgs {
		slog.Error("invalid usage", "cmd", "auto-pr "+cmd, "err", errUsage)
		os.Exit(1)
	}
}

func usage() {
	_, _ = fmt.Fprintln(os.Stdout, `AutoPR

Commands:
  auto-pr run <ticket-number> [<ticket-number>...]
  auto-pr wait-for-job <job-id>
  auto-pr status [<ticket-number>]
  auto-pr action <ticket-number> --label "<action-label>" [--message "..."]
  auto-pr cleanup <ticket-number>
  auto-pr cleanup --done
  auto-pr cleanup --all`)
}

func printNextSteps(svc orchestrator.Service, ticket string) {
	msg, err := svc.NextSteps(ticket)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintln(os.Stdout)
	_, _ = fmt.Fprintln(os.Stdout, msg)
}
