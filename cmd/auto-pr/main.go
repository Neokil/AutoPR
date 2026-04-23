package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Neokil/AutoPR/internal/application/orchestrator"
	"github.com/Neokil/AutoPR/internal/gitutil"
)

const defaultServerURL = "http://127.0.0.1:8080"

func resolveServerURL() string {
	serverURL := strings.TrimSpace(os.Getenv("AUTO_PR_SERVER_URL"))
	if serverURL != "" {
		return serverURL
	}
	return defaultServerURL
}

func main() {
	ctx := context.Background()
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cwd, err := os.Getwd()
	fatalIf(err)

	repoRoot, err := gitutil.RepoRoot(ctx, cwd)
	fatalIf(err)
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
		fatalIf(svc.StartFlow(ctx, ticket))
	}
}

func statusCmd(svc orchestrator.Service, args []string) {
	if len(args) > 1 {
		fatalIf(errors.New("usage: auto-pr status [ticket-number]"))
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

func actionCmd(ctx context.Context, svc orchestrator.Service, args []string) {
	requireArgs("action", args, 1)
	ticket := args[0]

	fs := flag.NewFlagSet("action", flag.ExitOnError)
	label := fs.String("label", "", "action label (required)")
	message := fs.String("message", "", "optional message (for provide_feedback actions)")
	_ = fs.Parse(args[1:])

	if strings.TrimSpace(*label) == "" {
		fatalIf(errors.New("action requires --label"))
	}
	fatalIf(svc.ApplyAction(ctx, ticket, *label, *message))
}

func waitForJobCmd(ctx context.Context, svc orchestrator.Service, args []string) {
	requireArgs("wait-for-job", args, 1)
	remote, ok := svc.(*orchestrator.RemoteService)
	if !ok {
		fatalIf(errors.New("wait-for-job is only supported in server mode"))
	}
	job, err := remote.WaitForJob(ctx, args[0])
	fatalIf(err)
	fmt.Printf("job %s completed with status %s\n", job.ID, job.Status)
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
		fatalIf(errors.New("usage: auto-pr cleanup <ticket-number> | --done | --all"))
	}
	fatalIf(svc.CleanupTicket(ctx, rest[0]))
}

func requireArgs(cmd string, args []string, min int) {
	if len(args) < min {
		fatalIf(fmt.Errorf("usage: auto-pr %s ...", cmd))
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
	fmt.Println(`AutoPR

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
	fmt.Println()
	fmt.Println(msg)
}
