package ports

import "ai-ticket-worker/internal/domain/ticket"

// StateStore abstracts ticket workflow persistence so storage can be swapped.
type StateStore interface {
	EnsureTicketDir(ticketNumber string) (string, error)
	LoadState(ticketNumber string) (ticket.State, error)
	SaveState(ticketNumber string, st ticket.State) error
	SaveTicket(ticketNumber string, t ticket.Ticket) (string, error)
	LoadTicket(ticketNumber string) (ticket.Ticket, error)
	Paths(ticketNumber string) map[string]string
	ListTicketDirs() ([]string, error)
	RemoveTicketDir(ticketNumber string) error
}
