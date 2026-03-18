package orchestrator

import (
	"context"

	"ai-ticket-worker/internal/config"
	"ai-ticket-worker/internal/providers"
	"ai-ticket-worker/internal/workflow"
)

// Service defines application-level orchestrator use-cases shared by clients.
type Service interface {
	RunTickets(ctx context.Context, ticketNumbers []string) error
	Status(ticketNumber string) error
	Approve(ctx context.Context, ticketNumber string) error
	Feedback(ticketNumber, message string) error
	Reject(ticketNumber string) error
	ResumeTicket(ctx context.Context, ticketNumber string) error
	GeneratePR(ctx context.Context, ticketNumber string) error
	ApplyPRComments(ctx context.Context, ticketNumber string) error
	CleanupDone(ctx context.Context) error
	CleanupAll(ctx context.Context) error
	CleanupTicket(ctx context.Context, ticketNumber string) error
	NextSteps(ticketNumber string) (string, error)
}

type WorkflowService struct {
	orch *workflow.Orchestrator
}

func NewWorkflowService(cfg config.Config, repoRoot string, provider providers.AIProvider) *WorkflowService {
	return &WorkflowService{orch: workflow.New(cfg, repoRoot, provider)}
}

func (s *WorkflowService) RunTickets(ctx context.Context, ticketNumbers []string) error {
	return s.orch.RunTickets(ctx, ticketNumbers)
}

func (s *WorkflowService) Status(ticketNumber string) error {
	return s.orch.Status(ticketNumber)
}

func (s *WorkflowService) Approve(ctx context.Context, ticketNumber string) error {
	return s.orch.Approve(ctx, ticketNumber)
}

func (s *WorkflowService) Feedback(ticketNumber, message string) error {
	return s.orch.Feedback(ticketNumber, message)
}

func (s *WorkflowService) Reject(ticketNumber string) error {
	return s.orch.Reject(ticketNumber)
}

func (s *WorkflowService) ResumeTicket(ctx context.Context, ticketNumber string) error {
	return s.orch.ResumeTicket(ctx, ticketNumber)
}

func (s *WorkflowService) GeneratePR(ctx context.Context, ticketNumber string) error {
	return s.orch.GeneratePR(ctx, ticketNumber)
}

func (s *WorkflowService) ApplyPRComments(ctx context.Context, ticketNumber string) error {
	return s.orch.ApplyPRComments(ctx, ticketNumber)
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

func (s *WorkflowService) NextSteps(ticketNumber string) (string, error) {
	return s.orch.NextSteps(ticketNumber)
}
