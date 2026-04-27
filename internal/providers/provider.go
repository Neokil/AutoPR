package providers

import (
	"context"
)

// ExecuteRequest carries the inputs needed to run a prompt through an AI provider.
type ExecuteRequest struct {
	PromptPath string // absolute path to the prompt file
	WorkDir    string // worktree root — AI execution is scoped here
	RuntimeDir string // provider-specific scratch space for artifacts
}

// ExecuteResult holds the output produced by an AI provider after executing a prompt.
type ExecuteResult struct {
	RawOutput string
	Stderr    string
}

// AIProvider is the interface that all AI execution backends must implement.
type AIProvider interface {
	Name() string
	Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error)
}
