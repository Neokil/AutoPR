package main

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"ai-ticket-worker/internal/servermeta"
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

func (s *server) setJobStatus(job servermeta.JobRecord, status, errMsg string) {
	_ = s.meta.UpdateJobStatus(job.ID, status, errMsg)
	s.broadcast(serverEvent{
		Type:         "job",
		RepoID:       job.RepoID,
		RepoPath:     job.RepoPath,
		TicketNumber: job.TicketNumber,
		JobID:        job.ID,
		Action:       job.Action,
		Scope:        job.Scope,
		Status:       status,
		Error:        strings.TrimSpace(errMsg),
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

	rt, err := s.runtimeForRepo(repoRoot)
	if err != nil {
		return err
	}

	switch job.record.Action {
	case jobRun:
		err = rt.svc.StartFlow(context.Background(), ticket)
		if err == nil {
			err = s.syncTicketFromRepo(repoID, repoRoot, ticket, rt, true)
		}
	case jobAction:
		err = rt.svc.ApplyAction(context.Background(), ticket, job.actionLabel, job.message)
		if err == nil {
			err = s.syncTicketFromRepo(repoID, repoRoot, ticket, rt, true)
		}
	case jobMoveToState:
		err = rt.svc.MoveToState(context.Background(), ticket, job.targetState)
		if err == nil {
			err = s.syncTicketFromRepo(repoID, repoRoot, ticket, rt, true)
		}
	case jobCleanup:
		err = rt.svc.CleanupTicket(context.Background(), ticket)
		if err == nil {
			err = s.meta.DeleteTicket(repoID, ticket)
			if err == nil {
				s.broadcast(serverEvent{
					Type:         "ticket_deleted",
					RepoID:       repoID,
					RepoPath:     repoRoot,
					TicketNumber: ticket,
				})
			}
		}
	case jobCleanupDone:
		err = rt.svc.CleanupDone(context.Background())
		if err == nil {
			err = s.syncRepoTickets(repoID, repoRoot, rt, true)
		}
	case jobCleanupAll:
		err = rt.svc.CleanupAll(context.Background())
		if err == nil {
			err = s.syncRepoTickets(repoID, repoRoot, rt, true)
		}
	default:
		err = fmt.Errorf("unsupported job action: %s", job.record.Action)
	}
	if err != nil && ticket != "" {
		if persistErr := s.persistTicketFailure(repoID, repoRoot, ticket, rt, job, err); persistErr != nil {
			return fmt.Errorf("%w (also failed to persist ticket failure: %v)", err, persistErr)
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
