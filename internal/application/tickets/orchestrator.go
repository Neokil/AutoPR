package tickets

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Neokil/AutoPR/internal/config"
	ticketdomain "github.com/Neokil/AutoPR/internal/domain/ticket"
	"github.com/Neokil/AutoPR/internal/gitutil"
	"github.com/Neokil/AutoPR/internal/markdown"
	"github.com/Neokil/AutoPR/internal/ports"
	"github.com/Neokil/AutoPR/internal/providers"
	"github.com/Neokil/AutoPR/internal/shell"
	"github.com/Neokil/AutoPR/internal/state"
	"github.com/Neokil/AutoPR/internal/workflow"
	"github.com/Neokil/AutoPR/internal/worktree"
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

func NewWithStore(cfg config.Config, repoRoot string, store ports.StateStore, provider providers.AIProvider,
) *Orchestrator {
	return &Orchestrator{
		Cfg:      cfg,
		RepoRoot: repoRoot,
		Store:    store,
		Provider: provider,
	}
}

// StartFlow begins or re-runs the workflow for a ticket. Creates a worktree on
// first call; re-runs the current state if the ticket is already waiting or failed.
func (o *Orchestrator) StartFlow(ctx context.Context, ticketNumber string) error {
	wf, err := workflow.Load(o.RepoRoot)
	if err != nil {
		return fmt.Errorf("load workflow: %w", err)
	}

	st, err := o.Store.LoadState(ticketNumber)
	if os.IsNotExist(err) {
		st = ticketdomain.NewState(ticketNumber)
		if err := o.Store.SaveState(ticketNumber, st); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if st.FlowStatus == ticketdomain.FlowStatusDone || st.FlowStatus == ticketdomain.FlowStatusCancelled {
		slog.Info("skipping ticket", "ticket", ticketNumber, "status", st.FlowStatus)
		return nil
	}
	if st.FlowStatus == ticketdomain.FlowStatusRunning {
		return fmt.Errorf("ticket %s: %w", ticketNumber, ErrTicketRunning)
	}
	if err := o.ensureWorktreeAndContext(ctx, &st); err != nil {
		return err
	}

	// Determine which state to run.
	stateCfg, err := resolveStateForStart(st, wf)
	if err != nil {
		return err
	}

	slog.Info("starting flow", "ticket", ticketNumber, "state", stateCfg.Name)
	return o.runState(ctx, &st, stateCfg)
}

// ApplyAction applies the named action to a ticket that is waiting for input.
func (o *Orchestrator) ApplyAction(ctx context.Context, ticketNumber, actionLabel, message string) error {
	wf, err := workflow.Load(o.RepoRoot)
	if err != nil {
		return fmt.Errorf("load workflow: %w", err)
	}

	st, err := o.Store.LoadState(ticketNumber)
	if err != nil {
		return err
	}
	if st.FlowStatus != ticketdomain.FlowStatusWaiting {
		return fmt.Errorf("ticket %s (status: %s): %w", ticketNumber, st.FlowStatus, ErrTicketNotWaiting)
	}

	stateCfg, ok := wf.StateByName(st.CurrentState)
	if !ok {
		return fmt.Errorf("state %q: %w", st.CurrentState, ErrStateNotFound)
	}

	var action *workflow.ActionConfig
	for i, a := range stateCfg.Actions {
		if strings.EqualFold(a.Label, actionLabel) {
			action = &stateCfg.Actions[i]
			break
		}
	}
	if action == nil {
		labels := make([]string, len(stateCfg.Actions))
		for i, a := range stateCfg.Actions {
			labels[i] = a.Label
		}
		return fmt.Errorf("action %q in state %q (available: %s): %w",
			actionLabel, st.CurrentState, strings.Join(labels, ", "), ErrActionNotFound)
	}

	slog.Info("applying action", "ticket", ticketNumber, "action", actionLabel, "state", st.CurrentState)
	return o.dispatchAction(ctx, &st, wf, *action, message)
}

func (o *Orchestrator) MoveToState(ctx context.Context, ticketNumber, target string) error {
	wf, err := workflow.Load(o.RepoRoot)
	if err != nil {
		return fmt.Errorf("load workflow: %w", err)
	}
	if strings.TrimSpace(target) == "" {
		return ErrTargetStateRequired
	}

	st, err := o.Store.LoadState(ticketNumber)
	if os.IsNotExist(err) {
		st = ticketdomain.NewState(ticketNumber)
		if err := o.Store.SaveState(ticketNumber, st); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if st.FlowStatus == ticketdomain.FlowStatusRunning {
		return fmt.Errorf("ticket %s: %w", ticketNumber, ErrTicketRunning)
	}
	if err := o.ensureWorktreeAndContext(ctx, &st); err != nil {
		return err
	}

	slog.Info("force moving to state", "ticket", ticketNumber, "target", target)
	return o.transitionTo(ctx, &st, wf, target)
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
			slog.Error("status failed", "ticket", t, "err", err)
		}
	}
	return nil
}

