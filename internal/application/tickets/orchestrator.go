package tickets

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Neokil/AutoPR/internal/config"
	workflowstate "github.com/Neokil/AutoPR/internal/domain/workflowstate"
	"github.com/Neokil/AutoPR/internal/gitutil"
	"github.com/Neokil/AutoPR/internal/markdown"
	"github.com/Neokil/AutoPR/internal/providers"
	"github.com/Neokil/AutoPR/internal/shell"
	"github.com/Neokil/AutoPR/internal/state"
	"github.com/Neokil/AutoPR/internal/workflow"
)

type stateStore interface {
	LoadState(ticketNumber string) (workflowstate.State, error)
	SaveState(ticketNumber string, st workflowstate.State) error
	ListTicketDirs() ([]string, error)
	RemoveTicketDir(ticketNumber string) error
}

type promptExecutor interface {
	Name() string
	Execute(ctx context.Context, req providers.ExecuteRequest) (providers.ExecuteResult, error)
}

// Orchestrator drives the workflow state machine for a single repository.
type Orchestrator struct {
	Cfg      config.Config
	RepoRoot string
	Store    stateStore
	Provider promptExecutor
}

// New returns an Orchestrator using the default filesystem state store.
func New(cfg config.Config, repoRoot string, provider promptExecutor) *Orchestrator {
	return NewWithStore(cfg, repoRoot, state.NewStore(repoRoot, cfg.StateDirName), provider)
}

// NewWithStore returns an Orchestrator with an explicitly provided state store (used in tests).
func NewWithStore(cfg config.Config, repoRoot string, store stateStore, provider promptExecutor) *Orchestrator {
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
	wflow, err := workflow.Load(o.RepoRoot)
	if err != nil {
		return fmt.Errorf("load workflow: %w", err)
	}

	state, loadErr := o.Store.LoadState(ticketNumber)
	if os.IsNotExist(loadErr) {
		state = workflowstate.New(ticketNumber)
		saveErr := o.Store.SaveState(ticketNumber, state)
		if saveErr != nil {
			return fmt.Errorf("save initial ticket state: %w", saveErr)
		}
	} else if loadErr != nil {
		return fmt.Errorf("load ticket state: %w", loadErr)
	}
	if state.FlowStatus == workflowstate.FlowStatusDone || state.FlowStatus == workflowstate.FlowStatusCancelled {
		slog.Info("skipping ticket", "ticket", ticketNumber, "status", state.FlowStatus)

		return nil
	}
	if state.FlowStatus == workflowstate.FlowStatusRunning {
		return fmt.Errorf("ticket %s: %w", ticketNumber, ErrTicketRunning)
	}
	err = o.ensureWorktreeAndContext(ctx, &state)
	if err != nil {
		return err
	}

	// Determine which state to run.
	stateCfg, err := resolveStateForStart(state, wflow)
	if err != nil {
		return err
	}

	slog.Info("starting flow", "ticket", ticketNumber, "state", stateCfg.Name)

	return o.runState(ctx, &state, stateCfg)
}

// ApplyAction applies the named action to a ticket that is waiting for input.
func (o *Orchestrator) ApplyAction(ctx context.Context, ticketNumber, actionLabel, message string) error {
	wflow, err := workflow.Load(o.RepoRoot)
	if err != nil {
		return fmt.Errorf("load workflow: %w", err)
	}

	state, err := o.Store.LoadState(ticketNumber)
	if err != nil {
		return fmt.Errorf("load ticket state: %w", err)
	}
	if state.FlowStatus != workflowstate.FlowStatusWaiting {
		return fmt.Errorf("ticket %s (status: %s): %w", ticketNumber, state.FlowStatus, ErrTicketNotWaiting)
	}

	stateCfg, ok := wflow.StateByName(state.CurrentState)
	if !ok {
		return fmt.Errorf("state %q: %w", state.CurrentState, ErrStateNotFound)
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

		return fmt.Errorf("action %q in state %q (available: %s): %w", actionLabel, state.CurrentState, strings.Join(labels, ", "), ErrActionNotFound)
	}

	slog.Info("applying action", "ticket", ticketNumber, "action", actionLabel, "state", state.CurrentState)

	return o.dispatchAction(ctx, &state, wflow, *action, message)
}

