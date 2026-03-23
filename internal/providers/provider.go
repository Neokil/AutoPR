package providers

import (
	"context"

	"ai-ticket-worker/internal/domain/ticket"
)

type InvestigateRequest struct {
	Ticket         ticket.Ticket
	RepoPath       string
	WorktreePath   string
	GuidelinesPath string
	LogPath        string
	ProposalPath   string
	Feedback       string
}

type InvestigateResult struct {
	Proposal string
	RawOut   string
}

type ImplementRequest struct {
	Ticket            ticket.Ticket
	RepoPath          string
	WorktreePath      string
	GuidelinesPath    string
	LogPath           string
	ProposalPath      string
	FinalSolutionPath string
	FailureContext    string
}

type ImplementResult struct {
	Summary string
	RawOut  string
}

type PRRequest struct {
	Ticket            ticket.Ticket
	WorktreePath      string
	LogPath           string
	ProposalPath      string
	FinalSolutionPath string
	ChecksLogPath     string
}

type PRResult struct {
	Body   string
	RawOut string
}

type AIProvider interface {
	Name() string
	GetTicket(ctx context.Context, ticketNumber, repoPath, runtimeDir string) (ticket.Ticket, string, error)
	Investigate(ctx context.Context, req InvestigateRequest, runtimeDir string) (InvestigateResult, error)
	Implement(ctx context.Context, req ImplementRequest, runtimeDir string) (ImplementResult, error)
	SummarizePR(ctx context.Context, req PRRequest, runtimeDir string) (PRResult, error)
}
