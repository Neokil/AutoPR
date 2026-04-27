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
