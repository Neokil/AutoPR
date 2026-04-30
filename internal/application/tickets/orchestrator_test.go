package tickets_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Neokil/AutoPR/internal/application/tickets"
	"github.com/Neokil/AutoPR/internal/config"
	workflowstate "github.com/Neokil/AutoPR/internal/domain/workflowstate"
	"github.com/Neokil/AutoPR/internal/providers"
)

// ── in-memory mocks ────────────────────────────────────────────────────────

type memStore struct {
	states map[string]workflowstate.State
}

func newMemStore() *memStore { return &memStore{states: map[string]workflowstate.State{}} }

func (m *memStore) LoadState(ticketNumber string) (workflowstate.State, error) {
	st, ok := m.states[ticketNumber]
	if !ok {
		return workflowstate.State{}, &os.PathError{Op: "open", Err: os.ErrNotExist}
	}

	return st, nil
}

func (m *memStore) SaveState(ticketNumber string, st workflowstate.State) error {
	m.states[ticketNumber] = st

	return nil
}

func (m *memStore) ListTicketDirs() ([]string, error) {
	dirs := make([]string, 0, len(m.states))
	for k := range m.states {
		dirs = append(dirs, k)
	}

	return dirs, nil
}

func (m *memStore) RemoveTicketDir(ticketNumber string) error {
	delete(m.states, ticketNumber)

	return nil
}

type mockProvider struct {
	result  providers.ExecuteResult
	err     error
	lastReq providers.ExecuteRequest
}

func (p *mockProvider) Name() string { return "mock" }
func (p *mockProvider) Execute(_ context.Context, req providers.ExecuteRequest) (providers.ExecuteResult, error) {
	p.lastReq = req
	return p.result, p.err
}

// ── test helpers ───────────────────────────────────────────────────────────

