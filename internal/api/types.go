// Package api defines the HTTP request and response types for the AutoPR REST API.
package api

import "time"

// RepoRequest is the minimal body sent to endpoints that only need a repo path.
type RepoRequest struct {
	RepoPath string `json:"repo_path"`
}

// FeedbackRequest is the body sent when providing human feedback to a waiting ticket.
type FeedbackRequest struct {
	RepoPath string `json:"repo_path"`
	Message  string `json:"message"`
}

// CleanupScopeRequest is the body sent to the cleanup endpoint; Scope is "done" or "all".
type CleanupScopeRequest struct {
	RepoPath string `json:"repo_path"`
	Scope    string `json:"scope"`
}

// ActionRequest is the body sent when applying a named action to a waiting ticket.
type ActionRequest struct {
	RepoPath string `json:"repo_path"`
	Label    string `json:"label"`
	Message  string `json:"message,omitempty"`
}

// MoveToStateRequest is the body sent when force-transitioning a ticket to a named state.
type MoveToStateRequest struct {
	RepoPath string `json:"repo_path"`
	Target   string `json:"target"`
}

// ActionAcceptedResponse is returned when an action has been enqueued as a background job.
type ActionAcceptedResponse struct {
	Status       string `json:"status"`
	JobID        string `json:"job_id"`
	Action       string `json:"action"`
	RepoID       string `json:"repo_id"`
	RepoPath     string `json:"repo_path"`
	TicketNumber string `json:"ticket_number,omitempty"`
}

// ErrorResponse is the standard JSON body returned on non-2xx responses.
type ErrorResponse struct {
	Error string `json:"error"`
}

// JobStatusResponse describes the current state of a background job.
type JobStatusResponse struct {
	ID           string     `json:"id"`
	Action       string     `json:"action"`
	RepoID       string     `json:"repo_id"`
	RepoPath     string     `json:"repo_path"`
	TicketNumber string     `json:"ticket_number,omitempty"`
	Status       string     `json:"status"`
	Scope        string     `json:"scope,omitempty"`
	Error        string     `json:"error,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}

// ActionInfo describes a user-selectable action on a waiting ticket.
type ActionInfo struct {
	Label string `json:"label"`
	Type  string `json:"type"`
}

// WorkflowStateInfo describes a workflow state for timeline rendering.
type WorkflowStateInfo struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
}

// StateRunResponse describes a single execution of a workflow state.
type StateRunResponse struct {
	ID               string    `json:"id"`
	StateName        string    `json:"state_name"`
	StateDisplayName string    `json:"state_display_name,omitempty"`
	StartedAt        time.Time `json:"started_at"`
	ArtifactRef      string    `json:"artifact_ref,omitempty"`
	LogRef           string    `json:"log_ref,omitempty"`
}

// TicketStateResponse is the HTTP representation of persisted workflow state.
type TicketStateResponse struct {
	TicketNumber string             `json:"ticket_number"`
	CurrentState string             `json:"current_state"`
	CurrentRunID string             `json:"current_run_id,omitempty"`
	FlowStatus   string             `json:"flow_status"`
	BranchName   string             `json:"branch_name"`
	WorktreePath string             `json:"worktree_path"`
	LastError    string             `json:"last_error,omitempty"`
	PRURL        string             `json:"pr_url,omitempty"`
	StateHistory []StateRunResponse `json:"state_history,omitempty"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
}

// TicketSummaryResponse is the list-item response for tickets.
type TicketSummaryResponse struct {
	RepoID       string              `json:"repo_id"`
	RepoPath     string              `json:"repo_path"`
	TicketNumber string              `json:"ticket_number"`
	Title        string              `json:"title,omitempty"`
	Status       string              `json:"status"`
	Busy         bool                `json:"busy"`
	Approved     bool                `json:"approved"`
	LastError    string              `json:"last_error,omitempty"`
	UpdatedAt    time.Time           `json:"updated_at"`
	PRURL        string              `json:"pr_url,omitempty"`
	Jobs         []JobStatusResponse `json:"jobs,omitempty"`
}

// TicketDetailsResponse is the full ticket details payload.
type TicketDetailsResponse struct {
	RepoID           string              `json:"repo_id"`
	RepoPath         string              `json:"repo_path"`
	TicketNumber     string              `json:"ticket_number"`
	GitHubBlobBase   string              `json:"github_blob_base,omitempty"`
	State            TicketStateResponse `json:"state"`
	Ticket           any                 `json:"ticket,omitempty"`
	NextSteps        string              `json:"next_steps,omitempty"`
	WorkflowStates   []WorkflowStateInfo `json:"workflow_states,omitempty"`
	AvailableActions []ActionInfo        `json:"available_actions"`
}

// ExecutionLogResponse returns the content of a run-scoped execution log.
type ExecutionLogResponse struct {
	RunID            string `json:"run_id"`
	State            string `json:"state"`
	StateDisplayName string `json:"state_display_name,omitempty"`
	Timestamp        string `json:"timestamp"`
	Path             string `json:"path"`
	Content          string `json:"content"`
}

// LogEventResponse is a parsed state-log section.
type LogEventResponse struct {
	Title     string `json:"title"`
	Timestamp string `json:"timestamp"`
	Body      string `json:"body"`
}

// ServerEvent describes an SSE event emitted by the daemon.
type ServerEvent struct {
	Type         string `json:"type"`
	RepoID       string `json:"repo_id,omitempty"`
	RepoPath     string `json:"repo_path,omitempty"`
	TicketNumber string `json:"ticket_number,omitempty"`
	Title        string `json:"title,omitempty"`
	Status       string `json:"status,omitempty"`
	JobID        string `json:"job_id,omitempty"`
	Action       string `json:"action,omitempty"`
	Scope        string `json:"scope,omitempty"`
	Error        string `json:"error,omitempty"`
	PRURL        string `json:"pr_url,omitempty"`
}
