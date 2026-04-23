package main

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Neokil/AutoPR/internal/servermeta"
	"github.com/Neokil/AutoPR/internal/shell"
)

func (s *server) prMonitorLoop() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	time.Sleep(3 * time.Second)
	s.checkPRStatesOnce()
	for range ticker.C {
		s.checkPRStatesOnce()
	}
}

func (s *server) checkPRStatesOnce() {
	tickets := s.meta.ListTickets("")
	for _, rec := range tickets {
		if strings.TrimSpace(rec.PRURL) == "" {
			continue
		}
		open, err := s.isPullRequestOpen(rec.RepoPath, rec.PRURL)
		if err != nil {
			slog.Error("pr monitor check failed", "repo", rec.RepoPath, "ticket", rec.TicketNumber, "err", err)
			continue
		}
		if open {
			continue
		}
		if err := s.autoCleanupTicket(rec); err != nil {
			slog.Error("pr monitor auto-cleanup failed", "repo", rec.RepoPath, "ticket", rec.TicketNumber, "err", err)
			continue
		}
		slog.Info("pr monitor auto-cleaned ticket", "repo", rec.RepoPath, "ticket", rec.TicketNumber)
	}
}

func (s *server) isPullRequestOpen(repoPath, prURL string) (bool, error) {
	owner, repo, number, err := parseGitHubPRURL(prURL)
	if err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	path := fmt.Sprintf("repos/%s/%s/pulls/%d", owner, repo, number)
	res, err := shell.Run(ctx, repoPath, nil, "", "gh", "api", path, "--jq", ".state")
	if err != nil {
		return false, err
	}
	state := strings.TrimSpace(strings.ToLower(res.Stdout))
	return state == "open", nil
}

func parseGitHubPRURL(prURL string) (string, string, int, error) {
	m := githubPRURLPattern.FindStringSubmatch(strings.TrimSpace(prURL))
	if len(m) != 4 {
		return "", "", 0, fmt.Errorf("%w: %s", errUnsupportedPRURL, prURL)
	}
	n, convErr := strconv.Atoi(m[3])
	if convErr != nil {
		return "", "", 0, fmt.Errorf("parse PR number: %w", convErr)
	}
	return m[1], m[2], n, nil
}

func (s *server) autoCleanupTicket(rec servermeta.TicketRecord) error {
	repoMu := s.getRepoLock(rec.RepoID)
	repoMu.RLock()
	defer repoMu.RUnlock()
	ticketMu := s.getTicketLock(rec.RepoID, rec.TicketNumber)
	ticketMu.Lock()
	defer ticketMu.Unlock()

	rt, err := s.runtimeForRepo(rec.RepoPath)
	if err != nil {
		return err
	}
	if err := rt.svc.CleanupTicket(context.Background(), rec.TicketNumber); err != nil {
		return err
	}
	if err := s.meta.DeleteTicket(rec.RepoID, rec.TicketNumber); err != nil {
		return err
	}
	if err := s.meta.DeleteJobs(rec.RepoID, rec.TicketNumber); err != nil {
		return err
	}
	s.broadcast(serverEvent{
		Type:         "ticket_deleted",
		RepoID:       rec.RepoID,
		RepoPath:     rec.RepoPath,
		TicketNumber: rec.TicketNumber,
	})
	return nil
}