func (o *Orchestrator) NextSteps(ticketNumber string) (string, error) {
	st, err := o.Store.LoadState(ticketNumber)
	if err != nil {
		return "", err
	}
	wf, _ := workflow.Load(o.RepoRoot)
	return buildNextSteps(st, wf), nil
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
	slog.Info("cleaned ticket", "ticket", ticketNumber)
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
		if st.FlowStatus == ticketdomain.FlowStatusDone {
			if err := o.CleanupTicket(ctx, ticket); err != nil {
				slog.Error("cleanup failed", "ticket", ticket, "err", err)
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
			slog.Error("cleanup failed", "ticket", ticket, "err", err)
		}
	}
	return nil
}

func (o *Orchestrator) ensureWorktreeAndContext(ctx context.Context, st *ticketdomain.State) error {
	if st.WorktreePath == "" {
		branchName := "auto-pr/" + st.TicketNumber
		slog.Info("creating worktree", "ticket", st.TicketNumber, "branch", branchName)
		wtPath, err := worktree.Ensure(ctx, o.RepoRoot, o.Cfg.StateDirName, st.TicketNumber, branchName, o.Cfg.BaseBranch)
		if err != nil {
			return fmt.Errorf("create worktree: %w", err)
		}
		st.BranchName = branchName
		st.WorktreePath = wtPath
		if err := o.Store.SaveState(st.TicketNumber, *st); err != nil {
			return err
		}
	}

	autoPRDir := filepath.Join(st.WorktreePath, ".auto-pr")
	if err := os.MkdirAll(autoPRDir, 0o755); err != nil { //nolint:gosec // G301: 0755 correct for project directories
		return fmt.Errorf("create .auto-pr dir: %w", err)
	}

	contextPath := st.ArtifactPath("context.md")
	if _, statErr := os.Stat(contextPath); os.IsNotExist(statErr) {
		guidelinesPath := config.ResolveGuidelinesPath(o.RepoRoot, o.Cfg)
		content := fmt.Sprintf("Ticket: %s\nWorktree: %s\nRepo: %s\nGuidelines: %s\n",
			st.TicketNumber, st.WorktreePath, o.RepoRoot, guidelinesPath)
		if err := os.WriteFile(contextPath, []byte(content), 0o644); err != nil { //nolint:gosec // G306: 0644 intentional for user-readable context files
			return err
		}
	}
	return nil
}

// --- internal helpers ---

func (o *Orchestrator) runState(ctx context.Context, st *ticketdomain.State, stateCfg workflow.StateConfig) error {
	slog.Info("running state", "ticket", st.TicketNumber, "state", stateCfg.Name)
	run, err := startStateRun(st, stateCfg)
	if err != nil {
		return err
	}
	logPath := st.ResolveRef(run.LogRef)

	st.CurrentState = stateCfg.Name
	st.FlowStatus = ticketdomain.FlowStatusRunning
	st.LastError = ""
	if err := o.Store.SaveState(st.TicketNumber, *st); err != nil {
		return err
	}
	if err := o.prepareRunContext(*st, stateCfg, run); err != nil {
		return o.failState(st, err)
	}

	if err := o.runCommands(ctx, st.WorktreePath, stateCfg.PrePromptCommands, logPath, "Pre-prompt"); err != nil {
		return o.failState(st, err)
	}

	promptContent, err := workflow.ReadPrompt(o.RepoRoot, stateCfg.Prompt)
	if err != nil {
		return o.failState(st, fmt.Errorf("read prompt %s: %w", stateCfg.Prompt, err))
	}

	promptPath := st.RunPath(run.ID, "prompt.md")
	if err := os.WriteFile(promptPath, promptContent, 0o644); err != nil { //nolint:gosec // G306: 0644 intentional for user-readable prompt files
		return o.failState(st, err)
	}

	runtimeDir := st.RunPath(run.ID, "provider")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil { //nolint:gosec // G301: 0755 correct for project directories
		return o.failState(st, err)
	}

	slog.Info("executing provider", "ticket", st.TicketNumber, "state", stateCfg.Name)
	result, err := o.Provider.Execute(ctx, providers.ExecuteRequest{
		PromptPath: promptPath,
		WorkDir:    st.WorktreePath,
		RuntimeDir: runtimeDir,
	})
	rawLogPath := st.RunPath(run.ID, "raw-provider.log")
	_ = os.WriteFile(rawLogPath, []byte(result.RawOutput+"\n\n[stderr]\n"+result.Stderr), 0o644) //nolint:gosec // G306: 0644 intentional for user-readable log files
	if err != nil {
		_ = markdown.AppendSection(logPath, stateCfg.Name+" Failed", err.Error())
		return o.failState(st, err)
	}

	_ = markdown.AppendSection(logPath, stateCfg.Name, result.RawOutput)

	if err := o.runCommands(ctx, st.WorktreePath, stateCfg.PostPromptCommands, logPath, "Post-prompt"); err != nil {
		return o.failState(st, err)
	}

	// Remove feedback.md so stale feedback is not visible to the next run.
	_ = os.Remove(st.ArtifactPath("feedback.md"))

	slog.Info("state done, waiting for action", "ticket", st.TicketNumber, "state", stateCfg.Name)
	st.FlowStatus = ticketdomain.FlowStatusWaiting
	return o.Store.SaveState(st.TicketNumber, *st)
}

