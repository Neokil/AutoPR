package serverstate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Neokil/AutoPR/internal/serverstate"
)

func TestNewJobPersistsAndReloadsTypedAction(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "state.json")
	store, err := serverstate.NewStore(statePath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	job, err := store.NewJob(serverstate.JobAction("cleanup_all"), "repo-1", "/tmp/repo", "SC-1", "all")
	if err != nil {
		t.Fatalf("NewJob() error = %v", err)
	}

	raw, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(raw), `"action": "cleanup_all"`) {
		t.Fatalf("persisted state missing action string: %s", string(raw))
	}

	reloaded, err := serverstate.NewStore(statePath)
	if err != nil {
		t.Fatalf("reloaded NewStore() error = %v", err)
	}
	stored, ok := reloaded.GetJob(job.ID)
	if !ok {
		t.Fatalf("GetJob(%q) did not find reloaded job", job.ID)
	}
	if stored.Action != serverstate.JobAction("cleanup_all") {
		t.Fatalf("stored.Action = %q, want %q", stored.Action, serverstate.JobAction("cleanup_all"))
	}
}
