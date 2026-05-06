// Package workflowstate defines the persisted execution state for AutoPR workflows.
package workflowstate

import (
	"path/filepath"
	"slices"
	"time"
)

// FlowStatus represents the lifecycle stage of a ticket's workflow execution.
type FlowStatus string

// Flow lifecycle stages from creation through completion or failure.
const (
	FlowStatusPending   FlowStatus = "pending"
	FlowStatusRunning   FlowStatus = "running"
	FlowStatusWaiting   FlowStatus = "waiting"
	FlowStatusDone      FlowStatus = "done"
	FlowStatusFailed    FlowStatus = "failed"
	FlowStatusCancelled FlowStatus = "cancelled"
)

// StateRun records a single execution of a workflow state for a ticket.
type StateRun struct {
	ID               string    `json:"id"`
	StateName        string    `json:"state_name"`
	StateDisplayName string    `json:"state_display_name,omitempty"`
	StartedAt        time.Time `json:"started_at"`
	ArtifactRef      string    `json:"artifact_ref,omitempty"`
	LogRef           string    `json:"log_ref,omitempty"`
}

// State is the persisted workflow state for a single ticket, written to state.json.
type State struct {
	TicketNumber        string     `json:"ticket_number"`
	CurrentState        string     `json:"current_state"`
	CurrentRunID        string     `json:"current_run_id,omitempty"`
	FlowStatus          FlowStatus `json:"flow_status"`
	BranchName          string     `json:"branch_name"`
	WorktreePath        string     `json:"worktree_path"`
	LastError           string     `json:"last_error,omitempty"`
	PRURL               string     `json:"pr_url,omitempty"`
	ProviderSessionData string     `json:"provider_session_data,omitempty"`
	StateHistory        []StateRun `json:"state_history,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// New returns a new pending State for the given ticket number.
func New(ticketNumber string) State {
	now := time.Now().UTC()

	return State{
		TicketNumber: ticketNumber,
		FlowStatus:   FlowStatusPending,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// Touch updates UpdatedAt to the current UTC time.
func (s *State) Touch() {
	s.UpdatedAt = time.Now().UTC()
}

// ArtifactPath returns the path to a named file within the worktree's .auto-pr directory.
func (s *State) ArtifactPath(name string) string {
	return filepath.Join(s.WorktreePath, ".auto-pr", name)
}

// RunPath returns the filesystem path to a file or directory within a state run's directory.
func (s *State) RunPath(runID string, parts ...string) string {
	pathParts := make([]string, 0, 4+len(parts)) //nolint:mnd // 4 fixed path components: WorktreePath + ".auto-pr" + "runs" + runID
	pathParts = append(pathParts, s.WorktreePath, ".auto-pr", "runs", runID)
	pathParts = append(pathParts, parts...)

	return filepath.Join(pathParts...)
}

// ResolveRef converts a relative artifact ref to an absolute path.
func (s *State) ResolveRef(ref string) string {
	return filepath.Join(s.WorktreePath, ".auto-pr", ref)
}

// CurrentRunLogPath returns the absolute path to the log file for the current state run.
func (s *State) CurrentRunLogPath() string {
	for _, run := range s.StateHistory {
		if run.ID == s.CurrentRunID && run.LogRef != "" {
			return s.ResolveRef(run.LogRef)
		}
	}

	return ""
}

// LatestArtifactRef returns the artifact ref from the most recent run of the named state.
func (s *State) LatestArtifactRef(stateName string) string {
	for _, run := range slices.Backward(s.StateHistory) {
		if run.StateName == stateName && run.ArtifactRef != "" {
			return run.ArtifactRef
		}
	}

	return ""
}
