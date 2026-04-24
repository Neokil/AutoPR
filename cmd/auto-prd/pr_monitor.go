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

const (
	prMonitorInterval    = 2 * time.Minute
	prMonitorInitialWait = 3 * time.Second
	githubAPITimeout     = 20 * time.Second
	prURLMatchLen        = 4 // full match + 3 capture groups (owner, repo, number)
)

func (s *server) prMonitorLoop() {
	ticker := time.NewTicker(prMonitorInterval)
	defer ticker.Stop()
	time.Sleep(prMonitorInitialWait)
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
		autoCleanupErr := s.autoCleanupTicket(rec)
		if autoCleanupErr != nil {
			slog.Error("pr monitor auto-cleanup failed", "repo", rec.RepoPath, "ticket", rec.TicketNumber, "err", autoCleanupErr)

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
	ctx, cancel := context.WithTimeout(context.Background(), githubAPITimeout)
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
	if len(m) != prURLMatchLen {
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
	err = rt.svc.CleanupTicket(context.Background(), rec.TicketNumber)
	if err != nil {
		return err
	}
	err = s.meta.DeleteTicket(rec.RepoID, rec.TicketNumber)
	if err != nil {
		return err
	}
	err = s.meta.DeleteJobs(rec.RepoID, rec.TicketNumber)
	if err != nil {
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
