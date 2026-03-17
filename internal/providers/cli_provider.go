package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ai-ticket-worker/internal/config"
	"ai-ticket-worker/internal/models"
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
	if cfg.Provider == "codex" && len(pc.Args) == 0 {
		// Interactive `codex` requires a TTY; use non-interactive mode by default.
		pc.Args = []string{"exec", "-"}
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

func (p *CLIProvider) getTicket(ctx context.Context, ticketNumber, repoPath, runtimeDir string) (models.Ticket, string, error) {
	prompt := fmt.Sprintf(`Fetch Shortcut ticket details for story/ticket number %s using your configured MCP integration.

Return ONLY valid JSON (no markdown fences, no extra text) with this shape:
{
  "number": "%s",
  "id": "string",
  "title": "string",
  "description": "string",
  "acceptance_criteria": "string",
  "priority": "string",
  "url": "string",
  "labels": ["string"],
  "workflow_fields": {"key":"value"}
}

If a field is unknown, use empty string or empty arrays/maps.
`, ticketNumber, ticketNumber)
	out, err := p.runPrompt(ctx, repoPath, runtimeDir, "ticket", prompt)
	if err != nil {
		return models.Ticket{}, "", err
	}
	ticket, err := decodeTicketPayload(out)
	if err != nil {
		return models.Ticket{}, out, err
	}
	if ticket.Number == "" {
		ticket.Number = ticketNumber
	}
	if ticket.ID == "" {
		ticket.ID = ticket.Number
	}
	return ticket, out, nil
}

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

func (p *GeminiProvider) GetTicket(ctx context.Context, ticketNumber, repoPath, runtimeDir string) (models.Ticket, string, error) {
	return p.getTicket(ctx, ticketNumber, repoPath, runtimeDir)
}

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

func (p *CodexProvider) GetTicket(ctx context.Context, ticketNumber, repoPath, runtimeDir string) (models.Ticket, string, error) {
	return p.getTicket(ctx, ticketNumber, repoPath, runtimeDir)
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

func decodeTicketPayload(raw string) (models.Ticket, error) {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "```") {
		trimmed = stripCodeFence(trimmed)
	}
	if start := strings.Index(trimmed, "{"); start >= 0 {
		if end := strings.LastIndex(trimmed, "}"); end > start {
			trimmed = trimmed[start : end+1]
		}
	}
	var direct models.Ticket
	if err := json.Unmarshal([]byte(trimmed), &direct); err == nil && strings.TrimSpace(direct.Title) != "" {
		return direct, nil
	}

	var wrapped struct {
		Ticket models.Ticket `json:"ticket"`
	}
	if err := json.Unmarshal([]byte(trimmed), &wrapped); err == nil && strings.TrimSpace(wrapped.Ticket.Title) != "" {
		return wrapped.Ticket, nil
	}

	var shortcut struct {
		ID          interface{} `json:"id"`
		Name        string      `json:"name"`
		Description string      `json:"description"`
		AppURL      string      `json:"app_url"`
		Labels      []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	if err := json.Unmarshal([]byte(trimmed), &shortcut); err != nil {
		return models.Ticket{}, fmt.Errorf("parse provider ticket JSON: %w", err)
	}
	if strings.TrimSpace(shortcut.Name) == "" {
		return models.Ticket{}, fmt.Errorf("ticket title missing in provider output")
	}
	labels := make([]string, 0, len(shortcut.Labels))
	for _, l := range shortcut.Labels {
		if strings.TrimSpace(l.Name) != "" {
			labels = append(labels, l.Name)
		}
	}
	return models.Ticket{
		ID:          fmt.Sprintf("%v", shortcut.ID),
		Title:       shortcut.Name,
		Description: shortcut.Description,
		URL:         shortcut.AppURL,
		Labels:      labels,
	}, nil
}

func stripCodeFence(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) < 3 {
		return s
	}
	if strings.HasPrefix(strings.TrimSpace(lines[0]), "```") && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
		return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
	}
	return s
}