func (o *Orchestrator) failState(st *ticketdomain.State, cause error) error {
	slog.Error("state failed", "ticket", st.TicketNumber, "state", st.CurrentState, "err", cause)
	st.FlowStatus = ticketdomain.FlowStatusFailed
	st.LastError = cause.Error()
	_ = o.Store.SaveState(st.TicketNumber, *st)
	return cause
}

func (o *Orchestrator) dispatchAction(
	ctx context.Context, st *ticketdomain.State, wf workflow.WorkflowConfig, action workflow.ActionConfig, message string,
) error {
	logPath := st.CurrentRunLogPath()
	_ = markdown.AppendSection(logPath, "Human Action: "+action.Label, "")

	switch action.Type {
	case workflow.ActionProvideFeedback:
		return o.writeFeedbackAndRerun(ctx, st, wf, message)
	case workflow.ActionMoveToState:
		return o.transitionTo(ctx, st, wf, action.Target)
	case workflow.ActionRunScript:
		return o.executeScript(ctx, st, wf, action)
	default:
		return fmt.Errorf("action type %q: %w", action.Type, ErrUnknownActionType)
	}
}

func (o *Orchestrator) transitionTo(
	ctx context.Context, st *ticketdomain.State, wf workflow.WorkflowConfig, target string,
) error {
	if workflow.IsTerminal(target) {
		slog.Info("reached terminal state", "ticket", st.TicketNumber, "state", target)
		switch target {
		case "done":
			st.FlowStatus = ticketdomain.FlowStatusDone
		case "cancelled":
			st.FlowStatus = ticketdomain.FlowStatusCancelled
		default:
			st.FlowStatus = ticketdomain.FlowStatusFailed
		}
		return o.Store.SaveState(st.TicketNumber, *st)
	}
	slog.Info("transitioning to state", "ticket", st.TicketNumber, "target", target)
	stateCfg, ok := wf.StateByName(target)
	if !ok {
		return fmt.Errorf("state %q: %w", target, ErrTargetNotFound)
	}
	return o.runState(ctx, st, stateCfg)
}

func (o *Orchestrator) writeFeedbackAndRerun(
	ctx context.Context, st *ticketdomain.State, wf workflow.WorkflowConfig, message string,
) error {
	if strings.TrimSpace(message) == "" {
		return ErrFeedbackRequired
	}
	slog.Info("applying feedback", "ticket", st.TicketNumber, "state", st.CurrentState)
	feedbackPath := st.ArtifactPath("feedback.md")
	if err := os.WriteFile(feedbackPath, []byte(strings.TrimSpace(message)), 0o644); err != nil { //nolint:gosec // G306: 0644 intentional for user-readable feedback files
		return err
	}
	stateCfg, ok := wf.StateByName(st.CurrentState)
	if !ok {
		return fmt.Errorf("state %q: %w", st.CurrentState, ErrStateNotFound)
	}
	return o.runState(ctx, st, stateCfg)
}

