package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"ai-ticket-worker/internal/config"
	"ai-ticket-worker/internal/gitutil"
	"ai-ticket-worker/internal/markdown"
	"ai-ticket-worker/internal/models"
	"ai-ticket-worker/internal/ports"
	"ai-ticket-worker/internal/providers"
	"ai-ticket-worker/internal/shell"
	"ai-ticket-worker/internal/state"
	"ai-ticket-worker/internal/worktree"
)

type Orchestrator struct {
	Cfg      config.Config
	RepoRoot string
	Store    ports.StateStore
	Provider providers.AIProvider
}

func New(cfg config.Config, repoRoot string, provider providers.AIProvider) *Orchestrator {
	return NewWithStore(cfg, repoRoot, state.NewStore(repoRoot, cfg.StateDirName), provider)
}

func NewWithStore(cfg config.Config, repoRoot string, store ports.StateStore, provider providers.AIProvider) *Orchestrator {
	return &Orchestrator{
		Cfg:      cfg,
		RepoRoot: repoRoot,
		Store:    store,
		Provider: provider,
	}
}

func (o *Orchestrator) RunTickets(ctx context.Context, ticketNumbers []string) error {
	for _, n := range ticketNumbers {
		if err := o.RunTicket(ctx, n); err != nil {
			fmt.Fprintf(os.Stderr, "ticket %s failed: %v\n", n, err)
		}
	}
	return nil
}

func (o *Orchestrator) RunTicket(ctx context.Context, ticketNumber string) error {
	st, err := o.initOrLoad(ctx, ticketNumber)
	if err != nil {
		return err
	}
	if st.Status == models.StatePRReady {
		t, err := o.Store.LoadTicket(ticketNumber)
		if err != nil {
			return err
		}
		return o.generatePR(ctx, st, t)
	}
	if st.Status == models.StateWaitingForHuman && !st.Approved {
		fmt.Printf("ticket %s is waiting for human input. Run approve/feedback/reject.\n", ticketNumber)
		return nil
	}
	if st.Status == models.StateQueued ||
		st.Status == models.StateInvestigating ||
		st.Status == models.StateProposalReady {
		return o.investigate(ctx, st)
	}
	if st.Approved ||
		st.Status == models.StateImplementing ||
		st.Status == models.StateValidating ||
		st.Status == models.StatePRReady {
		return o.implementationPipeline(ctx, st)
	}
	if st.Status == models.StateDone {
		fmt.Printf("ticket %s already done\n", ticketNumber)
	}
	return nil
}

func (o *Orchestrator) ResumeTicket(ctx context.Context, ticketNumber string) error {
	st, err := o.Store.LoadState(ticketNumber)
	if err != nil {
		return err
	}
	if st.Status == models.StatePRReady {
		t, err := o.Store.LoadTicket(ticketNumber)
		if err != nil {
			return err
		}
		return o.generatePR(ctx, &st, t)
	}
	if st.Status == models.StateWaitingForHuman && !st.Approved {
		fmt.Printf("ticket %s is waiting for human input.\n", ticketNumber)
		return nil
	}
	if st.Status == models.StateQueued ||
		st.Status == models.StateInvestigating ||
		st.Status == models.StateProposalReady {
		return o.investigate(ctx, &st)
	}
	if st.Approved ||
		st.Status == models.StateImplementing ||
		st.Status == models.StateValidating ||
		st.Status == models.StatePRReady {
		return o.implementationPipeline(ctx, &st)
	}
	return nil
}

func (o *Orchestrator) Approve(ctx context.Context, ticketNumber string) error {
	st, err := o.Store.LoadState(ticketNumber)
	if err != nil {
		return err
	}
	st.Approved = true
	st.Status = models.StateImplementing
	if err := o.Store.SaveState(ticketNumber, st); err != nil {
		return err
	}
	_ = markdown.AppendSection(st.LogPath, "Human Approval", "Approved for implementation.")
	return o.ResumeTicket(ctx, ticketNumber)
}

func (o *Orchestrator) Feedback(ticketNumber, message string) error {
	st, err := o.Store.LoadState(ticketNumber)
	if err != nil {
		return err
	}
	st.LastFeedback = message
	st.Approved = false
	st.Status = models.StateInvestigating
	if err := markdown.AppendSection(st.LogPath, "Human Feedback", message); err != nil {
		return err
	}
	return o.Store.SaveState(ticketNumber, st)
}

