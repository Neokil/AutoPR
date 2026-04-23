package main

import (
	"errors"
	"os"
	"strings"

	ticketdomain "github.com/Neokil/AutoPR/internal/domain/ticket"
	"github.com/Neokil/AutoPR/internal/servermeta"
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

	st := ticketdomain.NewState(ticket)
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
	title := ticketTitleForDisplay(st)
	rec := servermeta.TicketRecord{
		RepoID:       repoID,
		RepoPath:     repoRoot,
		TicketNumber: ticket,
		Title:        title,
		Status:       string(st.FlowStatus),
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
		records = append(records, servermeta.TicketRecord{
			RepoID:       repoID,
			RepoPath:     repoRoot,
			TicketNumber: t,
			Title:        ticketTitleForDisplay(st),
			Status:       string(st.FlowStatus),
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

func ticketTitleForDisplay(st ticketdomain.State) string {
	artifactRef := st.LatestArtifactRef("fetch-ticket-data")
	if artifactRef == "" {
		return ""
	}
	data, err := os.ReadFile(st.ResolveRef(artifactRef))
	if err != nil {
		return ""
	}
	return extractMarkdownTitle(string(data))
}


func extractMarkdownTitle(content string) string {
	for rawLine := range strings.SplitSeq(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			title := strings.TrimSpace(strings.TrimLeft(line, "#"))
			if title != "" {
				return title
			}
		}
		for _, prefix := range []string{"Title:", "**Title:**", "- Title:", "* Title:"} {
			if rest, ok := strings.CutPrefix(line, prefix); ok {
				title := strings.TrimSpace(rest)
				title = strings.Trim(title, "*_` ")
				if title != "" {
					return title
				}
			}
		}
		return strings.Trim(line, "*_` ")
	}
	return ""
}
