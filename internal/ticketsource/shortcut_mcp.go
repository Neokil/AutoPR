package ticketsource

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ai-ticket-worker/internal/config"
	"ai-ticket-worker/internal/models"
	"ai-ticket-worker/internal/shell"
)

type ShortcutMCPSource struct {
	Cfg     config.ShortcutMCPConfig
	RepoDir string
}

func NewShortcutMCPSource(cfg config.ShortcutMCPConfig, repoDir string) *ShortcutMCPSource {
	return &ShortcutMCPSource{Cfg: cfg, RepoDir: repoDir}
}

func (s *ShortcutMCPSource) GetTicket(ctx context.Context, ticketNumber string) (models.Ticket, error) {
	if !s.Cfg.Enabled {
		return models.Ticket{}, fmt.Errorf("shortcut MCP source is disabled")
	}
	if s.Cfg.Command == "" {
		return models.Ticket{}, fmt.Errorf("shortcut_mcp.command is required")
	}
	args := make([]string, 0, len(s.Cfg.Args)+1)
	replaced := false
	for _, a := range s.Cfg.Args {
		if strings.Contains(a, "{ticket}") {
			replaced = true
		}
		a = strings.ReplaceAll(a, "{ticket}", ticketNumber)
		args = append(args, a)
	}
	if !replaced {
		args = append(args, ticketNumber)
	}
	res, err := shell.Run(ctx, s.RepoDir, s.Cfg.Env, "", s.Cfg.Command, args...)
	if err != nil {
		return models.Ticket{}, fmt.Errorf("shortcut mcp command failed: %w\nstderr: %s", err, strings.TrimSpace(res.Stderr))
	}
	ticket, err := decodeTicketPayload(res.Stdout)
	if err != nil {
		return models.Ticket{}, fmt.Errorf("decode shortcut output: %w", err)
	}
	if ticket.Number == "" {
		ticket.Number = ticketNumber
	}
	if ticket.ID == "" {
		ticket.ID = ticket.Number
	}
	return ticket, nil
}

func decodeTicketPayload(raw string) (models.Ticket, error) {
	var direct models.Ticket
	if err := json.Unmarshal([]byte(raw), &direct); err == nil && direct.Title != "" {
		return direct, nil
	}

	var wrapped struct {
		Ticket models.Ticket `json:"ticket"`
	}
	if err := json.Unmarshal([]byte(raw), &wrapped); err == nil && wrapped.Ticket.Title != "" {
		return wrapped.Ticket, nil
	}

	var shortcut struct {
		ID          interface{} `json:"id"`
		Name        string      `json:"name"`
		Description string      `json:"description"`
		AppURL      string      `json:"app_url"`
		ExternalID  string      `json:"external_id"`
		Labels      []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	if err := json.Unmarshal([]byte(raw), &shortcut); err != nil {
		return models.Ticket{}, err
	}
	if shortcut.Name == "" {
		return models.Ticket{}, fmt.Errorf("ticket title is empty in MCP output")
	}
	labels := make([]string, 0, len(shortcut.Labels))
	for _, l := range shortcut.Labels {
		if l.Name != "" {
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
