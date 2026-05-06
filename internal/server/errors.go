// Package server hosts the AutoPR daemon implementation.
package server

import "errors"

var (
	errRepoPathEmpty        = errors.New("repo_path is empty")
	errUnsupportedJobAction = errors.New("unsupported job action")
	errUnsupportedPRURL     = errors.New("unsupported PR URL format")
	errJobQueueFull         = errors.New("job queue is full")
)

const (
	eventTypeJob            = "job"
	eventTypeTicketDeleted  = "ticket_deleted"
	errMsgTicketNotFound    = "ticket not found"
	errMsgUnexpectedEnqueue = "unexpected enqueue status"
)
