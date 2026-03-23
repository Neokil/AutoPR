package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ai-ticket-worker/internal/config"
	"ai-ticket-worker/internal/domain/ticket"
)

type CLIProvider struct {
	name     string
	renderer *PromptRenderer
	runner   *PromptCommandRunner
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
	promptsDir, err := config.PromptsDirPath()
	if err != nil {
		return nil, err
	}
	renderer, err := NewPromptRenderer(promptsDir)
	if err != nil {
		return nil, err
	}
	base := CLIProvider{
		name:     cfg.Provider,
		renderer: renderer,
		runner: &PromptCommandRunner{
			providerName: cfg.Provider,
			command:      pc.Command,
			args:         pc.Args,
		},
	}
	return &base, nil
}

func (p *CLIProvider) Name() string { return p.name }

func (p *CLIProvider) getTicket(ctx context.Context, ticketNumber, repoPath, runtimeDir string) (ticket.Ticket, string, error) {
	prompt, err := p.renderer.Render(tplTicket, map[string]string{
		"TicketNumber": ticketNumber,
	})
	if err != nil {
		return ticket.Ticket{}, "", err
	}
	out, err := p.runner.Run(ctx, repoPath, runtimeDir, "ticket", prompt)
	if err != nil {
		return ticket.Ticket{}, "", err
	}
	parsedTicket, err := decodeTicketPayload(out)
	if err != nil {
		return ticket.Ticket{}, out, err
	}
	if parsedTicket.Number == "" {
		parsedTicket.Number = ticketNumber
	}
	if parsedTicket.ID == "" {
		parsedTicket.ID = parsedTicket.Number
	}
	return parsedTicket, out, nil
}

func renderTicketContext(ticket ticket.Ticket) string {
	var b strings.Builder
	if ticket.ParentTicket != nil {
		parent := ticket.ParentTicket
		fmt.Fprintf(&b, "Parent Ticket:\n- id: %s\n- number: %s\n- title: %s\n- url: %s\n- description: %s\n\n", parent.ID, parent.Number, parent.Title, parent.URL, parent.Description)
	}
	if ticket.Epic != nil {
		epic := ticket.Epic
		fmt.Fprintf(&b, "Epic:\n- id: %s\n- title: %s\n- url: %s\n- description: %s\n\n", epic.ID, epic.Title, epic.URL, epic.Description)
	}
	if b.Len() == 0 {
		return "None"
	}
	return strings.TrimSpace(b.String())
}

func (p *CLIProvider) GetTicket(ctx context.Context, ticketNumber, repoPath, runtimeDir string) (ticket.Ticket, string, error) {
	return p.getTicket(ctx, ticketNumber, repoPath, runtimeDir)
}

func (p *CLIProvider) Investigate(ctx context.Context, req InvestigateRequest, runtimeDir string) (InvestigateResult, error) {
	prompt, err := p.renderer.Render(tplInvestigate, map[string]string{
		"TicketNumber":             req.Ticket.Number,
		"TicketTitle":              req.Ticket.Title,
		"TicketURL":                req.Ticket.URL,
		"TicketDescription":        req.Ticket.Description,
		"TicketAcceptanceCriteria": req.Ticket.AcceptanceCriteria,
		"RelatedContext":           renderTicketContext(req.Ticket),
		"RepoPath":                 req.RepoPath,
		"WorktreePath":             req.WorktreePath,
		"GuidelinesPath":           req.GuidelinesPath,
		"LogPath":                  req.LogPath,
		"ProposalPath":             req.ProposalPath,
		"Feedback":                 req.Feedback,
	})
	if err != nil {
		return InvestigateResult{}, err
	}
	out, err := p.runner.Run(ctx, req.WorktreePath, runtimeDir, "investigate", prompt)
	if err != nil {
		return InvestigateResult{}, err
	}
	return InvestigateResult{Proposal: out, RawOut: out}, nil
}

func (p *CLIProvider) Implement(ctx context.Context, req ImplementRequest, runtimeDir string) (ImplementResult, error) {
	prompt, err := p.renderer.Render(tplImplement, map[string]string{
		"TicketNumber":      req.Ticket.Number,
		"TicketTitle":       req.Ticket.Title,
		"TicketDescription": req.Ticket.Description,
		"RelatedContext":    renderTicketContext(req.Ticket),
		"ProposalPath":      req.ProposalPath,
		"LogPath":           req.LogPath,
		"GuidelinesPath":    req.GuidelinesPath,
		"FailureContext":    req.FailureContext,
	})
	if err != nil {
		return ImplementResult{}, err
	}
	out, err := p.runner.Run(ctx, req.WorktreePath, runtimeDir, "implement", prompt)
	if err != nil {
		return ImplementResult{}, err
	}
	return ImplementResult{Summary: out, RawOut: out}, nil
}

func (p *CLIProvider) SummarizePR(ctx context.Context, req PRRequest, runtimeDir string) (PRResult, error) {
	prompt, err := p.renderer.Render(tplPR, map[string]string{
		"TicketNumber":      req.Ticket.Number,
		"TicketTitle":       req.Ticket.Title,
		"TicketDescription": req.Ticket.Description,
		"RelatedContext":    renderTicketContext(req.Ticket),
		"WorktreePath":      req.WorktreePath,
		"LogPath":           req.LogPath,
		"ProposalPath":      req.ProposalPath,
		"FinalSolutionPath": req.FinalSolutionPath,
		"ChecksLogPath":     req.ChecksLogPath,
	})
	if err != nil {
		return PRResult{}, err
	}
	out, err := p.runner.Run(ctx, req.WorktreePath, runtimeDir, "pr", prompt)
	if err != nil {
		return PRResult{}, err
	}
	return PRResult{Body: out, RawOut: out}, nil
}

func decodeTicketPayload(raw string) (ticket.Ticket, error) {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "```") {
		trimmed = stripCodeFence(trimmed)
	}
	if start := strings.Index(trimmed, "{"); start >= 0 {
		if end := strings.LastIndex(trimmed, "}"); end > start {
			trimmed = trimmed[start : end+1]
		}
	}
	var direct ticket.Ticket
	if err := json.Unmarshal([]byte(trimmed), &direct); err == nil && strings.TrimSpace(direct.Title) != "" {
		return direct, nil
	}

	var wrapped struct {
		Ticket ticket.Ticket `json:"ticket"`
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
		return ticket.Ticket{}, fmt.Errorf("parse provider ticket JSON: %w", err)
	}
	if strings.TrimSpace(shortcut.Name) == "" {
		return ticket.Ticket{}, fmt.Errorf("ticket title missing in provider output")
	}
	labels := make([]string, 0, len(shortcut.Labels))
	for _, l := range shortcut.Labels {
		if strings.TrimSpace(l.Name) != "" {
			labels = append(labels, l.Name)
		}
	}
	return ticket.Ticket{
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
