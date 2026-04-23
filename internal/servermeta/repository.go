package servermeta

type RepoStore interface {
	UpsertRepo(repoPath string) (RepoRecord, error)
	ListRepos() []RepoRecord
}

type TicketStore interface {
	UpsertTicket(rec TicketRecord) error
	DeleteTicket(repoID, ticketNumber string) error
	ReplaceRepoTickets(repoID string, tickets []TicketRecord) error
	ListTickets(repoID string) []TicketRecord
}

type JobStore interface {
	NewJob(action, repoID, repoPath, ticketNumber, scope string) (JobRecord, error)
	UpdateJobStatus(id, status, errMsg string) error
	GetJob(id string) (JobRecord, bool)
	DeleteJobs(repoID, ticketNumber string) error
	PruneTicketJobs(repoID string, keepTickets []string) error
}

type Repository interface {
	RepoStore
	TicketStore
	JobStore
}
