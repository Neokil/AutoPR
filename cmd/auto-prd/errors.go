// Package main is the entry point for the auto-prd server daemon.
package main

import "errors"

var (
	errRepoPathEmpty       = errors.New("repo_path is empty")
	errUnsupportedJobAction = errors.New("unsupported job action")
	errUnsupportedPRURL    = errors.New("unsupported PR URL format")
)
