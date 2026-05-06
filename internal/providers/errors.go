package providers

import "errors"

// Sentinel errors returned by provider construction and execution.
var (
	ErrProviderMissing      = errors.New("provider missing from config")
	ErrProviderCommandEmpty = errors.New("provider command is empty")
	ErrEmptyOutput          = errors.New("provider returned empty output")
	ErrTokensExhausted      = errors.New("provider token usage limit reached")
	ErrSessionFailed        = errors.New("provider session error")
)