func (o *Orchestrator) Reject(ticketNumber string) error {
	st, err := o.Store.LoadState(ticketNumber)
	if err != nil {
		return err
	}
	st.Status = models.StateFailed
	st.Approved = false
	st.LastError = "rejected by human"
	if err := markdown.AppendSection(st.LogPath, "Human Rejection", "Rejected by reviewer."); err != nil {
		return err
	}
	return o.Store.SaveState(ticketNumber, st)
}

func (o *Orchestrator) Status(ticketNumber string) error {
	if ticketNumber != "" {
		return o.printStatus(ticketNumber)
	}
	tickets, err := o.Store.ListTicketDirs()
	if err != nil {
		return err
	}
	sort.Strings(tickets)
	for _, t := range tickets {
		if err := o.printStatus(t); err != nil {
			fmt.Fprintf(os.Stderr, "status %s: %v\n", t, err)
		}
	}
	return nil
}

func (o *Orchestrator) NextSteps(ticketNumber string) (string, error) {
	st, err := o.Store.LoadState(ticketNumber)
	if err != nil {
		return "", err
	}
	switch st.Status {
	case models.StateQueued, models.StateInvestigating, models.StateProposalReady, models.StateWaitingForHuman:
		return fmt.Sprintf("Next steps for ticket %s:\n  1. Review proposal: %s\n  2. Approve: auto-pr approve %s\n  3. Provide feedback: auto-pr feedback %s --message \"...\"\n  4. Reject: auto-pr reject %s", st.TicketNumber, st.ProposalPath, st.TicketNumber, st.TicketNumber, st.TicketNumber), nil
	case models.StateImplementing, models.StateValidating:
		return fmt.Sprintf("Next steps for ticket %s:\n  1. Continue workflow: auto-pr resume %s\n  2. Check progress: auto-pr status %s", st.TicketNumber, st.TicketNumber, st.TicketNumber), nil
	case models.StatePRReady:
		if strings.TrimSpace(st.PRURL) != "" {
			return fmt.Sprintf("Next steps for ticket %s:\n  1. Review PR markdown: %s\n  2. Review GitHub PR: %s\n  3. Apply open review comments: auto-pr apply-pr-comments %s", st.TicketNumber, st.PRPath, st.PRURL, st.TicketNumber), nil
		}
		return fmt.Sprintf("Next steps for ticket %s:\n  1. Generate/create PR: auto-pr pr %s\n  2. Review PR markdown: %s", st.TicketNumber, st.TicketNumber, st.PRPath), nil
	case models.StateDone:
		if strings.TrimSpace(st.PRURL) != "" {
			return fmt.Sprintf("Next steps for ticket %s:\n  1. Review final PR markdown: %s\n  2. Review GitHub PR: %s\n  3. Apply open review comments: auto-pr apply-pr-comments %s", st.TicketNumber, st.PRPath, st.PRURL, st.TicketNumber), nil
		}
		return fmt.Sprintf("Next steps for ticket %s:\n  1. Review final PR markdown: %s\n  2. Check current state: auto-pr status %s", st.TicketNumber, st.PRPath, st.TicketNumber), nil
	case models.StateFailed:
		return fmt.Sprintf("Next steps for ticket %s:\n  1. Inspect log: %s\n  2. Add feedback: auto-pr feedback %s --message \"...\"\n  3. Retry: auto-pr resume %s", st.TicketNumber, st.LogPath, st.TicketNumber, st.TicketNumber), nil
	default:
		return fmt.Sprintf("Next steps for ticket %s:\n  1. Check status: auto-pr status %s\n  2. Continue: auto-pr resume %s", st.TicketNumber, st.TicketNumber, st.TicketNumber), nil
	}
}

func (o *Orchestrator) CleanupTicket(ctx context.Context, ticketNumber string) error {
	st, err := o.Store.LoadState(ticketNumber)
	if err != nil {
		return err
	}
	_ = gitutil.WorktreeRemove(ctx, o.RepoRoot, st.WorktreePath)
	if err := o.Store.RemoveTicketDir(ticketNumber); err != nil {
		return err
	}
	fmt.Printf("cleaned ticket %s\n", ticketNumber)
	return nil
}

func (o *Orchestrator) CleanupDone(ctx context.Context) error {
	tickets, err := o.Store.ListTicketDirs()
	if err != nil {
		return err
	}
	sort.Strings(tickets)
	for _, ticket := range tickets {
		st, err := o.Store.LoadState(ticket)
		if err != nil {
			continue
		}
		if st.Status == models.StateDone {
			if err := o.CleanupTicket(ctx, ticket); err != nil {
				fmt.Fprintf(os.Stderr, "cleanup %s failed: %v\n", ticket, err)
			}
		}
	}
	return nil
}

