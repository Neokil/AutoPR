package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"ai-ticket-worker/internal/application/orchestrator"
	"ai-ticket-worker/internal/config"
	"ai-ticket-worker/internal/gitutil"
	"ai-ticket-worker/internal/providers"
	"ai-ticket-worker/internal/servermeta"
	"ai-ticket-worker/internal/state"
)

type repoRuntime struct {
	svc      orchestrator.Service
	repoRoot string
	store    *state.Store
}

type server struct {
	cfg      config.Config
	meta     *servermeta.Store
	runtimes map[string]*repoRuntime
	mu       sync.Mutex
}

type repoRequest struct {
	RepoPath string `json:"repo_path"`
}

type feedbackRequest struct {
	RepoPath string `json:"repo_path"`
	Message  string `json:"message"`
}

type cleanupScopeRequest struct {
	RepoPath string `json:"repo_path"`
	Scope    string `json:"scope"`
}

type ticketDetails struct {
	RepoID       string      `json:"repo_id"`
	RepoPath     string      `json:"repo_path"`
	TicketNumber string      `json:"ticket_number"`
	State        interface{} `json:"state"`
	Ticket       interface{} `json:"ticket,omitempty"`
	NextSteps    string      `json:"next_steps,omitempty"`
}

type logEvent struct {
	Title     string `json:"title"`
	Timestamp string `json:"timestamp"`
	Body      string `json:"body"`
}

var sectionHeaderRE = regexp.MustCompile(`^## (.+) \(([^)]+)\)$`)

func main() {
	portFlag := flag.Int("port", 0, "HTTP port override (default uses config server_port)")
	flag.Parse()

	cfg, err := config.Load()
	fatalIf(err)

	metaPath, err := servermeta.DefaultPath()
	fatalIf(err)
	meta, err := servermeta.NewStore(metaPath)
	fatalIf(err)

	s := &server{
		cfg:      cfg,
		meta:     meta,
		runtimes: map[string]*repoRuntime{},
	}

	port := cfg.ServerPort
	if *portFlag > 0 {
		port = *portFlag
	}
	if port <= 0 {
		port = 9000
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/tickets", s.handleListTickets)
	mux.HandleFunc("GET /api/tickets/{id}", s.handleGetTicket)
	mux.HandleFunc("GET /api/tickets/{id}/events", s.handleTicketEvents)
	mux.HandleFunc("GET /api/tickets/{id}/artifacts/{name}", s.handleTicketArtifact)
	mux.HandleFunc("POST /api/tickets/{id}/run", s.handleRunTicket)
	mux.HandleFunc("POST /api/tickets/{id}/resume", s.handleResumeTicket)
	mux.HandleFunc("POST /api/tickets/{id}/approve", s.handleApproveTicket)
	mux.HandleFunc("POST /api/tickets/{id}/reject", s.handleRejectTicket)
	mux.HandleFunc("POST /api/tickets/{id}/feedback", s.handleFeedbackTicket)
	mux.HandleFunc("POST /api/tickets/{id}/cleanup", s.handleCleanupTicket)
	mux.HandleFunc("POST /api/cleanup", s.handleCleanupScope)

	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("orchestratord listening on %s\n", addr)
	fatalIf(http.ListenAndServe(addr, mux))
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":       "ok",
		"server_state": "~/.ai-orchestrator/server/state.json",
	})
}

func (s *server) handleRunTicket(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	repoRoot, repoID, rt, ok := s.repoRuntimeFromBody(w, r)
	if !ok {
		return
	}
	err := rt.svc.RunTickets(r.Context(), []string{ticket})
	_ = s.syncTicketFromRepo(repoID, repoRoot, ticket, rt)
	s.respondAction(w, repoID, repoRoot, ticket, rt, err)
}

func (s *server) handleResumeTicket(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	repoRoot, repoID, rt, ok := s.repoRuntimeFromBody(w, r)
	if !ok {
		return
	}
	err := rt.svc.ResumeTicket(r.Context(), ticket)
	_ = s.syncTicketFromRepo(repoID, repoRoot, ticket, rt)
	s.respondAction(w, repoID, repoRoot, ticket, rt, err)
}

func (s *server) handleApproveTicket(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	repoRoot, repoID, rt, ok := s.repoRuntimeFromBody(w, r)
	if !ok {
		return
	}
	err := rt.svc.Approve(r.Context(), ticket)
	_ = s.syncTicketFromRepo(repoID, repoRoot, ticket, rt)
	s.respondAction(w, repoID, repoRoot, ticket, rt, err)
}

func (s *server) handleRejectTicket(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	repoRoot, repoID, rt, ok := s.repoRuntimeFromBody(w, r)
	if !ok {
		return
	}
	err := rt.svc.Reject(ticket)
	_ = s.syncTicketFromRepo(repoID, repoRoot, ticket, rt)
	s.respondAction(w, repoID, repoRoot, ticket, rt, err)
}

