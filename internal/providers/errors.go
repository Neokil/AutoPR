package providers

import "errors"

var (
	ErrProviderMissing      = errors.New("provider missing from config")
	ErrProviderCommandEmpty = errors.New("provider command is empty")
	ErrEmptyOutput          = errors.New("provider returned empty output")
)
