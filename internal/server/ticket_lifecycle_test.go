package server

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Neokil/AutoPR/internal/serverstate"
	"github.com/Neokil/AutoPR/internal/state"
)

func newTestServer(t *testing.T, repoRoot string) (*server, *state.Store, string) {
	t.Helper()

	meta, err := serverstate.NewStore(filepath.Join(t.TempDir(), "server-state.json"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	store := state.NewStore(repoRoot, ".auto-pr-state")
	repoID := "repo-1"

	return &server{
		meta: meta,
		runtimes: map[string]*repoRuntime{
			repoRoot: {
				repoRoot: repoRoot,
				store:    store,
			},
		},
	}, store, repoID
}

func TestEnsureQueuedTicketDoesNotPersistFreshState(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	srv, store, repoID := newTestServer(t, repoRoot)

	err := srv.ensureQueuedTicket(repoID, repoRoot, "GH-12")
	if err != nil {
		t.Fatalf("ensureQueuedTicket() error = %v", err)
	}

	_, loadErr := store.LoadState("GH-12")
	if !errors.Is(loadErr, os.ErrNotExist) {
		t.Fatalf("expected no persisted state, got err=%v", loadErr)
	}

	_, statErr := os.Stat(store.TicketDir("GH-12"))
	if !os.IsNotExist(statErr) {
		t.Fatalf("expected no legacy ticket dir, got err=%v", statErr)
	}
}

func TestPersistTicketFailureWithoutStateDeletesMetadataOnly(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	srv, store, repoID := newTestServer(t, repoRoot)
	err := srv.meta.UpsertTicket(serverstate.TicketRecord{
		RepoID:       repoID,
		RepoPath:     repoRoot,
		TicketNumber: "GH-12",
		Status:       "queued",
		UpdatedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("UpsertTicket() error = %v", err)
	}

	err = srv.persistTicketFailure(repoID, repoRoot, "GH-12", &repoRuntime{repoRoot: repoRoot, store: store}, queuedJob{
		record: serverstate.JobRecord{Action: jobRun},
	}, errors.New("worktree creation failed"))
	if err != nil {
		t.Fatalf("persistTicketFailure() error = %v", err)
	}

	_, loadErr := store.LoadState("GH-12")
	if !errors.Is(loadErr, os.ErrNotExist) {
		t.Fatalf("expected no persisted state, got err=%v", loadErr)
	}

	_, statErr := os.Stat(store.TicketDir("GH-12"))
	if !os.IsNotExist(statErr) {
		t.Fatalf("expected no legacy ticket dir, got err=%v", statErr)
	}

	if records := srv.meta.ListTickets(repoID); len(records) != 0 {
		t.Fatalf("expected metadata to be removed, got %#v", records)
	}
}
