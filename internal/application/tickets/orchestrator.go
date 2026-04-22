package tickets

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ai-ticket-worker/internal/config"
	ticketdomain "ai-ticket-worker/internal/domain/ticket"
	"ai-ticket-worker/internal/gitutil"
	"ai-ticket-worker/internal/markdown"
	"ai-ticket-worker/internal/ports"
	"ai-ticket-worker/internal/providers"
	"ai-ticket-worker/internal/shell"
	"ai-ticket-worker/internal/state"
	"ai-ticket-worker/internal/workflow"
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
	ensureRunHistory(&st)

	if st.FlowStatus == ticketdomain.FlowStatusDone || st.FlowStatus == ticketdomain.FlowStatusCancelled {
		log.Printf("[%s] already %s, skipping", ticketNumber, st.FlowStatus)
		return nil
	}
	if st.FlowStatus == ticketdomain.FlowStatusRunning {
		return fmt.Errorf("ticket %s is already running", ticketNumber)
	}

	// Ensure worktree exists.
	if st.WorktreePath == "" {
		branchName := fmt.Sprintf("auto-pr/%s", ticketNumber)
		log.Printf("[%s] creating worktree on branch %s", ticketNumber, branchName)
		wtPath, err := worktree.Ensure(ctx, o.RepoRoot, o.Cfg.StateDirName, ticketNumber, branchName, o.Cfg.BaseBranch)
		if err != nil {
			return fmt.Errorf("create worktree: %w", err)
		}
		st.BranchName = branchName
		st.WorktreePath = wtPath
		if err := o.Store.SaveState(ticketNumber, st); err != nil {
			return err
		}
	}

	// Ensure the .auto-pr artifact directory exists inside the worktree.
	autoPRDir := filepath.Join(st.WorktreePath, ".auto-pr")
	if err := os.MkdirAll(autoPRDir, 0o755); err != nil {
		return fmt.Errorf("create .auto-pr dir: %w", err)
	}

	// Write context.md once (skip if already present).
	contextPath := st.ArtifactPath("context.md")
	if _, statErr := os.Stat(contextPath); os.IsNotExist(statErr) {
		guidelinesPath := config.ResolveGuidelinesPath(o.RepoRoot, o.Cfg)
		content := fmt.Sprintf("Ticket: %s\nWorktree: %s\nRepo: %s\nGuidelines: %s\n", ticketNumber, st.WorktreePath, o.RepoRoot, guidelinesPath)
		if err := os.WriteFile(contextPath, []byte(content), 0o644); err != nil {
			return err
		}
	}

	// Determine which state to run.
	var stateCfg workflow.StateConfig
	if st.CurrentState == "" {
		first, ok := wf.FirstState()
		if !ok {
			return fmt.Errorf("workflow has no states defined")
		}
		stateCfg = first
	} else {
		cfg, ok := wf.StateByName(st.CurrentState)
		if !ok {
			return fmt.Errorf("current state %q not found in workflow", st.CurrentState)
		}
		stateCfg = cfg
	}

	log.Printf("[%s] starting flow, entering state %q", ticketNumber, stateCfg.Name)
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
	ensureRunHistory(&st)

	if st.FlowStatus != ticketdomain.FlowStatusWaiting {
		return fmt.Errorf("ticket %s is not waiting for an action (status: %s)", ticketNumber, st.FlowStatus)
	}

	stateCfg, ok := wf.StateByName(st.CurrentState)
	if !ok {
		return fmt.Errorf("current state %q not found in workflow", st.CurrentState)
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
		return fmt.Errorf("action %q not found in state %q (available: %s)", actionLabel, st.CurrentState, strings.Join(labels, ", "))
	}

	log.Printf("[%s] applying action %q in state %q", ticketNumber, actionLabel, st.CurrentState)
	return o.dispatchAction(ctx, &st, wf, *action, message)
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
	ensureRunHistory(&st)
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
		if st.FlowStatus == ticketdomain.FlowStatusDone {
			if err := o.CleanupTicket(ctx, ticket); err != nil {
				fmt.Fprintf(os.Stderr, "cleanup %s: %v\n", ticket, err)
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
			fmt.Fprintf(os.Stderr, "cleanup %s: %v\n", ticket, err)
		}
	}
	return nil
}

