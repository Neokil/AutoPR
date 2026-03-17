package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"ai-ticket-worker/internal/models"
)

const (
	StateFileName         = "state.json"
	TicketFileName        = "ticket.json"
	LogFileName           = "log.md"
	ProposalFileName      = "proposal.md"
	FinalSolutionFileName = "final_solution.md"
	PRFileName            = "pr.md"
	ChecksFileName        = "checks.log"
	ProviderDirName       = "provider"
)

type Store struct {
	RepoRoot  string
	StateRoot string
}

func NewStore(repoRoot, stateDirName string) *Store {
	return &Store{
		RepoRoot:  repoRoot,
		StateRoot: filepath.Join(repoRoot, stateDirName),
	}
}

func (s *Store) TicketDir(ticketNumber string) string {
	return filepath.Join(s.StateRoot, ticketNumber)
}

func (s *Store) EnsureTicketDir(ticketNumber string) (string, error) {
	dir := s.TicketDir(ticketNumber)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create ticket runtime dir: %w", err)
	}
	providerDir := filepath.Join(dir, ProviderDirName)
	if err := os.MkdirAll(providerDir, 0o755); err != nil {
		return "", fmt.Errorf("create provider dir: %w", err)
	}
	return dir, nil
}

func (s *Store) LoadState(ticketNumber string) (models.TicketState, error) {
	path := filepath.Join(s.TicketDir(ticketNumber), StateFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return models.TicketState{}, err
	}
	var st models.TicketState
	if err := json.Unmarshal(data, &st); err != nil {
		return models.TicketState{}, fmt.Errorf("parse state file: %w", err)
	}
	return st, nil
}

func (s *Store) SaveState(ticketNumber string, st models.TicketState) error {
	dir, err := s.EnsureTicketDir(ticketNumber)
	if err != nil {
		return err
	}
	st.Touch()
	path := filepath.Join(dir, StateFileName)
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

func (s *Store) SaveTicket(ticketNumber string, t models.Ticket) (string, error) {
	dir, err := s.EnsureTicketDir(ticketNumber)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, TicketFileName)
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode ticket: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func (s *Store) LoadTicket(ticketNumber string) (models.Ticket, error) {
	path := filepath.Join(s.TicketDir(ticketNumber), TicketFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return models.Ticket{}, err
	}
	var t models.Ticket
	if err := json.Unmarshal(data, &t); err != nil {
		return models.Ticket{}, fmt.Errorf("parse ticket file: %w", err)
	}
	return t, nil
}

func (s *Store) Paths(ticketNumber string) map[string]string {
	dir := s.TicketDir(ticketNumber)
	return map[string]string{
		"dir":         dir,
		"state":       filepath.Join(dir, StateFileName),
		"ticket":      filepath.Join(dir, TicketFileName),
		"log":         filepath.Join(dir, LogFileName),
		"proposal":    filepath.Join(dir, ProposalFileName),
		"final":       filepath.Join(dir, FinalSolutionFileName),
		"pr":          filepath.Join(dir, PRFileName),
		"checks":      filepath.Join(dir, ChecksFileName),
		"providerDir": filepath.Join(dir, ProviderDirName),
	}
}

func (s *Store) ListTicketDirs() ([]string, error) {
	entries, err := os.ReadDir(s.StateRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "worktrees" {
			continue
		}
		out = append(out, e.Name())
	}
	return out, nil
}
