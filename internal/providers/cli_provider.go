package providers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ai-ticket-worker/internal/config"
	"ai-ticket-worker/internal/shell"
)

type CLIProvider struct {
	name    string
	command string
	args    []string
}

func NewFromConfig(cfg config.Config) (AIProvider, error) {
	pc, ok := cfg.Providers[cfg.Provider]
	if !ok {
		return nil, fmt.Errorf("provider %q missing from config providers", cfg.Provider)
	}
	if pc.Command == "" {
		return nil, fmt.Errorf("provider %q command is empty", cfg.Provider)
	}
	switch cfg.Provider {
	case "gemini":
		return &GeminiProvider{CLIProvider{name: "gemini", command: pc.Command, args: pc.Args}}, nil
	case "codex":
		return &CodexProvider{CLIProvider{name: "codex", command: pc.Command, args: pc.Args}}, nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}
}

func (p *CLIProvider) Name() string { return p.name }

func (p *CLIProvider) runPrompt(ctx context.Context, worktreePath, runtimeDir, phase, prompt string) (string, error) {
	inputPath := filepath.Join(runtimeDir, fmt.Sprintf("%s-input.md", phase))
	outputPath := filepath.Join(runtimeDir, fmt.Sprintf("%s-output.md", phase))
	stderrPath := filepath.Join(runtimeDir, fmt.Sprintf("%s-stderr.log", phase))
	if err := os.WriteFile(inputPath, []byte(prompt), 0o644); err != nil {
		return "", err
	}
	res, err := shell.Run(ctx, worktreePath, nil, prompt, p.command, p.args...)
	_ = os.WriteFile(outputPath, []byte(res.Stdout), 0o644)
	_ = os.WriteFile(stderrPath, []byte(res.Stderr), 0o644)
	if err != nil {
		return "", fmt.Errorf("provider %s phase %s failed: %w", p.name, phase, err)
	}
	if strings.TrimSpace(res.Stdout) == "" {
		return "", fmt.Errorf("provider %s phase %s returned empty output", p.name, phase)
	}
	return res.Stdout, nil
}

func buildInvestigatePrompt(req InvestigateRequest) string {
	return fmt.Sprintf(`You are assisting with software ticket investigation.

Ticket #%s: %s
URL: %s

Description:
%s

Acceptance Criteria:
%s

Repo path: %s
Worktree path: %s
Guidelines file: %s
Existing log path: %s
Existing proposal path: %s
Human feedback: %s

Return markdown with sections:
- Problem Summary
- Suggested Solution
- Likely Files To Change
- Risks
- Test Plan
- Open Questions
`, req.Ticket.Number, req.Ticket.Title, req.Ticket.URL, req.Ticket.Description, req.Ticket.AcceptanceCriteria, req.RepoPath, req.WorktreePath, req.GuidelinesPath, req.LogPath, req.ProposalPath, req.Feedback)
}

func buildImplementPrompt(req ImplementRequest) string {
	return fmt.Sprintf(`Implement the approved solution for the following ticket in this worktree.

Ticket #%s: %s
Description:
%s

Use proposal at: %s
Use log at: %s
Guidelines file: %s

If validation failed previously, address these failures:
%s

After making changes, return markdown with sections:
- Changes Made
- Notable Files Changed
- Remaining Risks
- Tests To Run
`, req.Ticket.Number, req.Ticket.Title, req.Ticket.Description, req.ProposalPath, req.LogPath, req.GuidelinesPath, req.FailureContext)
}

func buildPRPrompt(req PRRequest) string {
	return fmt.Sprintf(`Generate a PR description in markdown.

Ticket #%s: %s
Description:
%s

Use these files as source of truth:
- worktree: %s
- log: %s
- proposal: %s
- final solution: %s
- checks: %s

Include sections:
- Summary
- Problem Being Solved
- Implementation Overview
- Notable Files Changed
- Risks / Follow-ups
- Test Results
`, req.Ticket.Number, req.Ticket.Title, req.Ticket.Description, req.WorktreePath, req.LogPath, req.ProposalPath, req.FinalSolutionPath, req.ChecksLogPath)
}

type GeminiProvider struct{ CLIProvider }

type CodexProvider struct{ CLIProvider }

func (p *GeminiProvider) Investigate(ctx context.Context, req InvestigateRequest, runtimeDir string) (InvestigateResult, error) {
	out, err := p.runPrompt(ctx, req.WorktreePath, runtimeDir, "investigate", buildInvestigatePrompt(req))
	if err != nil {
		return InvestigateResult{}, err
	}
	return InvestigateResult{Proposal: out, RawOut: out}, nil
}

func (p *GeminiProvider) Implement(ctx context.Context, req ImplementRequest, runtimeDir string) (ImplementResult, error) {
	out, err := p.runPrompt(ctx, req.WorktreePath, runtimeDir, "implement", buildImplementPrompt(req))
	if err != nil {
		return ImplementResult{}, err
	}
	return ImplementResult{Summary: out, RawOut: out}, nil
}

func (p *GeminiProvider) SummarizePR(ctx context.Context, req PRRequest, runtimeDir string) (PRResult, error) {
	out, err := p.runPrompt(ctx, req.WorktreePath, runtimeDir, "pr", buildPRPrompt(req))
	if err != nil {
		return PRResult{}, err
	}
	return PRResult{Body: out, RawOut: out}, nil
}

func (p *CodexProvider) Investigate(ctx context.Context, req InvestigateRequest, runtimeDir string) (InvestigateResult, error) {
	out, err := p.runPrompt(ctx, req.WorktreePath, runtimeDir, "investigate", buildInvestigatePrompt(req))
	if err != nil {
		return InvestigateResult{}, err
	}
	return InvestigateResult{Proposal: out, RawOut: out}, nil
}

func (p *CodexProvider) Implement(ctx context.Context, req ImplementRequest, runtimeDir string) (ImplementResult, error) {
	out, err := p.runPrompt(ctx, req.WorktreePath, runtimeDir, "implement", buildImplementPrompt(req))
	if err != nil {
		return ImplementResult{}, err
	}
	return ImplementResult{Summary: out, RawOut: out}, nil
}

func (p *CodexProvider) SummarizePR(ctx context.Context, req PRRequest, runtimeDir string) (PRResult, error) {
	out, err := p.runPrompt(ctx, req.WorktreePath, runtimeDir, "pr", buildPRPrompt(req))
	if err != nil {
		return PRResult{}, err
	}
	return PRResult{Body: out, RawOut: out}, nil
}