// MoveToState force-transitions the ticket to target, creating a worktree if needed.
func (o *Orchestrator) MoveToState(ctx context.Context, ticketNumber, target string) error {
	wflow, err := workflow.Load(o.RepoRoot)
	if err != nil {
		return fmt.Errorf("load workflow: %w", err)
	}
	if strings.TrimSpace(target) == "" {
		return ErrTargetStateRequired
	}

	state, loadErr := o.Store.LoadState(ticketNumber)
	if os.IsNotExist(loadErr) {
		state = workflowstate.New(ticketNumber)
		saveErr := o.Store.SaveState(ticketNumber, state)
		if saveErr != nil {
			return fmt.Errorf("save initial ticket state: %w", saveErr)
		}
	} else if loadErr != nil {
		return fmt.Errorf("load ticket state: %w", loadErr)
	}
	if state.FlowStatus == workflowstate.FlowStatusRunning {
		return fmt.Errorf("ticket %s: %w", ticketNumber, ErrTicketRunning)
	}
	err = o.ensureWorktreeAndContext(ctx, &state)
	if err != nil {
		return err
	}

	slog.Info("force moving to state", "ticket", ticketNumber, "target", target)

	return o.transitionTo(ctx, &state, wflow, target)
}

// Status prints the workflow status for one ticket, or all tickets if ticketNumber is empty.
func (o *Orchestrator) Status(ticketNumber string) error {
	if ticketNumber != "" {
		return o.printStatus(ticketNumber)
	}
	tickets, err := o.Store.ListTicketDirs()
	if err != nil {
		return fmt.Errorf("list ticket dirs: %w", err)
	}
	sort.Strings(tickets)
	for _, t := range tickets {
		err = o.printStatus(t)
		if err != nil {
			slog.Error("status failed", "ticket", t, "err", err)
		}
	}

	return nil
}

// NextSteps returns a human-readable description of the available next actions for the ticket.
func (o *Orchestrator) NextSteps(ticketNumber string) (string, error) {
	state, err := o.Store.LoadState(ticketNumber)
	if err != nil {
		return "", fmt.Errorf("load ticket state: %w", err)
	}
	wflow, _ := workflow.Load(o.RepoRoot)

	return buildNextSteps(state, wflow), nil
}

// CleanupTicket removes the worktree and state directory for the given ticket.
func (o *Orchestrator) CleanupTicket(ctx context.Context, ticketNumber string) error {
	st, err := o.Store.LoadState(ticketNumber)
	if err != nil {
		return fmt.Errorf("load ticket state: %w", err)
	}
	_ = gitutil.WorktreeRemove(ctx, o.RepoRoot, st.WorktreePath)
	err = o.Store.RemoveTicketDir(ticketNumber)
	if err != nil {
		return fmt.Errorf("remove ticket dir: %w", err)
	}
	slog.Info("cleaned ticket", "ticket", ticketNumber)

	return nil
}

// CleanupDone removes worktrees and state for all tickets with FlowStatusDone.
func (o *Orchestrator) CleanupDone(ctx context.Context) error {
	tickets, err := o.Store.ListTicketDirs()
	if err != nil {
		return fmt.Errorf("list ticket dirs: %w", err)
	}
	sort.Strings(tickets)
	for _, ticket := range tickets {
		st, err := o.Store.LoadState(ticket)
		if err != nil {
			continue
		}
		if st.FlowStatus == workflowstate.FlowStatusDone {
			err = o.CleanupTicket(ctx, ticket)
			if err != nil {
				slog.Error("cleanup failed", "ticket", ticket, "err", err)
			}
		}
	}

	return nil
}

// CleanupAll removes worktrees and state for every ticket regardless of status.
func (o *Orchestrator) CleanupAll(ctx context.Context) error {
	tickets, err := o.Store.ListTicketDirs()
	if err != nil {
		return fmt.Errorf("list ticket dirs: %w", err)
	}
	sort.Strings(tickets)
	for _, ticket := range tickets {
		err = o.CleanupTicket(ctx, ticket)
		if err != nil {
			slog.Error("cleanup failed", "ticket", ticket, "err", err)
		}
	}

	return nil
}

