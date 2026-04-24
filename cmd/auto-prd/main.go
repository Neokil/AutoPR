package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"log/slog"
	"sync"
	"time"

	"github.com/Neokil/AutoPR/internal/application/orchestrator"
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

	jobQueueSize        = 256
	httpReadHeaderTimeout = 30 * time.Second
	sectionMatchLen     = 3 // full match + 2 capture groups
)

type repoRuntime struct {
	svc      orchestrator.Service
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
	meta     servermeta.Repository
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

	s := &server{
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
		go s.workerLoop()
	}
	go s.prMonitorLoop()

	port := cfg.ServerPort
	if *portFlag > 0 {
		port = *portFlag
	}
	if port <= 0 {
		port = 8080
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/repositories", s.handleListRepositories)
	mux.HandleFunc("GET /api/tickets", s.handleListTickets)
	mux.HandleFunc("GET /api/tickets/{id}", s.handleGetTicket)
	mux.HandleFunc("GET /api/tickets/{id}/events", s.handleTicketEvents)
	mux.HandleFunc("GET /api/tickets/{id}/artifacts/{name...}", s.handleTicketArtifact)
	mux.HandleFunc("GET /api/tickets/{id}/execution-logs", s.handleExecutionLogs)
	mux.HandleFunc("GET /api/jobs/{id}", s.handleGetJob)
	mux.HandleFunc("GET /api/events", s.handleEvents)
	mux.HandleFunc("POST /api/tickets/{id}/run", s.handleRunTicket)
	mux.HandleFunc("POST /api/tickets/{id}/action", s.handleActionTicket)
	mux.HandleFunc("POST /api/tickets/{id}/move-to-state", s.handleMoveToStateTicket)
	mux.HandleFunc("POST /api/tickets/{id}/cleanup", s.handleCleanupTicket)
	mux.HandleFunc("POST /api/cleanup", s.handleCleanupScope)
	mux.HandleFunc("/", s.handleFrontend)

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

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "ok",
		"server_state": "~/.auto-pr/server/state.json",
		"queue_depth":  len(s.jobs),
		"frontend":     "embedded",
	})
}

func (s *server) handleRunTicket(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	repoRoot, repoID, ok := s.repoRuntimeFromBody(w, r)
	if !ok {
		return
	}
	s.enqueueAndRespond(w, jobRun, repoID, repoRoot, ticket, enqueueOptions{})
}

func (s *server) handleActionTicket(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	var req api.ActionRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")

		return
	}
	if strings.TrimSpace(req.RepoPath) == "" {
		writeError(w, http.StatusBadRequest, "repo_path is required")

		return
	}
	if strings.TrimSpace(req.Label) == "" {
		writeError(w, http.StatusBadRequest, "label is required")

		return
	}
	repoRoot, repoID, _, err := s.runtimeForRepoPath(r.Context(), req.RepoPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())

		return
	}
	s.enqueueAndRespond(w, jobAction, repoID, repoRoot, ticket, enqueueOptions{
		message:     req.Message,
		actionLabel: req.Label,
	})
}

func (s *server) handleMoveToStateTicket(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	var req api.MoveToStateRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")

		return
	}
	if strings.TrimSpace(req.RepoPath) == "" {
		writeError(w, http.StatusBadRequest, "repo_path is required")

		return
	}
	if strings.TrimSpace(req.Target) == "" {
		writeError(w, http.StatusBadRequest, "target is required")

		return
	}
	repoRoot, repoID, _, err := s.runtimeForRepoPath(r.Context(), req.RepoPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())

		return
	}
	s.enqueueAndRespond(w, jobMoveToState, repoID, repoRoot, ticket, enqueueOptions{
		targetState: req.Target,
	})
}

func (s *server) handleCleanupTicket(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	repoRoot, repoID, ok := s.repoRuntimeFromBody(w, r)
	if !ok {
		return
	}
	s.enqueueAndRespond(w, jobCleanup, repoID, repoRoot, ticket, enqueueOptions{})
}

func (s *server) handleCleanupScope(w http.ResponseWriter, r *http.Request) {
	var req api.CleanupScopeRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
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
	repoRoot, repoID, _, err := s.runtimeForRepoPath(r.Context(), req.RepoPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())

		return
	}

	switch scope {
	case "done":
		s.enqueueAndRespond(w, jobCleanupDone, repoID, repoRoot, "", enqueueOptions{scope: scope})
	case "all":
		s.enqueueAndRespond(w, jobCleanupAll, repoID, repoRoot, "", enqueueOptions{scope: scope})
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

func (s *server) handleListTickets(w http.ResponseWriter, r *http.Request) {
	repoPath := strings.TrimSpace(r.URL.Query().Get("repo_path"))
	if repoPath == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"tickets": s.meta.ListTickets(""),
		})

		return
	}
	repoRoot, repoID, rt, err := s.runtimeForRepoPath(r.Context(), repoPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())

		return
	}
	err = s.syncRepoTickets(repoID, repoRoot, rt, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())

		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"repo_id":   repoID,
		"repo_path": repoRoot,
		"tickets":   s.meta.ListTickets(repoID),
	})
}

func (s *server) handleListRepositories(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusOK, map[string]any{
		"repositories": paths,
	})
}