// --- internal helpers ---

func (o *Orchestrator) runState(ctx context.Context, st *ticketdomain.State, stateCfg workflow.StateConfig) error {
	log.Printf("[%s] running state %q", st.TicketNumber, stateCfg.Name)
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
	if err := os.WriteFile(promptPath, promptContent, 0o644); err != nil {
		return o.failState(st, err)
	}

	runtimeDir := st.RunPath(run.ID, "provider")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		return o.failState(st, err)
	}

	log.Printf("[%s] executing provider for state %q", st.TicketNumber, stateCfg.Name)
	result, err := o.Provider.Execute(ctx, providers.ExecuteRequest{
		PromptPath: promptPath,
		WorkDir:    st.WorktreePath,
		RuntimeDir: runtimeDir,
	})
	rawLogPath := st.RunPath(run.ID, "raw-provider.log")
	_ = os.WriteFile(rawLogPath, []byte(result.RawOutput+"\n\n[stderr]\n"+result.Stderr), 0o644)
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

	log.Printf("[%s] state %q done, waiting for action", st.TicketNumber, stateCfg.Name)
	st.FlowStatus = ticketdomain.FlowStatusWaiting
	return o.Store.SaveState(st.TicketNumber, *st)
}

func (o *Orchestrator) failState(st *ticketdomain.State, cause error) error {
	log.Printf("[%s] state %q failed: %v", st.TicketNumber, st.CurrentState, cause)
	st.FlowStatus = ticketdomain.FlowStatusFailed
	st.LastError = cause.Error()
	_ = o.Store.SaveState(st.TicketNumber, *st)
	return cause
}

func (o *Orchestrator) dispatchAction(ctx context.Context, st *ticketdomain.State, wf workflow.WorkflowConfig, action workflow.ActionConfig, message string) error {
	logPath := currentRunLogPath(*st)
	_ = markdown.AppendSection(logPath, "Human Action: "+action.Label, "")

	switch action.Type {
	case workflow.ActionProvideFeedback:
		return o.writeFeedbackAndRerun(ctx, st, wf, message)
	case workflow.ActionMoveToState:
		return o.transitionTo(ctx, st, wf, action.Target)
	case workflow.ActionRunScript:
		return o.executeScript(ctx, st, wf, action)
	default:
		return fmt.Errorf("unknown action type: %s", action.Type)
	}
}

