// Package providers defines prompt execution inputs/results and concrete provider implementations.
package providers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Neokil/AutoPR/internal/config"
)

// CLIProvider shells out to a configured command-line tool.
type CLIProvider struct {
	name   string
	runner *PromptCommandRunner
}

// NewFromConfig constructs a CLIProvider from the named provider in cfg.
func NewFromConfig(cfg config.Config) (*CLIProvider, error) {
	providerCmd, ok := cfg.Providers[cfg.Provider]
	if !ok {
		return nil, fmt.Errorf("provider %q: %w", cfg.Provider, ErrProviderMissing)
	}
	if cfg.Provider == "codex" && len(providerCmd.Args) == 0 {
		providerCmd.Args = []string{"exec", "-"}
	}
	if providerCmd.Command == "" {
		return nil, fmt.Errorf("provider %q: %w", cfg.Provider, ErrProviderCommandEmpty)
	}

	return &CLIProvider{
		name: cfg.Provider,
		runner: &PromptCommandRunner{
			providerName: cfg.Provider,
			command:      providerCmd.Command,
			args:         providerCmd.Args,
			sessionCfg:   providerCmd.Session,
		},
	}, nil
}

// Name returns the configured provider name (e.g. "codex", "claude-code").
func (p *CLIProvider) Name() string { return p.name }

// Execute reads the prompt file and runs the provider CLI, returning the captured output.
func (p *CLIProvider) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error) {
	content, err := os.ReadFile(req.PromptPath)
	if err != nil {
		return ExecuteResult{}, fmt.Errorf("read prompt %s: %w", req.PromptPath, err)
	}
	phase := strings.TrimSuffix(filepath.Base(req.PromptPath), ".md")
	text, stderr, sessionData, err := p.runner.Run(ctx, req.WorkDir, req.RuntimeDir, phase, string(content), req.SessionData)
	result := ExecuteResult{
		RawOutput:   text,
		Stderr:      stderr,
		SessionData: sessionData,
	}
	if err != nil {
		return result, err
	}

	return result, nil
}
