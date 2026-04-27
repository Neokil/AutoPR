package server

import (
	"fmt"
	"os"
	"strings"

	workflowstate "github.com/Neokil/AutoPR/internal/domain/workflowstate"
	"github.com/Neokil/AutoPR/internal/markdown"
)

func (s *server) persistTicketFailure(repoID, repoRoot, ticket string, repoRt *repoRuntime, job queuedJob, cause error) error {
	if strings.TrimSpace(ticket) == "" {
		return nil
	}

	ticketState, err := repoRt.store.LoadState(ticket)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("load ticket state: %w", err)
		}
		ticketState = workflowstate.New(ticket)
	}

	msg := strings.TrimSpace(cause.Error())
	ticketState.FlowStatus = workflowstate.FlowStatusFailed
	ticketState.LastError = msg
	saveErr := repoRt.store.SaveState(ticket, ticketState)
	if saveErr != nil {
		return fmt.Errorf("save ticket state: %w", saveErr)
	}

	body := msg
	if job.record.Action != "" {
		body = fmt.Sprintf("Action: %s\n\n%s", job.record.Action, msg)
	}
	if ticketState.WorktreePath != "" && ticketState.CurrentState != "" {
		logPath := ticketState.CurrentRunLogPath()
		_ = markdown.AppendSection(logPath, "Job Failed", body)
	}

	return s.syncTicketFromRepo(repoID, repoRoot, ticket, repoRt, true)
}
