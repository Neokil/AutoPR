package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Neokil/AutoPR/internal/domain/ticket"
)

const StateFileName = "state.json"

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

func (s *Store) LoadState(ticketNumber string) (ticket.State, error) {
	// Prefer the worktree location when it exists.
	wtStatePath := filepath.Join(s.worktreePath(ticketNumber), ".auto-pr", StateFileName)
	data, err := os.ReadFile(wtStatePath) //nolint:gosec // G304: path constructed from internal worktree state
	if err == nil {
		return parseStateJSON(ticketNumber, data)
	}
	// Fall back to the pre-worktree location.
	data, err = os.ReadFile(filepath.Join(s.TicketDir(ticketNumber), StateFileName))
	if err != nil {
		return ticket.State{}, fmt.Errorf("read state file: %w", err)
	}

	return parseStateJSON(ticketNumber, data)
}

func (s *Store) SaveState(ticketNumber string, st ticket.State) error {
	st.Touch()
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}

	if st.WorktreePath != "" {
		// Once the worktree exists, state lives inside it.
		autoPRDir := filepath.Join(st.WorktreePath, ".auto-pr")
		err = os.MkdirAll(autoPRDir, 0o755) //nolint:gosec,mnd // G301: 0755 correct for project directories
		if err != nil {
			return fmt.Errorf("create worktree state dir: %w", err)
		}
		err = os.WriteFile(filepath.Join(autoPRDir, StateFileName), data, 0o644) //nolint:gosec,mnd // G306: 0644 intentional for user-readable state files
		if err != nil {
			return fmt.Errorf("write worktree state: %w", err)
		}
		// Remove the pre-worktree copy so there is only one source of truth.
		_ = os.Remove(filepath.Join(s.TicketDir(ticketNumber), StateFileName))

		return nil
	}

	dir, err := s.ensureTicketDir(ticketNumber)
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(dir, StateFileName), data, 0o644) //nolint:gosec,mnd // G306: 0644 intentional for user-readable state files
	if err != nil {
		return fmt.Errorf("write state: %w", err)
	}

	return nil
}

func (s *Store) ListTicketDirs() ([]string, error) {
	seen := map[string]struct{}{}
	var out []string

	// Tickets with state still in the pre-worktree location.
	entries, err := os.ReadDir(s.StateRoot)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read state root: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "worktrees" {
			continue
		}
		_, statErr := os.Stat(filepath.Join(s.StateRoot, e.Name(), StateFileName))
		if statErr == nil {
			seen[e.Name()] = struct{}{}
			out = append(out, e.Name())
		}
	}

	// Tickets whose state has moved into the worktree.
	worktreesDir := filepath.Join(s.StateRoot, "worktrees")
	wtEntries, err := os.ReadDir(worktreesDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read worktrees dir: %w", err)
	}
	for _, e := range wtEntries {
		if !e.IsDir() {
			continue
		}
		statePath := filepath.Join(worktreesDir, e.Name(), ".auto-pr", StateFileName)
		_, statErr := os.Stat(statePath)
		if statErr != nil {
			continue
		}
		if _, ok := seen[e.Name()]; !ok {
			seen[e.Name()] = struct{}{}
			out = append(out, e.Name())
		}
	}

	return out, nil
}

func (s *Store) RemoveTicketDir(ticketNumber string) error {
	err := os.RemoveAll(s.TicketDir(ticketNumber))
	if err != nil {
		return fmt.Errorf("remove ticket dir: %w", err)
	}

	return nil
}

func (s *Store) worktreePath(ticketNumber string) string {
	return filepath.Join(s.StateRoot, "worktrees", ticketNumber)
}

func (s *Store) ensureTicketDir(ticketNumber string) (string, error) {
	dir := s.TicketDir(ticketNumber)
	err := os.MkdirAll(dir, 0o755) //nolint:gosec,mnd // G301: 0755 correct for project directories
	if err != nil {
		return "", fmt.Errorf("create ticket runtime dir: %w", err)
	}

	return dir, nil
}

func parseStateJSON(ticketNumber string, data []byte) (ticket.State, error) {
	if isV2StateJSON(data) {
		return ticket.State{}, fmt.Errorf("ticket %s: %w", ticketNumber, ErrV2StateFile)
	}
	var st ticket.State
	err := json.Unmarshal(data, &st)
	if err != nil {
		return ticket.State{}, fmt.Errorf("parse state file: %w", err)
	}

	return st, nil
}

func isV2StateJSON(data []byte) bool {
	var raw map[string]json.RawMessage
	err := json.Unmarshal(data, &raw)
	if err != nil {
		return false
	}
	rawStatus, hasStatus := raw["status"]
	_, hasFlowStatus := raw["flow_status"]
	if !hasStatus || hasFlowStatus {
		return false
	}
	var statusStr string
	err = json.Unmarshal(rawStatus, &statusStr)
	if err != nil {
		return false
	}
	v2States := map[string]bool{
		"queued": true, "investigating": true, "proposal_ready": true,
		"waiting_for_human": true, "implementing": true, "validating": true,
		"pr_ready": true,
	}

	return v2States[statusStr]
}