// setupRepo creates a temp directory with a minimal .auto-pr/workflow.yaml and
// a prompt file. Returns the repo root.
func setupRepo(t *testing.T, yaml string, promptContent string) string {
	t.Helper()
	root := t.TempDir()
	autopr := filepath.Join(root, ".auto-pr")
	if err := os.MkdirAll(filepath.Join(autopr, "prompts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(autopr, "workflow.yaml"), []byte(yaml), 0o644); err != nil { //nolint:gosec // test file
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(autopr, "prompts", "step.md"), []byte(promptContent), 0o644); err != nil { //nolint:gosec // test file
		t.Fatal(err)
	}

	return root
}

const minimalWorkflow = `
states:
  - name: investigate
    prompt: prompts/step.md
    actions:
      - label: Approve
        type: move_to_state
        target: done
      - label: Feedback
        type: provide_feedback
`

// prepareWorktree creates a temp dir to act as the worktree and pre-populates
// the directories that runState needs, then saves the state so ensureWorktreeAndContext
// is skipped (WorktreePath != "").
func prepareWorktree(t *testing.T, store *memStore, ticketNumber string) string {
	t.Helper()
	wt := t.TempDir()
	st := workflowstate.New(ticketNumber)
	st.WorktreePath = wt
	st.BranchName = "auto-pr/" + ticketNumber
	st.FlowStatus = workflowstate.FlowStatusPending
	if err := store.SaveState(ticketNumber, st); err != nil {
		t.Fatal(err)
	}
	// Create the .auto-pr sub-directory so context file writes succeed.
	if err := os.MkdirAll(filepath.Join(wt, ".auto-pr"), 0o755); err != nil {
		t.Fatal(err)
	}

	return wt
}

func newOrchestrator(repoRoot string, store *memStore, prov *mockProvider) *tickets.Orchestrator {
	return tickets.NewWithStore(
		config.Config{StateDirName: ".auto-pr-state"},
		repoRoot,
		store,
		prov,
	)
}

// ── StartFlow ─────────────────────────────────────────────────────────────

func TestStartFlow_newTicket_endsWaiting(t *testing.T) {
	t.Parallel()
	root := setupRepo(t, minimalWorkflow, "Investigate this ticket.")
	store := newMemStore()
	prov := &mockProvider{result: providers.ExecuteResult{RawOutput: "analysis done"}}
	prepareWorktree(t, store, "42")

	err := newOrchestrator(root, store, prov).StartFlow(context.Background(), "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	st, _ := store.LoadState("42")
	if st.FlowStatus != workflowstate.FlowStatusWaiting {
		t.Errorf("expected waiting, got %q", st.FlowStatus)
	}
	if st.CurrentState != "investigate" {
		t.Errorf("expected state=investigate, got %q", st.CurrentState)
	}
}

func TestStartFlow_doneTicket_isNoop(t *testing.T) {
	t.Parallel()
	root := setupRepo(t, minimalWorkflow, ".")
	store := newMemStore()
	prov := &mockProvider{}
	wt := t.TempDir()
	st := workflowstate.New("10")
	st.WorktreePath = wt
	st.FlowStatus = workflowstate.FlowStatusDone
	_ = store.SaveState("10", st)

	err := newOrchestrator(root, store, prov).StartFlow(context.Background(), "10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Provider must not have been called.
	if prov.err != nil {
		t.Error("provider should not have been invoked")
	}
}

func TestStartFlow_runningTicket_returnsError(t *testing.T) {
	t.Parallel()
	root := setupRepo(t, minimalWorkflow, ".")
	store := newMemStore()
	wt := t.TempDir()
	st := workflowstate.New("7")
	st.WorktreePath = wt
	st.FlowStatus = workflowstate.FlowStatusRunning
	_ = store.SaveState("7", st)

	err := newOrchestrator(root, store, &mockProvider{}).StartFlow(context.Background(), "7")
	if !errors.Is(err, tickets.ErrTicketRunning) {
		t.Errorf("expected ErrTicketRunning, got %v", err)
	}
}

func TestStartFlow_providerError_setsFailedStatus(t *testing.T) {
	t.Parallel()
	root := setupRepo(t, minimalWorkflow, ".")
	store := newMemStore()
	provErr := errors.New("provider exploded")
	prov := &mockProvider{err: provErr}
	prepareWorktree(t, store, "5")

	err := newOrchestrator(root, store, prov).StartFlow(context.Background(), "5")
	if err == nil {
		t.Fatal("expected error from provider")
	}
	st, _ := store.LoadState("5")
	if st.FlowStatus != workflowstate.FlowStatusFailed {
		t.Errorf("expected failed, got %q", st.FlowStatus)
	}
	if st.LastError == "" {
		t.Error("expected LastError to be set")
	}
}

func TestDiscoverTickets_persistsLogsUnderUserHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	root := setupRepo(t, minimalWorkflow, ".")
	store := newMemStore()
	prov := &mockProvider{}
	orch := tickets.NewWithStore(
		config.Config{
			StateDirName:           ".auto-pr-state",
			DiscoverTicketsCommand: `printf '%s\n' '[{"ticket_number":"SC-1","title":"Test"}]'`,
		},
		root,
		store,
		prov,
	)

	found, err := orch.DiscoverTickets(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(found) != 1 || found[0].TicketNumber != "SC-1" {
		t.Fatalf("unexpected discovered tickets: %#v", found)
	}

	logsRoot := filepath.Join(home, ".auto-pr", "logs", "discover-tickets")
	entries, err := os.ReadDir(logsRoot)
	if err != nil {
		t.Fatalf("read discover logs dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one discover log dir, got %d", len(entries))
	}
	runDir := filepath.Join(logsRoot, entries[0].Name())
	if _, err := os.Stat(filepath.Join(runDir, "command.sh")); err != nil {
		t.Fatalf("expected persisted command.sh: %v", err)
	}
	stdoutPath := filepath.Join(runDir, "command-output.json")
	if _, err := os.Stat(stdoutPath); err != nil {
		t.Fatalf("expected persisted command-output.json: %v", err)
	}
	resultPath := filepath.Join(runDir, "result.json")
	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatalf("read result.json: %v", err)
	}
	if strings.TrimSpace(string(data)) != `[{"ticket_number":"SC-1","title":"Test"}]` {
		t.Fatalf("unexpected result.json contents: %q", string(data))
	}
}

// ── ApplyAction ────────────────────────────────────────────────────────────

func TestApplyAction_notWaiting_returnsError(t *testing.T) {
	t.Parallel()
	root := setupRepo(t, minimalWorkflow, ".")
	store := newMemStore()
	wt := t.TempDir()
	st := workflowstate.New("3")
	st.WorktreePath = wt
	st.FlowStatus = workflowstate.FlowStatusRunning
	_ = store.SaveState("3", st)

	err := newOrchestrator(root, store, &mockProvider{}).ApplyAction(context.Background(), "3", "Approve", "")
	if !errors.Is(err, tickets.ErrTicketNotWaiting) {
		t.Errorf("expected ErrTicketNotWaiting, got %v", err)
	}
}

func TestApplyAction_unknownLabel_returnsError(t *testing.T) {
	t.Parallel()
	root := setupRepo(t, minimalWorkflow, ".")
	store := newMemStore()
	wt := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wt, ".auto-pr"), 0o755); err != nil {
		t.Fatal(err)
	}
	st := workflowstate.New("8")
	st.WorktreePath = wt
	st.CurrentState = "investigate"
	st.FlowStatus = workflowstate.FlowStatusWaiting
	_ = store.SaveState("8", st)

	err := newOrchestrator(root, store, &mockProvider{}).ApplyAction(context.Background(), "8", "NoSuchAction", "")
	if !errors.Is(err, tickets.ErrActionNotFound) {
		t.Errorf("expected ErrActionNotFound, got %v", err)
	}
}

