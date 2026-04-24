package main

import (
	"fmt"
	"os"
	"strings"

	ticketdomain "github.com/Neokil/AutoPR/internal/domain/ticket"
	"github.com/Neokil/AutoPR/internal/markdown"
)

func (s *server) persistTicketFailure(repoID, repoRoot, ticket string, rt *repoRuntime, job queuedJob, cause error) error {
	if strings.TrimSpace(ticket) == "" {
		return nil
	}

	st, err := rt.store.LoadState(ticket)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("load ticket state: %w", err)
		}
		st = ticketdomain.NewState(ticket)
	}

	msg := strings.TrimSpace(cause.Error())
	st.FlowStatus = ticketdomain.FlowStatusFailed
	st.LastError = msg
	saveErr := rt.store.SaveState(ticket, st)
	if saveErr != nil {
		return fmt.Errorf("save ticket state: %w", saveErr)
	}

	body := msg
	if job.record.Action != "" {
		body = fmt.Sprintf("Action: %s\n\n%s", job.record.Action, msg)
	}
	if st.WorktreePath != "" && st.CurrentState != "" {
		logPath := st.CurrentRunLogPath()
		_ = markdown.AppendSection(logPath, "Job Failed", body)
	}

	return s.syncTicketFromRepo(repoID, repoRoot, ticket, rt, true)
}
