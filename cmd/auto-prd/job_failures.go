package main

import (
	"fmt"
	"os"
	"strings"

	ticketdomain "ai-ticket-worker/internal/domain/ticket"
	"ai-ticket-worker/internal/markdown"
)

func (s *server) persistTicketFailure(repoID, repoRoot, ticket string, rt *repoRuntime, job queuedJob, cause error) error {
	if strings.TrimSpace(ticket) == "" {
		return nil
	}

	paths := rt.store.Paths(ticket)
	st, err := rt.store.LoadState(ticket)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		st = ticketdomain.NewState(ticket)
		st.ProposalPath = paths.Proposal
		st.FinalPath = paths.Final
		st.LogPath = paths.Log
		st.PRPath = paths.PR
		st.ChecksLogPath = paths.Checks
		st.TicketJSONPath = paths.Ticket
		st.ProviderDirPath = paths.ProviderDir
	}

	msg := strings.TrimSpace(cause.Error())
	st.Status = ticketdomain.StateFailed
	st.LastError = msg
	if saveErr := rt.store.SaveState(ticket, st); saveErr != nil {
		return saveErr
	}

	body := msg
	if job.record.Action != "" {
		body = fmt.Sprintf("Action: %s\n\n%s", job.record.Action, msg)
	}
	if logErr := markdown.AppendSection(st.LogPath, "Job Failed", body); logErr != nil {
		return logErr
	}

	return s.syncTicketFromRepo(repoID, repoRoot, ticket, rt, true)
}
