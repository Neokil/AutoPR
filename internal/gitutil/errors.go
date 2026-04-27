// Package gitutil provides helper functions for running git and gh commands.
package gitutil

import "errors"

// ErrUnsupportedGitHubURL is returned when the origin remote cannot be parsed as a GitHub URL.
var ErrUnsupportedGitHubURL = errors.New("origin remote is not a supported GitHub URL")
