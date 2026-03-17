package models

import "ai-ticket-worker/internal/domain/ticket"

// Backward-compatible aliases. New code should prefer internal/domain/ticket.
type WorkflowState = ticket.WorkflowState

const (
	StateQueued          = ticket.StateQueued
	StateInvestigating   = ticket.StateInvestigating
	StateProposalReady   = ticket.StateProposalReady
	StateWaitingForHuman = ticket.StateWaitingForHuman
	StateImplementing    = ticket.StateImplementing
	StateValidating      = ticket.StateValidating
	StatePRReady         = ticket.StatePRReady
	StateDone            = ticket.StateDone
	StateFailed          = ticket.StateFailed
)

type Ticket = ticket.Ticket
type TicketContext = ticket.TicketContext
type TicketState = ticket.State

func NewTicketState(ticketNumber string) TicketState {
	return ticket.NewState(ticketNumber)
}
