package api

import "time"

type RepoRequest struct {
	RepoPath string `json:"repo_path"`
}

type FeedbackRequest struct {
	RepoPath string `json:"repo_path"`
	Message  string `json:"message"`
}

type CleanupScopeRequest struct {
	RepoPath string `json:"repo_path"`
	Scope    string `json:"scope"`
}

type ActionRequest struct {
	RepoPath string `json:"repo_path"`
	Label    string `json:"label"`
	Message  string `json:"message,omitempty"`
}

type ActionAcceptedResponse struct {
	Status       string `json:"status"`
	JobID        string `json:"job_id"`
	Action       string `json:"action"`
	RepoID       string `json:"repo_id"`
	RepoPath     string `json:"repo_path"`
	TicketNumber string `json:"ticket_number,omitempty"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

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
