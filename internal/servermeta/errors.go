// Package servermeta manages the server-side metadata store: repos, tickets, and background jobs.
package servermeta

import "errors"

// ErrJobNotFound is returned when a job ID does not exist in the store.
var ErrJobNotFound = errors.New("job not found")