func (o *Orchestrator) ensureWorktreeAndContext(ctx context.Context, state *workflowstate.State) error {
	if state.WorktreePath == "" {
		branchName := "auto-pr/" + state.TicketNumber
		slog.Info("creating worktree", "ticket", state.TicketNumber, "branch", branchName)
		wtPath, err := gitutil.EnsureWorktree(ctx, o.RepoRoot, o.Cfg.StateDirName, state.TicketNumber, branchName, o.Cfg.BaseBranch)
		if err != nil {
			return fmt.Errorf("create worktree: %w", err)
		}
		state.BranchName = branchName
		state.WorktreePath = wtPath
		err = o.Store.SaveState(state.TicketNumber, *state)
		if err != nil {
			return fmt.Errorf("save ticket state: %w", err)
		}
	}

	autoPRDir := filepath.Join(state.WorktreePath, ".auto-pr")
	err := os.MkdirAll(autoPRDir, 0o755) //nolint:gosec,mnd // G301: 0755 correct for project directories
	if err != nil {
		return fmt.Errorf("create .auto-pr dir: %w", err)
	}

	contextPath := state.ArtifactPath("context.md")
	_, statErr := os.Stat(contextPath)
	if os.IsNotExist(statErr) {
		guidelinesPath := config.ResolveGuidelinesPath(o.RepoRoot, o.Cfg)
		content := fmt.Sprintf("Ticket: %s\nWorktree: %s\nRepo: %s\nGuidelines: %s\n", state.TicketNumber, state.WorktreePath, o.RepoRoot, guidelinesPath)
		err = os.WriteFile(contextPath, []byte(content), 0o644) //nolint:gosec,mnd // G306: 0644 intentional for user-readable context files
		if err != nil {
			return fmt.Errorf("write context file: %w", err)
		}
	}

	return nil
}

// --- internal helpers ---

func (o *Orchestrator) runState(ctx context.Context, state *workflowstate.State, stateCfg workflow.StateConfig) error {
	slog.Info("running state", "ticket", state.TicketNumber, "state", stateCfg.Name)
	run, err := startStateRun(state, stateCfg)
	if err != nil {
		return err
	}
	logPath := state.ResolveRef(run.LogRef)

	state.CurrentState = stateCfg.Name
	state.FlowStatus = workflowstate.FlowStatusRunning
	state.LastError = ""
	err = o.Store.SaveState(state.TicketNumber, *state)
	if err != nil {
		return fmt.Errorf("save ticket state: %w", err)
	}
	err = o.prepareRunContext(*state, stateCfg, run)
	if err != nil {
		return o.failState(state, err)
	}

	err = o.runCommands(ctx, state.WorktreePath, stateCfg.PrePromptCommands, logPath, "Pre-prompt")
	if err != nil {
		return o.failState(state, err)
	}

	promptContent, err := workflow.ReadPrompt(o.RepoRoot, stateCfg.Prompt)
	if err != nil {
		return o.failState(state, fmt.Errorf("read prompt %s: %w", stateCfg.Prompt, err))
	}

	promptPath := state.RunPath(run.ID, "prompt.md")
	err = os.WriteFile(promptPath, promptContent, 0o644) //nolint:gosec,mnd // G306: 0644 intentional for user-readable prompt files
	if err != nil {
		return o.failState(state, err)
	}

	runtimeDir := state.RunPath(run.ID, "provider")
	err = os.MkdirAll(runtimeDir, 0o755) //nolint:gosec,mnd // G301: 0755 correct for project directories
	if err != nil {
		return o.failState(state, err)
	}

	slog.Info("executing provider", "ticket", state.TicketNumber, "state", stateCfg.Name)
	result, err := o.Provider.Execute(ctx, providers.ExecuteRequest{
		PromptPath:  promptPath,
		WorkDir:     state.WorktreePath,
		RuntimeDir:  runtimeDir,
		SessionData: state.ProviderSessionData,
	})
	if result.SessionData != "" {
		state.ProviderSessionData = result.SessionData
	}
	rawLogPath := state.RunPath(run.ID, "raw-provider.log")
	_ = os.WriteFile(rawLogPath, []byte(result.RawOutput+"\n\n[stderr]\n"+result.Stderr), 0o644) //nolint:gosec,mnd // G306: 0644 intentional for user-readable log files
	if err != nil {
		if errors.Is(err, providers.ErrTokensExhausted) {
			err = fmt.Errorf("token usage limit reached — wait for your quota to reset, then rerun this ticket to continue: %w", err)
		}
		_ = markdown.AppendSection(logPath, stateCfg.Name+" Failed", err.Error())

		return o.failState(state, err)
	}

	_ = markdown.AppendSection(logPath, stateCfg.Name, result.RawOutput)

	err = o.runCommands(ctx, state.WorktreePath, stateCfg.PostPromptCommands, logPath, "Post-prompt")
	if err != nil {
		return o.failState(state, err)
	}

	// Remove feedback.md so stale feedback is not visible to the next run.
	_ = os.Remove(state.ArtifactPath("feedback.md"))

	slog.Info("state done, waiting for action", "ticket", state.TicketNumber, "state", stateCfg.Name)
	state.FlowStatus = workflowstate.FlowStatusWaiting
	saveErr := o.Store.SaveState(state.TicketNumber, *state)
	if saveErr != nil {
		return fmt.Errorf("save ticket state: %w", saveErr)
	}

	return nil
}

