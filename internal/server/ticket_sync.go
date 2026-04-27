package server

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Neokil/AutoPR/internal/api"
	workflowstate "github.com/Neokil/AutoPR/internal/domain/workflowstate"
	"github.com/Neokil/AutoPR/internal/serverstate"
)

func (s *server) ensureQueuedTicket(repoID, repoRoot, ticket string) error {
	repoRt, err := s.runtimeForRepo(repoRoot)
	if err != nil {
		return err
	}
	_, loadErr := repoRt.store.LoadState(ticket)
	if loadErr == nil {
		return s.syncTicketFromRepo(repoID, repoRoot, ticket, repoRt, true)
	}
	if !errors.Is(loadErr, os.ErrNotExist) {
		return fmt.Errorf("load ticket state: %w", loadErr)
	}

	st := workflowstate.New(ticket)
	err = repoRt.store.SaveState(ticket, st)
	if err != nil {
		return fmt.Errorf("save initial ticket state: %w", err)
	}

	return s.syncTicketFromRepo(repoID, repoRoot, ticket, repoRt, true)
}

func (s *server) syncTicketFromRepo(repoID, repoRoot, ticket string, rt *repoRuntime, emitEvent bool) error {
	ticketState, err := rt.store.LoadState(ticket)
	if errors.Is(err, os.ErrNotExist) {
		delErr := s.meta.DeleteTicket(repoID, ticket)
		if delErr != nil {
			return fmt.Errorf("delete ticket metadata: %w", delErr)
		}
		if emitEvent {
			s.broadcast(api.ServerEvent{
				Type:         "ticket_deleted",
				RepoId:       stringPtr(repoID),
				RepoPath:     stringPtr(repoRoot),
				TicketNumber: stringPtr(ticket),
			})
		}

		return nil
	}
	if err != nil {
		return fmt.Errorf("load ticket state: %w", err)
	}
	title := ticketTitleForDisplay(ticketState)
	rec := serverstate.TicketRecord{
		RepoID:       repoID,
		RepoPath:     repoRoot,
		TicketNumber: ticket,
		Title:        title,
		Status:       string(ticketState.FlowStatus),
		LastError:    strings.TrimSpace(ticketState.LastError),
		UpdatedAt:    ticketState.UpdatedAt.UTC(),
		PRURL:        ticketState.PRURL,
	}
	err = s.meta.UpsertTicket(rec)
	if err != nil {
		return fmt.Errorf("upsert ticket metadata: %w", err)
	}
	if emitEvent {
		s.broadcast(api.ServerEvent{
			Type:         "ticket_updated",
			RepoId:       stringPtr(repoID),
			RepoPath:     stringPtr(repoRoot),
			TicketNumber: stringPtr(ticket),
			Title:        stringPtr(rec.Title),
			Status:       stringPtr(rec.Status),
			Error:        stringPtr(rec.LastError),
			PrUrl:        stringPtr(rec.PRURL),
		})
	}

	return nil
}

func (s *server) syncRepoTickets(repoID, repoRoot string, repoRt *repoRuntime, emitEvent bool) error {
	tickets, err := repoRt.store.ListTicketDirs()
	if err != nil {
		return fmt.Errorf("list ticket dirs: %w", err)
	}
	records := make([]serverstate.TicketRecord, 0, len(tickets))
	for _, ticketNum := range tickets {
		ticketState, err := repoRt.store.LoadState(ticketNum)
		if err != nil {
			continue
		}
		records = append(records, serverstate.TicketRecord{
			RepoID:       repoID,
			RepoPath:     repoRoot,
			TicketNumber: ticketNum,
			Title:        ticketTitleForDisplay(ticketState),
			Status:       string(ticketState.FlowStatus),
			LastError:    strings.TrimSpace(ticketState.LastError),
			UpdatedAt:    ticketState.UpdatedAt.UTC(),
			PRURL:        ticketState.PRURL,
		})
	}
	err = s.meta.ReplaceRepoTickets(repoID, records)
	if err != nil {
		return fmt.Errorf("replace repo tickets: %w", err)
	}
	err = s.meta.PruneTicketJobs(repoID, tickets)
	if err != nil {
		return fmt.Errorf("prune ticket jobs: %w", err)
	}
	if emitEvent {
		s.broadcast(api.ServerEvent{
			Type:     "repo_tickets_synced",
			RepoId:   stringPtr(repoID),
			RepoPath: stringPtr(repoRoot),
		})
	}

	return nil
}

func ticketTitleForDisplay(st workflowstate.State) string {
	artifactRef := st.LatestArtifactRef("fetch-ticket-data")
	if artifactRef == "" {
		return ""
	}
	data, err := os.ReadFile(st.ResolveRef(artifactRef)) //nolint:gosec // G703: path resolved from trusted internal state
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
