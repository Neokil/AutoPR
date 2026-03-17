package ticketsource

import (
	"context"

	"ai-ticket-worker/internal/models"
)

type TicketSource interface {
	GetTicket(ctx context.Context, ticketNumber string) (models.Ticket, error)
}
