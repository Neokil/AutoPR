//nolint:nilerr,ireturn
package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Neokil/AutoPR/internal/api"
	workflowstate "github.com/Neokil/AutoPR/internal/domain/workflowstate"
	"github.com/Neokil/AutoPR/internal/gitutil"
	"github.com/Neokil/AutoPR/internal/serverstate"
	"github.com/Neokil/AutoPR/internal/state"
	"github.com/Neokil/AutoPR/internal/workflow"
)

var _ api.StrictServerInterface = (*server)(nil)

func (s *server) DiscoverTickets(ctx context.Context, request api.DiscoverTicketsRequestObject) (api.DiscoverTicketsResponseObject, error) {
	if request.Body == nil {
		return api.DiscoverTickets400JSONResponse{ErrorResponseJSONResponse: api.ErrorResponseJSONResponse{Error: "invalid json body"}}, nil
	}
	repoPath := strings.TrimSpace(request.Body.RepoPath)
	if repoPath == "" {
		return api.DiscoverTickets400JSONResponse{ErrorResponseJSONResponse: api.ErrorResponseJSONResponse{Error: "repo_path is required"}}, nil
	}
	_, _, repoRt, err := s.runtimeForRepoPath(ctx, repoPath)
	if err != nil {
		return api.DiscoverTickets400JSONResponse{ErrorResponseJSONResponse: api.ErrorResponseJSONResponse{Error: err.Error()}}, nil
	}
	found, err := repoRt.svc.DiscoverTickets(ctx)
	if err != nil {
		return api.DiscoverTickets500JSONResponse{Error: err.Error()}, nil
	}
	tickets := make([]api.DiscoveredTicket, len(found))
	for i, t := range found {
		tickets[i] = api.DiscoveredTicket{TicketNumber: t.TicketNumber, Title: t.Title}
	}

	return api.DiscoverTickets200JSONResponse(api.DiscoverTicketsResponse{Tickets: tickets}), nil
}

func (s *server) CleanupScope(ctx context.Context, request api.CleanupScopeRequestObject) (api.CleanupScopeResponseObject, error) {
	if request.Body == nil {
		return badCleanupScope("invalid json body"), nil
	}
	req := *request.Body
	if strings.TrimSpace(req.RepoPath) == "" {
		return badCleanupScope("repo_path is required"), nil
	}
	scope := strings.TrimSpace(string(req.Scope))
	repoRoot, repoID, _, err := s.runtimeForRepoPath(ctx, req.RepoPath)
	if err != nil {
		return badCleanupScope(err.Error()), nil
	}

	switch scope {
	case "done":
		return acceptedCleanupScope(s.enqueueJob(jobCleanupDone, repoID, repoRoot, "", enqueueOptions{scope: scope}))
	case "all":
		return acceptedCleanupScope(s.enqueueJob(jobCleanupAll, repoID, repoRoot, "", enqueueOptions{scope: scope}))
	default:
		return badCleanupScope("scope must be 'done' or 'all'"), nil
	}
}

func (s *server) GetHealth(_ context.Context, _ api.GetHealthRequestObject) (api.GetHealthResponseObject, error) {
	return api.GetHealth200JSONResponse(api.HealthResponse{
		Status:                    "ok",
		ServerState:               "~/.auto-pr/server/state.json",
		QueueDepth:                len(s.jobs),
		Frontend:                  "embedded",
		DiscoverTicketsConfigured: strings.TrimSpace(s.cfg.DiscoverTicketsCommand) != "",
	}), nil
}

func (s *server) GetJob(_ context.Context, request api.GetJobRequestObject) (api.GetJobResponseObject, error) {
	job, ok := s.meta.GetJob(request.Id)
	if !ok {
		return api.GetJob404JSONResponse{ErrorResponseJSONResponse: api.ErrorResponseJSONResponse{Error: "job not found"}}, nil
	}

	return api.GetJob200JSONResponse(toJobResponse(job)), nil
}

func (s *server) ListRepositories(_ context.Context, _ api.ListRepositoriesRequestObject) (api.ListRepositoriesResponseObject, error) {
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

	return api.ListRepositories200JSONResponse(api.RepositoryListResponse{Repositories: paths}), nil
}

