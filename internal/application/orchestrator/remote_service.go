package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ai-ticket-worker/internal/contracts/api"
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

func (s *RemoteService) RunTickets(ctx context.Context, ticketNumbers []string) error {
	for _, ticket := range ticketNumbers {
		if _, err := s.enqueueAndWait(ctx, http.MethodPost, fmt.Sprintf("/api/tickets/%s/run", url.PathEscape(ticket)), api.RepoRequest{RepoPath: s.repoPath}); err != nil {
			return err
		}
	}
	return nil
}

func (s *RemoteService) Status(ticketNumber string) error {
	if strings.TrimSpace(ticketNumber) == "" {
		var out struct {
			Tickets []map[string]interface{} `json:"tickets"`
		}
		if err := s.doJSON(http.MethodGet, "/api/tickets?repo_path="+url.QueryEscape(s.repoPath), nil, &out); err != nil {
			return err
		}
		for _, t := range out.Tickets {
			fmt.Printf("ticket %v  state=%v  approved=%v  updated=%v\n", t["ticket_number"], t["status"], t["approved"], t["updated_at"])
		}
		return nil
	}
	var out struct {
		TicketNumber string                 `json:"ticket_number"`
		State        map[string]interface{} `json:"state"`
		NextSteps    string                 `json:"next_steps"`
	}
	path := "/api/tickets/" + url.PathEscape(ticketNumber) + "?repo_path=" + url.QueryEscape(s.repoPath)
	if err := s.doJSON(http.MethodGet, path, nil, &out); err != nil {
		return err
	}
	fmt.Printf("ticket %s\n", out.TicketNumber)
	fmt.Printf("  state: %v\n", out.State["status"])
	fmt.Printf("  approved: %v\n", out.State["approved"])
	fmt.Printf("  branch: %v\n", out.State["branch_name"])
	fmt.Printf("  worktree: %v\n", out.State["worktree_path"])
	fmt.Printf("  proposal: %v\n", out.State["proposal_path"])
	fmt.Printf("  pr: %v\n", out.State["pr_path"])
	if v := strings.TrimSpace(fmt.Sprintf("%v", out.State["pr_url"])); v != "" && v != "<nil>" {
		fmt.Printf("  pr_url: %s\n", v)
	}
	if v := strings.TrimSpace(fmt.Sprintf("%v", out.State["last_error"])); v != "" && v != "<nil>" {
		fmt.Printf("  last_error: %s\n", v)
	}
	return nil
}

func (s *RemoteService) Approve(ctx context.Context, ticketNumber string) error {
	_, err := s.enqueueAndWait(ctx, http.MethodPost, fmt.Sprintf("/api/tickets/%s/approve", url.PathEscape(ticketNumber)), api.RepoRequest{RepoPath: s.repoPath})
	return err
}

func (s *RemoteService) Feedback(ticketNumber, message string) error {
	_, err := s.enqueueAndWait(context.Background(), http.MethodPost, fmt.Sprintf("/api/tickets/%s/feedback", url.PathEscape(ticketNumber)), api.FeedbackRequest{
		RepoPath: s.repoPath,
		Message:  message,
	})
	return err
}

func (s *RemoteService) Reject(ticketNumber string) error {
	_, err := s.enqueueAndWait(context.Background(), http.MethodPost, fmt.Sprintf("/api/tickets/%s/reject", url.PathEscape(ticketNumber)), api.RepoRequest{RepoPath: s.repoPath})
	return err
}

func (s *RemoteService) ResumeTicket(ctx context.Context, ticketNumber string) error {
	_, err := s.enqueueAndWait(ctx, http.MethodPost, fmt.Sprintf("/api/tickets/%s/resume", url.PathEscape(ticketNumber)), api.RepoRequest{RepoPath: s.repoPath})
	return err
}

func (s *RemoteService) GeneratePR(ctx context.Context, ticketNumber string) error {
	_, err := s.enqueueAndWait(ctx, http.MethodPost, fmt.Sprintf("/api/tickets/%s/pr", url.PathEscape(ticketNumber)), api.RepoRequest{RepoPath: s.repoPath})
	return err
}

func (s *RemoteService) CleanupDone(ctx context.Context) error {
	_, err := s.enqueueAndWait(ctx, http.MethodPost, "/api/cleanup", api.CleanupScopeRequest{RepoPath: s.repoPath, Scope: "done"})
	return err
}

func (s *RemoteService) CleanupAll(ctx context.Context) error {
	_, err := s.enqueueAndWait(ctx, http.MethodPost, "/api/cleanup", api.CleanupScopeRequest{RepoPath: s.repoPath, Scope: "all"})
	return err
}

func (s *RemoteService) CleanupTicket(ctx context.Context, ticketNumber string) error {
	_, err := s.enqueueAndWait(ctx, http.MethodPost, fmt.Sprintf("/api/tickets/%s/cleanup", url.PathEscape(ticketNumber)), api.RepoRequest{RepoPath: s.repoPath})
	return err
}

func (s *RemoteService) NextSteps(ticketNumber string) (string, error) {
	var out struct {
		NextSteps string `json:"next_steps"`
	}
	path := "/api/tickets/" + url.PathEscape(ticketNumber) + "?repo_path=" + url.QueryEscape(s.repoPath)
	if err := s.doJSON(http.MethodGet, path, nil, &out); err != nil {
		return "", err
	}
	return out.NextSteps, nil
}

func (s *RemoteService) enqueueAndWait(ctx context.Context, method, path string, body interface{}) (api.JobStatusResponse, error) {
	var accepted api.ActionAcceptedResponse
	if err := s.doJSON(method, path, body, &accepted); err != nil {
		return api.JobStatusResponse{}, err
	}
	jobID := strings.TrimSpace(accepted.JobID)
	if jobID == "" {
		return api.JobStatusResponse{}, fmt.Errorf("server response missing job_id")
	}
	return s.waitForJob(ctx, jobID)
}

func (s *RemoteService) waitForJob(ctx context.Context, jobID string) (api.JobStatusResponse, error) {
	for {
		var job api.JobStatusResponse
		if err := s.doJSON(http.MethodGet, "/api/jobs/"+url.PathEscape(jobID), nil, &job); err != nil {
			return api.JobStatusResponse{}, err
		}
		switch job.Status {
		case "done":
			return job, nil
		case "failed":
			if strings.TrimSpace(job.Error) == "" {
				return job, fmt.Errorf("job %s failed", jobID)
			}
			return job, fmt.Errorf(job.Error)
		case "queued", "running":
			select {
			case <-ctx.Done():
				return api.JobStatusResponse{}, ctx.Err()
			case <-time.After(600 * time.Millisecond):
			}
		default:
			return job, fmt.Errorf("unexpected job status: %s", job.Status)
		}
	}
}

func (s *RemoteService) doJSON(method, path string, body interface{}, out interface{}) error {
	u := s.baseURL + path
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, u, reader)
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
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		var er api.ErrorResponse
		if err := json.Unmarshal(data, &er); err == nil && strings.TrimSpace(er.Error) != "" {
			return fmt.Errorf(er.Error)
		}
		if strings.TrimSpace(string(data)) == "" {
			return fmt.Errorf("http %d", res.StatusCode)
		}
		return fmt.Errorf(strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}
