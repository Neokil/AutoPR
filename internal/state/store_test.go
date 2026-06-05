package state_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	workflowstate "github.com/Neokil/AutoPR/internal/domain/workflowstate"
	"github.com/Neokil/AutoPR/internal/state"
)

func TestSaveStateCreatesPreWorktreeStateFile(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	store := state.NewStore(repoRoot, ".auto-pr-state")
	ticketState := workflowstate.New("GH-12")

	err := store.SaveState(ticketState.TicketNumber, ticketState)
	if err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	statePath := filepath.Join(store.TicketDir(ticketState.TicketNumber), state.StateFileName)
	_, statErr := os.Stat(statePath)
	if statErr != nil {
		t.Fatalf("expected pre-worktree state file at %s: %v", statePath, statErr)
	}
}

func TestSaveStateMigratesStateAndRemovesEmptyLegacyDir(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	store := state.NewStore(repoRoot, ".auto-pr-state")
	ticketState := workflowstate.New("GH-12")

	err := store.SaveState(ticketState.TicketNumber, ticketState)
	if err != nil {
		t.Fatalf("initial SaveState() error = %v", err)
	}

	ticketState.WorktreePath = filepath.Join(repoRoot, ".auto-pr-state", "worktrees", ticketState.TicketNumber)
	err = store.SaveState(ticketState.TicketNumber, ticketState)
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

	err := store.SaveState(ticketState.TicketNumber, ticketState)
	if err != nil {
		t.Fatalf("initial SaveState() error = %v", err)
	}

	notePath := filepath.Join(store.TicketDir(ticketState.TicketNumber), "note.txt")
	err = os.WriteFile(notePath, []byte("keep me\n"), 0o644)
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

	err := store.SaveState(ticketState.TicketNumber, ticketState)
	if err != nil {
		t.Fatalf("initial SaveState() error = %v", err)
	}

	ticketState.WorktreePath = filepath.Join(repoRoot, ".auto-pr-state", "worktrees", ticketState.TicketNumber)
	err = store.SaveState(ticketState.TicketNumber, ticketState)
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
