package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/Neokil/AutoPR/internal/contracts/api"
)

type actionInfo struct {
	Label string `json:"label"`
	Type  string `json:"type"`
}

type workflowStateInfo struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
}

type ticketDetails struct {
	RepoID           string              `json:"repo_id"`
	RepoPath         string              `json:"repo_path"`
	TicketNumber     string              `json:"ticket_number"`
	GitHubBlobBase   string              `json:"github_blob_base,omitempty"`
	State            any         `json:"state"`
	Ticket           any         `json:"ticket,omitempty"`
	NextSteps        string              `json:"next_steps,omitempty"`
	WorkflowStates   []workflowStateInfo `json:"workflow_states,omitempty"`
	AvailableActions []actionInfo        `json:"available_actions"`
}

type logEvent struct {
	Title     string `json:"title"`
	Timestamp string `json:"timestamp"`
	Body      string `json:"body"`
}

type executionLog struct {
	RunID            string `json:"run_id"`
	State            string `json:"state"`
	StateDisplayName string `json:"state_display_name,omitempty"`
	Timestamp        string `json:"timestamp"`
	Path             string `json:"path"`
	Content          string `json:"content"`
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
	if s.serveEmbeddedFile(w, r, "index.html") {
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_, _ = fmt.Fprint(w, "embedded frontend index.html not found")
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
		_, _ = w.Write(data) //nolint:gosec // G705: writing pre-marshalled JSON, not HTML
	}
	return true
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, api.ErrorResponse{Error: msg})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		http.Error(w, fmt.Sprintf("json marshal: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(data)
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
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "duration", time.Since(start).Round(time.Millisecond)) //nolint:gosec // G706: internal access log, not user-controlled injection risk
	})
}
