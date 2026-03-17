package models

import "time"

type WorkflowState string

const (
	StateQueued          WorkflowState = "queued"
	StateInvestigating   WorkflowState = "investigating"
	StateProposalReady   WorkflowState = "proposal_ready"
	StateWaitingForHuman WorkflowState = "waiting_for_human"
	StateImplementing    WorkflowState = "implementing"
	StateValidating      WorkflowState = "validating"
	StatePRReady         WorkflowState = "pr_ready"
	StateDone            WorkflowState = "done"
	StateFailed          WorkflowState = "failed"
)

type Ticket struct {
	Number             string            `json:"number"`
	ID                 string            `json:"id"`
	Title              string            `json:"title"`
	Description        string            `json:"description"`
	AcceptanceCriteria string            `json:"acceptance_criteria"`
	Priority           string            `json:"priority"`
	URL                string            `json:"url"`
	Labels             []string          `json:"labels,omitempty"`
	WorkflowFields     map[string]string `json:"workflow_fields,omitempty"`
}

type TicketState struct {
	TicketNumber    string        `json:"ticket_number"`
	BranchName      string        `json:"branch_name"`
	WorktreePath    string        `json:"worktree_path"`
	Status          WorkflowState `json:"status"`
	Approved        bool          `json:"approved"`
	FixAttempts     int           `json:"fix_attempts"`
	LastError       string        `json:"last_error,omitempty"`
	LastFeedback    string        `json:"last_feedback,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
	ProposalPath    string        `json:"proposal_path"`
	FinalPath       string        `json:"final_solution_path"`
	LogPath         string        `json:"log_path"`
	PRPath          string        `json:"pr_path"`
	ChecksLogPath   string        `json:"checks_log_path"`
	TicketJSONPath  string        `json:"ticket_json_path"`
	ProviderDirPath string        `json:"provider_dir_path"`
}

func NewTicketState(ticketNumber string) TicketState {
	now := time.Now().UTC()
	return TicketState{
		TicketNumber: ticketNumber,
		Status:       StateQueued,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func (s *TicketState) Touch() {
	s.UpdatedAt = time.Now().UTC()
}