func (o *Orchestrator) CleanupAll(ctx context.Context) error {
	tickets, err := o.Store.ListTicketDirs()
	if err != nil {
		return err
	}
	sort.Strings(tickets)
	for _, ticket := range tickets {
		if err := o.CleanupTicket(ctx, ticket); err != nil {
			fmt.Fprintf(os.Stderr, "cleanup %s failed: %v\n", ticket, err)
		}
	}
	return nil
}

func (o *Orchestrator) GeneratePR(ctx context.Context, ticketNumber string) error {
	st, err := o.Store.LoadState(ticketNumber)
	if err != nil {
		return err
	}
	if err := o.validatePRPrereqs(ctx, st); err != nil {
		return err
	}
	t, err := o.Store.LoadTicket(ticketNumber)
	if err != nil {
		return err
	}
	if err := o.generatePR(ctx, &st, t); err != nil {
		return err
	}
	st.Status = models.StateDone
	if err := o.Store.SaveState(ticketNumber, st); err != nil {
		return err
	}
	return nil
}

func (o *Orchestrator) ApplyPRComments(ctx context.Context, ticketNumber string) error {
	st, err := o.Store.LoadState(ticketNumber)
	if err != nil {
		return err
	}
	if strings.TrimSpace(st.PRURL) == "" {
		return fmt.Errorf("ticket %s has no PR URL; create the PR first with auto-pr pr %s", ticketNumber, ticketNumber)
	}
	ticket, err := o.Store.LoadTicket(ticketNumber)
	if err != nil {
		return err
	}

	comments, err := o.fetchOpenPRComments(ctx, st.PRURL, st.WorktreePath)
	if err != nil {
		return err
	}
	if len(comments) == 0 {
		_ = markdown.AppendSection(st.LogPath, "PR Comments", "No open PR review comments found.")
		fmt.Printf("ticket %s has no open PR review comments\n", ticketNumber)
		return nil
	}

	commentContext := formatPRCommentContext(comments)
	_ = markdown.AppendSection(st.LogPath, "PR Comments", commentContext)

	prevStatus := st.Status
	st.Status = models.StateImplementing
	if err := o.Store.SaveState(st.TicketNumber, st); err != nil {
		return err
	}

	impl, err := o.Provider.Implement(ctx, providers.ImplementRequest{
		Ticket:            ticket,
		RepoPath:          o.RepoRoot,
		WorktreePath:      st.WorktreePath,
		GuidelinesPath:    config.ResolveGuidelinesPath(o.RepoRoot, o.Cfg),
		LogPath:           st.LogPath,
		ProposalPath:      st.ProposalPath,
		FinalSolutionPath: st.FinalPath,
		FailureContext: "Address the following open GitHub PR review comments.\n" +
			"Only implement requested changes that are relevant and correct.\n\n" + commentContext,
	}, st.ProviderDirPath)
	if err != nil {
		st.Status = models.StateFailed
		st.LastError = err.Error()
		_ = markdown.AppendSection(st.LogPath, "Apply PR Comments Failed", err.Error())
		_ = o.Store.SaveState(st.TicketNumber, st)
		return err
	}
	if err := markdown.AppendSection(st.LogPath, "Apply PR Comments", impl.RawOut); err != nil {
		return err
	}
	if err := markdown.Write(st.FinalPath, impl.Summary); err != nil {
		return err
	}

	st.Status = models.StateValidating
	if err := o.Store.SaveState(st.TicketNumber, st); err != nil {
		return err
	}
	ok, checkOutput, err := o.runChecks(ctx, st)
	if err != nil {
		return err
	}
	if !ok {
		st.Status = models.StateFailed
		st.LastError = "checks failed after applying PR comments"
		_ = markdown.AppendSection(st.LogPath, "Validation Failed", checkOutput)
		_ = o.Store.SaveState(st.TicketNumber, st)
		return fmt.Errorf("ticket %s: checks failed after applying PR comments", st.TicketNumber)
	}
	_ = markdown.AppendSection(st.LogPath, "Validation", "All checks passed after applying PR comments.")

	if err := o.ensureCommitForTicket(ctx, st, ticket); err != nil {
		st.Status = models.StateFailed
		st.LastError = err.Error()
		_ = markdown.AppendSection(st.LogPath, "Commit Failed", err.Error())
		_ = o.Store.SaveState(st.TicketNumber, st)
		return err
	}
	if err := gitutil.PushBranch(ctx, st.WorktreePath, st.BranchName); err != nil {
		msg := fmt.Sprintf("failed to push updates for ticket %s: %v", st.TicketNumber, err)
		st.Status = models.StateFailed
		st.LastError = msg
		_ = markdown.AppendSection(st.LogPath, "PR Update Push Failed", msg)
		_ = o.Store.SaveState(st.TicketNumber, st)
		return fmt.Errorf(msg)
	}
	_ = markdown.AppendSection(st.LogPath, "PR Updated", fmt.Sprintf("Pushed updates for %s", st.PRURL))

	st.Status = prevStatus
	st.LastError = ""
	if st.Status == "" {
		st.Status = models.StateDone
	}
	if err := o.Store.SaveState(st.TicketNumber, st); err != nil {
		return err
	}
	return nil
}

