package gitutil

import "errors"

var ErrUnsupportedGitHubURL = errors.New("origin remote is not a supported GitHub URL")
