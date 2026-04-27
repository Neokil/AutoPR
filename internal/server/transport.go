package server

import (
	"github.com/Neokil/AutoPR/internal/api"
	workflowstate "github.com/Neokil/AutoPR/internal/domain/workflowstate"
	"github.com/Neokil/AutoPR/internal/serverstate"
)

func toTicketStateResponse(st workflowstate.State) api.TicketStateResponse {
	history := make([]api.StateRunResponse, 0, len(st.StateHistory))
	for _, run := range st.StateHistory {
		history = append(history, api.StateRunResponse{
			Id:               run.ID,
			StateName:        run.StateName,
			StateDisplayName: stringPtr(run.StateDisplayName),
			StartedAt:        run.StartedAt,
			ArtifactRef:      stringPtr(run.ArtifactRef),
			LogRef:           stringPtr(run.LogRef),
		})
	}

	return api.TicketStateResponse{
		TicketNumber: st.TicketNumber,
		CurrentState: st.CurrentState,
		CurrentRunId: stringPtr(st.CurrentRunID),
		FlowStatus:   api.FlowStatus(st.FlowStatus),
		BranchName:   st.BranchName,
		WorktreePath: st.WorktreePath,
		LastError:    stringPtr(st.LastError),
		PrUrl:        stringPtr(st.PRURL),
		StateHistory: slicePtr(history),
		CreatedAt:    st.CreatedAt,
		UpdatedAt:    st.UpdatedAt,
	}
}

func toJobResponse(job serverstate.JobRecord) api.JobStatusResponse {
	return api.JobStatusResponse{
		Id:           job.ID,
		Action:       job.Action,
		RepoId:       job.RepoID,
		RepoPath:     job.RepoPath,
		TicketNumber: stringPtr(job.TicketNumber),
		Status:       api.JobStatus(job.Status),
		Scope:        stringPtr(job.Scope),
		Error:        stringPtr(job.Error),
		CreatedAt:    job.CreatedAt,
		StartedAt:    job.StartedAt,
		FinishedAt:   job.FinishedAt,
	}
}

func toTicketSummaryResponse(rec serverstate.TicketRecord) api.TicketSummaryResponse {
	jobs := make([]api.JobStatusResponse, 0, len(rec.Jobs))
	for _, job := range rec.Jobs {
		jobs = append(jobs, toJobResponse(job))
	}

	return api.TicketSummaryResponse{
		RepoId:       rec.RepoID,
		RepoPath:     rec.RepoPath,
		TicketNumber: rec.TicketNumber,
		Title:        stringPtr(rec.Title),
		Status:       api.FlowStatus(rec.Status),
		Busy:         rec.Busy,
		Approved:     rec.Approved,
		LastError:    stringPtr(rec.LastError),
		UpdatedAt:    rec.UpdatedAt,
		PrUrl:        stringPtr(rec.PRURL),
		Jobs:         slicePtr(jobs),
	}
}

func toTicketSummaryResponses(records []serverstate.TicketRecord) []api.TicketSummaryResponse {
	out := make([]api.TicketSummaryResponse, 0, len(records))
	for _, rec := range records {
		out = append(out, toTicketSummaryResponse(rec))
	}

	return out
}

func toWorkflowStateResponses(states []workflowStateInfo) []api.WorkflowStateInfo {
	out := make([]api.WorkflowStateInfo, 0, len(states))
	for _, st := range states {
		out = append(out, api.WorkflowStateInfo{
			Name:        st.Name,
			DisplayName: stringPtr(st.DisplayName),
		})
	}

	return out
}

func stringPtr(v string) *string {
	if v == "" {
		return nil
	}

	return &v
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}

	return *v
}

func slicePtr[T any](v []T) *[]T {
	if len(v) == 0 {
		return nil
	}

	return &v
}

func toActionResponses(actions []actionInfo) []api.ActionInfo {
	out := make([]api.ActionInfo, 0, len(actions))
	for _, action := range actions {
		out = append(out, api.ActionInfo{
			Label: action.Label,
			Type:  action.Type,
		})
	}

	return out
}
