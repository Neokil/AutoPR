package api

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
