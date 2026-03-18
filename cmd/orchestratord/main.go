package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"ai-ticket-worker/internal/application/orchestrator"
	"ai-ticket-worker/internal/config"
	"ai-ticket-worker/internal/contracts/api"
	"ai-ticket-worker/internal/gitutil"
	"ai-ticket-worker/internal/providers"
	"ai-ticket-worker/internal/servermeta"
	"ai-ticket-worker/internal/state"
	"ai-ticket-worker/web"
)

const (
	jobRun         = "run"
	jobResume      = "resume"
	jobApprove     = "approve"
	jobReject      = "reject"
	jobFeedback    = "feedback"
	jobPR          = "pr"
	jobCleanup     = "cleanup_ticket"
	jobCleanupDone = "cleanup_done"
	jobCleanupAll  = "cleanup_all"
)

type repoRuntime struct {
	svc      orchestrator.Service
	repoRoot string
	store    *state.Store
}

type queuedJob struct {
	record  servermeta.JobRecord
	message string
}

type server struct {
	cfg      config.Config
	meta     *servermeta.Store
	runtimes map[string]*repoRuntime
	mu       sync.Mutex
	jobs     chan queuedJob
	webFS    fs.FS

	subsMu      sync.Mutex
	subscribers map[string]chan serverEvent

	repoLockMu sync.Mutex
	repoLocks  map[string]*sync.RWMutex

	ticketLockMu sync.Mutex
	ticketLocks  map[string]*sync.Mutex
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

type serverEvent struct {
	Type         string `json:"type"`
	RepoID       string `json:"repo_id,omitempty"`
	RepoPath     string `json:"repo_path,omitempty"`
	TicketNumber string `json:"ticket_number,omitempty"`
	Status       string `json:"status,omitempty"`
	JobID        string `json:"job_id,omitempty"`
	Action       string `json:"action,omitempty"`
	Scope        string `json:"scope,omitempty"`
	Error        string `json:"error,omitempty"`
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
	distFS, err := web.Dist()
	fatalIf(err)

	s := &server{
		cfg:         cfg,
		meta:        meta,
		runtimes:    map[string]*repoRuntime{},
		jobs:        make(chan queuedJob, 256),
		repoLocks:   map[string]*sync.RWMutex{},
		ticketLocks: map[string]*sync.Mutex{},
		webFS:       distFS,
		subscribers: map[string]chan serverEvent{},
	}
	for i := 0; i < cfg.ServerWorkers; i++ {
		go s.workerLoop()
	}

	port := cfg.ServerPort
	if *portFlag > 0 {
		port = *portFlag
	}
	if port <= 0 {
		port = 8080
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/tickets", s.handleListTickets)
	mux.HandleFunc("GET /api/tickets/{id}", s.handleGetTicket)
	mux.HandleFunc("GET /api/tickets/{id}/events", s.handleTicketEvents)
	mux.HandleFunc("GET /api/tickets/{id}/artifacts/{name}", s.handleTicketArtifact)
	mux.HandleFunc("GET /api/jobs/{id}", s.handleGetJob)
	mux.HandleFunc("GET /api/events", s.handleEvents)
	mux.HandleFunc("POST /api/tickets/{id}/run", s.handleRunTicket)
	mux.HandleFunc("POST /api/tickets/{id}/resume", s.handleResumeTicket)
	mux.HandleFunc("POST /api/tickets/{id}/approve", s.handleApproveTicket)
	mux.HandleFunc("POST /api/tickets/{id}/reject", s.handleRejectTicket)
	mux.HandleFunc("POST /api/tickets/{id}/feedback", s.handleFeedbackTicket)
	mux.HandleFunc("POST /api/tickets/{id}/pr", s.handlePRTicket)
	mux.HandleFunc("POST /api/tickets/{id}/cleanup", s.handleCleanupTicket)
	mux.HandleFunc("POST /api/cleanup", s.handleCleanupScope)
	mux.HandleFunc("/", s.handleFrontend)

	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("orchestratord listening on %s\n", addr)
	fatalIf(http.ListenAndServe(addr, loggingMiddleware(mux)))
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":       "ok",
		"server_state": "~/.ai-orchestrator/server/state.json",
		"queue_depth":  len(s.jobs),
		"frontend":     "embedded",
	})
}

