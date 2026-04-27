package servermeta

// RepoStore manages the set of known repositories.
type RepoStore interface {
	UpsertRepo(repoPath string) (RepoRecord, error)
	ListRepos() []RepoRecord
}

// TicketStore manages ticket metadata records.
type TicketStore interface {
	UpsertTicket(rec TicketRecord) error
	DeleteTicket(repoID, ticketNumber string) error
	ReplaceRepoTickets(repoID string, tickets []TicketRecord) error
	ListTickets(repoID string) []TicketRecord
}

// JobStore manages background job records.
type JobStore interface {
	NewJob(action, repoID, repoPath, ticketNumber, scope string) (JobRecord, error)
	UpdateJobStatus(id, status, errMsg string) error
	GetJob(id string) (JobRecord, bool)
	DeleteJobs(repoID, ticketNumber string) error
	PruneTicketJobs(repoID string, keepTickets []string) error
}

// Repository is the unified storage interface combining repo, ticket, and job operations.
type Repository interface {
	RepoStore
	TicketStore
	JobStore
}