func (s *server) ListTickets(ctx context.Context, request api.ListTicketsRequestObject) (api.ListTicketsResponseObject, error) {
	if request.Params.RepoPath == nil || strings.TrimSpace(*request.Params.RepoPath) == "" {
		return api.ListTickets200JSONResponse(api.TicketListResponse{
			Tickets: toTicketSummaryResponses(s.meta.ListTickets("")),
		}), nil
	}

	repoRoot, repoID, repoRt, err := s.runtimeForRepoPath(ctx, *request.Params.RepoPath)
	if err != nil {
		return api.ListTickets400JSONResponse{ErrorResponseJSONResponse: api.ErrorResponseJSONResponse{Error: err.Error()}}, nil
	}
	syncErr := s.syncRepoTickets(repoID, repoRoot, repoRt, false)
	if syncErr != nil {
		return api.ListTickets500JSONResponse{Error: syncErr.Error()}, nil
	}

	return api.ListTickets200JSONResponse(api.TicketListResponse{
		RepoId:   stringPtr(repoID),
		RepoPath: stringPtr(repoRoot),
		Tickets:  toTicketSummaryResponses(s.meta.ListTickets(repoID)),
	}), nil
}

func (s *server) GetTicket(ctx context.Context, request api.GetTicketRequestObject) (api.GetTicketResponseObject, error) {
	repoRoot, repoID, repoRt, err := s.runtimeForRepoPath(ctx, request.Params.RepoPath)
	if err != nil {
		return api.GetTicket400JSONResponse{ErrorResponseJSONResponse: api.ErrorResponseJSONResponse{Error: err.Error()}}, nil
	}
	syncErr := s.syncTicketFromRepo(repoID, repoRoot, request.Id, repoRt, false)
	if syncErr != nil {
		if errors.Is(syncErr, os.ErrNotExist) {
			return api.GetTicket404JSONResponse{Error: errMsgTicketNotFound}, nil
		}

		return api.GetTicket500JSONResponse{Error: syncErr.Error()}, nil
	}

	ticketState, err := repoRt.store.LoadState(request.Id)
	if err != nil {
		return api.GetTicket404JSONResponse{Error: errMsgTicketNotFound}, nil
	}
	nextSteps, _ := repoRt.svc.NextSteps(request.Id)
	githubBlobBase, _ := gitutil.GitHubBlobBase(ctx, repoRoot, s.cfg.BaseBranch)

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

	return api.GetTicket200JSONResponse(api.TicketDetailsResponse{
		RepoId:           repoID,
		RepoPath:         repoRoot,
		TicketNumber:     request.Id,
		GithubBlobBase:   stringPtr(githubBlobBase),
		State:            toTicketStateResponse(ticketState),
		NextSteps:        stringPtr(nextSteps),
		WorkflowStates:   toWorkflowStateResponses(workflowStates),
		AvailableActions: toActionResponses(availableActions),
	}), nil
}

func (s *server) ApplyAction(ctx context.Context, request api.ApplyActionRequestObject) (api.ApplyActionResponseObject, error) {
	if request.Body == nil {
		return badApplyAction("invalid json body"), nil
	}
	req := *request.Body
	if strings.TrimSpace(req.RepoPath) == "" {
		return badApplyAction("repo_path is required"), nil
	}
	if strings.TrimSpace(req.Label) == "" {
		return badApplyAction("label is required"), nil
	}
	repoRoot, repoID, _, err := s.runtimeForRepoPath(ctx, req.RepoPath)
	if err != nil {
		return badApplyAction(err.Error()), nil
	}

	return acceptedApplyAction(s.enqueueJob(jobAction, repoID, repoRoot, request.Id, enqueueOptions{
		message:     derefString(req.Message),
		actionLabel: req.Label,
	}))
}