func (s *server) handleFeedbackTicket(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	var req feedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if strings.TrimSpace(req.RepoPath) == "" {
		writeError(w, http.StatusBadRequest, "repo_path is required")
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}
	repoRoot, repoID, rt, err := s.runtimeForRepoPath(req.RepoPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	err = rt.svc.Feedback(ticket, req.Message)
	_ = s.syncTicketFromRepo(repoID, repoRoot, ticket, rt)
	s.respondAction(w, repoID, repoRoot, ticket, rt, err)
}

func (s *server) handleCleanupTicket(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	repoRoot, repoID, rt, ok := s.repoRuntimeFromBody(w, r)
	if !ok {
		return
	}
	err := rt.svc.CleanupTicket(r.Context(), ticket)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = s.meta.DeleteTicket(repoID, ticket)
	writeJSON(w, http.StatusOK, map[string]string{
		"repo_id":       repoID,
		"repo_path":     repoRoot,
		"ticket_number": ticket,
		"status":        "cleaned",
	})
}

func (s *server) handleCleanupScope(w http.ResponseWriter, r *http.Request) {
	var req cleanupScopeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	scope := strings.TrimSpace(req.Scope)
	if scope == "" {
		scope = strings.TrimSpace(r.URL.Query().Get("scope"))
	}
	if strings.TrimSpace(req.RepoPath) == "" {
		writeError(w, http.StatusBadRequest, "repo_path is required")
		return
	}

	repoRoot, repoID, rt, err := s.runtimeForRepoPath(req.RepoPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	switch scope {
	case "done":
		err = rt.svc.CleanupDone(r.Context())
	case "all":
		err = rt.svc.CleanupAll(r.Context())
	default:
		writeError(w, http.StatusBadRequest, "scope must be 'done' or 'all'")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.syncRepoTickets(repoID, repoRoot, rt); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "ok",
		"scope":     scope,
		"repo_id":   repoID,
		"repo_path": repoRoot,
	})
}

func (s *server) handleListTickets(w http.ResponseWriter, r *http.Request) {
	repoPath := strings.TrimSpace(r.URL.Query().Get("repo_path"))
	if repoPath == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"tickets": s.meta.ListTickets(""),
		})
		return
	}

	repoRoot, repoID, rt, err := s.runtimeForRepoPath(repoPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.syncRepoTickets(repoID, repoRoot, rt); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"repo_id":   repoID,
		"repo_path": repoRoot,
		"tickets":   s.meta.ListTickets(repoID),
	})
}

func (s *server) handleGetTicket(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	repoPath := strings.TrimSpace(r.URL.Query().Get("repo_path"))
	if repoPath == "" {
		writeError(w, http.StatusBadRequest, "repo_path query param is required")
		return
	}
	repoRoot, repoID, rt, err := s.runtimeForRepoPath(repoPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.syncTicketFromRepo(repoID, repoRoot, ticket, rt); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "ticket not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	st, err := rt.store.LoadState(ticket)
	if err != nil {
		writeError(w, http.StatusNotFound, "ticket not found")
		return
	}
	t, err := rt.store.LoadTicket(ticket)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	nextSteps, _ := rt.svc.NextSteps(ticket)
	resp := ticketDetails{
		RepoID:       repoID,
		RepoPath:     repoRoot,
		TicketNumber: ticket,
		State:        st,
		NextSteps:    nextSteps,
	}
	if err == nil {
		resp.Ticket = t
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) handleTicketEvents(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	repoPath := strings.TrimSpace(r.URL.Query().Get("repo_path"))
	if repoPath == "" {
		writeError(w, http.StatusBadRequest, "repo_path query param is required")
		return
	}
	repoRoot, repoID, rt, err := s.runtimeForRepoPath(repoPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	paths := rt.store.Paths(ticket)
	events, err := parseLogEvents(paths["log"])
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"repo_id":       repoID,
				"repo_path":     repoRoot,
				"ticket_number": ticket,
				"events":        []logEvent{},
			})
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"repo_id":       repoID,
		"repo_path":     repoRoot,
		"ticket_number": ticket,
		"events":        events,
	})
}

func (s *server) handleTicketArtifact(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	name := r.PathValue("name")
	repoPath := strings.TrimSpace(r.URL.Query().Get("repo_path"))
	if repoPath == "" {
		writeError(w, http.StatusBadRequest, "repo_path query param is required")
		return
	}
	repoRoot, repoID, rt, err := s.runtimeForRepoPath(repoPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	path, ok := artifactPath(rt.store.Paths(ticket), name)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown artifact")
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "artifact not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"repo_id":       repoID,
		"repo_path":     repoRoot,
		"ticket_number": ticket,
		"name":          name,
		"path":          path,
		"content":       string(data),
	})
}

func (s *server) repoRuntimeFromBody(w http.ResponseWriter, r *http.Request) (repoRoot, repoID string, rt *repoRuntime, ok bool) {
	var req repoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return "", "", nil, false
	}
	if strings.TrimSpace(req.RepoPath) == "" {
		writeError(w, http.StatusBadRequest, "repo_path is required")
		return "", "", nil, false
	}
	root, id, runtime, err := s.runtimeForRepoPath(req.RepoPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return "", "", nil, false
	}
	return root, id, runtime, true
}