func (o *Orchestrator) failState(st *workflowstate.State, cause error) error {
	slog.Error("state failed", "ticket", st.TicketNumber, "state", st.CurrentState, "err", cause)
	st.FlowStatus = workflowstate.FlowStatusFailed
	st.LastError = cause.Error()
	_ = o.Store.SaveState(st.TicketNumber, *st)

	return cause
}

func (o *Orchestrator) dispatchAction(ctx context.Context, state *workflowstate.State, wflow workflow.Config, action workflow.ActionConfig, message string) error {
	logPath := state.CurrentRunLogPath()
	_ = markdown.AppendSection(logPath, "Human Action: "+action.Label, "")

	switch action.Type {
	case workflow.ActionProvideFeedback:
		return o.writeFeedbackAndRerun(ctx, state, wflow, message)
	case workflow.ActionMoveToState:
		return o.transitionTo(ctx, state, wflow, action.Target)
	case workflow.ActionRunScript:
		return o.executeScript(ctx, state, wflow, action)
	default:
		return fmt.Errorf("action type %q: %w", action.Type, ErrUnknownActionType)
	}
}

func (o *Orchestrator) transitionTo(ctx context.Context, state *workflowstate.State, wflow workflow.Config, target string) error {
	if workflow.IsTerminal(target) {
		slog.Info("reached terminal state", "ticket", state.TicketNumber, "state", target)
		switch target {
		case "done":
			state.FlowStatus = workflowstate.FlowStatusDone
		case "cancelled":
			state.FlowStatus = workflowstate.FlowStatusCancelled
		default:
			state.FlowStatus = workflowstate.FlowStatusFailed
		}

		saveErr := o.Store.SaveState(state.TicketNumber, *state)
		if saveErr != nil {
			return fmt.Errorf("save ticket state: %w", saveErr)
		}

		return nil
	}
	slog.Info("transitioning to state", "ticket", state.TicketNumber, "target", target)
	stateCfg, ok := wflow.StateByName(target)
	if !ok {
		return fmt.Errorf("state %q: %w", target, ErrTargetNotFound)
	}

	return o.runState(ctx, state, stateCfg)
}

func (o *Orchestrator) writeFeedbackAndRerun(ctx context.Context, state *workflowstate.State, wflow workflow.Config, message string) error {
	if strings.TrimSpace(message) == "" {
		return ErrFeedbackRequired
	}
	slog.Info("applying feedback", "ticket", state.TicketNumber, "state", state.CurrentState)
	feedbackPath := state.ArtifactPath("feedback.md")
	err := os.WriteFile(feedbackPath, []byte(strings.TrimSpace(message)), 0o644) //nolint:gosec,mnd // G306: 0644 intentional for user-readable feedback files
	if err != nil {
		return fmt.Errorf("write feedback file: %w", err)
	}
	stateCfg, ok := wflow.StateByName(state.CurrentState)
	if !ok {
		return fmt.Errorf("state %q: %w", state.CurrentState, ErrStateNotFound)
	}

	return o.runState(ctx, state, stateCfg)
}

func (o *Orchestrator) executeScript(ctx context.Context, state *workflowstate.State, wflow workflow.Config, action workflow.ActionConfig) error {
	logPath := state.CurrentRunLogPath()

	var out strings.Builder
	var scriptErr error
	for _, cmd := range action.Commands {
		res, err := shell.Run(ctx, state.WorktreePath, nil, "", "/bin/sh", "-c", cmd)
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
		err := o.dispatchSubAction(ctx, state, wflow, *action.OnSuccess, captured)
		if err != nil {
			return err
		}
	} else if scriptErr != nil && action.OnFailure != nil {
		err := o.dispatchSubAction(ctx, state, wflow, *action.OnFailure, captured)
		if err != nil {
			return err
		}
	}

	if action.Always != nil {
		err := o.dispatchSubAction(ctx, state, wflow, *action.Always, captured)
		if err != nil {
			return err
		}
	}

	return nil
}