func (s *server) GetTicketArtifact(ctx context.Context, request api.GetTicketArtifactRequestObject) (api.GetTicketArtifactResponseObject, error) {
	repoRoot, repoID, repoRt, err := s.runtimeForRepoPath(ctx, request.Params.RepoPath)
	if err != nil {
		return api.GetTicketArtifact400JSONResponse{ErrorResponseJSONResponse: api.ErrorResponseJSONResponse{Error: err.Error()}}, nil
	}
	ticketState, err := repoRt.store.LoadState(request.Id)
	if err != nil {
		return api.GetTicketArtifact404JSONResponse{Error: errMsgTicketNotFound}, nil
	}
	path, ok := artifactPath(ticketState, filepath.Join(repoRt.store.TicketDir(request.Id), state.StateFileName), request.Name)
	if !ok {
		return api.GetTicketArtifact400JSONResponse{ErrorResponseJSONResponse: api.ErrorResponseJSONResponse{Error: "unknown artifact"}}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return api.GetTicketArtifact200JSONResponse(api.TicketArtifactResponse{
				RepoId:       repoID,
				RepoPath:     repoRoot,
				TicketNumber: request.Id,
				Name:         request.Name,
				Path:         path,
				Content:      "",
			}), nil
		}

		return api.GetTicketArtifact500JSONResponse{Error: err.Error()}, nil
	}

	return api.GetTicketArtifact200JSONResponse(api.TicketArtifactResponse{
		RepoId:       repoID,
		RepoPath:     repoRoot,
		TicketNumber: request.Id,
		Name:         request.Name,
		Path:         path,
		Content:      string(data),
	}), nil
}

func (s *server) CleanupTicket(ctx context.Context, request api.CleanupTicketRequestObject) (api.CleanupTicketResponseObject, error) {
	if request.Body == nil {
		return badCleanupTicket("invalid json body"), nil
	}
	req := *request.Body
	if strings.TrimSpace(req.RepoPath) == "" {
		return badCleanupTicket("repo_path is required"), nil
	}
	repoRoot, repoID, _, err := s.runtimeForRepoPath(ctx, req.RepoPath)
	if err != nil {
		return badCleanupTicket(err.Error()), nil
	}

	return acceptedCleanupTicket(s.enqueueJob(jobCleanup, repoID, repoRoot, request.Id, enqueueOptions{}))
}

func (s *server) GetTicketEvents(ctx context.Context, request api.GetTicketEventsRequestObject) (api.GetTicketEventsResponseObject, error) {
	repoRoot, repoID, repoRt, err := s.runtimeForRepoPath(ctx, request.Params.RepoPath)
	if err != nil {
		return api.GetTicketEvents400JSONResponse{ErrorResponseJSONResponse: api.ErrorResponseJSONResponse{Error: err.Error()}}, nil
	}
	ticketState, stErr := repoRt.store.LoadState(request.Id)
	var logPath string
	if stErr == nil && ticketState.WorktreePath != "" && ticketState.CurrentState != "" {
		logPath = ticketState.CurrentRunLogPath()
	}
	events, err := parseLogEvents(logPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return api.GetTicketEvents200JSONResponse(api.TicketEventsResponse{
				RepoId:       repoID,
				RepoPath:     repoRoot,
				TicketNumber: request.Id,
				Events:       []api.LogEventResponse{},
			}), nil
		}

		return api.GetTicketEvents500JSONResponse{Error: err.Error()}, nil
	}

	return api.GetTicketEvents200JSONResponse(api.TicketEventsResponse{
		RepoId:       repoID,
		RepoPath:     repoRoot,
		TicketNumber: request.Id,
		Events:       events,
	}), nil
}