func (o *Orchestrator) initOrLoad(ctx context.Context, ticketNumber string) (*models.TicketState, error) {
	if st, err := o.Store.LoadState(ticketNumber); err == nil {
		if _, ticketErr := o.Store.LoadTicket(ticketNumber); ticketErr == nil {
			return &st, nil
		} else if !os.IsNotExist(ticketErr) {
			return nil, ticketErr
		}
	}

	paths := o.Store.Paths(ticketNumber)
	if _, err := o.Store.EnsureTicketDir(ticketNumber); err != nil {
		return nil, err
	}
	ticket, rawTicket, err := o.Provider.GetTicket(ctx, ticketNumber, o.RepoRoot, paths["providerDir"])
	if err != nil {
		return nil, err
	}
	ticket.Number = ticketNumber
	st := models.NewTicketState(ticketNumber)
	st.ProposalPath = paths["proposal"]
	st.FinalPath = paths["final"]
	st.LogPath = paths["log"]
	st.PRPath = paths["pr"]
	st.ChecksLogPath = paths["checks"]
	st.TicketJSONPath = paths["ticket"]
	st.ProviderDirPath = paths["providerDir"]
	st.BranchName = branchName(ticket)

	worktreePath, err := worktree.Ensure(ctx, o.RepoRoot, o.Cfg.StateDirName, ticketNumber, st.BranchName, o.Cfg.BaseBranch)
	if err != nil {
		return nil, err
	}
	st.WorktreePath = worktreePath

	if _, err := o.Store.SaveTicket(ticketNumber, ticket); err != nil {
		return nil, err
	}
	_ = markdown.AppendSection(st.LogPath, "Ticket Fetch (Provider)", rawTicket)
	if err := markdown.AppendSection(st.LogPath, "Ticket Loaded", fmt.Sprintf("#%s %s\n\n%s\n\nAcceptance Criteria:\n%s\n\nRelated Context:\n%s", ticket.Number, ticket.Title, ticket.Description, ticket.AcceptanceCriteria, relatedContextSummary(ticket))); err != nil {
		return nil, err
	}
	if err := o.Store.SaveState(ticketNumber, st); err != nil {
		return nil, err
	}
	return &st, nil
}

func (o *Orchestrator) investigate(ctx context.Context, st *models.TicketState) error {
	ticket, err := o.Store.LoadTicket(st.TicketNumber)
	if err != nil {
		return err
	}

	st.Status = models.StateInvestigating
	if err := o.Store.SaveState(st.TicketNumber, *st); err != nil {
		return err
	}

	res, err := o.Provider.Investigate(ctx, providers.InvestigateRequest{
		Ticket:         ticket,
		RepoPath:       o.RepoRoot,
		WorktreePath:   st.WorktreePath,
		GuidelinesPath: config.ResolveGuidelinesPath(o.RepoRoot, o.Cfg),
		LogPath:        st.LogPath,
		ProposalPath:   st.ProposalPath,
		Feedback:       st.LastFeedback,
	}, st.ProviderDirPath)
	if err != nil {
		st.Status = models.StateFailed
		st.LastError = err.Error()
		_ = o.Store.SaveState(st.TicketNumber, *st)
		_ = markdown.AppendSection(st.LogPath, "Investigation Failed", err.Error())
		return err
	}

	if err := markdown.Write(st.ProposalPath, res.Proposal); err != nil {
		return err
	}
	if err := markdown.AppendSection(st.LogPath, "Investigation", res.RawOut); err != nil {
		return err
	}
	st.Status = models.StateProposalReady
	if err := o.Store.SaveState(st.TicketNumber, *st); err != nil {
		return err
	}
	st.Status = models.StateWaitingForHuman
	st.Approved = false
	if err := o.Store.SaveState(st.TicketNumber, *st); err != nil {
		return err
	}
	fmt.Printf("ticket %s proposal ready: %s\n", st.TicketNumber, st.ProposalPath)
	return nil
}

