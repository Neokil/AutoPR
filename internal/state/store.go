package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Neokil/AutoPR/internal/domain/workflowstate"
	"github.com/Neokil/AutoPR/internal/gitutil"
)

// StateFileName is the name of the JSON file that holds a ticket's persisted workflow state.
const StateFileName = "state.json"

// Store keeps ticket state files inside the configured state directory under the repo root.
type Store struct {
	RepoRoot  string
	StateRoot string
}

// NewStore returns a Store rooted at stateDirName inside repoRoot.
func NewStore(repoRoot, stateDirName string) *Store {
	return &Store{
		RepoRoot:  repoRoot,
		StateRoot: filepath.Join(repoRoot, stateDirName),
	}
}

// TicketDir returns the directory used to store state for the given ticket before a worktree exists.
func (s *Store) TicketDir(ticketNumber string) string {
	return filepath.Join(s.StateRoot, ticketNumber)
}

// LoadState reads the persisted state for the ticket, preferring the worktree location
// when it exists and falling back to the pre-worktree state directory.
func (s *Store) LoadState(ticketNumber string) (workflowstate.State, error) {
	// Prefer the worktree location when it exists.
	wtStatePath := filepath.Join(s.worktreePath(ticketNumber), ".auto-pr", StateFileName)
	data, err := os.ReadFile(wtStatePath) //nolint:gosec // G304: path constructed from internal worktree state
	if err == nil {
		return parseStateJSON(ticketNumber, data)
	}
	// Fall back to the pre-worktree location.
	data, err = os.ReadFile(filepath.Join(s.TicketDir(ticketNumber), StateFileName))
	if err != nil {
		return workflowstate.State{}, fmt.Errorf("read state file: %w", err)
	}

	return parseStateJSON(ticketNumber, data)
}

// SaveState persists st, writing to the worktree location once a worktree exists
// and removing the pre-worktree copy to keep a single source of truth.
func (s *Store) SaveState(ticketNumber string, state workflowstate.State) error {
	state.Touch()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}

	if state.WorktreePath != "" {
		// Once the worktree exists, state lives inside it.
		autoPRDir := filepath.Join(state.WorktreePath, ".auto-pr")
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

// ListTicketDirs returns the ticket numbers of all tickets that have persisted state,
// searching both the pre-worktree directory and the worktrees directory.
func (s *Store) ListTicketDirs() ([]string, error) {
	seen := map[string]struct{}{}
	var out []string

	// Tickets with state still in the pre-worktree location.
	entries, err := os.ReadDir(s.StateRoot)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read state root: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "worktrees" {
			continue
		}
		_, statErr := os.Stat(filepath.Join(s.StateRoot, entry.Name(), StateFileName))
		if statErr == nil {
			seen[entry.Name()] = struct{}{}
			out = append(out, entry.Name())
		}
	}

	// Tickets whose state has moved into the worktree.
	worktreesDir := filepath.Join(s.StateRoot, "worktrees")
	wtEntries, err := os.ReadDir(worktreesDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read worktrees dir: %w", err)
	}
	for _, entry := range wtEntries {
		if !entry.IsDir() {
			continue
		}
		statePath := filepath.Join(worktreesDir, entry.Name(), ".auto-pr", StateFileName)
		_, statErr := os.Stat(statePath)
		if statErr != nil {
			continue
		}
		if _, ok := seen[entry.Name()]; !ok {
			seen[entry.Name()] = struct{}{}
			out = append(out, entry.Name())
		}
	}

	return out, nil
}

// RemoveTicketDir deletes the pre-worktree state directory for the given ticket.
func (s *Store) RemoveTicketDir(ticketNumber string) error {
	err := os.RemoveAll(s.TicketDir(ticketNumber))
	if err != nil {
		return fmt.Errorf("remove ticket dir: %w", err)
	}

	return nil
}

func (s *Store) worktreePath(ticketNumber string) string {
	return gitutil.WorktreePath(s.RepoRoot, filepath.Base(s.StateRoot), ticketNumber)
}

func (s *Store) ensureTicketDir(ticketNumber string) (string, error) {
	dir := s.TicketDir(ticketNumber)
	err := os.MkdirAll(dir, 0o755) //nolint:gosec,mnd // G301: 0755 correct for project directories
	if err != nil {
		return "", fmt.Errorf("create ticket runtime dir: %w", err)
	}

	return dir, nil
}

func parseStateJSON(ticketNumber string, data []byte) (workflowstate.State, error) {
	if isV2StateJSON(data) {
		return workflowstate.State{}, fmt.Errorf("ticket %s: %w", ticketNumber, ErrV2StateFile)
	}
	var state workflowstate.State
	err := json.Unmarshal(data, &state)
	if err != nil {
		return workflowstate.State{}, fmt.Errorf("parse state file: %w", err)
	}

	return state, nil
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
