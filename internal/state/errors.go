// Package state implements the filesystem-backed StateStore used in production.
package state

import "errors"

// ErrV2StateFile is returned when a state file written by the v2 engine is encountered.
var ErrV2StateFile = errors.New("v2 state file detected; v3 flows must be started fresh (cleanup and re-run)")