func (s *server) handleGetTicket(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	repoPath := strings.TrimSpace(r.URL.Query().Get("repo_path"))
	if repoPath == "" {
		writeError(w, http.StatusBadRequest, "repo_path query param is required")

		return
	}
	repoRoot, repoID, rt, err := s.runtimeForRepoPath(r.Context(), repoPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())

		return
	}
	err = s.syncTicketFromRepo(repoID, repoRoot, ticket, rt, false)
	if err != nil {
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
	nextSteps, _ := rt.svc.NextSteps(ticket)
	githubBlobBase, _ := gitutil.GitHubBlobBase(r.Context(), repoRoot, s.cfg.BaseBranch)

	var availableActions []actionInfo
	var workflowStates []workflowStateInfo
	wf, wfErr := workflow.Load(repoRoot)
	if wfErr == nil {
		for _, stateCfg := range wf.States {
			workflowStates = append(workflowStates, workflowStateInfo{
				Name:        stateCfg.Name,
				DisplayName: stateCfg.TimelineLabel(),
			})
		}
		if st.FlowStatus == ticketdomain.FlowStatusWaiting {
			if stateCfg, ok := wf.StateByName(st.CurrentState); ok {
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

	resp := ticketDetails{
		RepoID:           repoID,
		RepoPath:         repoRoot,
		TicketNumber:     ticket,
		GitHubBlobBase:   githubBlobBase,
		State:            st,
		NextSteps:        nextSteps,
		WorkflowStates:   workflowStates,
		AvailableActions: availableActions,
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
	repoRoot, repoID, rt, err := s.runtimeForRepoPath(r.Context(), repoPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())

		return
	}
	st, stErr := rt.store.LoadState(ticket)
	var logPath string
	if stErr == nil && st.WorktreePath != "" && st.CurrentState != "" {
		logPath = st.CurrentRunLogPath()
	}
	events, err := parseLogEvents(logPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(w, http.StatusOK, map[string]any{
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
	writeJSON(w, http.StatusOK, map[string]any{
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
	repoRoot, repoID, rt, err := s.runtimeForRepoPath(r.Context(), repoPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())

		return
	}
	st, stErr := rt.store.LoadState(ticket)
	if stErr != nil {
		writeError(w, http.StatusNotFound, "ticket not found")

		return
	}
	path, ok := artifactPath(st, filepath.Join(rt.store.TicketDir(ticket), state.StateFileName), name)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown artifact")

		return
	}
	data, err := os.ReadFile(path) //nolint:gosec // G304: path resolved from trusted internal state
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(w, http.StatusOK, map[string]string{
				"repo_id":       repoID,
				"repo_path":     repoRoot,
				"ticket_number": ticket,
				"name":          name,
				"path":          path,
				"content":       "",
			})

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

func (s *server) handleExecutionLogs(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	repoPath := strings.TrimSpace(r.URL.Query().Get("repo_path"))
	if repoPath == "" {
		writeError(w, http.StatusBadRequest, "repo_path query param is required")

		return
	}
	repoRoot, repoID, rt, err := s.runtimeForRepoPath(r.Context(), repoPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())

		return
	}
	st, err := rt.store.LoadState(ticket)
	if err != nil {
		writeError(w, http.StatusNotFound, "ticket not found")

		return
	}
	logs := make([]executionLog, 0, len(st.StateHistory))
	for _, run := range st.StateHistory {
		runPath := filepath.ToSlash(filepath.Join("runs", run.ID, "raw-provider.log"))
		content, readErr := os.ReadFile(st.ResolveRef(runPath)) //nolint:gosec // G703: path is constructed from trusted internal state
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			writeError(w, http.StatusInternalServerError, readErr.Error())

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
	writeJSON(w, http.StatusOK, map[string]any{
		"repo_id":       repoID,
		"repo_path":     repoRoot,
		"ticket_number": ticket,
		"logs":          logs,
	})
}

func (s *server) enqueueAndRespond(w http.ResponseWriter, action, repoID, repoPath, ticket string, opts enqueueOptions) {
	if action == jobRun && strings.TrimSpace(ticket) != "" {
		err := s.ensureQueuedTicket(repoID, repoPath, ticket)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())

			return
		}
	}
	job, err := s.meta.NewJob(action, repoID, repoPath, ticket, opts.scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())

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
			Scope:        opts.scope,
			Status:       "failed",
			Error:        "job queue is full",
		})
		writeError(w, http.StatusServiceUnavailable, "job queue is full")
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

func artifactPath(st ticketdomain.State, stateFilePath, name string) (string, bool) {
	if path, ok := resolveArtifactRef(st, name); ok {
		return path, true
	}
	switch name {
	case "state":
		if st.WorktreePath != "" {
			return st.ArtifactPath("state.json"), true
		}

		return stateFilePath, true
	case "log":
		if st.WorktreePath != "" && st.CurrentState != "" {
			return st.CurrentRunLogPath(), true
		}

		return "", false
	default:
		return "", false
	}
}

func resolveArtifactRef(st ticketdomain.State, name string) (string, bool) {
	if st.WorktreePath == "" || strings.TrimSpace(name) == "" {
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

	return st.ResolveRef(filepath.ToSlash(clean)), true
}