func (o *Orchestrator) executeScript(
	ctx context.Context, st *ticketdomain.State, wf workflow.WorkflowConfig, action workflow.ActionConfig,
) error {
	logPath := st.CurrentRunLogPath()

	var out strings.Builder
	var scriptErr error
	for _, cmd := range action.Commands {
		res, err := shell.Run(ctx, st.WorktreePath, nil, "", "/bin/sh", "-c", cmd)
		output := res.Stdout
		if strings.TrimSpace(res.Stderr) != "" {
			output += "\n[stderr]\n" + res.Stderr
		}
		out.WriteString(output)
		_ = markdown.AppendSection(logPath, "Script: "+cmd, strings.TrimSpace(output))
		if err != nil {
			scriptErr = err
			break
		}
	}

	captured := strings.TrimSpace(out.String())

	if scriptErr == nil && action.OnSuccess != nil {
		if err := o.dispatchSubAction(ctx, st, wf, *action.OnSuccess, captured); err != nil {
			return err
		}
	} else if scriptErr != nil && action.OnFailure != nil {
		if err := o.dispatchSubAction(ctx, st, wf, *action.OnFailure, captured); err != nil {
			return err
		}
	}

	if action.Always != nil {
		if err := o.dispatchSubAction(ctx, st, wf, *action.Always, captured); err != nil {
			return err
		}
	}

	return nil
}

func (o *Orchestrator) dispatchSubAction(
	ctx context.Context, st *ticketdomain.State, wf workflow.WorkflowConfig, action workflow.ActionConfig, message string,
) error {
	switch action.Type {
	case workflow.ActionProvideFeedback:
		if strings.TrimSpace(message) == "" {
			return nil // no script output to feed back
		}
		return o.writeFeedbackAndRerun(ctx, st, wf, message)
	case workflow.ActionMoveToState:
		return o.transitionTo(ctx, st, wf, action.Target)
	case workflow.ActionRunScript:
		return ErrScriptSubAction
	default:
		return fmt.Errorf("action type %q: %w", action.Type, ErrUnsupportedSubAction)
	}
}

func (o *Orchestrator) runCommands(
	ctx context.Context, worktreePath string, commands []string, logPath, section string,
) error {
	if len(commands) == 0 {
		return nil
	}
	var b strings.Builder
	for _, cmd := range commands {
		res, err := shell.Run(ctx, worktreePath, nil, "", "/bin/sh", "-c", cmd)
		fmt.Fprintf(&b, "$ %s\n%s\n", cmd, res.Stdout)
		if err != nil {
			_ = markdown.AppendSection(logPath, section+" Failed", b.String()+"\nerror: "+err.Error())
			return fmt.Errorf("command %q: %w", cmd, err)
		}
	}
	_ = markdown.AppendSection(logPath, section, b.String())
	return nil
}

func (o *Orchestrator) printStatus(ticketNumber string) error {
	st, err := o.Store.LoadState(ticketNumber)
	if err != nil {
		return err
	}
	attrs := []any{
		"ticket", ticketNumber,
		"status", st.FlowStatus,
		"state", st.CurrentState,
		"branch", st.BranchName,
		"worktree", st.WorktreePath,
	}
	if st.PRURL != "" {
		attrs = append(attrs, "pr_url", st.PRURL)
	}
	if st.LastError != "" {
		attrs = append(attrs, "error", st.LastError)
	}
	slog.Info("ticket status", attrs...)
	return nil
}

func buildNextSteps(st ticketdomain.State, wf workflow.WorkflowConfig) string {
	switch st.FlowStatus {
	case ticketdomain.FlowStatusPending:
		return "Run the ticket to start the workflow: auto-pr run " + st.TicketNumber
	case ticketdomain.FlowStatusRunning:
		return "Ticket is currently running."
	case ticketdomain.FlowStatusWaiting:
		stateCfg, ok := wf.StateByName(st.CurrentState)
		if !ok {
			return fmt.Sprintf("Waiting for action in state %q.", st.CurrentState)
		}
		var b strings.Builder
		fmt.Fprintf(&b, "State: %s\nAvailable actions:\n", st.CurrentState)
		for _, a := range stateCfg.Actions {
			fmt.Fprintf(&b, "  - %s\n", a.Label)
		}
		return strings.TrimSpace(b.String())
	case ticketdomain.FlowStatusDone:
		return "Ticket is done."
	case ticketdomain.FlowStatusFailed:
		return fmt.Sprintf("Ticket failed: %s\n\nRetry: auto-pr run %s", st.LastError, st.TicketNumber)
	case ticketdomain.FlowStatusCancelled:
		return "Ticket was cancelled."
	}
	return ""
}

func resolveStateForStart(st ticketdomain.State, wf workflow.WorkflowConfig) (workflow.StateConfig, error) {
	if st.CurrentState == "" {
		first, ok := wf.FirstState()
		if !ok {
			return workflow.StateConfig{}, ErrWorkflowNoStates
		}
		return first, nil
	}
	stateCfg, ok := wf.StateByName(st.CurrentState)
	if !ok {
		return workflow.StateConfig{}, fmt.Errorf("state %q: %w", st.CurrentState, ErrStateNotFound)
	}
	return stateCfg, nil
}