func TestApplyAction_moveToStateDone_setsDone(t *testing.T) {
	t.Parallel()
	root := setupRepo(t, minimalWorkflow, ".")
	store := newMemStore()
	wt := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wt, ".auto-pr"), 0o755); err != nil {
		t.Fatal(err)
	}
	st := workflowstate.New("99")
	st.WorktreePath = wt
	st.CurrentState = "investigate"
	st.FlowStatus = workflowstate.FlowStatusWaiting
	_ = store.SaveState("99", st)

	err := newOrchestrator(root, store, &mockProvider{}).ApplyAction(context.Background(), "99", "Approve", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	result, _ := store.LoadState("99")
	if result.FlowStatus != workflowstate.FlowStatusDone {
		t.Errorf("expected done, got %q", result.FlowStatus)
	}
}

func TestApplyAction_provideFeedback_emptyMessage_returnsError(t *testing.T) {
	t.Parallel()
	root := setupRepo(t, minimalWorkflow, ".")
	store := newMemStore()
	wt := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wt, ".auto-pr"), 0o755); err != nil {
		t.Fatal(err)
	}
	st := workflowstate.New("11")
	st.WorktreePath = wt
	st.CurrentState = "investigate"
	st.FlowStatus = workflowstate.FlowStatusWaiting
	_ = store.SaveState("11", st)

	err := newOrchestrator(root, store, &mockProvider{}).ApplyAction(context.Background(), "11", "Feedback", "")
	if !errors.Is(err, tickets.ErrFeedbackRequired) {
		t.Errorf("expected ErrFeedbackRequired, got %v", err)
	}
}

