package ticket

import (
	"path/filepath"
	"time"
)

type FlowStatus string

const (
	FlowStatusPending   FlowStatus = "pending" // ticket exists, flow not yet started
	FlowStatusRunning   FlowStatus = "running" // prompt or commands currently executing
	FlowStatusWaiting   FlowStatus = "waiting" // waiting for human action
	FlowStatusDone      FlowStatus = "done"
	FlowStatusFailed    FlowStatus = "failed"
	FlowStatusCancelled FlowStatus = "cancelled"
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
	ParentTicket       *TicketContext    `json:"parent_ticket,omitempty"`
	Epic               *TicketContext    `json:"epic,omitempty"`
}

type TicketContext struct {
	ID          string `json:"id"`
	Number      string `json:"number,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

type StateRun struct {
	ID               string    `json:"id"`
	StateName        string    `json:"state_name"`
	StateDisplayName string    `json:"state_display_name,omitempty"`
	StartedAt        time.Time `json:"started_at"`
	ArtifactRef      string    `json:"artifact_ref,omitempty"`
	LogRef           string    `json:"log_ref,omitempty"`
}

type State struct {
	TicketNumber string     `json:"ticket_number"`
	CurrentState string     `json:"current_state"` // matches a StateConfig.Name from workflow
	CurrentRunID string     `json:"current_run_id,omitempty"`
	FlowStatus   FlowStatus `json:"flow_status"`
	BranchName   string     `json:"branch_name"`
	WorktreePath string     `json:"worktree_path"`
	LastError    string     `json:"last_error,omitempty"`
	PRURL        string     `json:"pr_url,omitempty"`
	StateHistory []StateRun `json:"state_history,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func NewState(ticketNumber string) State {
	now := time.Now().UTC()
	return State{
		TicketNumber: ticketNumber,
		FlowStatus:   FlowStatusPending,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func (s *State) Touch() {
	s.UpdatedAt = time.Now().UTC()
}

// ArtifactPath returns the path to a named file within the worktree's .auto-pr directory.
func (s State) ArtifactPath(name string) string {
	return filepath.Join(s.WorktreePath, ".auto-pr", name)
}

func (s State) RunPath(runID string, parts ...string) string {
	pathParts := make([]string, 0, 4+len(parts)) //nolint:mnd // 4 = worktreePath + .auto-pr + runs + runID
	pathParts = append(pathParts, s.WorktreePath, ".auto-pr", "runs", runID)
	pathParts = append(pathParts, parts...)
	return filepath.Join(pathParts...)
}

func (s State) ResolveRef(ref string) string {
	return filepath.Join(s.WorktreePath, ".auto-pr", ref)
}

func (s State) CurrentRunLogPath() string {
	for _, run := range s.StateHistory {
		if run.ID == s.CurrentRunID && run.LogRef != "" {
			return s.ResolveRef(run.LogRef)
		}
	}
	return ""
}

func (s State) LatestArtifactRef(stateName string) string {
	for i := len(s.StateHistory) - 1; i >= 0; i-- {
		run := s.StateHistory[i]
		if run.StateName == stateName && run.ArtifactRef != "" {
			return run.ArtifactRef
		}
	}
	return ""
}