func (o *Orchestrator) dispatchSubAction(
	ctx context.Context, state *workflowstate.State, wflow workflow.Config, action workflow.ActionConfig, message string,
) error {
	switch action.Type {
	case workflow.ActionProvideFeedback:
		if strings.TrimSpace(message) == "" {
			return nil // no script output to feed back
		}

		return o.writeFeedbackAndRerun(ctx, state, wflow, message)
	case workflow.ActionMoveToState:
		return o.transitionTo(ctx, state, wflow, action.Target)
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
	var buf strings.Builder
	for _, cmd := range commands {
		res, err := shell.Run(ctx, worktreePath, nil, "", "/bin/sh", "-c", cmd)
		fmt.Fprintf(&buf, "$ %s\n%s\n", cmd, res.Stdout)
		if err != nil {
			_ = markdown.AppendSection(logPath, section+" Failed", buf.String()+"\nerror: "+err.Error())

			return fmt.Errorf("command %q: %w", cmd, err)
		}
	}
	_ = markdown.AppendSection(logPath, section, buf.String())

	return nil
}

func (o *Orchestrator) printStatus(ticketNumber string) error {
	state, err := o.Store.LoadState(ticketNumber)
	if err != nil {
		return fmt.Errorf("load ticket state: %w", err)
	}
	attrs := []any{
		"ticket", ticketNumber,
		"status", state.FlowStatus,
		"state", state.CurrentState,
		"branch", state.BranchName,
		"worktree", state.WorktreePath,
	}
	if state.PRURL != "" {
		attrs = append(attrs, "pr_url", state.PRURL)
	}
	if state.LastError != "" {
		attrs = append(attrs, "error", state.LastError)
	}
	slog.Info("ticket status", attrs...)

	return nil
}

func buildNextSteps(state workflowstate.State, wflow workflow.Config) string {
	switch state.FlowStatus {
	case workflowstate.FlowStatusPending:
		return "Run the ticket to start the workflow: auto-pr run " + state.TicketNumber
	case workflowstate.FlowStatusRunning:
		return "Ticket is currently running."
	case workflowstate.FlowStatusWaiting:
		stateCfg, ok := wflow.StateByName(state.CurrentState)
		if !ok {
			return fmt.Sprintf("Waiting for action in state %q.", state.CurrentState)
		}
		var buf strings.Builder
		fmt.Fprintf(&buf, "State: %s\nAvailable actions:\n", state.CurrentState)
		for _, a := range stateCfg.Actions {
			fmt.Fprintf(&buf, "  - %s\n", a.Label)
		}

		return strings.TrimSpace(buf.String())
	case workflowstate.FlowStatusDone:
		return "Ticket is done."
	case workflowstate.FlowStatusFailed:
		return fmt.Sprintf("Ticket failed: %s\n\nRetry: auto-pr run %s", state.LastError, state.TicketNumber)
	case workflowstate.FlowStatusCancelled:
		return "Ticket was cancelled."
	}

	return ""
}

func resolveStateForStart(state workflowstate.State, wflow workflow.Config) (workflow.StateConfig, error) {
	if state.CurrentState == "" {
		first, ok := wflow.FirstState()
		if !ok {
			return workflow.StateConfig{}, ErrWorkflowNoStates
		}

		return first, nil
	}
	stateCfg, ok := wflow.StateByName(state.CurrentState)
	if !ok {
		return workflow.StateConfig{}, fmt.Errorf("state %q: %w", state.CurrentState, ErrStateNotFound)
	}

	return stateCfg, nil
}

func startStateRun(state *workflowstate.State, stateCfg workflow.StateConfig) (workflowstate.StateRun, error) {
	runID, err := newUUID()
	if err != nil {
		return workflowstate.StateRun{}, fmt.Errorf("generate state run id: %w", err)
	}
	artifactName := stateCfg.PrimaryArtifact
	if strings.TrimSpace(artifactName) == "" {
		artifactName = stateCfg.Name + ".md"
	}
	run := workflowstate.StateRun{
		ID:               runID,
		StateName:        stateCfg.Name,
		StateDisplayName: stateCfg.TimelineLabel(),
		StartedAt:        time.Now().UTC(),
		ArtifactRef:      filepath.ToSlash(filepath.Join("runs", runID, "artifacts", artifactName)),
		LogRef:           filepath.ToSlash(filepath.Join("runs", runID, "state.log")),
	}
	state.CurrentRunID = run.ID
	state.StateHistory = append(state.StateHistory, run)

	return run, nil
}

func (o *Orchestrator) prepareRunContext(
	state workflowstate.State, stateCfg workflow.StateConfig, run workflowstate.StateRun,
) error {
	runDir := state.RunPath(run.ID)
	err := os.MkdirAll(filepath.Join(runDir, "artifacts"), 0o755) //nolint:gosec,mnd // G301: 0755 correct for project directories
	if err != nil {
		return fmt.Errorf("create run artifacts dir: %w", err)
	}

	var buf strings.Builder
	fmt.Fprintf(&buf, "Ticket Number: %s\n", state.TicketNumber)
	fmt.Fprintf(&buf, "Current State: %s\n", stateCfg.Name)
	fmt.Fprintf(&buf, "Current State Display Name: %s\n", stateCfg.TimelineLabel())
	fmt.Fprintf(&buf, "Current Run ID: %s\n", run.ID)
	fmt.Fprintf(&buf, "Current Run Directory: %s\n", filepath.ToSlash(filepath.Join(".auto-pr", "runs", run.ID)))
	fmt.Fprintf(&buf, "Current Primary Artifact: %s\n", filepath.ToSlash(filepath.Join(".auto-pr", run.ArtifactRef)))
	fmt.Fprintf(&buf, "Current State Log: %s\n", filepath.ToSlash(filepath.Join(".auto-pr", run.LogRef)))
	rawProviderLog := filepath.ToSlash(filepath.Join(".auto-pr", "runs", run.ID, "raw-provider.log"))
	fmt.Fprintf(&buf, "Current Raw Provider Log: %s\n", rawProviderLog)
	fmt.Fprintf(&buf, "Feedback File: %s\n", filepath.ToSlash(filepath.Join(".auto-pr", "feedback.md")))
	fmt.Fprintf(&buf, "Shared Context File: %s\n", filepath.ToSlash(filepath.Join(".auto-pr", "context.md")))
	guidelinesPath := config.ResolveGuidelinesPath(o.RepoRoot, o.Cfg)
	if guidelinesPath != "" {
		fmt.Fprintf(&buf, "Guidelines File: %s\n", guidelinesPath)
	}
	buf.WriteString("\nLatest State Artifacts:\n")
	seen := map[string]bool{}
	for i := len(state.StateHistory) - 1; i >= 0; i-- {
		stateName := state.StateHistory[i].StateName
		if seen[stateName] {
			continue
		}
		seen[stateName] = true
		if ref := state.LatestArtifactRef(stateName); ref != "" {
			fmt.Fprintf(&buf, "- %s: %s\n", stateName, filepath.ToSlash(filepath.Join(".auto-pr", ref)))
		}
	}

	err = os.WriteFile(state.ArtifactPath("run-context.md"), []byte(buf.String()), 0o644) //nolint:gosec,mnd // G306: 0644 intentional for user-readable context files
	if err != nil {
		return fmt.Errorf("write run-context: %w", err)
	}

	return nil
}

func newUUID() (string, error) {
	var buf [16]byte
	_, err := rand.Read(buf[:])
	if err != nil {
		return "", fmt.Errorf("rand read: %w", err)
	}
	buf[6] = (buf[6] & 0x0f) | 0x40 //nolint:mnd // UUID v4 version bits
	buf[8] = (buf[8] & 0x3f) | 0x80 //nolint:mnd // UUID v4 variant bits

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4],
		buf[4:6],
		buf[6:8],
		buf[8:10],
		buf[10:16],
	), nil
}

// EnsureStateIgnored ensures the state directory is listed in .gitignore.
func EnsureStateIgnored(repoRoot, stateDirName string) error {
	ignorePath := filepath.Join(repoRoot, ".gitignore")
	entry := stateDirName + "/"
	contents, err := os.ReadFile(ignorePath) //nolint:gosec // G304: path built from trusted repo root
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read .gitignore: %w", err)
	}
	if strings.Contains(string(contents), entry) {
		return nil
	}
	file, err := os.OpenFile(ignorePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644) //nolint:gosec,mnd // G302: 0644 is correct for .gitignore
	if err != nil {
		return fmt.Errorf("open .gitignore: %w", err)
	}
	defer func() { _ = file.Close() }()
	if len(contents) > 0 && !strings.HasSuffix(string(contents), "\n") {
		_, err = file.WriteString("\n")
		if err != nil {
			return fmt.Errorf("write .gitignore newline: %w", err)
		}
	}
	_, err = file.WriteString(entry + "\n")
	if err != nil {
		return fmt.Errorf("write .gitignore entry: %w", err)
	}

	return nil
}
