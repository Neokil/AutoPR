package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"

	"ai-ticket-worker/internal/application/orchestrator"
	"ai-ticket-worker/internal/config"
	"ai-ticket-worker/internal/gitutil"
	"ai-ticket-worker/internal/providers"
	"ai-ticket-worker/internal/state"
	"ai-ticket-worker/internal/workflow"
)

type server struct {
	svc   orchestrator.Service
	store *state.Store
}

type feedbackRequest struct {
	Message string `json:"message"`
}

type ticketSummary struct {
	TicketNumber string `json:"ticket_number"`
	Status       string `json:"status"`
	Approved     bool   `json:"approved"`
	UpdatedAt    string `json:"updated_at"`
	PRURL        string `json:"pr_url,omitempty"`
}

type ticketDetails struct {
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
	ctx := context.Background()

	repoFlag := flag.String("repo", "", "repository path (defaults to current working directory)")
	portFlag := flag.Int("port", 0, "HTTP port override (default uses config server_port)")
	flag.Parse()

	cfg, err := config.Load()
	fatalIf(err)

	cwd, err := os.Getwd()
	fatalIf(err)

	repoBase := cwd
	if strings.TrimSpace(*repoFlag) != "" {
		repoBase = *repoFlag
	}
	repoRoot, err := gitutil.RepoRoot(ctx, repoBase)
	fatalIf(err)
	fatalIf(workflow.EnsureStateIgnored(repoRoot, cfg.StateDirName))

	provider, err := providers.NewFromConfig(cfg)
	fatalIf(err)

	svc := orchestrator.NewWorkflowService(cfg, repoRoot, provider)
	store := state.NewStore(repoRoot, cfg.StateDirName)
	s := &server{svc: svc, store: store}

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
	fmt.Printf("orchestratord listening on %s (repo: %s)\n", addr, repoRoot)
	fatalIf(http.ListenAndServe(addr, mux))
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) handleRunTicket(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	err := s.svc.RunTickets(r.Context(), []string{ticket})
	s.respondAction(w, ticket, err)
}

func (s *server) handleResumeTicket(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	err := s.svc.ResumeTicket(r.Context(), ticket)
	s.respondAction(w, ticket, err)
}

func (s *server) handleApproveTicket(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	err := s.svc.Approve(r.Context(), ticket)
	s.respondAction(w, ticket, err)
}

func (s *server) handleRejectTicket(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	err := s.svc.Reject(ticket)
	s.respondAction(w, ticket, err)
}

func (s *server) handleFeedbackTicket(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	var req feedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}
	err := s.svc.Feedback(ticket, req.Message)
	s.respondAction(w, ticket, err)
}

func (s *server) handleCleanupTicket(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	err := s.svc.CleanupTicket(r.Context(), ticket)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"ticket_number": ticket,
		"status":        "cleaned",
	})
}

func (s *server) handleCleanupScope(w http.ResponseWriter, r *http.Request) {
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	var err error
	switch scope {
	case "done":
		err = s.svc.CleanupDone(r.Context())
	case "all":
		err = s.svc.CleanupAll(r.Context())
	default:
		writeError(w, http.StatusBadRequest, "scope must be 'done' or 'all'")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "scope": scope})
}

func (s *server) handleListTickets(w http.ResponseWriter, r *http.Request) {
	tickets, err := s.store.ListTicketDirs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sort.Strings(tickets)

	out := make([]ticketSummary, 0, len(tickets))
	for _, t := range tickets {
		st, err := s.store.LoadState(t)
		if err != nil {
			continue
		}
		out = append(out, ticketSummary{
			TicketNumber: t,
			Status:       string(st.Status),
			Approved:     st.Approved,
			UpdatedAt:    st.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			PRURL:        st.PRURL,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"tickets": out})
}

func (s *server) handleGetTicket(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	st, err := s.store.LoadState(ticket)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "ticket not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	t, err := s.store.LoadTicket(ticket)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	nextSteps, _ := s.svc.NextSteps(ticket)
	resp := ticketDetails{
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
	paths := s.store.Paths(ticket)
	events, err := parseLogEvents(paths["log"])
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ticket_number": ticket,
				"events":        []logEvent{},
			})
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ticket_number": ticket,
		"events":        events,
	})
}

func (s *server) handleTicketArtifact(w http.ResponseWriter, r *http.Request) {
	ticket := r.PathValue("id")
	name := r.PathValue("name")
	path, ok := artifactPath(s.store.Paths(ticket), name)
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
		"ticket_number": ticket,
		"name":          name,
		"path":          path,
		"content":       string(data),
	})
}

func (s *server) respondAction(w http.ResponseWriter, ticket string, err error) {
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	nextSteps, _ := s.svc.NextSteps(ticket)
	writeJSON(w, http.StatusOK, map[string]string{
		"ticket_number": ticket,
		"status":        "ok",
		"next_steps":    nextSteps,
	})
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
