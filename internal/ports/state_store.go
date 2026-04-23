package ports

import "ai-ticket-worker/internal/domain/ticket"

// StateStore abstracts ticket workflow persistence so storage can be swapped.
type StateStore interface {
	LoadState(ticketNumber string) (ticket.State, error)
	SaveState(ticketNumber string, st ticket.State) error
	ListTicketDirs() ([]string, error)
	RemoveTicketDir(ticketNumber string) error
}
