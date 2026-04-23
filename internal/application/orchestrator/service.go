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
	MoveToState(ctx context.Context, ticketNumber, target string) error
	Status(ticketNumber string) error
	NextSteps(ticketNumber string) (string, error)
	CleanupDone(ctx context.Context) error
	CleanupAll(ctx context.Context) error
	CleanupTicket(ctx context.Context, ticketNumber string) error
}

func NewWorkflowService(cfg config.Config, repoRoot string, provider providers.AIProvider) Service {
	return tickets.New(cfg, repoRoot, provider)
}
