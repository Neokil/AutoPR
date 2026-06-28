package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/Neokil/AutoPR/internal/api"
	"github.com/Neokil/AutoPR/internal/application/tickets"
	workflowstate "github.com/Neokil/AutoPR/internal/domain/workflowstate"
	"github.com/Neokil/AutoPR/internal/serverstate"
)

func (s *server) workerLoop() {
	for job := range s.jobs {
		s.waitIfQuotaReached()
		s.setJobStatus(job.record, serverstate.JobStatusRunning, "")
		outcome, err := s.executeJob(job)
		if outcome.QuotaReached {
			slog.Warn("LLM quota reached during job execution. Marking quota as reached and pausing further jobs.")
			s.setJobStatus(job.record, "queued", "")

			s.setQuotaReached(true)
			err := s.reQueueJob(job)
			if err != nil {
				slog.Error("quota re-queue failed", "job", job.record.ID, "err", err)
			}

			continue
		}
		if err != nil {
			s.setJobStatus(job.record, serverstate.JobStatusFailed, err.Error())

			continue
		}
		s.setJobStatus(job.record, serverstate.JobStatusDone, "")
		if job.record.Action == jobCleanup && strings.TrimSpace(job.record.TicketNumber) != "" {
			_ = s.meta.DeleteJobs(job.record.RepoID, job.record.TicketNumber)
		}
	}
}

func (s *server) setJobStatus(job serverstate.JobRecord, status, errMsg string) {
	_ = s.meta.UpdateJobStatus(job.ID, status, errMsg)
	s.broadcast(api.ServerEvent{
		Type:         eventTypeJob,
		RepoId:       stringPtr(job.RepoID),
		RepoPath:     stringPtr(job.RepoPath),
		TicketNumber: stringPtr(job.TicketNumber),
		JobId:        stringPtr(job.ID),
		Action:       stringPtr(job.Action),
		Scope:        stringPtr(job.Scope),
		Status:       stringPtr(status),
		Error:        stringPtr(strings.TrimSpace(errMsg)),
	})
}

func (s *server) executeJob(job queuedJob) (tickets.RunOutcome, error) {
	repoRoot, repoID := job.record.RepoPath, job.record.RepoID
	ticket := job.record.TicketNumber

	repoMu := s.getRepoLock(repoID)
	switch job.record.Action {
	case jobCleanupDone, jobCleanupAll:
		repoMu.Lock()
		defer repoMu.Unlock()
	default:
		repoMu.RLock()
		defer repoMu.RUnlock()
		if ticket != "" {
			ticketMu := s.getTicketLock(repoID, ticket)
			ticketMu.Lock()
			defer ticketMu.Unlock()
		}
	}

	repoRt, err := s.runtimeForRepo(repoRoot)
	if err != nil {
		return tickets.RunOutcome{}, err
	}

	var outcome tickets.RunOutcome
	switch job.record.Action {
	case jobRun:
		outcome, err = repoRt.svc.StartFlow(context.Background(), ticket)
		if err == nil {
			err = s.syncTicketFromRepo(repoID, repoRoot, ticket, repoRt, true)
		}
	case jobAction:
		outcome, err = repoRt.svc.ApplyAction(context.Background(), ticket, job.actionLabel, job.message)
		if err == nil {
			err = s.syncTicketFromRepo(repoID, repoRoot, ticket, repoRt, true)
		}
	case jobMoveToState:
		outcome, err = repoRt.svc.MoveToState(context.Background(), ticket, job.targetState)
		if err == nil {
			err = s.syncTicketFromRepo(repoID, repoRoot, ticket, repoRt, true)
		}
	case jobCleanup:
		err = repoRt.svc.CleanupTicket(context.Background(), ticket)
		if err == nil {
			err = s.meta.DeleteTicket(repoID, ticket)
			if err == nil {
				s.broadcast(api.ServerEvent{
					Type:         eventTypeTicketDeleted,
					RepoId:       stringPtr(repoID),
					RepoPath:     stringPtr(repoRoot),
					TicketNumber: stringPtr(ticket),
				})
			}
		}
	case jobCleanupDone:
		err = repoRt.svc.CleanupDone(context.Background())
		if err == nil {
			err = s.syncRepoTickets(repoID, repoRoot, repoRt, true)
		}
	case jobCleanupAll:
		err = repoRt.svc.CleanupAll(context.Background())
		if err == nil {
			err = s.syncRepoTickets(repoID, repoRoot, repoRt, true)
		}
	default:
		err = fmt.Errorf("%w: %s", errUnsupportedJobAction, job.record.Action)
	}
	if err != nil && ticket != "" && !outcome.QuotaReached {
		persistErr := s.persistTicketFailure(repoID, repoRoot, ticket, repoRt, job, err)
		if persistErr != nil {
			return outcome, fmt.Errorf("%w (also failed to persist ticket failure: %w)", err, persistErr)
		}
	}

	return outcome, err
}

