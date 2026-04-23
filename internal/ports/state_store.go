package ports

import "ai-ticket-worker/internal/domain/ticket"

type TicketPaths struct {
	Dir         string
	State       string
	Log         string
	Proposal    string
	Final       string
	PR          string
	Checks      string
	ProviderDir string
}

// StateStore abstracts ticket workflow persistence so storage can be swapped.
type StateStore interface {
	EnsureTicketDir(ticketNumber string) (string, error)
	LoadState(ticketNumber string) (ticket.State, error)
	SaveState(ticketNumber string, st ticket.State) error
	Paths(ticketNumber string) TicketPaths
	ListTicketDirs() ([]string, error)
	RemoveTicketDir(ticketNumber string) error
}
