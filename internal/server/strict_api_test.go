package server //nolint:testpackage // needs access to unexported enqueueJob and queuedJob internals

import (
	"net/http"
	"testing"

	"github.com/Neokil/AutoPR/internal/api"
	"github.com/Neokil/AutoPR/internal/serverstate"
)

func TestEnqueueJobPreservesActionValue(t *testing.T) {
	t.Parallel()

	statePath := t.TempDir() + "/state.json"
	store, err := serverstate.NewStore(statePath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	srv := &server{
		meta:        store,
		jobs:        make(chan queuedJob, 1),
		subscribers: map[string]chan api.ServerEvent{},
	}

	resp, code, err := srv.enqueueJob(jobCleanupAll, "repo-1", "/tmp/repo", "", enqueueOptions{scope: "all"})
	if err != nil {
		t.Fatalf("enqueueJob() error = %v", err)
	}
	if code != http.StatusAccepted {
		t.Fatalf("enqueueJob() code = %d, want %d", code, http.StatusAccepted)
	}
	if resp.Action != string(jobCleanupAll) {
		t.Fatalf("resp.Action = %q, want %q", resp.Action, jobCleanupAll)
	}

	stored, ok := store.GetJob(resp.JobId)
	if !ok {
		t.Fatalf("GetJob(%q) did not find stored job", resp.JobId)
	}
	if stored.Action != jobCleanupAll {
		t.Fatalf("stored.Action = %q, want %q", stored.Action, jobCleanupAll)
	}

	select {
	case queued := <-srv.jobs:
		if queued.record.Action != jobCleanupAll {
			t.Fatalf("queued.record.Action = %q, want %q", queued.record.Action, jobCleanupAll)
		}
	default:
		t.Fatal("expected queued job in channel")
	}
}