func (s *server) getRepoLock(repoID string) *sync.RWMutex {
	s.repoLockMu.Lock()
	defer s.repoLockMu.Unlock()
	if m, ok := s.repoLocks[repoID]; ok {
		return m
	}
	m := &sync.RWMutex{}
	s.repoLocks[repoID] = m

	return m
}

func (s *server) getTicketLock(repoID, ticket string) *sync.Mutex {
	key := repoID + "::" + ticket
	s.ticketLockMu.Lock()
	defer s.ticketLockMu.Unlock()
	if m, ok := s.ticketLocks[key]; ok {
		return m
	}
	m := &sync.Mutex{}
	s.ticketLocks[key] = m

	return m
}

func (s *server) recoverStuckTickets() {
	repos := s.meta.ListRepos()
	for _, repo := range repos {
		tickets := s.meta.ListTickets(repo.ID)
		for _, ticket := range tickets {
			if ticket.Status != string(workflowstate.FlowStatusRunning) {
				continue
			}

			slog.Info("recoverStuckTickets: found stuck ticket, marking as failed", "repo", repo.Path, "ticket", ticket.TicketNumber, "status", ticket.Status)
			runtime, err := s.runtimeForRepo(repo.Path)
			if err != nil {
				slog.Warn("recoverStuckTickets: no runtime", "repo", repo.Path, "err", err)

				continue
			}

			state, err := runtime.store.LoadState(ticket.TicketNumber)
			if err != nil {
				slog.Warn("recoverStuckTickets: load state failed", "ticket", ticket.TicketNumber, "err", err)

				continue
			}

			state.FlowStatus = workflowstate.FlowStatusFailed
			state.LastError = "daemon restarted while ticket was running — rerun to continue"
			saveErr := runtime.store.SaveState(ticket.TicketNumber, state)
			if saveErr != nil {
				slog.Warn("recoverStuckTickets: save failed", "ticket", ticket.TicketNumber, "err", saveErr)
			}

			for _, job := range ticket.Jobs {
				if job.Status == "running" || job.Status == "queued" {
					_ = s.meta.UpdateJobStatus(job.ID, "failed", "daemon restarted")
				}
			}
		}
	}
}

func (s *server) waitIfQuotaReached() {
	s.quotaMu.RLock()
	quotaReached := s.quotaReached
	resetCh := s.quotaResetCh
	s.quotaMu.RUnlock()

	if !quotaReached {
		return
	}

	<-resetCh
	slog.Info("LLM quota reset detected. Resuming job execution.")
}

func (s *server) reQueueJob(job queuedJob) error {
	select {
	case s.jobs <- job:
		return nil
	// here we could also listen for a shutdown signal if we had one, to avoid trying to re-queue when the server is shutting down. For now, we'll just return an error if the job queue is full.
	// case <-context.Background().Done():
	// 	return fmt.Errorf("re-queue aborted: server shutting down")
	default:
		return errJobQueueFull
	}
}
