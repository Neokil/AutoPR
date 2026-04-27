package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Neokil/AutoPR/internal/application/tickets"
	"github.com/Neokil/AutoPR/internal/config"
	"github.com/Neokil/AutoPR/internal/contracts/api"
	ticketdomain "github.com/Neokil/AutoPR/internal/domain/ticket"
	"github.com/Neokil/AutoPR/internal/gitutil"
	"github.com/Neokil/AutoPR/internal/servermeta"
	"github.com/Neokil/AutoPR/internal/state"
	"github.com/Neokil/AutoPR/internal/workflow"
	"github.com/Neokil/AutoPR/web"
)

const (
	jobRun         = "run"
	jobAction      = "action"
	jobMoveToState = "move_to_state"
	jobCleanup     = "cleanup_ticket"
	jobCleanupDone = "cleanup_done"
	jobCleanupAll  = "cleanup_all"

	jobQueueSize          = 256
	httpReadHeaderTimeout = 30 * time.Second
	sectionMatchLen       = 3 // full match + 2 capture groups
)

type repoRuntime struct {
	svc      *tickets.Orchestrator
	repoRoot string
	store    *state.Store
}

type queuedJob struct {
	record      servermeta.JobRecord
	message     string
	actionLabel string // used by jobAction
	targetState string // used by jobMoveToState
}

