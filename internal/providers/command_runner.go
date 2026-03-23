package providers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ai-ticket-worker/internal/shell"
)

type PromptCommandRunner struct {
	providerName string
	command      string
	args         []string
}

func (r *PromptCommandRunner) Run(ctx context.Context, worktreePath, runtimeDir, phase, prompt string) (string, error) {
	if err := writePromptArtifacts(runtimeDir, phase, prompt, "", ""); err != nil {
		return "", err
	}

	res, err := shell.Run(ctx, worktreePath, nil, prompt, r.command, r.args...)
	_ = writePromptArtifacts(runtimeDir, phase, prompt, res.Stdout, res.Stderr)
	if err != nil {
		return "", fmt.Errorf("provider %s phase %s failed: %w", r.providerName, phase, err)
	}
	if strings.TrimSpace(res.Stdout) == "" {
		return "", fmt.Errorf("provider %s phase %s returned empty output", r.providerName, phase)
	}
	return res.Stdout, nil
}

func writePromptArtifacts(runtimeDir, phase, prompt, stdout, stderr string) error {
	inputPath := filepath.Join(runtimeDir, fmt.Sprintf("%s-input.md", phase))
	outputPath := filepath.Join(runtimeDir, fmt.Sprintf("%s-output.md", phase))
	stderrPath := filepath.Join(runtimeDir, fmt.Sprintf("%s-stderr.log", phase))
	if err := os.WriteFile(inputPath, []byte(prompt), 0o644); err != nil {
		return err
	}
	_ = os.WriteFile(outputPath, []byte(stdout), 0o644)
	_ = os.WriteFile(stderrPath, []byte(stderr), 0o644)
	return nil
}