func (s *server) handleFrontend(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	requestPath := filepath.ToSlash(filepath.Clean("/" + r.URL.Path))
	rel := strings.TrimPrefix(requestPath, "/")
	if rel == "" {
		rel = "index.html"
	}
	if s.serveEmbeddedFile(w, r, rel) {
		return
	}
	// SPA fallback for client-side routes.
	if s.serveEmbeddedFile(w, r, "index.html") {
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_, _ = fmt.Fprint(w, "embedded frontend index.html not found")
}

func (s *server) handleRunTicket(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	repoRoot, repoID, _, ok := s.repoRuntimeFromBody(w, r)
	if !ok {
		return
	}
	s.enqueueAndRespond(w, jobRun, repoID, repoRoot, ticket, "", "")
}

func (s *server) handleResumeTicket(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	repoRoot, repoID, _, ok := s.repoRuntimeFromBody(w, r)
	if !ok {
		return
	}
	s.enqueueAndRespond(w, jobResume, repoID, repoRoot, ticket, "", "")
}

func (s *server) handleApproveTicket(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	repoRoot, repoID, _, ok := s.repoRuntimeFromBody(w, r)
	if !ok {
		return
	}
	s.enqueueAndRespond(w, jobApprove, repoID, repoRoot, ticket, "", "")
}

func (s *server) handleRejectTicket(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	repoRoot, repoID, _, ok := s.repoRuntimeFromBody(w, r)
	if !ok {
		return
	}
	s.enqueueAndRespond(w, jobReject, repoID, repoRoot, ticket, "", "")
}

func (s *server) handleFeedbackTicket(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	var req api.FeedbackRequest
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
	repoRoot, repoID, _, err := s.runtimeForRepoPath(req.RepoPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.enqueueAndRespond(w, jobFeedback, repoID, repoRoot, ticket, req.Message, "")
}

func (s *server) handleCleanupTicket(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	repoRoot, repoID, _, ok := s.repoRuntimeFromBody(w, r)
	if !ok {
		return
	}
	s.enqueueAndRespond(w, jobCleanup, repoID, repoRoot, ticket, "", "")
}

func (s *server) handlePRTicket(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	repoRoot, repoID, _, ok := s.repoRuntimeFromBody(w, r)
	if !ok {
		return
	}
	s.enqueueAndRespond(w, jobPR, repoID, repoRoot, ticket, "", "")
}

func (s *server) handleCleanupScope(w http.ResponseWriter, r *http.Request) {
	var req api.CleanupScopeRequest
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
	repoRoot, repoID, _, err := s.runtimeForRepoPath(req.RepoPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	switch scope {
	case "done":
		s.enqueueAndRespond(w, jobCleanupDone, repoID, repoRoot, "", "", scope)
	case "all":
		s.enqueueAndRespond(w, jobCleanupAll, repoID, repoRoot, "", "", scope)
	default:
		writeError(w, http.StatusBadRequest, "scope must be 'done' or 'all'")
	}
}

func (s *server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, ok := s.meta.GetJob(id)
	if !ok {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	subID, ch := s.addSubscriber()
	defer s.removeSubscriber(subID)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	keepAlive := time.NewTicker(20 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepAlive.C:
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case evt := <-ch:
			data, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, data)
			flusher.Flush()
		}
	}
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
	var req api.RepoRequest
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

func (s *server) enqueueAndRespond(w http.ResponseWriter, action, repoID, repoPath, ticket, message, scope string) {
	job, err := s.meta.NewJob(action, repoID, repoPath, ticket, scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	qj := queuedJob{record: job, message: message}
	s.broadcast(serverEvent{
		Type:         "job",
		RepoID:       repoID,
		RepoPath:     repoPath,
		TicketNumber: ticket,
		JobID:        job.ID,
		Action:       action,
		Scope:        scope,
		Status:       "queued",
	})
	select {
	case s.jobs <- qj:
		writeJSON(w, http.StatusAccepted, api.ActionAcceptedResponse{
			Status:       "accepted",
			JobID:        job.ID,
			Action:       action,
			RepoID:       repoID,
			RepoPath:     repoPath,
			TicketNumber: ticket,
		})
	default:
		_ = s.meta.UpdateJobStatus(job.ID, "failed", "job queue is full")
		s.broadcast(serverEvent{
			Type:         "job",
			RepoID:       repoID,
			RepoPath:     repoPath,
			TicketNumber: ticket,
			JobID:        job.ID,
			Action:       action,
			Scope:        scope,
			Status:       "failed",
			Error:        "job queue is full",
		})
		writeError(w, http.StatusServiceUnavailable, "job queue is full")
	}
}

func (s *server) workerLoop() {
	for job := range s.jobs {
		s.setJobStatus(job.record, "running", "")
		err := s.executeJob(job)
		if err != nil {
			s.setJobStatus(job.record, "failed", err.Error())
			continue
		}
		s.setJobStatus(job.record, "done", "")
	}
}

func (s *server) setJobStatus(job servermeta.JobRecord, status, errMsg string) {
	_ = s.meta.UpdateJobStatus(job.ID, status, errMsg)
	s.broadcast(serverEvent{
		Type:         "job",
		RepoID:       job.RepoID,
		RepoPath:     job.RepoPath,
		TicketNumber: job.TicketNumber,
		JobID:        job.ID,
		Action:       job.Action,
		Scope:        job.Scope,
		Status:       status,
		Error:        strings.TrimSpace(errMsg),
	})
}

func (s *server) executeJob(job queuedJob) error {
	repoRoot, repoID := job.record.RepoPath, job.record.RepoID
	ticket := job.record.TicketNumber

	repoMu := s.getRepoLock(repoID)
	switch job.record.Action {
	case jobCleanupDone, jobCleanupAll:
		repoMu.Lock()
		defer repoMu.Unlock()
	default:
		repoMu.RLock()
		defer repoMu.RUnlock()
		if ticket != "" {
			ticketMu := s.getTicketLock(repoID, ticket)
			ticketMu.Lock()
			defer ticketMu.Unlock()
		}
	}

	rt, err := s.runtimeForRepo(repoRoot)
	if err != nil {
		return err
	}

	switch job.record.Action {
	case jobRun:
		err = rt.svc.RunTickets(context.Background(), []string{ticket})
		if err == nil {
			err = s.syncTicketFromRepo(repoID, repoRoot, ticket, rt)
		}
	case jobResume:
		err = rt.svc.ResumeTicket(context.Background(), ticket)
		if err == nil {
			err = s.syncTicketFromRepo(repoID, repoRoot, ticket, rt)
		}
	case jobApprove:
		err = rt.svc.Approve(context.Background(), ticket)
		if err == nil {
			err = s.syncTicketFromRepo(repoID, repoRoot, ticket, rt)
		}
	case jobReject:
		err = rt.svc.Reject(ticket)
		if err == nil {
			err = s.syncTicketFromRepo(repoID, repoRoot, ticket, rt)
		}
	case jobFeedback:
		err = rt.svc.Feedback(ticket, job.message)
		if err == nil {
			err = s.syncTicketFromRepo(repoID, repoRoot, ticket, rt)
		}
	case jobCleanup:
		err = rt.svc.CleanupTicket(context.Background(), ticket)
		if err == nil {
			err = s.meta.DeleteTicket(repoID, ticket)
		}
	case jobPR:
		err = rt.svc.GeneratePR(context.Background(), ticket)
		if err == nil {
			err = s.syncTicketFromRepo(repoID, repoRoot, ticket, rt)
		}
	case jobCleanupDone:
		err = rt.svc.CleanupDone(context.Background())
		if err == nil {
			err = s.syncRepoTickets(repoID, repoRoot, rt)
		}
	case jobCleanupAll:
		err = rt.svc.CleanupAll(context.Background())
		if err == nil {
			err = s.syncRepoTickets(repoID, repoRoot, rt)
		}
	default:
		err = fmt.Errorf("unsupported job action: %s", job.record.Action)
	}
	return err
}

func (s *server) getRepoLock(repoID string) *sync.RWMutex {
	s.repoLockMu.Lock()
	defer s.repoLockMu.Unlock()
	if m, ok := s.repoLocks[repoID]; ok {
		return m
	}
	m := &sync.RWMutex{}
	s.repoLocks[repoID] = m
	return m
}

func (s *server) getTicketLock(repoID, ticket string) *sync.Mutex {
	key := repoID + "::" + ticket
	s.ticketLockMu.Lock()
	defer s.ticketLockMu.Unlock()
	if m, ok := s.ticketLocks[key]; ok {
		return m
	}
	m := &sync.Mutex{}
	s.ticketLocks[key] = m
	return m
}

func (s *server) syncTicketFromRepo(repoID, repoRoot, ticket string, rt *repoRuntime) error {
	st, err := rt.store.LoadState(ticket)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := s.meta.DeleteTicket(repoID, ticket); err != nil {
				return err
			}
			s.broadcast(serverEvent{
				Type:         "ticket_deleted",
				RepoID:       repoID,
				RepoPath:     repoRoot,
				TicketNumber: ticket,
			})
			return nil
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
	if err := s.meta.UpsertTicket(rec); err != nil {
		return err
	}
	s.broadcast(serverEvent{
		Type:         "ticket_updated",
		RepoID:       repoID,
		RepoPath:     repoRoot,
		TicketNumber: ticket,
		Status:       rec.Status,
	})
	return nil
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
	if err := s.meta.ReplaceRepoTickets(repoID, records); err != nil {
		return err
	}
	s.broadcast(serverEvent{
		Type:     "repo_tickets_synced",
		RepoID:   repoID,
		RepoPath: repoRoot,
	})
	return nil
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

func (s *server) serveEmbeddedFile(w http.ResponseWriter, r *http.Request, rel string) bool {
	if strings.Contains(rel, "..") {
		return false
	}
	data, err := fs.ReadFile(s.webFS, rel)
	if err != nil {
		return false
	}
	ext := filepath.Ext(rel)
	if ct := mime.TypeByExtension(ext); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
	return true
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, api.ErrorResponse{Error: msg})
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

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		fmt.Printf("%s %s -> %d (%s)\n", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}

func (s *server) addSubscriber() (string, chan serverEvent) {
	id := fmt.Sprintf("sub-%d", time.Now().UnixNano())
	ch := make(chan serverEvent, 128)
	s.subsMu.Lock()
	s.subscribers[id] = ch
	s.subsMu.Unlock()
	return id, ch
}

func (s *server) removeSubscriber(id string) {
	s.subsMu.Lock()
	ch, ok := s.subscribers[id]
	if ok {
		delete(s.subscribers, id)
	}
	s.subsMu.Unlock()
	if ok {
		close(ch)
	}
}

func (s *server) broadcast(evt serverEvent) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	for _, ch := range s.subscribers {
		select {
		case ch <- evt:
		default:
		}
	}
}