func (s *server) runtimeForRepoPath(repoPath string) (repoRoot, repoID string, rt *repoRuntime, err error) {
	repoRoot, err = resolveRepoRoot(repoPath)
	if err != nil {
		return "", "", nil, err
	}
	repoRec, err := s.meta.UpsertRepo(repoRoot)
	if err != nil {
		return "", "", nil, err
	}
	rt, err = s.runtimeForRepo(repoRoot)
	if err != nil {
		return "", "", nil, err
	}
	return repoRoot, repoRec.ID, rt, nil
}

func (s *server) runtimeForRepo(repoRoot string) (*repoRuntime, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rt, ok := s.runtimes[repoRoot]; ok {
		return rt, nil
	}
	provider, err := providers.NewFromConfig(s.cfg)
	if err != nil {
		return nil, err
	}
	rt := &repoRuntime{
		svc:      orchestrator.NewWorkflowService(s.cfg, repoRoot, provider),
		repoRoot: repoRoot,
		store:    state.NewStore(repoRoot, s.cfg.StateDirName),
	}
	s.runtimes[repoRoot] = rt
	return rt, nil
}

func (s *server) respondAction(w http.ResponseWriter, repoID, repoRoot, ticket string, rt *repoRuntime, err error) {
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	nextSteps, _ := rt.svc.NextSteps(ticket)
	writeJSON(w, http.StatusOK, map[string]string{
		"repo_id":       repoID,
		"repo_path":     repoRoot,
		"ticket_number": ticket,
		"status":        "ok",
		"next_steps":    nextSteps,
	})
}

func (s *server) syncTicketFromRepo(repoID, repoRoot, ticket string, rt *repoRuntime) error {
	st, err := rt.store.LoadState(ticket)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s.meta.DeleteTicket(repoID, ticket)
		}
		return err
	}
	t, _ := rt.store.LoadTicket(ticket)
	rec := servermeta.TicketRecord{
		RepoID:       repoID,
		RepoPath:     repoRoot,
		TicketNumber: ticket,
		Title:        strings.TrimSpace(t.Title),
		Status:       string(st.Status),
		Approved:     st.Approved,
		UpdatedAt:    st.UpdatedAt.UTC(),
		PRURL:        st.PRURL,
	}
	return s.meta.UpsertTicket(rec)
}

func (s *server) syncRepoTickets(repoID, repoRoot string, rt *repoRuntime) error {
	tickets, err := rt.store.ListTicketDirs()
	if err != nil {
		return err
	}
	records := make([]servermeta.TicketRecord, 0, len(tickets))
	for _, t := range tickets {
		st, err := rt.store.LoadState(t)
		if err != nil {
			continue
		}
		ticketData, _ := rt.store.LoadTicket(t)
		records = append(records, servermeta.TicketRecord{
			RepoID:       repoID,
			RepoPath:     repoRoot,
			TicketNumber: t,
			Title:        strings.TrimSpace(ticketData.Title),
			Status:       string(st.Status),
			Approved:     st.Approved,
			UpdatedAt:    st.UpdatedAt.UTC(),
			PRURL:        st.PRURL,
		})
	}
	return s.meta.ReplaceRepoTickets(repoID, records)
}

func resolveRepoRoot(repoPath string) (string, error) {
	if strings.TrimSpace(repoPath) == "" {
		return "", errors.New("repo_path is empty")
	}
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return "", fmt.Errorf("resolve repo_path: %w", err)
	}
	root, err := gitutil.RepoRoot(context.Background(), abs)
	if err != nil {
		return "", fmt.Errorf("repo_path is not a git repository: %w", err)
	}
	return root, nil
}

func parseLogEvents(path string) ([]logEvent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	events := make([]logEvent, 0)
	cur := logEvent{}
	bodyLines := make([]string, 0)
	flush := func() {
		if strings.TrimSpace(cur.Title) == "" {
			return
		}
		cur.Body = strings.TrimSpace(strings.Join(bodyLines, "\n"))
		events = append(events, cur)
	}
	for _, line := range lines {
		if m := sectionHeaderRE.FindStringSubmatch(line); len(m) == 3 {
			flush()
			cur = logEvent{Title: strings.TrimSpace(m[1]), Timestamp: strings.TrimSpace(m[2])}
			bodyLines = bodyLines[:0]
			continue
		}
		bodyLines = append(bodyLines, line)
	}
	flush()
	return events, nil
}

func artifactPath(paths map[string]string, name string) (string, bool) {
	switch name {
	case "state":
		return paths["state"], true
	case "ticket":
		return paths["ticket"], true
	case "log":
		return paths["log"], true
	case "proposal":
		return paths["proposal"], true
	case "final":
		return paths["final"], true
	case "pr":
		return paths["pr"], true
	case "checks":
		return paths["checks"], true
	default:
		return "", false
	}
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fatalIf(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
