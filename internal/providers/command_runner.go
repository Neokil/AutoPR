package providers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Neokil/AutoPR/internal/shell"
)

type PromptCommandRunner struct {
	providerName string
	command      string
	args         []string
}

func (r *PromptCommandRunner) Run(ctx context.Context, worktreePath, runtimeDir, phase, prompt string) (string, string, error) {
	err := writePromptArtifacts(runtimeDir, phase, prompt, "", "")
	if err != nil {
		return "", "", err
	}

	res, err := shell.Run(ctx, worktreePath, nil, prompt, r.command, r.args...)
	_ = writePromptArtifacts(runtimeDir, phase, prompt, res.Stdout, res.Stderr)
	if err != nil {
		return res.Stdout, res.Stderr, fmt.Errorf("provider %s phase %s failed: %w", r.providerName, phase, err)
	}
	if strings.TrimSpace(res.Stdout) == "" {
		return "", res.Stderr, fmt.Errorf("provider %s phase %s: %w", r.providerName, phase, ErrEmptyOutput)
	}

	return res.Stdout, res.Stderr, nil
}

func writePromptArtifacts(runtimeDir, phase, prompt, stdout, stderr string) error {
	inputPath := filepath.Join(runtimeDir, phase+"-input.md")
	outputPath := filepath.Join(runtimeDir, phase+"-output.md")
	stderrPath := filepath.Join(runtimeDir, phase+"-stderr.log")
	err := os.WriteFile(inputPath, []byte(prompt), 0o644) //nolint:gosec,mnd // G306: 0644 intentional for user-readable run artifacts
	if err != nil {
		return fmt.Errorf("write input file: %w", err)
	}
	_ = os.WriteFile(outputPath, []byte(stdout), 0o644) //nolint:gosec,mnd // G306: 0644 intentional for user-readable run artifacts
	_ = os.WriteFile(stderrPath, []byte(stderr), 0o644) //nolint:gosec,mnd // G306: 0644 intentional for user-readable run artifacts

	return nil
}
