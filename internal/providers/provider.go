package providers

import (
	"context"
)

type ExecuteRequest struct {
	PromptPath string // absolute path to the prompt file
	WorkDir    string // worktree root — AI execution is scoped here
	RuntimeDir string // provider-specific scratch space for artifacts
}

type ExecuteResult struct {
	RawOutput string
}

type AIProvider interface {
	Name() string
	Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error)
}
