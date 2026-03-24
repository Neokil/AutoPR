package main

import (
	"errors"
	"os"
	"strings"

	ticketdomain "ai-ticket-worker/internal/domain/ticket"
	"ai-ticket-worker/internal/servermeta"
)

func (s *server) ensureQueuedTicket(repoID, repoRoot, ticket string) error {
	rt, err := s.runtimeForRepo(repoRoot)
	if err != nil {
		return err
	}
	if _, err := rt.store.LoadState(ticket); err == nil {
		return s.syncTicketFromRepo(repoID, repoRoot, ticket, rt, true)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	paths := rt.store.Paths(ticket)
	st := ticketdomain.NewState(ticket)
	st.ProposalPath = paths.Proposal
	st.FinalPath = paths.Final
	st.LogPath = paths.Log
	st.PRPath = paths.PR
	st.ChecksLogPath = paths.Checks
	st.TicketJSONPath = paths.Ticket
	st.ProviderDirPath = paths.ProviderDir
	if err := rt.store.SaveState(ticket, st); err != nil {
		return err
	}
	return s.syncTicketFromRepo(repoID, repoRoot, ticket, rt, true)
}

func (s *server) syncTicketFromRepo(repoID, repoRoot, ticket string, rt *repoRuntime, emitEvent bool) error {
	st, err := rt.store.LoadState(ticket)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := s.meta.DeleteTicket(repoID, ticket); err != nil {
				return err
			}
			if emitEvent {
				s.broadcast(serverEvent{
					Type:         "ticket_deleted",
					RepoID:       repoID,
					RepoPath:     repoRoot,
					TicketNumber: ticket,
				})
			}
			return nil
		}
		return err
	}
	t, _ := rt.store.LoadTicket(ticket)
	rec := servermeta.TicketRecord{
		RepoID:       repoID,
		RepoPath:     repoRoot,
		TicketNumber: ticket,
		Title:        strings.TrimSpace(t.Title),
		Status:       string(st.Status),
		Approved:     st.Approved,
		LastError:    strings.TrimSpace(st.LastError),
		UpdatedAt:    st.UpdatedAt.UTC(),
		PRURL:        st.PRURL,
	}
	if err := s.meta.UpsertTicket(rec); err != nil {
		return err
	}
	if emitEvent {
		s.broadcast(serverEvent{
			Type:         "ticket_updated",
			RepoID:       repoID,
			RepoPath:     repoRoot,
			TicketNumber: ticket,
			Title:        rec.Title,
			Status:       rec.Status,
			Error:        rec.LastError,
			PRURL:        rec.PRURL,
		})
	}
	return nil
}

func (s *server) syncRepoTickets(repoID, repoRoot string, rt *repoRuntime, emitEvent bool) error {
	tickets, err := rt.store.ListTicketDirs()
	if err != nil {
		return err
	}
	records := make([]servermeta.TicketRecord, 0, len(tickets))
	for _, t := range tickets {
		st, err := rt.store.LoadState(t)
		if err != nil {
			continue
		}
		ticketData, _ := rt.store.LoadTicket(t)
		records = append(records, servermeta.TicketRecord{
			RepoID:       repoID,
			RepoPath:     repoRoot,
			TicketNumber: t,
			Title:        strings.TrimSpace(ticketData.Title),
			Status:       string(st.Status),
			Approved:     st.Approved,
			LastError:    strings.TrimSpace(st.LastError),
			UpdatedAt:    st.UpdatedAt.UTC(),
			PRURL:        st.PRURL,
		})
	}
	if err := s.meta.ReplaceRepoTickets(repoID, records); err != nil {
		return err
	}
	if err := s.meta.PruneTicketJobs(repoID, tickets); err != nil {
		return err
	}
	if emitEvent {
		s.broadcast(serverEvent{
			Type:     "repo_tickets_synced",
			RepoID:   repoID,
			RepoPath: repoRoot,
		})
	}
	return nil
}