func (o *Orchestrator) implementationPipeline(ctx context.Context, st *models.TicketState) error {
	ticket, err := o.Store.LoadTicket(st.TicketNumber)
	if err != nil {
		return err
	}
	var failureContext string

	for {
		st.Status = models.StateImplementing
		if err := o.Store.SaveState(st.TicketNumber, *st); err != nil {
			return err
		}

		impl, err := o.Provider.Implement(ctx, providers.ImplementRequest{
			Ticket:            ticket,
			RepoPath:          o.RepoRoot,
			WorktreePath:      st.WorktreePath,
			GuidelinesPath:    config.ResolveGuidelinesPath(o.RepoRoot, o.Cfg),
			LogPath:           st.LogPath,
			ProposalPath:      st.ProposalPath,
			FinalSolutionPath: st.FinalPath,
			FailureContext:    failureContext,
		}, st.ProviderDirPath)
		if err != nil {
			st.Status = models.StateFailed
			st.LastError = err.Error()
			_ = markdown.AppendSection(st.LogPath, "Implementation Failed", err.Error())
			_ = o.Store.SaveState(st.TicketNumber, *st)
			return err
		}

		if err := markdown.AppendSection(st.LogPath, "Implementation", impl.RawOut); err != nil {
			return err
		}
		if err := markdown.Write(st.FinalPath, impl.Summary); err != nil {
			return err
		}

		st.Status = models.StateValidating
		if err := o.Store.SaveState(st.TicketNumber, *st); err != nil {
			return err
		}
		ok, checkOutput, err := o.runChecks(ctx, *st)
		if err != nil {
			return err
		}
		if ok {
			_ = markdown.AppendSection(st.LogPath, "Validation", "All checks passed.")
			break
		}

		failureContext = checkOutput
		_ = markdown.AppendSection(st.LogPath, "Validation Failed", checkOutput)
		st.FixAttempts++
		if st.FixAttempts > o.Cfg.MaxFixAttempts {
			st.Status = models.StateFailed
			st.LastError = "checks failed after max attempts"
			_ = o.Store.SaveState(st.TicketNumber, *st)
			return fmt.Errorf("ticket %s: checks failed after %d attempts", st.TicketNumber, st.FixAttempts)
		}
		if err := o.Store.SaveState(st.TicketNumber, *st); err != nil {
			return err
		}
	}

	st.Status = models.StatePRReady
	if err := o.ensureCommitForTicket(ctx, *st, ticket); err != nil {
		st.Status = models.StateFailed
		st.LastError = err.Error()
		_ = o.Store.SaveState(st.TicketNumber, *st)
		_ = markdown.AppendSection(st.LogPath, "Commit Failed", err.Error())
		return err
	}
	if err := o.Store.SaveState(st.TicketNumber, *st); err != nil {
		return err
	}
	if err := o.generatePR(ctx, st, ticket); err != nil {
		return err
	}
	st.Status = models.StateDone
	if err := o.Store.SaveState(st.TicketNumber, *st); err != nil {
		return err
	}
	fmt.Printf("ticket %s done. PR markdown: %s\n", st.TicketNumber, st.PRPath)
	return nil
}

func (o *Orchestrator) generatePR(ctx context.Context, st *models.TicketState, ticket models.Ticket) error {
	pr, err := o.Provider.SummarizePR(ctx, providers.PRRequest{
		Ticket:            ticket,
		WorktreePath:      st.WorktreePath,
		LogPath:           st.LogPath,
		ProposalPath:      st.ProposalPath,
		FinalSolutionPath: st.FinalPath,
		ChecksLogPath:     st.ChecksLogPath,
	}, st.ProviderDirPath)
	if err != nil {
		fallback, ferr := o.localPR(ticket, *st)
		if ferr != nil {
			return err
		}
		pr.Body = fallback
	}
	if err := markdown.Write(st.PRPath, pr.Body); err != nil {
		return err
	}
	_ = markdown.AppendSection(st.LogPath, "PR Description", fmt.Sprintf("generated at %s", st.PRPath))

	if o.Cfg.CreatePR {
		title := fmt.Sprintf("sc-%s: %s", ticket.Number, ticket.Title)
		if err := o.ensureBranchHasCommits(ctx, *st); err != nil {
			_ = markdown.AppendSection(st.LogPath, "PR Create Failed", err.Error())
			return err
		}
		if err := gitutil.PushBranch(ctx, st.WorktreePath, st.BranchName); err != nil {
			msg := fmt.Sprintf("failed to push branch %s before PR creation: %v\n\nNext steps:\n  1. Verify remote/auth: git -C %s remote -v && gh auth status\n  2. Push manually: git -C %s push -u origin %s\n  3. Retry: auto-pr pr %s", st.BranchName, err, st.WorktreePath, st.WorktreePath, st.BranchName, st.TicketNumber)
			_ = markdown.AppendSection(st.LogPath, "PR Push Failed", msg)
			return fmt.Errorf(msg)
		}
		url, err := gitutil.CreatePR(ctx, st.WorktreePath, title, st.PRPath, o.Cfg.BaseBranch)
		if err != nil {
			msg := fmt.Sprintf("failed to create PR for ticket %s on branch %s: %v\n\nNext steps:\n  1. Verify auth: gh auth status\n  2. Ensure branch is pushed: git -C %s push -u origin %s\n  3. Retry: auto-pr pr %s", st.TicketNumber, st.BranchName, err, st.WorktreePath, st.BranchName, st.TicketNumber)
			_ = markdown.AppendSection(st.LogPath, "PR Create Failed", msg)
			return fmt.Errorf(msg)
		}
		st.PRURL = strings.TrimSpace(url)
		_ = markdown.AppendSection(st.LogPath, "PR Created", url)
	}
	return nil
}

