package providers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ai-ticket-worker/internal/config"
)

type CLIProvider struct {
	name   string
	runner *PromptCommandRunner
}

func NewFromConfig(cfg config.Config) (AIProvider, error) {
	pc, ok := cfg.Providers[cfg.Provider]
	if !ok {
		return nil, fmt.Errorf("provider %q missing from config providers", cfg.Provider)
	}
	if cfg.Provider == "codex" && len(pc.Args) == 0 {
		pc.Args = []string{"exec", "-"}
	}
	if pc.Command == "" {
		return nil, fmt.Errorf("provider %q command is empty", cfg.Provider)
	}
	return &CLIProvider{
		name: cfg.Provider,
		runner: &PromptCommandRunner{
			providerName: cfg.Provider,
			command:      pc.Command,
			args:         pc.Args,
		},
	}, nil
}

func (p *CLIProvider) Name() string { return p.name }

func (p *CLIProvider) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error) {
	content, err := os.ReadFile(req.PromptPath)
	if err != nil {
		return ExecuteResult{}, fmt.Errorf("read prompt %s: %w", req.PromptPath, err)
	}
	phase := strings.TrimSuffix(filepath.Base(req.PromptPath), ".md")
	stdout, stderr, err := p.runner.Run(ctx, req.WorkDir, req.RuntimeDir, phase, string(content))
	if err != nil {
		return ExecuteResult{RawOutput: stdout, Stderr: stderr}, err
	}
	return ExecuteResult{RawOutput: stdout, Stderr: stderr}, nil
}
