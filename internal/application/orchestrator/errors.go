// Package orchestrator provides the Service interface and its implementations for driving
// ticket workflows either in-process or via the AutoPR HTTP server.
package orchestrator

import "errors"

// Sentinel errors returned by RemoteService when communicating with the AutoPR server.
var (
	ErrJobFailed          = errors.New("job failed")
	ErrUnexpectedStatus   = errors.New("unexpected job status")
	ErrMissingJobID       = errors.New("server response missing job_id")
	ErrRemote             = errors.New("remote error")
	ErrHTTP               = errors.New("HTTP request failed")
)