func (o *Orchestrator) runChecks(ctx context.Context, st models.TicketState) (bool, string, error) {
	commands := make([]string, 0, len(o.Cfg.FormatCommands)+len(o.Cfg.LintCommands)+len(o.Cfg.CheckCommands))
	commands = append(commands, o.Cfg.FormatCommands...)
	commands = append(commands, o.Cfg.LintCommands...)
	commands = append(commands, o.Cfg.CheckCommands...)
	if len(commands) == 0 {
		return true, "no checks configured", nil
	}

	var b strings.Builder
	allOK := true
	for _, cmd := range commands {
		res, err := shell.Run(ctx, st.WorktreePath, nil, "", "/bin/zsh", "-lc", cmd)
		fmt.Fprintf(&b, "\n$ %s\n", cmd)
		b.WriteString(res.Stdout)
		if strings.TrimSpace(res.Stderr) != "" {
			b.WriteString("\n[stderr]\n")
			b.WriteString(res.Stderr)
		}
		if err != nil {
			allOK = false
			fmt.Fprintf(&b, "\n[exit] failed: %v\n", err)
		}
	}
	if err := os.WriteFile(st.ChecksLogPath, []byte(b.String()), 0o644); err != nil {
		return false, "", err
	}
	return allOK, b.String(), nil
}

func (o *Orchestrator) printStatus(ticketNumber string) error {
	st, err := o.Store.LoadState(ticketNumber)
	if err != nil {
		return err
	}
	fmt.Printf("ticket %s\n", ticketNumber)
	fmt.Printf("  state: %s\n", st.Status)
	fmt.Printf("  approved: %v\n", st.Approved)
	fmt.Printf("  branch: %s\n", st.BranchName)
	fmt.Printf("  worktree: %s\n", st.WorktreePath)
	fmt.Printf("  proposal: %s\n", st.ProposalPath)
	fmt.Printf("  pr: %s\n", st.PRPath)
	if strings.TrimSpace(st.PRURL) != "" {
		fmt.Printf("  pr_url: %s\n", st.PRURL)
	}
	if st.LastError != "" {
		fmt.Printf("  last_error: %s\n", st.LastError)
	}
	tail := strings.TrimSpace(markdown.Tail(st.LogPath, 12))
	if tail != "" {
		fmt.Printf("  log_tail:\n%s\n", indent(tail, "    "))
	}
	return nil
}

func (o *Orchestrator) localPR(ticket models.Ticket, st models.TicketState) (string, error) {
	proposal, _ := os.ReadFile(st.ProposalPath)
	final, _ := os.ReadFile(st.FinalPath)
	checks, _ := os.ReadFile(st.ChecksLogPath)
	checksText := strings.TrimSpace(string(checks))
	testFailuresSection := ""
	if hasCheckFailures(checksText) {
		testFailuresSection = fmt.Sprintf(`
# Test Failures / Blockers

`+"```"+`
%s
`+"```"+`
`, checksText)
	}
	body := fmt.Sprintf(`# Summary

Ticket #%s: %s

# Problem Being Solved

%s

# Implementation Overview

%s

# Risks / Follow-ups

See final solution notes.
%s
`, ticket.Number, ticket.Title, strings.TrimSpace(string(proposal)), strings.TrimSpace(string(final)), testFailuresSection)
	return body, nil
}

func hasCheckFailures(checks string) bool {
	if checks == "" || checks == "no checks configured" {
		return false
	}
	s := strings.ToLower(checks)
	return strings.Contains(s, "[exit] failed:") || strings.Contains(s, "failed")
}

