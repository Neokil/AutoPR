package ticket

import (
	"fmt"
	"strings"
)

func (s State) ShouldGeneratePROnRun() bool {
	return s.Status == StatePRReady
}

func (s State) WaitsForHumanInput() bool {
	return s.Status == StateWaitingForHuman && !s.Approved
}

func (s State) ShouldInvestigate() bool {
	return s.Status == StateQueued ||
		s.Status == StateInvestigating ||
		s.Status == StateProposalReady
}

func (s State) ShouldImplement() bool {
	return s.Approved ||
		s.Status == StateImplementing ||
		s.Status == StateValidating ||
		s.Status == StatePRReady
}

func (s *State) ApproveForImplementation() {
	s.Approved = true
	s.Status = StateImplementing
}

func (s *State) ApplyFeedback(message string) {
	s.LastFeedback = message
	s.Approved = false
	s.Status = StateInvestigating
}

func (s *State) RejectByHuman() {
	s.Status = StateFailed
	s.Approved = false
	s.LastError = "rejected by human"
}

func (s State) NextStepsCLI() string {
	switch s.Status {
	case StateQueued:
		return ""
	case StateInvestigating, StateProposalReady, StateWaitingForHuman:
		return fmt.Sprintf("Next steps for ticket %s:\n  1. Review proposal: %s\n  2. Approve: auto-pr approve %s\n  3. Provide feedback: auto-pr feedback %s --message \"...\"\n  4. Reject: auto-pr reject %s", s.TicketNumber, s.ProposalPath, s.TicketNumber, s.TicketNumber, s.TicketNumber)
	case StateImplementing, StateValidating:
		return fmt.Sprintf("Next steps for ticket %s:\n  1. Continue workflow: auto-pr resume %s\n  2. Check progress: auto-pr status %s", s.TicketNumber, s.TicketNumber, s.TicketNumber)
	case StatePRReady:
		if strings.TrimSpace(s.PRURL) != "" {
			return fmt.Sprintf("Next steps for ticket %s:\n  1. Review PR markdown: %s\n  2. Review GitHub PR: %s\n  3. Apply open review comments: auto-pr apply-pr-comments %s", s.TicketNumber, s.PRPath, s.PRURL, s.TicketNumber)
		}
		return fmt.Sprintf("Next steps for ticket %s:\n  1. Generate/create PR: auto-pr pr %s\n  2. Review PR markdown: %s", s.TicketNumber, s.TicketNumber, s.PRPath)
	case StateDone:
		if strings.TrimSpace(s.PRURL) != "" {
			return fmt.Sprintf("Next steps for ticket %s:\n  1. Review final PR markdown: %s\n  2. Review GitHub PR: %s\n  3. Apply open review comments: auto-pr apply-pr-comments %s", s.TicketNumber, s.PRPath, s.PRURL, s.TicketNumber)
		}
		return fmt.Sprintf("Next steps for ticket %s:\n  1. Review final PR markdown: %s\n  2. Check current state: auto-pr status %s", s.TicketNumber, s.PRPath, s.TicketNumber)
	case StateFailed:
		return fmt.Sprintf("Next steps for ticket %s:\n  1. Inspect log: %s\n  2. Add feedback: auto-pr feedback %s --message \"...\"\n  3. Retry: auto-pr resume %s", s.TicketNumber, s.LogPath, s.TicketNumber, s.TicketNumber)
	default:
		return fmt.Sprintf("Next steps for ticket %s:\n  1. Check status: auto-pr status %s\n  2. Continue: auto-pr resume %s", s.TicketNumber, s.TicketNumber, s.TicketNumber)
	}
}
