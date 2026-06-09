package state_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	workflowstate "github.com/Neokil/AutoPR/internal/domain/workflowstate"
	"github.com/Neokil/AutoPR/internal/state"
)

func writeLegacyState(t *testing.T, store *state.Store, ticketState workflowstate.State) {
	t.Helper()
	err := os.MkdirAll(store.TicketDir(ticketState.TicketNumber), 0o755)
	if err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	data, err := json.MarshalIndent(ticketState, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	err = os.WriteFile(filepath.Join(store.TicketDir(ticketState.TicketNumber), state.StateFileName), data, 0o644)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func TestLoadStateFallsBackToLegacyStateFile(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	store := state.NewStore(repoRoot, ".auto-pr-state")
	ticketState := workflowstate.New("GH-12")
	writeLegacyState(t, store, ticketState)

	got, err := store.LoadState(ticketState.TicketNumber)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if got.TicketNumber != ticketState.TicketNumber {
		t.Fatalf("expected ticket %q, got %q", ticketState.TicketNumber, got.TicketNumber)
	}
}

func TestSaveStateWritesWorktreeStateWithoutLegacyDir(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	store := state.NewStore(repoRoot, ".auto-pr-state")
	ticketState := workflowstate.New("GH-12")
	ticketState.WorktreePath = filepath.Join(repoRoot, ".auto-pr-state", "worktrees", ticketState.TicketNumber)

	err := store.SaveState(ticketState.TicketNumber, ticketState)
	if err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	worktreeStatePath := filepath.Join(ticketState.WorktreePath, ".auto-pr", state.StateFileName)
	_, worktreeStateErr := os.Stat(worktreeStatePath)
	if worktreeStateErr != nil {
		t.Fatalf("expected worktree state file at %s: %v", worktreeStatePath, worktreeStateErr)
	}

	_, legacyDirErr := os.Stat(store.TicketDir(ticketState.TicketNumber))
	if !os.IsNotExist(legacyDirErr) {
		t.Fatalf("expected no legacy ticket dir, got err=%v", legacyDirErr)
	}
}

func TestSaveStateMigratesStateAndRemovesEmptyLegacyDir(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	store := state.NewStore(repoRoot, ".auto-pr-state")
	ticketState := workflowstate.New("GH-12")

	writeLegacyState(t, store, ticketState)

	ticketState.WorktreePath = filepath.Join(repoRoot, ".auto-pr-state", "worktrees", ticketState.TicketNumber)
	err := store.SaveState(ticketState.TicketNumber, ticketState)
	if err != nil {
		t.Fatalf("migrating SaveState() error = %v", err)
	}

	worktreeStatePath := filepath.Join(ticketState.WorktreePath, ".auto-pr", state.StateFileName)
	_, worktreeStateErr := os.Stat(worktreeStatePath)
	if worktreeStateErr != nil {
		t.Fatalf("expected worktree state file at %s: %v", worktreeStatePath, worktreeStateErr)
	}

	legacyStatePath := filepath.Join(store.TicketDir(ticketState.TicketNumber), state.StateFileName)
	_, legacyStateErr := os.Stat(legacyStatePath)
	if !os.IsNotExist(legacyStateErr) {
		t.Fatalf("expected legacy state file to be removed, got err=%v", legacyStateErr)
	}

	_, legacyDirErr := os.Stat(store.TicketDir(ticketState.TicketNumber))
	if !os.IsNotExist(legacyDirErr) {
		t.Fatalf("expected empty legacy ticket dir to be removed, got err=%v", legacyDirErr)
	}
}

func TestSaveStateKeepsLegacyDirWhenItContainsOtherFiles(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	store := state.NewStore(repoRoot, ".auto-pr-state")
	ticketState := workflowstate.New("GH-12")

	writeLegacyState(t, store, ticketState)

	notePath := filepath.Join(store.TicketDir(ticketState.TicketNumber), "note.txt")
	err := os.WriteFile(notePath, []byte("keep me\n"), 0o644)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	ticketState.WorktreePath = filepath.Join(repoRoot, ".auto-pr-state", "worktrees", ticketState.TicketNumber)
	err = store.SaveState(ticketState.TicketNumber, ticketState)
	if err != nil {
		t.Fatalf("migrating SaveState() error = %v", err)
	}

	_, noteStatErr := os.Stat(notePath)
	if noteStatErr != nil {
		t.Fatalf("expected extra file to remain in legacy dir: %v", noteStatErr)
	}
}

func TestListTicketDirsIncludesMigratedWorktreeState(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	store := state.NewStore(repoRoot, ".auto-pr-state")
	ticketState := workflowstate.New("GH-12")

	writeLegacyState(t, store, ticketState)

	ticketState.WorktreePath = filepath.Join(repoRoot, ".auto-pr-state", "worktrees", ticketState.TicketNumber)
	err := store.SaveState(ticketState.TicketNumber, ticketState)
	if err != nil {
		t.Fatalf("migrating SaveState() error = %v", err)
	}

	tickets, err := store.ListTicketDirs()
	if err != nil {
		t.Fatalf("ListTicketDirs() error = %v", err)
	}
	if !slices.Contains(tickets, ticketState.TicketNumber) {
		t.Fatalf("expected migrated ticket %q in %v", ticketState.TicketNumber, tickets)
	}
}
