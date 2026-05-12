package server

import "github.com/Neokil/AutoPR/internal/serverstate"

// JobAction identifies the type of background job the daemon should execute.
type JobAction = serverstate.JobAction

const (
	jobRun         JobAction = "run"
	jobAction      JobAction = "action"
	jobMoveToState JobAction = "move_to_state"
	jobCleanup     JobAction = "cleanup_ticket"
	jobCleanupDone JobAction = "cleanup_done"
	jobCleanupAll  JobAction = "cleanup_all"
)