func TestApplyAction_provideFeedback_reruns(t *testing.T) {
	t.Parallel()
	root := setupRepo(t, minimalWorkflow, ".")
	store := newMemStore()
	wt := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wt, ".auto-pr"), 0o755); err != nil {
		t.Fatal(err)
	}
	st := workflowstate.New("12")
	st.WorktreePath = wt
	st.CurrentState = "investigate"
	st.FlowStatus = workflowstate.FlowStatusWaiting
	_ = store.SaveState("12", st)
	prov := &mockProvider{result: providers.ExecuteResult{RawOutput: "re-investigated"}}

	err := newOrchestrator(root, store, prov).ApplyAction(context.Background(), "12", "Feedback", "please dig deeper")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	result, _ := store.LoadState("12")
	if result.FlowStatus != workflowstate.FlowStatusWaiting {
		t.Errorf("expected waiting after rerun, got %q", result.FlowStatus)
	}
}

// ── CleanupTicket ──────────────────────────────────────────────────────────

func TestCleanupTicket_removesState(t *testing.T) {
	t.Parallel()
	root := setupRepo(t, minimalWorkflow, ".")
	store := newMemStore()
	wt := t.TempDir()
	st := workflowstate.New("20")
	st.WorktreePath = wt
	st.FlowStatus = workflowstate.FlowStatusDone
	_ = store.SaveState("20", st)

	err := newOrchestrator(root, store, &mockProvider{}).CleanupTicket(context.Background(), "20")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dirs, _ := store.ListTicketDirs()
	for _, d := range dirs {
		if d == "20" {
			t.Error("expected ticket 20 to be removed from store")
		}
	}
}

// ── EnsureStateIgnored ─────────────────────────────────────────────────────

func TestEnsureStateIgnored_appendsEntry(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := tickets.EnsureStateIgnored(root, ".auto-pr-state"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	contents, err := os.ReadFile(filepath.Join(root, ".gitignore")) //nolint:gosec // G304: test path
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !strings.Contains(string(contents), ".auto-pr-state/") {
		t.Errorf(".gitignore does not contain .auto-pr-state/, got: %q", string(contents))
	}
}

func TestEnsureStateIgnored_idempotent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ignorePath := filepath.Join(root, ".gitignore")
	if err := os.WriteFile(ignorePath, []byte(".auto-pr-state/\n"), 0o644); err != nil { //nolint:gosec // test file
		t.Fatal(err)
	}
	before, _ := os.ReadFile(ignorePath) //nolint:gosec // G304: test path
	if err := tickets.EnsureStateIgnored(root, ".auto-pr-state"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	after, _ := os.ReadFile(ignorePath) //nolint:gosec // G304: test path
	if string(before) != string(after) {
		t.Errorf("file should not change when entry already present")
	}
}

// ── workflow config for multi-state transition ─────────────────────────────

const twoStateWorkflow = `
states:
  - name: investigate
    prompt: prompts/step.md
    actions:
      - label: Continue
        type: move_to_state
        target: implement
  - name: implement
    prompt: prompts/step.md
    actions:
      - label: Finish
        type: move_to_state
        target: done
`

func TestApplyAction_moveToNextState_runsNextState(t *testing.T) {
	t.Parallel()
	root := setupRepo(t, twoStateWorkflow, "do work")
	store := newMemStore()
	wt := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wt, ".auto-pr"), 0o755); err != nil {
		t.Fatal(err)
	}
	st := workflowstate.New("50")
	st.WorktreePath = wt
	st.CurrentState = "investigate"
	st.FlowStatus = workflowstate.FlowStatusWaiting
	_ = store.SaveState("50", st)
	prov := &mockProvider{result: providers.ExecuteResult{RawOutput: "implemented"}}

	err := newOrchestrator(root, store, prov).ApplyAction(context.Background(), "50", "Continue", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	result, _ := store.LoadState("50")
	if result.CurrentState != "implement" {
		t.Errorf("expected state=implement, got %q", result.CurrentState)
	}
	if result.FlowStatus != workflowstate.FlowStatusWaiting {
		t.Errorf("expected waiting, got %q", result.FlowStatus)
	}
}