func startStateRun(st *ticketdomain.State, stateCfg workflow.StateConfig) (ticketdomain.StateRun, error) {
	runID, err := newUUID()
	if err != nil {
		return ticketdomain.StateRun{}, fmt.Errorf("generate state run id: %w", err)
	}
	artifactName := stateCfg.PrimaryArtifact
	if strings.TrimSpace(artifactName) == "" {
		artifactName = stateCfg.Name + ".md"
	}
	run := ticketdomain.StateRun{
		ID:               runID,
		StateName:        stateCfg.Name,
		StateDisplayName: stateCfg.TimelineLabel(),
		StartedAt:        time.Now().UTC(),
		ArtifactRef:      filepath.ToSlash(filepath.Join("runs", runID, "artifacts", artifactName)),
		LogRef:           filepath.ToSlash(filepath.Join("runs", runID, "state.log")),
	}
	st.CurrentRunID = run.ID
	st.StateHistory = append(st.StateHistory, run)
	return run, nil
}

func (o *Orchestrator) prepareRunContext(
	st ticketdomain.State, stateCfg workflow.StateConfig, run ticketdomain.StateRun,
) error {
	runDir := st.RunPath(run.ID)
	if err := os.MkdirAll(filepath.Join(runDir, "artifacts"), 0o755); err != nil { //nolint:gosec // G301: 0755 correct for project directories
		return err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Ticket Number: %s\n", st.TicketNumber)
	fmt.Fprintf(&b, "Current State: %s\n", stateCfg.Name)
	fmt.Fprintf(&b, "Current State Display Name: %s\n", stateCfg.TimelineLabel())
	fmt.Fprintf(&b, "Current Run ID: %s\n", run.ID)
	fmt.Fprintf(&b, "Current Run Directory: %s\n", filepath.ToSlash(filepath.Join(".auto-pr", "runs", run.ID)))
	fmt.Fprintf(&b, "Current Primary Artifact: %s\n", filepath.ToSlash(filepath.Join(".auto-pr", run.ArtifactRef)))
	fmt.Fprintf(&b, "Current State Log: %s\n", filepath.ToSlash(filepath.Join(".auto-pr", run.LogRef)))
	rawProviderLog := filepath.ToSlash(filepath.Join(".auto-pr", "runs", run.ID, "raw-provider.log"))
	fmt.Fprintf(&b, "Current Raw Provider Log: %s\n", rawProviderLog)
	fmt.Fprintf(&b, "Feedback File: %s\n", filepath.ToSlash(filepath.Join(".auto-pr", "feedback.md")))
	fmt.Fprintf(&b, "Shared Context File: %s\n", filepath.ToSlash(filepath.Join(".auto-pr", "context.md")))
	guidelinesPath := config.ResolveGuidelinesPath(o.RepoRoot, o.Cfg)
	if guidelinesPath != "" {
		fmt.Fprintf(&b, "Guidelines File: %s\n", guidelinesPath)
	}
	b.WriteString("\nLatest State Artifacts:\n")
	seen := map[string]bool{}
	for i := len(st.StateHistory) - 1; i >= 0; i-- {
		stateName := st.StateHistory[i].StateName
		if seen[stateName] {
			continue
		}
		seen[stateName] = true
		if ref := st.LatestArtifactRef(stateName); ref != "" {
			fmt.Fprintf(&b, "- %s: %s\n", stateName, filepath.ToSlash(filepath.Join(".auto-pr", ref)))
		}
	}

	return os.WriteFile(st.ArtifactPath("run-context.md"), []byte(b.String()), 0o644) //nolint:gosec // G306: 0644 intentional for user-readable context files
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4],
		b[4:6],
		b[6:8],
		b[8:10],
		b[10:16],
	), nil
}

// EnsureStateIgnored ensures the state directory is listed in .gitignore.
func EnsureStateIgnored(repoRoot, stateDirName string) error {
	ignorePath := filepath.Join(repoRoot, ".gitignore")
	entry := stateDirName + "/"
	b, err := os.ReadFile(ignorePath) //nolint:gosec // G304: path built from trusted repo root
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if strings.Contains(string(b), entry) {
		return nil
	}
	f, err := os.OpenFile(ignorePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644) //nolint:gosec // G302: 0644 is correct for .gitignore
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if len(b) > 0 && !strings.HasSuffix(string(b), "\n") {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	_, err = f.WriteString(entry + "\n")
	return err
}
