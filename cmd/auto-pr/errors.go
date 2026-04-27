// Package main is the entry point for the auto-pr CLI client.
package main

import "errors"

var (
	errUsageStatus          = errors.New("usage: auto-pr status [ticket-number]")
	errActionRequiresLabel  = errors.New("action requires --label")
	errWaitForJobServerOnly = errors.New("wait-for-job is only supported in server mode")
	errCleanupFlags         = errors.New("cleanup: use either --done or --all, not both")
	errUsageCleanup         = errors.New("usage: auto-pr cleanup <ticket-number> | --done | --all")
	errUsage                = errors.New("invalid usage")
)
