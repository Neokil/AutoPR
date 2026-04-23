package orchestrator

import "errors"

var (
	ErrJobFailed          = errors.New("job failed")
	ErrUnexpectedStatus   = errors.New("unexpected job status")
	ErrMissingJobID       = errors.New("server response missing job_id")
	ErrRemote             = errors.New("remote error")
	ErrHTTP               = errors.New("HTTP request failed")
)