type enqueueOptions struct {
	message     string
	scope       string
	actionLabel string
	targetState string
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

var sectionHeaderRE = regexp.MustCompile(`^## (.+) \(([^)]+)\)$`)
var githubPRURLPattern = regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+)/pull/([0-9]+)`)

func main() {
	portFlag := flag.Int("port", 0, "HTTP port override (default uses config server_port)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	metaPath, err := servermeta.DefaultPath()
	if err != nil {
		slog.Error("resolve server meta path", "err", err)
		os.Exit(1)
	}
	meta, err := servermeta.NewStore(metaPath)
	if err != nil {
		slog.Error("open server meta store", "err", err)
		os.Exit(1)
	}
	distFS, err := web.Dist()
	if err != nil {
		slog.Error("load web assets", "err", err)
		os.Exit(1)
	}

	daemon := &server{
		cfg:         cfg,
		meta:        meta,
		runtimes:    map[string]*repoRuntime{},
		jobs:        make(chan queuedJob, jobQueueSize),
		repoLocks:   map[string]*sync.RWMutex{},
		ticketLocks: map[string]*sync.Mutex{},
		webFS:       distFS,
		subscribers: map[string]chan serverEvent{},
	}
	for range cfg.ServerWorkers {
		go daemon.workerLoop()
	}
	go daemon.prMonitorLoop()

	port := cfg.ServerPort
	if *portFlag > 0 {
		port = *portFlag
	}
	if port <= 0 {
		port = 8080
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", daemon.handleHealth)
	mux.HandleFunc("GET /api/repositories", daemon.handleListRepositories)
	mux.HandleFunc("GET /api/tickets", daemon.handleListTickets)
	mux.HandleFunc("GET /api/tickets/{id}", daemon.handleGetTicket)
	mux.HandleFunc("GET /api/tickets/{id}/events", daemon.handleTicketEvents)
	mux.HandleFunc("GET /api/tickets/{id}/artifacts/{name...}", daemon.handleTicketArtifact)
	mux.HandleFunc("GET /api/tickets/{id}/execution-logs", daemon.handleExecutionLogs)
	mux.HandleFunc("GET /api/jobs/{id}", daemon.handleGetJob)
	mux.HandleFunc("GET /api/events", daemon.handleEvents)
	mux.HandleFunc("POST /api/tickets/{id}/run", daemon.handleRunTicket)
	mux.HandleFunc("POST /api/tickets/{id}/action", daemon.handleActionTicket)
	mux.HandleFunc("POST /api/tickets/{id}/move-to-state", daemon.handleMoveToStateTicket)
	mux.HandleFunc("POST /api/tickets/{id}/cleanup", daemon.handleCleanupTicket)
	mux.HandleFunc("POST /api/cleanup", daemon.handleCleanupScope)
	mux.HandleFunc("/", daemon.handleFrontend)

	addr := fmt.Sprintf(":%d", port)
	slog.Info("AutoPR daemon listening", "addr", addr)
	srv := &http.Server{
		Addr:              addr,
		Handler:           loggingMiddleware(mux),
		ReadHeaderTimeout: httpReadHeaderTimeout,
	}
	err = srv.ListenAndServe()
	if err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

func (s *server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "ok",
		"server_state": "~/.auto-pr/server/state.json",
		"queue_depth":  len(s.jobs),
		"frontend":     "embedded",
	})
}

func (s *server) handleRunTicket(resp http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	repoRoot, repoID, ok := s.repoRuntimeFromBody(resp, r)
	if !ok {
		return
	}
	s.enqueueAndRespond(resp, jobRun, repoID, repoRoot, ticket, enqueueOptions{})
}

func (s *server) handleActionTicket(resp http.ResponseWriter, httpReq *http.Request) {
	ticket := httpReq.PathValue("id")
	var req api.ActionRequest
	err := json.NewDecoder(httpReq.Body).Decode(&req)
	if err != nil {
		writeError(resp, http.StatusBadRequest, "invalid json body")

		return
	}
	if strings.TrimSpace(req.RepoPath) == "" {
		writeError(resp, http.StatusBadRequest, "repo_path is required")

		return
	}
	if strings.TrimSpace(req.Label) == "" {
		writeError(resp, http.StatusBadRequest, "label is required")

		return
	}
	repoRoot, repoID, _, err := s.runtimeForRepoPath(httpReq.Context(), req.RepoPath)
	if err != nil {
		writeError(resp, http.StatusBadRequest, err.Error())

		return
	}
	s.enqueueAndRespond(resp, jobAction, repoID, repoRoot, ticket, enqueueOptions{
		message:     req.Message,
		actionLabel: req.Label,
	})
}

func (s *server) handleMoveToStateTicket(resp http.ResponseWriter, httpReq *http.Request) {
	ticket := httpReq.PathValue("id")
	var req api.MoveToStateRequest
	err := json.NewDecoder(httpReq.Body).Decode(&req)
	if err != nil {
		writeError(resp, http.StatusBadRequest, "invalid json body")

		return
	}
	if strings.TrimSpace(req.RepoPath) == "" {
		writeError(resp, http.StatusBadRequest, "repo_path is required")

		return
	}
	if strings.TrimSpace(req.Target) == "" {
		writeError(resp, http.StatusBadRequest, "target is required")

		return
	}
	repoRoot, repoID, _, err := s.runtimeForRepoPath(httpReq.Context(), req.RepoPath)
	if err != nil {
		writeError(resp, http.StatusBadRequest, err.Error())

		return
	}
	s.enqueueAndRespond(resp, jobMoveToState, repoID, repoRoot, ticket, enqueueOptions{
		targetState: req.Target,
	})
}

func (s *server) handleCleanupTicket(resp http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	repoRoot, repoID, ok := s.repoRuntimeFromBody(resp, r)
	if !ok {
		return
	}
	s.enqueueAndRespond(resp, jobCleanup, repoID, repoRoot, ticket, enqueueOptions{})
}

func (s *server) handleCleanupScope(resp http.ResponseWriter, httpReq *http.Request) {
	var req api.CleanupScopeRequest
	err := json.NewDecoder(httpReq.Body).Decode(&req)
	if err != nil {
		writeError(resp, http.StatusBadRequest, "invalid json body")

		return
	}
	scope := strings.TrimSpace(req.Scope)
	if scope == "" {
		scope = strings.TrimSpace(httpReq.URL.Query().Get("scope"))
	}
	if strings.TrimSpace(req.RepoPath) == "" {
		writeError(resp, http.StatusBadRequest, "repo_path is required")

		return
	}
	repoRoot, repoID, _, err := s.runtimeForRepoPath(httpReq.Context(), req.RepoPath)
	if err != nil {
		writeError(resp, http.StatusBadRequest, err.Error())

		return
	}

	switch scope {
	case "done":
		s.enqueueAndRespond(resp, jobCleanupDone, repoID, repoRoot, "", enqueueOptions{scope: scope})
	case "all":
		s.enqueueAndRespond(resp, jobCleanupAll, repoID, repoRoot, "", enqueueOptions{scope: scope})
	default:
		writeError(resp, http.StatusBadRequest, "scope must be 'done' or 'all'")
	}
}

func (s *server) handleGetJob(resp http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, ok := s.meta.GetJob(id)
	if !ok {
		writeError(resp, http.StatusNotFound, "job not found")

		return
	}
	writeJSON(resp, http.StatusOK, job)
}

func (s *server) handleListTickets(resp http.ResponseWriter, req *http.Request) {
	repoPath := strings.TrimSpace(req.URL.Query().Get("repo_path"))
	if repoPath == "" {
		writeJSON(resp, http.StatusOK, map[string]any{
			"tickets": s.meta.ListTickets(""),
		})

		return
	}
	repoRoot, repoID, repoRt, err := s.runtimeForRepoPath(req.Context(), repoPath)
	if err != nil {
		writeError(resp, http.StatusBadRequest, err.Error())

		return
	}
	err = s.syncRepoTickets(repoID, repoRoot, repoRt, false)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err.Error())

		return
	}
	writeJSON(resp, http.StatusOK, map[string]any{
		"repo_id":   repoID,
		"repo_path": repoRoot,
		"tickets":   s.meta.ListTickets(repoID),
	})
}

func (s *server) handleListRepositories(resp http.ResponseWriter, _ *http.Request) {
	configured := discoverRepositoriesFromConfig(s.cfg.RepositoryDirs)
	seen := s.meta.ListRepos()
	paths := make([]string, 0, len(configured)+len(seen))
	seenPaths := map[string]struct{}{}
	for _, p := range configured {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if _, ok := seenPaths[abs]; ok {
			continue
		}
		seenPaths[abs] = struct{}{}
		paths = append(paths, abs)
	}
	for _, rec := range seen {
		p := strings.TrimSpace(rec.Path)
		if p == "" {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if _, ok := seenPaths[abs]; ok {
			continue
		}
		seenPaths[abs] = struct{}{}
		paths = append(paths, abs)
	}
	sort.Strings(paths)
	writeJSON(resp, http.StatusOK, map[string]any{
		"repositories": paths,
	})
}

func (s *server) handleGetTicket(resp http.ResponseWriter, httpReq *http.Request) {
	ticket := httpReq.PathValue("id")
	repoPath := strings.TrimSpace(httpReq.URL.Query().Get("repo_path"))
	if repoPath == "" {
		writeError(resp, http.StatusBadRequest, "repo_path query param is required")

		return
	}
	repoRoot, repoID, repoRt, err := s.runtimeForRepoPath(httpReq.Context(), repoPath)
	if err != nil {
		writeError(resp, http.StatusBadRequest, err.Error())

		return
	}
	err = s.syncTicketFromRepo(repoID, repoRoot, ticket, repoRt, false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(resp, http.StatusNotFound, "ticket not found")

			return
		}
		writeError(resp, http.StatusInternalServerError, err.Error())

		return
	}

	ticketState, err := repoRt.store.LoadState(ticket)
	if err != nil {
		writeError(resp, http.StatusNotFound, "ticket not found")

		return
	}
	nextSteps, _ := repoRt.svc.NextSteps(ticket)
	githubBlobBase, _ := gitutil.GitHubBlobBase(httpReq.Context(), repoRoot, s.cfg.BaseBranch)

	var availableActions []actionInfo
	var workflowStates []workflowStateInfo
	wflow, wfErr := workflow.Load(repoRoot)
	if wfErr == nil {
		for _, stateCfg := range wflow.States {
			workflowStates = append(workflowStates, workflowStateInfo{
				Name:        stateCfg.Name,
				DisplayName: stateCfg.TimelineLabel(),
			})
		}
		if ticketState.FlowStatus == ticketdomain.FlowStatusWaiting {
			if stateCfg, ok := wflow.StateByName(ticketState.CurrentState); ok {
				for _, a := range stateCfg.Actions {
					availableActions = append(availableActions, actionInfo{
						Label: a.Label,
						Type:  string(a.Type),
					})
				}
			}
		}
	}
	if availableActions == nil {
		availableActions = []actionInfo{}
	}
	if workflowStates == nil {
		workflowStates = []workflowStateInfo{}
	}

	ticketResp := ticketDetails{
		RepoID:           repoID,
		RepoPath:         repoRoot,
		TicketNumber:     ticket,
		GitHubBlobBase:   githubBlobBase,
		State:            ticketState,
		NextSteps:        nextSteps,
		WorkflowStates:   workflowStates,
		AvailableActions: availableActions,
	}
	writeJSON(resp, http.StatusOK, ticketResp)
}

func (s *server) handleTicketEvents(resp http.ResponseWriter, req *http.Request) {
	ticket := req.PathValue("id")
	repoPath := strings.TrimSpace(req.URL.Query().Get("repo_path"))
	if repoPath == "" {
		writeError(resp, http.StatusBadRequest, "repo_path query param is required")

		return
	}
	repoRoot, repoID, repoRt, err := s.runtimeForRepoPath(req.Context(), repoPath)
	if err != nil {
		writeError(resp, http.StatusBadRequest, err.Error())

		return
	}
	ticketState, stErr := repoRt.store.LoadState(ticket)
	var logPath string
	if stErr == nil && ticketState.WorktreePath != "" && ticketState.CurrentState != "" {
		logPath = ticketState.CurrentRunLogPath()
	}
	events, err := parseLogEvents(logPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(resp, http.StatusOK, map[string]any{
				"repo_id":       repoID,
				"repo_path":     repoRoot,
				"ticket_number": ticket,
				"events":        []logEvent{},
			})

			return
		}
		writeError(resp, http.StatusInternalServerError, err.Error())

		return
	}
	writeJSON(resp, http.StatusOK, map[string]any{
		"repo_id":       repoID,
		"repo_path":     repoRoot,
		"ticket_number": ticket,
		"events":        events,
	})
}

func (s *server) handleTicketArtifact(resp http.ResponseWriter, req *http.Request) {
	ticket := req.PathValue("id")
	name := req.PathValue("name")
	repoPath := strings.TrimSpace(req.URL.Query().Get("repo_path"))
	if repoPath == "" {
		writeError(resp, http.StatusBadRequest, "repo_path query param is required")

		return
	}
	repoRoot, repoID, repoRt, err := s.runtimeForRepoPath(req.Context(), repoPath)
	if err != nil {
		writeError(resp, http.StatusBadRequest, err.Error())

		return
	}
	ticketState, stErr := repoRt.store.LoadState(ticket)
	if stErr != nil {
		writeError(resp, http.StatusNotFound, "ticket not found")

		return
	}
	path, ok := artifactPath(ticketState, filepath.Join(repoRt.store.TicketDir(ticket), state.StateFileName), name)
	if !ok {
		writeError(resp, http.StatusBadRequest, "unknown artifact")

		return
	}
	data, err := os.ReadFile(path) //nolint:gosec // G304: path resolved from trusted internal state
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(resp, http.StatusOK, map[string]string{
				"repo_id":       repoID,
				"repo_path":     repoRoot,
				"ticket_number": ticket,
				"name":          name,
				"path":          path,
				"content":       "",
			})

			return
		}
		writeError(resp, http.StatusInternalServerError, err.Error())

		return
	}
	writeJSON(resp, http.StatusOK, map[string]string{
		"repo_id":       repoID,
		"repo_path":     repoRoot,
		"ticket_number": ticket,
		"name":          name,
		"path":          path,
		"content":       string(data),
	})
}

func (s *server) handleExecutionLogs(resp http.ResponseWriter, req *http.Request) {
	ticket := req.PathValue("id")
	repoPath := strings.TrimSpace(req.URL.Query().Get("repo_path"))
	if repoPath == "" {
		writeError(resp, http.StatusBadRequest, "repo_path query param is required")

		return
	}
	repoRoot, repoID, repoRt, err := s.runtimeForRepoPath(req.Context(), repoPath)
	if err != nil {
		writeError(resp, http.StatusBadRequest, err.Error())

		return
	}
	ticketState, err := repoRt.store.LoadState(ticket)
	if err != nil {
		writeError(resp, http.StatusNotFound, "ticket not found")

		return
	}
	logs := make([]executionLog, 0, len(ticketState.StateHistory))
	for _, run := range ticketState.StateHistory {
		runPath := filepath.ToSlash(filepath.Join("runs", run.ID, "raw-provider.log"))
		content, readErr := os.ReadFile(ticketState.ResolveRef(runPath)) //nolint:gosec // G703: path is constructed from trusted internal state
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			writeError(resp, http.StatusInternalServerError, readErr.Error())

			return
		}
		logs = append(logs, executionLog{
			RunID:            run.ID,
			State:            run.StateName,
			StateDisplayName: run.StateDisplayName,
			Timestamp:        run.StartedAt.Format(time.RFC3339),
			Path:             runPath,
			Content:          string(content),
		})
	}
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].Timestamp < logs[j].Timestamp
	})
	writeJSON(resp, http.StatusOK, map[string]any{
		"repo_id":       repoID,
		"repo_path":     repoRoot,
		"ticket_number": ticket,
		"logs":          logs,
	})
}

func (s *server) enqueueAndRespond(resp http.ResponseWriter, action, repoID, repoPath, ticket string, opts enqueueOptions) {
	if action == jobRun && strings.TrimSpace(ticket) != "" {
		err := s.ensureQueuedTicket(repoID, repoPath, ticket)
		if err != nil {
			writeError(resp, http.StatusInternalServerError, err.Error())

			return
		}
	}
	job, err := s.meta.NewJob(action, repoID, repoPath, ticket, opts.scope)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err.Error())

		return
	}
	queued := queuedJob{
		record:      job,
		message:     opts.message,
		actionLabel: opts.actionLabel,
		targetState: opts.targetState,
	}
	s.broadcast(serverEvent{
		Type:         "job",
		RepoID:       repoID,
		RepoPath:     repoPath,
		TicketNumber: ticket,
		JobID:        job.ID,
		Action:       action,
		Scope:        opts.scope,
		Status:       "queued",
	})
	select {
	case s.jobs <- queued:
		writeJSON(resp, http.StatusAccepted, api.ActionAcceptedResponse{
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
			Scope:        opts.scope,
			Status:       "failed",
			Error:        "job queue is full",
		})
		writeError(resp, http.StatusServiceUnavailable, "job queue is full")
	}
}

func parseLogEvents(path string) ([]logEvent, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path built from trusted internal state
	if err != nil {
		return nil, fmt.Errorf("read log file: %w", err)
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
		if m := sectionHeaderRE.FindStringSubmatch(line); len(m) == sectionMatchLen {
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

func artifactPath(ticketState ticketdomain.State, stateFilePath, name string) (string, bool) {
	if path, ok := resolveArtifactRef(ticketState, name); ok {
		return path, true
	}
	switch name {
	case "state":
		if ticketState.WorktreePath != "" {
			return ticketState.ArtifactPath("state.json"), true
		}

		return stateFilePath, true
	case "log":
		if ticketState.WorktreePath != "" && ticketState.CurrentState != "" {
			return ticketState.CurrentRunLogPath(), true
		}

		return "", false
	default:
		return "", false
	}
}

func resolveArtifactRef(ticketState ticketdomain.State, name string) (string, bool) {
	if ticketState.WorktreePath == "" || strings.TrimSpace(name) == "" {
		return "", false
	}
	clean := filepath.Clean(strings.TrimSpace(name))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", false
	}
	parts := strings.Split(filepath.ToSlash(clean), "/")
	if len(parts) < 2 || parts[0] != "runs" {
		return "", false
	}

	return ticketState.ResolveRef(filepath.ToSlash(clean)), true
}
