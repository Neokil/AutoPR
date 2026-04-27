package servermeta

import (
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // G505: sha1 used for directory naming only, not cryptography
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// RepoRecord is the persisted metadata for a known repository.
type RepoRecord struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TicketRecord is the persisted metadata for a ticket tracked by the server.
type TicketRecord struct {
	RepoID       string      `json:"repo_id"`
	RepoPath     string      `json:"repo_path"`
	TicketNumber string      `json:"ticket_number"`
	Title        string      `json:"title,omitempty"`
	Status       string      `json:"status"`
	Busy         bool        `json:"busy"`
	Approved     bool        `json:"approved"`
	LastError    string      `json:"last_error,omitempty"`
	UpdatedAt    time.Time   `json:"updated_at"`
	PRURL        string      `json:"pr_url,omitempty"`
	Jobs         []JobRecord `json:"jobs,omitempty"`
}

// Data is the top-level structure serialized to the server state file.
type Data struct {
	Repos   map[string]RepoRecord   `json:"repos"`
	Tickets map[string]TicketRecord `json:"tickets"`
	Jobs    map[string]JobRecord    `json:"jobs"`
}

// JobRecord represents a single background job and its lifecycle timestamps.
type JobRecord struct {
	ID           string     `json:"id"`
	Action       string     `json:"action"`
	RepoID       string     `json:"repo_id"`
	RepoPath     string     `json:"repo_path"`
	TicketNumber string     `json:"ticket_number,omitempty"`
	Status       string     `json:"status"`
	Scope        string     `json:"scope,omitempty"`
	Error        string     `json:"error,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}

// Store is the JSON file-backed implementation of Repository, protected by a mutex.
type Store struct {
	path string
	mu   sync.Mutex
	data Data
}

var _ Repository = (*Store)(nil)

// DefaultPath returns the default path for the server state file (~/.auto-pr/server/state.json).
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}

	return filepath.Join(home, ".auto-pr", "server", "state.json"), nil
}

// NewStore creates a Store backed by the JSON file at path, loading existing data if present.
func NewStore(path string) (*Store, error) {
	store := &Store{path: path, data: Data{
		Repos:   map[string]RepoRecord{},
		Tickets: map[string]TicketRecord{},
		Jobs:    map[string]JobRecord{},
	}}
	err := store.load()
	if err != nil {
		return nil, err
	}

	return store, nil
}

// UpsertRepo inserts or updates the record for repoPath and returns it.
func (s *Store) UpsertRepo(repoPath string) (RepoRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := repoID(repoPath)
	rec := RepoRecord{
		ID:        id,
		Path:      repoPath,
		UpdatedAt: time.Now().UTC(),
	}
	s.data.Repos[id] = rec
	err := s.saveLocked()
	if err != nil {
		return RepoRecord{}, err
	}

	return rec, nil
}

// UpsertTicket inserts or replaces the ticket metadata record.
func (s *Store) UpsertTicket(rec TicketRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec.UpdatedAt = rec.UpdatedAt.UTC()
	s.data.Tickets[ticketKey(rec.RepoID, rec.TicketNumber)] = rec

	return s.saveLocked()
}

// DeleteTicket removes the metadata record for the given ticket.
func (s *Store) DeleteTicket(repoID, ticketNumber string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data.Tickets, ticketKey(repoID, ticketNumber))

	return s.saveLocked()
}

// DeleteJobs removes all job records associated with the given ticket.
func (s *Store) DeleteJobs(repoID, ticketNumber string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, job := range s.data.Jobs {
		if job.RepoID == repoID && job.TicketNumber == ticketNumber {
			delete(s.data.Jobs, id)
		}
	}

	return s.saveLocked()
}

// PruneTicketJobs deletes jobs for any ticket in repoID that is not in keepTickets.
func (s *Store) PruneTicketJobs(repoID string, keepTickets []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	keep := make(map[string]struct{}, len(keepTickets))
	for _, ticket := range keepTickets {
		keep[ticket] = struct{}{}
	}
	for id, job := range s.data.Jobs {
		if job.RepoID != repoID || job.TicketNumber == "" {
			continue
		}
		if _, ok := keep[job.TicketNumber]; ok {
			continue
		}
		delete(s.data.Jobs, id)
	}

	return s.saveLocked()
}

// ReplaceRepoTickets atomically replaces all ticket records for repoID with the provided slice.
func (s *Store) ReplaceRepoTickets(repoID string, tickets []TicketRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, rec := range s.data.Tickets {
		if rec.RepoID == repoID {
			delete(s.data.Tickets, key)
		}
	}
	for _, rec := range tickets {
		rec.UpdatedAt = rec.UpdatedAt.UTC()
		s.data.Tickets[ticketKey(rec.RepoID, rec.TicketNumber)] = rec
	}

	return s.saveLocked()
}

// ListTickets returns all ticket records, optionally filtered to repoID (empty = all repos).
// Each record has its Jobs slice populated and Busy flag set.
func (s *Store) ListTickets(repoID string) []TicketRecord {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobsByTicket := map[string][]JobRecord{}
	for _, job := range s.data.Jobs {
		if job.RepoID == "" || job.TicketNumber == "" {
			continue
		}
		key := ticketKey(job.RepoID, job.TicketNumber)
		jobsByTicket[key] = append(jobsByTicket[key], job)
	}
	for key := range jobsByTicket {
		sort.Slice(jobsByTicket[key], func(i, j int) bool {
			return jobsByTicket[key][i].CreatedAt.After(jobsByTicket[key][j].CreatedAt)
		})
	}

	out := make([]TicketRecord, 0, len(s.data.Tickets))
	for _, rec := range s.data.Tickets {
		if repoID != "" && rec.RepoID != repoID {
			continue
		}
		rec.Jobs = append([]JobRecord(nil), jobsByTicket[ticketKey(rec.RepoID, rec.TicketNumber)]...)
		rec.Busy = false
		for _, job := range rec.Jobs {
			if job.Status == "queued" || job.Status == "running" {
				rec.Busy = true

				break
			}
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			if out[i].RepoPath == out[j].RepoPath {
				return out[i].TicketNumber < out[j].TicketNumber
			}

			return out[i].RepoPath < out[j].RepoPath
		}

		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})

	return out
}

// NewJob creates a new queued job record and persists it.
func (s *Store) NewJob(action, repoID, repoPath, ticketNumber, scope string) (JobRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, err := randomID()
	if err != nil {
		return JobRecord{}, err
	}
	now := time.Now().UTC()
	rec := JobRecord{
		ID:           id,
		Action:       action,
		RepoID:       repoID,
		RepoPath:     repoPath,
		TicketNumber: ticketNumber,
		Scope:        scope,
		Status:       "queued",
		CreatedAt:    now,
	}
	s.data.Jobs[id] = rec
	err = s.saveLocked()
	if err != nil {
		return JobRecord{}, err
	}

	return rec, nil
}

// UpdateJobStatus sets the job's status and records the appropriate timestamp.
func (s *Store) UpdateJobStatus(id, status, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.data.Jobs[id]
	if !ok {
		return fmt.Errorf("job %s: %w", id, ErrJobNotFound)
	}
	now := time.Now().UTC()
	switch status {
	case "running":
		rec.StartedAt = &now
	case "done", "failed":
		rec.FinishedAt = &now
	}
	rec.Status = status
	rec.Error = strings.TrimSpace(errMsg)
	s.data.Jobs[id] = rec

	return s.saveLocked()
}

// GetJob returns the job record for id, and whether it was found.
func (s *Store) GetJob(id string) (JobRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.data.Jobs[id]

	return rec, ok
}

// ListRepos returns all known repository records, sorted by most recently updated.
func (s *Store) ListRepos() []RepoRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]RepoRecord, 0, len(s.data.Repos))
	for _, rec := range s.data.Repos {
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].Path < out[j].Path
		}

		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})

	return out
}

func (s *Store) load() error {
	err := os.MkdirAll(filepath.Dir(s.path), 0o755) //nolint:gosec,mnd // G301: 0755 correct for server meta directory
	if err != nil {
		return fmt.Errorf("create server meta dir: %w", err)
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s.saveLocked()
		}

		return fmt.Errorf("read server meta: %w", err)
	}
	var parsed Data
	err = json.Unmarshal(data, &parsed)
	if err != nil {
		return fmt.Errorf("parse server state: %w", err)
	}
	if parsed.Repos == nil {
		parsed.Repos = map[string]RepoRecord{}
	}
	if parsed.Tickets == nil {
		parsed.Tickets = map[string]TicketRecord{}
	}
	if parsed.Jobs == nil {
		parsed.Jobs = map[string]JobRecord{}
	}
	s.data = parsed

	return nil
}

func (s *Store) saveLocked() error {
	err := os.MkdirAll(filepath.Dir(s.path), 0o755) //nolint:gosec,mnd // G301: 0755 correct for server meta directory
	if err != nil {
		return fmt.Errorf("create server meta dir: %w", err)
	}
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal server meta: %w", err)
	}
	tmp := s.path + ".tmp"
	err = os.WriteFile(tmp, data, 0o644) //nolint:gosec,mnd // G306: 0644 intentional for user-readable server meta file
	if err != nil {
		return fmt.Errorf("write server meta tmp: %w", err)
	}
	err = os.Rename(tmp, s.path)
	if err != nil {
		return fmt.Errorf("rename server meta: %w", err)
	}

	return nil
}

func repoID(repoPath string) string {
	sum := sha1.Sum([]byte(strings.ToLower(filepath.Clean(repoPath)))) //nolint:gosec // G401: sha1 for directory naming, not a security primitive

	return hex.EncodeToString(sum[:])
}

func ticketKey(repoID, ticketNumber string) string {
	return repoID + "::" + ticketNumber
}

func randomID() (string, error) {
	var randBytes [12]byte
	_, err := rand.Read(randBytes[:])
	if err != nil {
		return "", fmt.Errorf("rand read: %w", err)
	}

	return hex.EncodeToString(randBytes[:]), nil
}
