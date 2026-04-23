package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Neokil/AutoPR/internal/contracts/api"
)

type RemoteService struct {
	baseURL    string
	repoPath   string
	httpClient *http.Client
}

func NewRemoteService(baseURL, repoPath string) *RemoteService {
	return &RemoteService{
		baseURL:  strings.TrimRight(baseURL, "/"),
		repoPath: repoPath,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (s *RemoteService) StartFlow(ctx context.Context, ticketNumber string) error {
	path := fmt.Sprintf("/api/tickets/%s/run", url.PathEscape(ticketNumber))
	_, err := s.enqueueOnly(ctx, http.MethodPost, path, api.RepoRequest{RepoPath: s.repoPath}, "run", ticketNumber)
	return err
}

func (s *RemoteService) ApplyAction(ctx context.Context, ticketNumber, actionLabel, message string) error {
	path := fmt.Sprintf("/api/tickets/%s/action", url.PathEscape(ticketNumber))
	_, err := s.enqueueOnly(ctx, http.MethodPost, path, api.ActionRequest{
		RepoPath: s.repoPath,
		Label:    actionLabel,
		Message:  message,
	}, actionLabel, ticketNumber)
	return err
}

func (s *RemoteService) MoveToState(ctx context.Context, ticketNumber, target string) error {
	path := fmt.Sprintf("/api/tickets/%s/move-to-state", url.PathEscape(ticketNumber))
	_, err := s.enqueueOnly(ctx, http.MethodPost, path, api.MoveToStateRequest{
		RepoPath: s.repoPath,
		Target:   target,
	}, "move to "+target, ticketNumber)
	return err
}

func (s *RemoteService) Status(ticketNumber string) error {
	if strings.TrimSpace(ticketNumber) == "" {
		var out struct {
			Tickets []map[string]any `json:"tickets"`
		}
		ticketsURL := "/api/tickets?repo_path=" + url.QueryEscape(s.repoPath)
		if err := s.doJSON(context.Background(), http.MethodGet, ticketsURL, nil, &out); err != nil {
			return err
		}
		for _, t := range out.Tickets {
			slog.Info("ticket",
				"ticket", t["ticket_number"], "status", t["status"],
				"state", t["current_state"], "updated", t["updated_at"])
		}
		return nil
	}
	var out struct {
		TicketNumber string         `json:"ticket_number"`
		State        map[string]any `json:"state"`
		NextSteps    string         `json:"next_steps"`
	}
	path := "/api/tickets/" + url.PathEscape(ticketNumber) + "?repo_path=" + url.QueryEscape(s.repoPath)
	if err := s.doJSON(context.Background(), http.MethodGet, path, nil, &out); err != nil {
		return err
	}
	attrs := []any{
		"ticket", out.TicketNumber,
		"status", out.State["flow_status"],
		"state", out.State["current_state"],
		"branch", out.State["branch_name"],
		"worktree", out.State["worktree_path"],
	}
	if v := strings.TrimSpace(fmt.Sprintf("%v", out.State["pr_url"])); v != "" && v != "<nil>" {
		attrs = append(attrs, "pr_url", v)
	}
	if v := strings.TrimSpace(fmt.Sprintf("%v", out.State["last_error"])); v != "" && v != "<nil>" {
		attrs = append(attrs, "error", v)
	}
	if out.NextSteps != "" {
		attrs = append(attrs, "next_steps", out.NextSteps)
	}
	slog.Info("ticket status", attrs...)
	return nil
}

func (s *RemoteService) NextSteps(ticketNumber string) (string, error) {
	var out struct {
		NextSteps string `json:"next_steps"`
	}
	path := "/api/tickets/" + url.PathEscape(ticketNumber) + "?repo_path=" + url.QueryEscape(s.repoPath)
	if err := s.doJSON(context.Background(), http.MethodGet, path, nil, &out); err != nil {
		return "", err
	}
	return out.NextSteps, nil
}

func (s *RemoteService) CleanupDone(ctx context.Context) error {
	_, err := s.enqueueOnly(ctx, http.MethodPost, "/api/cleanup",
		api.CleanupScopeRequest{RepoPath: s.repoPath, Scope: "done"}, "cleanup done", "")
	return err
}

func (s *RemoteService) CleanupAll(ctx context.Context) error {
	_, err := s.enqueueOnly(ctx, http.MethodPost, "/api/cleanup",
		api.CleanupScopeRequest{RepoPath: s.repoPath, Scope: "all"}, "cleanup all", "")
	return err
}

func (s *RemoteService) CleanupTicket(ctx context.Context, ticketNumber string) error {
	path := fmt.Sprintf("/api/tickets/%s/cleanup", url.PathEscape(ticketNumber))
	_, err := s.enqueueOnly(ctx, http.MethodPost, path, api.RepoRequest{RepoPath: s.repoPath}, "cleanup", ticketNumber)
	return err
}

func (s *RemoteService) WaitForJob(ctx context.Context, jobID string) (api.JobStatusResponse, error) {
	for {
		var job api.JobStatusResponse
		if err := s.doJSON(ctx, http.MethodGet, "/api/jobs/"+url.PathEscape(jobID), nil, &job); err != nil {
			return api.JobStatusResponse{}, err
		}
		switch job.Status {
		case "done":
			return job, nil
		case "failed":
			if strings.TrimSpace(job.Error) == "" {
				return job, fmt.Errorf("job %s: %w", jobID, ErrJobFailed)
			}
			return job, fmt.Errorf("%w: %s", ErrJobFailed, job.Error)
		case "queued", "running":
			select {
			case <-ctx.Done():
				return api.JobStatusResponse{}, ctx.Err()
			case <-time.After(600 * time.Millisecond):
			}
		default:
			return job, fmt.Errorf("job status %q: %w", job.Status, ErrUnexpectedStatus)
		}
	}
}

func (s *RemoteService) enqueueOnly(
	ctx context.Context, method, path string, body any, action, ticket string,
) (api.ActionAcceptedResponse, error) {
	var accepted api.ActionAcceptedResponse
	if err := s.doJSON(ctx, method, path, body, &accepted); err != nil {
		return api.ActionAcceptedResponse{}, err
	}
	jobID := strings.TrimSpace(accepted.JobID)
	if jobID == "" {
		return api.ActionAcceptedResponse{}, ErrMissingJobID
	}
	if ticket != "" {
		slog.Info("action scheduled", "action", action, "ticket", ticket, "job_id", jobID)
	} else {
		slog.Info("action scheduled", "action", action, "job_id", jobID)
	}
	return accepted, nil
}

func (s *RemoteService) doJSON(ctx context.Context, method, path string, body any, out any) error {
	u := s.baseURL + path
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	data, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		var er api.ErrorResponse
		if err := json.Unmarshal(data, &er); err == nil && strings.TrimSpace(er.Error) != "" {
			return fmt.Errorf("%w: %s", ErrRemote, er.Error)
		}
		if strings.TrimSpace(string(data)) == "" {
			return fmt.Errorf("HTTP %d: %w", res.StatusCode, ErrHTTP)
		}
		return fmt.Errorf("%w: %s", ErrRemote, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}