func branchName(ticket models.Ticket) string {
	slug := slugify(ticket.Title)
	if slug == "" {
		slug = "ticket"
	}
	return fmt.Sprintf("sc-%s-%s", ticket.Number, slug)
}

var slugNonAlphaNum = regexp.MustCompile(`[^a-z0-9\s-]`)
var slugSpace = regexp.MustCompile(`\s+`)
var slugDashes = regexp.MustCompile(`-+`)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugNonAlphaNum.ReplaceAllString(s, "")
	s = slugSpace.ReplaceAllString(s, "-")
	s = slugDashes.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func indent(s, pref string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = pref + l
	}
	return strings.Join(lines, "\n")
}

func relatedContextSummary(ticket models.Ticket) string {
	var parts []string
	if ticket.ParentTicket != nil {
		parts = append(parts, fmt.Sprintf("Parent Ticket: %s (%s)", ticket.ParentTicket.Title, ticket.ParentTicket.URL))
	}
	if ticket.Epic != nil {
		parts = append(parts, fmt.Sprintf("Epic: %s (%s)", ticket.Epic.Title, ticket.Epic.URL))
	}
	if len(parts) == 0 {
		return "None"
	}
	return strings.Join(parts, "\n")
}

func EnsureStateIgnored(repoRoot, stateDirName string) error {
	ignorePath := filepath.Join(repoRoot, ".gitignore")
	entry := stateDirName + "/"
	b, err := os.ReadFile(ignorePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if strings.Contains(string(b), entry) {
		return nil
	}
	f, err := os.OpenFile(ignorePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if len(b) > 0 && !strings.HasSuffix(string(b), "\n") {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	_, err = f.WriteString(entry + "\n")
	return err
}

func (o *Orchestrator) ensureCommitForTicket(ctx context.Context, st models.TicketState, ticket models.Ticket) error {
	hasChanges, err := gitutil.HasChanges(ctx, st.WorktreePath)
	if err != nil {
		return fmt.Errorf("check git status before commit: %w", err)
	}
	if !hasChanges {
		return fmt.Errorf("implementation finished but produced no file changes to commit for ticket %s", st.TicketNumber)
	}
	msg := fmt.Sprintf("sc-%s: %s", ticket.Number, ticket.Title)
	if err := gitutil.CommitAll(ctx, st.WorktreePath, msg); err != nil {
		return fmt.Errorf("create commit: %w", err)
	}
	_ = markdown.AppendSection(st.LogPath, "Commit Created", msg)
	return nil
}

func (o *Orchestrator) validatePRPrereqs(ctx context.Context, st models.TicketState) error {
	if st.Status == models.StateWaitingForHuman && !st.Approved {
		return fmt.Errorf("ticket %s is waiting for human approval.\n\nNext steps:\n  1. Review proposal: %s\n  2. Approve to continue: auto-pr approve %s\n  3. Or send more feedback: auto-pr feedback %s --message \"...\"", st.TicketNumber, st.ProposalPath, st.TicketNumber, st.TicketNumber)
	}
	if st.Status == models.StateFailed {
		return fmt.Errorf("ticket %s is in failed state.\n\nNext steps:\n  1. Inspect log: %s\n  2. Fix issue or provide feedback: auto-pr feedback %s --message \"...\"\n  3. Resume: auto-pr resume %s", st.TicketNumber, st.LogPath, st.TicketNumber, st.TicketNumber)
	}
	// If work is not done yet, guide users to resume instead of forcing PR creation.
	if st.Status != models.StatePRReady && st.Status != models.StateDone {
		return fmt.Errorf("ticket %s is in state %s and not ready for PR yet.\n\nNext steps:\n  1. Continue workflow: auto-pr resume %s\n  2. Check progress: auto-pr status %s", st.TicketNumber, st.Status, st.TicketNumber, st.TicketNumber)
	}
	return nil
}

type ghReviewThreadComment struct {
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	Body string `json:"body"`
	Path string `json:"path"`
}

type ghReviewThread struct {
	IsResolved bool `json:"isResolved"`
	Comments   struct {
		Nodes []ghReviewThreadComment `json:"nodes"`
	} `json:"comments"`
}

type ghReviewThreadsResponse struct {
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
	Data struct {
		Repository struct {
			PullRequest struct {
				ReviewThreads struct {
					Nodes []ghReviewThread `json:"nodes"`
				} `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

type prComment struct {
	Author string
	Path   string
	Body   string
}

func (o *Orchestrator) fetchOpenPRComments(ctx context.Context, prURL, worktreePath string) ([]prComment, error) {
	owner, repo, number, err := parsePRURL(prURL)
	if err != nil {
		return nil, err
	}
	query := `query($owner:String!, $repo:String!, $number:Int!) {
  repository(owner:$owner, name:$repo) {
    pullRequest(number:$number) {
      reviewThreads(first:100) {
        nodes {
          isResolved
          comments(first:20) {
            nodes {
              author { login }
              body
              path
            }
          }
        }
      }
    }
  }
}`
	var payload struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables"`
	}
	payload.Query = query
	payload.Variables = map[string]interface{}{
		"owner":  owner,
		"repo":   repo,
		"number": number,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	res, err := shell.Run(ctx, worktreePath, nil, string(b), "gh", "api", "graphql", "--input", "-")
	if err != nil {
		return nil, fmt.Errorf("fetch open PR comments: %w", err)
	}

	var out ghReviewThreadsResponse
	if err := json.Unmarshal([]byte(res.Stdout), &out); err != nil {
		return nil, fmt.Errorf("parse PR comments response: %w", err)
	}
	if len(out.Errors) > 0 {
		msg := strings.TrimSpace(out.Errors[0].Message)
		if msg == "" {
			msg = "unknown graphql error"
		}
		return nil, fmt.Errorf("fetch open PR comments: %s", msg)
	}
	threads := out.Data.Repository.PullRequest.ReviewThreads.Nodes
	comments := make([]prComment, 0)
	for _, t := range threads {
		if t.IsResolved {
			continue
		}
		for _, c := range t.Comments.Nodes {
			body := strings.TrimSpace(c.Body)
			if body == "" {
				continue
			}
			comments = append(comments, prComment{
				Author: strings.TrimSpace(c.Author.Login),
				Path:   strings.TrimSpace(c.Path),
				Body:   body,
			})
		}
	}
	return comments, nil
}

var prURLPattern = regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+)/pull/([0-9]+)`)

func parsePRURL(u string) (owner, repo string, number int, err error) {
	m := prURLPattern.FindStringSubmatch(strings.TrimSpace(u))
	if len(m) != 4 {
		return "", "", 0, fmt.Errorf("unsupported PR URL format: %s", u)
	}
	n, convErr := strconv.Atoi(m[3])
	if convErr != nil {
		return "", "", 0, fmt.Errorf("parse PR number: %w", convErr)
	}
	return m[1], m[2], n, nil
}

func formatPRCommentContext(comments []prComment) string {
	var b strings.Builder
	for i, c := range comments {
		fmt.Fprintf(&b, "%d. ", i+1)
		if c.Path != "" {
			fmt.Fprintf(&b, "[%s] ", c.Path)
		}
		if c.Author != "" {
			fmt.Fprintf(&b, "@%s: ", c.Author)
		}
		b.WriteString(c.Body)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func (o *Orchestrator) ensureBranchHasCommits(ctx context.Context, st models.TicketState) error {
	candidates := []string{}
	if strings.TrimSpace(o.Cfg.BaseBranch) != "" {
		candidates = append(candidates, strings.TrimSpace(o.Cfg.BaseBranch))
	}
	candidates = append(candidates, "origin/HEAD", "origin/main", "main")

	for _, base := range candidates {
		ahead, err := gitutil.AheadCount(ctx, st.WorktreePath, base)
		if err != nil {
			continue
		}
		if ahead > 0 {
			return nil
		}
		return fmt.Errorf("branch %s has no commits ahead of %s, so a PR cannot be created.\n\nNext steps:\n  1. Check whether implementation actually changed code in %s\n  2. If code is missing, request another pass: auto-pr feedback %s --message \"implementation produced no branch changes; please apply code changes\" && auto-pr resume %s\n  3. If changes exist but are uncommitted, commit/push manually: git -C %s add -A && git -C %s commit -m \"sc-%s: implement\" && git -C %s push -u origin %s\n  4. Retry PR: auto-pr pr %s", st.BranchName, base, st.WorktreePath, st.TicketNumber, st.TicketNumber, st.WorktreePath, st.WorktreePath, st.TicketNumber, st.WorktreePath, st.BranchName, st.TicketNumber)
	}

	// If base detection failed, fall back to checking any changes in working tree.
	res, err := shell.Run(ctx, st.WorktreePath, nil, "", "git", "status", "--short")
	if err == nil && strings.TrimSpace(res.Stdout) != "" {
		return nil
	}
	return fmt.Errorf("could not confirm branch %s has commits suitable for PR creation.\n\nNext steps:\n  1. Ensure there is committed work on branch %s\n  2. Push branch: git -C %s push -u origin %s\n  3. Retry: auto-pr pr %s", st.BranchName, st.BranchName, st.WorktreePath, st.BranchName, st.TicketNumber)
}
