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
			ID:               run.ID,
			StateName:        run.StateName,
			StateDisplayName: run.StateDisplayName,
			StartedAt:        run.StartedAt,
			ArtifactRef:      run.ArtifactRef,
			LogRef:           run.LogRef,
		})
	}

	return api.TicketStateResponse{
		TicketNumber: st.TicketNumber,
		CurrentState: st.CurrentState,
		CurrentRunID: st.CurrentRunID,
		FlowStatus:   string(st.FlowStatus),
		BranchName:   st.BranchName,
		WorktreePath: st.WorktreePath,
		LastError:    st.LastError,
		PRURL:        st.PRURL,
		StateHistory: history,
		CreatedAt:    st.CreatedAt,
		UpdatedAt:    st.UpdatedAt,
	}
}

func toJobResponse(job serverstate.JobRecord) api.JobStatusResponse {
	return api.JobStatusResponse{
		ID:           job.ID,
		Action:       job.Action,
		RepoID:       job.RepoID,
		RepoPath:     job.RepoPath,
		TicketNumber: job.TicketNumber,
		Status:       job.Status,
		Scope:        job.Scope,
		Error:        job.Error,
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
		RepoID:       rec.RepoID,
		RepoPath:     rec.RepoPath,
		TicketNumber: rec.TicketNumber,
		Title:        rec.Title,
		Status:       rec.Status,
		Busy:         rec.Busy,
		Approved:     rec.Approved,
		LastError:    rec.LastError,
		UpdatedAt:    rec.UpdatedAt,
		PRURL:        rec.PRURL,
		Jobs:         jobs,
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
			DisplayName: st.DisplayName,
		})
	}

	return out
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
