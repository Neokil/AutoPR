package server

import (
	"encoding/json"
	"errors"
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

	"github.com/Neokil/AutoPR/internal/api"
	"github.com/Neokil/AutoPR/internal/application/tickets"
	"github.com/Neokil/AutoPR/internal/config"
	workflowstate "github.com/Neokil/AutoPR/internal/domain/workflowstate"
	"github.com/Neokil/AutoPR/internal/gitutil"
	"github.com/Neokil/AutoPR/internal/serverstate"
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
	record      serverstate.JobRecord
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
	meta     *serverstate.Store
	runtimes map[string]*repoRuntime
	mu       sync.Mutex
	jobs     chan queuedJob
	webFS    fs.FS

	subsMu      sync.Mutex
	subscribers map[string]chan api.ServerEvent

	repoLockMu sync.Mutex
	repoLocks  map[string]*sync.RWMutex

	ticketLockMu sync.Mutex
	ticketLocks  map[string]*sync.Mutex
}

var sectionHeaderRE = regexp.MustCompile(`^## (.+) \(([^)]+)\)$`)
var githubPRURLPattern = regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+)/pull/([0-9]+)`)

// Run starts the AutoPR daemon.
func Run(portOverride int) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	metaPath, err := serverstate.DefaultPath()
	if err != nil {
		return fmt.Errorf("resolve server state path: %w", err)
	}
	meta, err := serverstate.NewStore(metaPath)
	if err != nil {
		return fmt.Errorf("open server state store: %w", err)
	}
	distFS, err := web.Dist()
	if err != nil {
		return fmt.Errorf("load web assets: %w", err)
	}

	daemon := &server{
		cfg:         cfg,
		meta:        meta,
		runtimes:    map[string]*repoRuntime{},
		jobs:        make(chan queuedJob, jobQueueSize),
		repoLocks:   map[string]*sync.RWMutex{},
		ticketLocks: map[string]*sync.Mutex{},
		webFS:       distFS,
		subscribers: map[string]chan api.ServerEvent{},
	}
	for range cfg.ServerWorkers {
		go daemon.workerLoop()
	}
	go daemon.prMonitorLoop()

	port := cfg.ServerPort
	if portOverride > 0 {
		port = portOverride
	}
	if port <= 0 {
		port = 8080
	}
	mux := http.NewServeMux()
	strictAPI := api.NewStrictHandlerWithOptions(daemon, nil, api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeJSON(w, http.StatusBadRequest, api.ErrorResponse{Error: err.Error()})
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeJSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		},
	})
	apiHandler := api.HandlerWithOptions(strictAPI, api.StdHTTPServerOptions{
		BaseRouter: mux,
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeJSON(w, http.StatusBadRequest, api.ErrorResponse{Error: err.Error()})
		},
	})
	mux.HandleFunc("GET /api/events", daemon.handleEvents)
	mux.HandleFunc("/", daemon.handleFrontend)

	addr := fmt.Sprintf(":%d", port)
	slog.Info("AutoPR daemon listening", "addr", addr)
	srv := &http.Server{
		Addr:              addr,
		Handler:           loggingMiddleware(apiHandler),
		ReadHeaderTimeout: httpReadHeaderTimeout,
	}
	err = srv.ListenAndServe()
	if err != nil {
		return fmt.Errorf("listen and serve: %w", err)
	}

	return nil
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
		message:     derefString(req.Message),
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
	scope := strings.TrimSpace(string(req.Scope))
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
	writeJSON(resp, http.StatusOK, toJobResponse(job))
}

func (s *server) handleListTickets(resp http.ResponseWriter, req *http.Request) {
	repoPath := strings.TrimSpace(req.URL.Query().Get("repo_path"))
	if repoPath == "" {
		writeJSON(resp, http.StatusOK, api.TicketListResponse{
			Tickets: toTicketSummaryResponses(s.meta.ListTickets("")),
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
	writeJSON(resp, http.StatusOK, api.TicketListResponse{
		RepoId:   stringPtr(repoID),
		RepoPath: stringPtr(repoRoot),
		Tickets:  toTicketSummaryResponses(s.meta.ListTickets(repoID)),
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
		if ticketState.FlowStatus == workflowstate.FlowStatusWaiting {
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

	ticketResp := api.TicketDetailsResponse{
		RepoId:           repoID,
		RepoPath:         repoRoot,
		TicketNumber:     ticket,
		GithubBlobBase:   stringPtr(githubBlobBase),
		State:            toTicketStateResponse(ticketState),
		NextSteps:        stringPtr(nextSteps),
		WorkflowStates:   toWorkflowStateResponses(workflowStates),
		AvailableActions: toActionResponses(availableActions),
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
			writeJSON(resp, http.StatusOK, api.TicketEventsResponse{
				RepoId:       repoID,
				RepoPath:     repoRoot,
				TicketNumber: ticket,
				Events:       []api.LogEventResponse{},
			})

			return
		}
		writeError(resp, http.StatusInternalServerError, err.Error())

		return
	}
	writeJSON(resp, http.StatusOK, api.TicketEventsResponse{
		RepoId:       repoID,
		RepoPath:     repoRoot,
		TicketNumber: ticket,
		Events:       events,
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
			writeJSON(resp, http.StatusOK, api.TicketArtifactResponse{
				RepoId:       repoID,
				RepoPath:     repoRoot,
				TicketNumber: ticket,
				Name:         name,
				Path:         path,
				Content:      "",
			})

			return
		}
		writeError(resp, http.StatusInternalServerError, err.Error())

		return
	}
	writeJSON(resp, http.StatusOK, api.TicketArtifactResponse{
		RepoId:       repoID,
		RepoPath:     repoRoot,
		TicketNumber: ticket,
		Name:         name,
		Path:         path,
		Content:      string(data),
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
	logs := make([]api.ExecutionLogResponse, 0, len(ticketState.StateHistory))
	for _, run := range ticketState.StateHistory {
		runPath := filepath.ToSlash(filepath.Join("runs", run.ID, "raw-provider.log"))
		content, readErr := os.ReadFile(ticketState.ResolveRef(runPath)) //nolint:gosec // G703: path is constructed from trusted internal state
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			writeError(resp, http.StatusInternalServerError, readErr.Error())

			return
		}
		logs = append(logs, api.ExecutionLogResponse{
			RunId:            run.ID,
			State:            run.StateName,
			StateDisplayName: stringPtr(run.StateDisplayName),
			Timestamp:        run.StartedAt.Format(time.RFC3339),
			Path:             runPath,
			Content:          string(content),
		})
	}
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].Timestamp < logs[j].Timestamp
	})
	writeJSON(resp, http.StatusOK, api.ExecutionLogsResponse{
		RepoId:       repoID,
		RepoPath:     repoRoot,
		TicketNumber: ticket,
		Logs:         logs,
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
	s.broadcast(api.ServerEvent{
		Type:         "job",
		RepoId:       stringPtr(repoID),
		RepoPath:     stringPtr(repoPath),
		TicketNumber: stringPtr(ticket),
		JobId:        stringPtr(job.ID),
		Action:       stringPtr(action),
		Scope:        stringPtr(opts.scope),
		Status:       stringPtr("queued"),
	})
	select {
	case s.jobs <- queued:
		writeJSON(resp, http.StatusAccepted, api.ActionAcceptedResponse{
			Status:       "accepted",
			JobId:        job.ID,
			Action:       action,
			RepoId:       repoID,
			RepoPath:     repoPath,
			TicketNumber: stringPtr(ticket),
		})
	default:
		_ = s.meta.UpdateJobStatus(job.ID, "failed", "job queue is full")
		s.broadcast(api.ServerEvent{
			Type:         "job",
			RepoId:       stringPtr(repoID),
			RepoPath:     stringPtr(repoPath),
			TicketNumber: stringPtr(ticket),
			JobId:        stringPtr(job.ID),
			Action:       stringPtr(action),
			Scope:        stringPtr(opts.scope),
			Status:       stringPtr("failed"),
			Error:        stringPtr("job queue is full"),
		})
		writeError(resp, http.StatusServiceUnavailable, "job queue is full")
	}
}

func parseLogEvents(path string) ([]api.LogEventResponse, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path built from trusted internal state
	if err != nil {
		return nil, fmt.Errorf("read log file: %w", err)
	}
	lines := strings.Split(string(data), "\n")
	events := make([]api.LogEventResponse, 0)
	cur := api.LogEventResponse{}
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
			cur = api.LogEventResponse{Title: strings.TrimSpace(m[1]), Timestamp: strings.TrimSpace(m[2])}
			bodyLines = bodyLines[:0]

			continue
		}
		bodyLines = append(bodyLines, line)
	}
	flush()

	return events, nil
}

func artifactPath(ticketState workflowstate.State, stateFilePath, name string) (string, bool) {
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

func resolveArtifactRef(ticketState workflowstate.State, name string) (string, bool) {
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