func (o *Orchestrator) transitionTo(ctx context.Context, st *ticketdomain.State, wf workflow.WorkflowConfig, target string) error {
	if workflow.IsTerminal(target) {
		log.Printf("[%s] reached terminal state %q", st.TicketNumber, target)
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
	log.Printf("[%s] transitioning to state %q", st.TicketNumber, target)
	stateCfg, ok := wf.StateByName(target)
	if !ok {
		return fmt.Errorf("target state %q not found in workflow", target)
	}
	return o.runState(ctx, st, stateCfg)
}

func (o *Orchestrator) writeFeedbackAndRerun(ctx context.Context, st *ticketdomain.State, wf workflow.WorkflowConfig, message string) error {
	if strings.TrimSpace(message) == "" {
		return fmt.Errorf("feedback message is required")
	}
	log.Printf("[%s] applying feedback, rerunning state %q", st.TicketNumber, st.CurrentState)
	feedbackPath := st.ArtifactPath("feedback.md")
	if err := os.WriteFile(feedbackPath, []byte(strings.TrimSpace(message)), 0o644); err != nil {
		return err
	}
	stateCfg, ok := wf.StateByName(st.CurrentState)
	if !ok {
		return fmt.Errorf("current state %q not found in workflow", st.CurrentState)
	}
	return o.runState(ctx, st, stateCfg)
}

func (o *Orchestrator) executeScript(ctx context.Context, st *ticketdomain.State, wf workflow.WorkflowConfig, action workflow.ActionConfig) error {
	logPath := currentRunLogPath(*st)

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

func (o *Orchestrator) dispatchSubAction(ctx context.Context, st *ticketdomain.State, wf workflow.WorkflowConfig, action workflow.ActionConfig, message string) error {
	switch action.Type {
	case workflow.ActionProvideFeedback:
		if strings.TrimSpace(message) == "" {
			return nil // no script output to feed back
		}
		return o.writeFeedbackAndRerun(ctx, st, wf, message)
	case workflow.ActionMoveToState:
		return o.transitionTo(ctx, st, wf, action.Target)
	default:
		return fmt.Errorf("unsupported sub-action type: %s", action.Type)
	}
}

func (o *Orchestrator) runCommands(ctx context.Context, worktreePath string, commands []string, logPath, section string) error {
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
	fmt.Printf("ticket %s\n", ticketNumber)
	fmt.Printf("  status:   %s\n", st.FlowStatus)
	fmt.Printf("  state:    %s\n", st.CurrentState)
	fmt.Printf("  branch:   %s\n", st.BranchName)
	fmt.Printf("  worktree: %s\n", st.WorktreePath)
	if st.PRURL != "" {
		fmt.Printf("  pr_url:   %s\n", st.PRURL)
	}
	if st.LastError != "" {
		fmt.Printf("  error:    %s\n", st.LastError)
	}
	return nil
}

func (o *Orchestrator) loadStateAndWorkflow(ticketNumber string) (ticketdomain.State, workflow.WorkflowConfig, error) {
	st, err := o.Store.LoadState(ticketNumber)
	if err != nil {
		return ticketdomain.State{}, workflow.WorkflowConfig{}, err
	}
	wf, err := workflow.Load(o.RepoRoot)
	if err != nil {
		return ticketdomain.State{}, workflow.WorkflowConfig{}, fmt.Errorf("load workflow: %w", err)
	}
	return st, wf, nil
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

func currentRunLogPath(st ticketdomain.State) string {
	if st.CurrentRunID == "" {
		return st.ArtifactPath(st.CurrentState + ".log")
	}
	for _, run := range st.StateHistory {
		if run.ID == st.CurrentRunID && run.LogRef != "" {
			return st.ResolveRef(run.LogRef)
		}
	}
	return st.ArtifactPath(st.CurrentState + ".log")
}

func ensureRunHistory(st *ticketdomain.State) {
	if st == nil || len(st.StateHistory) > 0 || st.CurrentState == "" {
		return
	}
	runID := "legacy-" + strings.ReplaceAll(st.CurrentState, "/", "-")
	if !st.UpdatedAt.IsZero() {
		runID = fmt.Sprintf("legacy-%s-%d", strings.ReplaceAll(st.CurrentState, "/", "-"), st.UpdatedAt.UTC().Unix())
	}
	st.CurrentRunID = runID
	st.StateHistory = []ticketdomain.StateRun{
		{
			ID:               runID,
			StateName:        st.CurrentState,
			StateDisplayName: st.CurrentState,
			StartedAt:        st.UpdatedAt,
			LogRef:           filepath.ToSlash(filepath.Join("runs", runID, "state.log")),
		},
	}
}

func latestArtifactRef(st ticketdomain.State, stateName string) string {
	for i := len(st.StateHistory) - 1; i >= 0; i-- {
		run := st.StateHistory[i]
		if run.StateName == stateName && run.ArtifactRef != "" {
			return run.ArtifactRef
		}
	}
	return ""
}

func (o *Orchestrator) prepareRunContext(st ticketdomain.State, stateCfg workflow.StateConfig, run ticketdomain.StateRun) error {
	runDir := st.RunPath(run.ID)
	if err := os.MkdirAll(filepath.Join(runDir, "artifacts"), 0o755); err != nil {
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
	fmt.Fprintf(&b, "Current Raw Provider Log: %s\n", filepath.ToSlash(filepath.Join(".auto-pr", "runs", run.ID, "raw-provider.log")))
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
		if ref := latestArtifactRef(st, stateName); ref != "" {
			fmt.Fprintf(&b, "- %s: %s\n", stateName, filepath.ToSlash(filepath.Join(".auto-pr", ref)))
		}
	}

	return os.WriteFile(st.ArtifactPath("run-context.md"), []byte(b.String()), 0o644)
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
