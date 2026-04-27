// Package serverstate manages the daemon state store: repos, tickets, and background jobs.
package serverstate

import "errors"

// ErrJobNotFound is returned when a job ID does not exist in the store.
var ErrJobNotFound = errors.New("job not found")
