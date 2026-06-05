package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/Neokil/AutoPR/internal/api"
	workflowstate "github.com/Neokil/AutoPR/internal/domain/workflowstate"
	"github.com/Neokil/AutoPR/internal/serverstate"
)

func (s *server) workerLoop() {
	for job := range s.jobs {
		s.setJobStatus(job.record, "running", "")
		err := s.executeJob(job)
		if err != nil {
			s.setJobStatus(job.record, "failed", err.Error())

			continue
		}
		s.setJobStatus(job.record, "done", "")
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

func (s *server) executeJob(job queuedJob) error {
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
		return err
	}

	switch job.record.Action {
	case jobRun:
		err = repoRt.svc.StartFlow(context.Background(), ticket)
		if err == nil {
			err = s.syncTicketFromRepo(repoID, repoRoot, ticket, repoRt, true)
		}
	case jobAction:
		err = repoRt.svc.ApplyAction(context.Background(), ticket, job.actionLabel, job.message)
		if err == nil {
			err = s.syncTicketFromRepo(repoID, repoRoot, ticket, repoRt, true)
		}
	case jobMoveToState:
		err = repoRt.svc.MoveToState(context.Background(), ticket, job.targetState)
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
	if err != nil && ticket != "" {
		persistErr := s.persistTicketFailure(repoID, repoRoot, ticket, repoRt, job, err)
		if persistErr != nil {
			return fmt.Errorf("%w (also failed to persist ticket failure: %w)", err, persistErr)
		}
	}

	return err
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

			if ticket.Status == string(workflowstate.FlowStatusDone) ||
				ticket.Status == string(workflowstate.FlowStatusFailed) ||
				ticket.Status == string(workflowstate.FlowStatusCancelled) ||
				ticket.Status == "" {
				continue
			}
			slog.Info("recoverStuckTickets: found stuck ticket, marking as failed", "repo", repo.Path, "ticket", ticket.TicketNumber, "status", ticket.Status)
			rt, err := s.runtimeForRepo(repo.Path)
			if err != nil {
				slog.Warn("recoverStuckTickets: no runtime", "repo", repo.Path, "err", err)
				continue
			}
			st, err := rt.store.LoadState(ticket.TicketNumber)
			if err != nil {
				slog.Warn("recoverStuckTickets: load state failed", "ticket", ticket.TicketNumber, "err", err)
				continue
			}
			st.FlowStatus = workflowstate.FlowStatusFailed
			st.LastError = "daemon restarted while ticket was running — rerun to continue"
			if saveErr := rt.store.SaveState(ticket.TicketNumber, st); saveErr != nil {
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