func (s *server) GetExecutionLogs(ctx context.Context, request api.GetExecutionLogsRequestObject) (api.GetExecutionLogsResponseObject, error) {
	repoRoot, repoID, repoRt, err := s.runtimeForRepoPath(ctx, request.Params.RepoPath)
	if err != nil {
		return api.GetExecutionLogs400JSONResponse{ErrorResponseJSONResponse: api.ErrorResponseJSONResponse{Error: err.Error()}}, nil
	}
	ticketState, err := repoRt.store.LoadState(request.Id)
	if err != nil {
		return api.GetExecutionLogs404JSONResponse{Error: errMsgTicketNotFound}, nil
	}
	logs := make([]api.ExecutionLogResponse, 0, len(ticketState.StateHistory))
	for _, run := range ticketState.StateHistory {
		runPath := filepath.ToSlash(filepath.Join("runs", run.ID, "raw-provider.log"))
		content, readErr := os.ReadFile(ticketState.ResolveRef(runPath))
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return api.GetExecutionLogs500JSONResponse{Error: readErr.Error()}, nil
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

	return api.GetExecutionLogs200JSONResponse(api.ExecutionLogsResponse{
		RepoId:       repoID,
		RepoPath:     repoRoot,
		TicketNumber: request.Id,
		Logs:         logs,
	}), nil
}

func (s *server) MoveTicketToState(ctx context.Context, request api.MoveTicketToStateRequestObject) (api.MoveTicketToStateResponseObject, error) {
	if request.Body == nil {
		return badMoveTicketToState("invalid json body"), nil
	}
	req := *request.Body
	if strings.TrimSpace(req.RepoPath) == "" {
		return badMoveTicketToState("repo_path is required"), nil
	}
	if strings.TrimSpace(req.Target) == "" {
		return badMoveTicketToState("target is required"), nil
	}
	repoRoot, repoID, _, err := s.runtimeForRepoPath(ctx, req.RepoPath)
	if err != nil {
		return badMoveTicketToState(err.Error()), nil
	}

	return acceptedMoveTicketToState(s.enqueueJob(jobMoveToState, repoID, repoRoot, request.Id, enqueueOptions{
		targetState: req.Target,
	}))
}

func (s *server) RunTicket(ctx context.Context, request api.RunTicketRequestObject) (api.RunTicketResponseObject, error) {
	if request.Body == nil {
		return badRunTicket("invalid json body"), nil
	}
	req := *request.Body
	if strings.TrimSpace(req.RepoPath) == "" {
		return badRunTicket("repo_path is required"), nil
	}
	repoRoot, repoID, _, err := s.runtimeForRepoPath(ctx, req.RepoPath)
	if err != nil {
		return badRunTicket(err.Error()), nil
	}

	return acceptedRunTicket(s.enqueueJob(jobRun, repoID, repoRoot, request.Id, enqueueOptions{}))
}

func (s *server) enqueueJob(action, repoID, repoPath, ticket string, opts enqueueOptions) (api.ActionAcceptedResponse, int, error) {
	if action == jobRun && strings.TrimSpace(ticket) != "" {
		queueErr := s.ensureQueuedTicket(repoID, repoPath, ticket)
		if queueErr != nil {
			return api.ActionAcceptedResponse{}, http.StatusInternalServerError, queueErr
		}
	}
	job, err := s.meta.NewJob(action, repoID, repoPath, ticket, opts.scope)
	if err != nil {
		return api.ActionAcceptedResponse{}, http.StatusInternalServerError, fmt.Errorf("create job: %w", err)
	}
	queued := queuedJob{
		record:      job,
		message:     opts.message,
		actionLabel: opts.actionLabel,
		targetState: opts.targetState,
	}
	s.broadcast(api.ServerEvent{
		Type:         eventTypeJob,
		RepoId:       stringPtr(repoID),
		RepoPath:     stringPtr(repoPath),
		TicketNumber: stringPtr(ticket),
		JobId:        stringPtr(job.ID),
		Action:       stringPtr(action),
		Scope:        stringPtr(opts.scope),
		Status:       stringPtr(serverstate.JobStatusQueued),
	})
	select {
	case s.jobs <- queued:
		return api.ActionAcceptedResponse{
			Status:       "accepted",
			JobId:        job.ID,
			Action:       action,
			RepoId:       repoID,
			RepoPath:     repoPath,
			TicketNumber: stringPtr(ticket),
		}, http.StatusAccepted, nil
	default:
		_ = s.meta.UpdateJobStatus(job.ID, serverstate.JobStatusFailed, "job queue is full")
		s.broadcast(api.ServerEvent{
			Type:         eventTypeJob,
			RepoId:       stringPtr(repoID),
			RepoPath:     stringPtr(repoPath),
			TicketNumber: stringPtr(ticket),
			JobId:        stringPtr(job.ID),
			Action:       stringPtr(action),
			Scope:        stringPtr(opts.scope),
			Status:       stringPtr(serverstate.JobStatusFailed),
			Error:        stringPtr("job queue is full"),
		})

		return api.ActionAcceptedResponse{}, http.StatusServiceUnavailable, errJobQueueFull
	}
}

func acceptedRunTicket(resp api.ActionAcceptedResponse, code int, err error) (api.RunTicketResponseObject, error) {
	switch code {
	case http.StatusAccepted:
		return api.RunTicket202JSONResponse(resp), nil
	case http.StatusInternalServerError:
		return api.RunTicket500JSONResponse{Error: err.Error()}, nil
	case http.StatusServiceUnavailable:
		return api.RunTicket503JSONResponse{Error: err.Error()}, nil
	default:
		return api.RunTicket500JSONResponse{Error: errMsgUnexpectedEnqueue}, nil
	}
}

func acceptedApplyAction(resp api.ActionAcceptedResponse, code int, err error) (api.ApplyActionResponseObject, error) {
	switch code {
	case http.StatusAccepted:
		return api.ApplyAction202JSONResponse(resp), nil
	case http.StatusInternalServerError:
		return api.ApplyAction500JSONResponse{Error: err.Error()}, nil
	case http.StatusServiceUnavailable:
		return api.ApplyAction503JSONResponse{Error: err.Error()}, nil
	default:
		return api.ApplyAction500JSONResponse{Error: errMsgUnexpectedEnqueue}, nil
	}
}

func acceptedMoveTicketToState(resp api.ActionAcceptedResponse, code int, err error) (api.MoveTicketToStateResponseObject, error) {
	switch code {
	case http.StatusAccepted:
		return api.MoveTicketToState202JSONResponse(resp), nil
	case http.StatusInternalServerError:
		return api.MoveTicketToState500JSONResponse{Error: err.Error()}, nil
	case http.StatusServiceUnavailable:
		return api.MoveTicketToState503JSONResponse{Error: err.Error()}, nil
	default:
		return api.MoveTicketToState500JSONResponse{Error: errMsgUnexpectedEnqueue}, nil
	}
}

func acceptedCleanupTicket(resp api.ActionAcceptedResponse, code int, err error) (api.CleanupTicketResponseObject, error) {
	switch code {
	case http.StatusAccepted:
		return api.CleanupTicket202JSONResponse(resp), nil
	case http.StatusInternalServerError:
		return api.CleanupTicket500JSONResponse{Error: err.Error()}, nil
	case http.StatusServiceUnavailable:
		return api.CleanupTicket503JSONResponse{Error: err.Error()}, nil
	default:
		return api.CleanupTicket500JSONResponse{Error: errMsgUnexpectedEnqueue}, nil
	}
}

func acceptedCleanupScope(resp api.ActionAcceptedResponse, code int, err error) (api.CleanupScopeResponseObject, error) {
	switch code {
	case http.StatusAccepted:
		return api.CleanupScope202JSONResponse(resp), nil
	case http.StatusInternalServerError:
		return api.CleanupScope500JSONResponse{Error: err.Error()}, nil
	case http.StatusServiceUnavailable:
		return api.CleanupScope503JSONResponse{Error: err.Error()}, nil
	default:
		return api.CleanupScope500JSONResponse{Error: errMsgUnexpectedEnqueue}, nil
	}
}

func badRunTicket(msg string) api.RunTicket400JSONResponse {
	return api.RunTicket400JSONResponse{ErrorResponseJSONResponse: api.ErrorResponseJSONResponse{Error: msg}}
}

func badApplyAction(msg string) api.ApplyAction400JSONResponse {
	return api.ApplyAction400JSONResponse{ErrorResponseJSONResponse: api.ErrorResponseJSONResponse{Error: msg}}
}

func badMoveTicketToState(msg string) api.MoveTicketToState400JSONResponse {
	return api.MoveTicketToState400JSONResponse{ErrorResponseJSONResponse: api.ErrorResponseJSONResponse{Error: msg}}
}

func badCleanupTicket(msg string) api.CleanupTicket400JSONResponse {
	return api.CleanupTicket400JSONResponse{ErrorResponseJSONResponse: api.ErrorResponseJSONResponse{Error: msg}}
}

func badCleanupScope(msg string) api.CleanupScope400JSONResponse {
	return api.CleanupScope400JSONResponse{ErrorResponseJSONResponse: api.ErrorResponseJSONResponse{Error: msg}}
}
