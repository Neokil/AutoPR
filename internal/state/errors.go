package state

import "errors"

var ErrV2StateFile = errors.New("v2 state file detected; v3 flows must be started fresh (cleanup and re-run)")
