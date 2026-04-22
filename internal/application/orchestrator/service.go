package orchestrator

import (
	"context"

	"ai-ticket-worker/internal/application/tickets"
	"ai-ticket-worker/internal/config"
	"ai-ticket-worker/internal/providers"
)

// Service defines application-level orchestrator use-cases shared by clients.
type Service interface {
	StartFlow(ctx context.Context, ticketNumber string) error
	ApplyAction(ctx context.Context, ticketNumber, actionLabel, message string) error
	Status(ticketNumber string) error
	NextSteps(ticketNumber string) (string, error)
	CleanupDone(ctx context.Context) error
	CleanupAll(ctx context.Context) error
	CleanupTicket(ctx context.Context, ticketNumber string) error
}

type WorkflowService struct {
	orch *tickets.Orchestrator
}

func NewWorkflowService(cfg config.Config, repoRoot string, provider providers.AIProvider) *WorkflowService {
	return &WorkflowService{orch: tickets.New(cfg, repoRoot, provider)}
}

func (s *WorkflowService) StartFlow(ctx context.Context, ticketNumber string) error {
	return s.orch.StartFlow(ctx, ticketNumber)
}

func (s *WorkflowService) ApplyAction(ctx context.Context, ticketNumber, actionLabel, message string) error {
	return s.orch.ApplyAction(ctx, ticketNumber, actionLabel, message)
}

func (s *WorkflowService) Status(ticketNumber string) error {
	return s.orch.Status(ticketNumber)
}

func (s *WorkflowService) NextSteps(ticketNumber string) (string, error) {
	return s.orch.NextSteps(ticketNumber)
}

func (s *WorkflowService) CleanupDone(ctx context.Context) error {
	return s.orch.CleanupDone(ctx)
}

func (s *WorkflowService) CleanupAll(ctx context.Context) error {
	return s.orch.CleanupAll(ctx)
}

func (s *WorkflowService) CleanupTicket(ctx context.Context, ticketNumber string) error {
	return s.orch.CleanupTicket(ctx, ticketNumber)
}
