// Package server hosts the AutoPR daemon implementation.
package server

import "errors"

var (
	errRepoPathEmpty        = errors.New("repo_path is empty")
	errUnsupportedJobAction = errors.New("unsupported job action")
	errUnsupportedPRURL     = errors.New("unsupported PR URL format")
)
