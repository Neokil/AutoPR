package server

import (
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Neokil/AutoPR/internal/api"
	"github.com/Neokil/AutoPR/internal/application/tickets"
	"github.com/Neokil/AutoPR/internal/config"
	workflowstate "github.com/Neokil/AutoPR/internal/domain/workflowstate"
	"github.com/Neokil/AutoPR/internal/serverstate"
	"github.com/Neokil/AutoPR/internal/state"
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

func parseLogEvents(path string) ([]api.LogEventResponse, error) {
	data, err := os.ReadFile(path)
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
