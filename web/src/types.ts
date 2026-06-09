import type { components } from "./generated/api";

export type JobStatus = components["schemas"]["JobStatus"];
export type FlowStatus = components["schemas"]["FlowStatus"];
export type TicketSummary = components["schemas"]["TicketSummaryResponse"];
export type TicketState = components["schemas"]["TicketStateResponse"];
export type StateRun = components["schemas"]["StateRunResponse"];
export type ActionInfo = components["schemas"]["ActionInfo"];
export type WorkflowStateInfo = components["schemas"]["WorkflowStateInfo"];
export type TicketDetails = components["schemas"]["TicketDetailsResponse"];
export type ExecutionLog = components["schemas"]["ExecutionLogResponse"];
export type Job = components["schemas"]["JobStatusResponse"];
export type AcceptedJob = components["schemas"]["ActionAcceptedResponse"];
export type ServerEvent = components["schemas"]["ServerEvent"];
export type RepositoryListResponse = components["schemas"]["RepositoryListResponse"];
export type DiscoveredTicket = components["schemas"]["DiscoveredTicket"];
export type HealthResponse = components["schemas"]["HealthResponse"];

export type DisplayStateRun = StateRun & {
  synthetic?: boolean;
  synthetic_status?: "queued" | "running";
};

export type OptimisticTransition = {
  ticket_key: string;
  repo_path: string;
  ticket_number: string;
  job_id: string;
  synthetic_run_id: string;
  target_state_name: string;
  target_state_display_name: string;
  previous_selected_run_id: string;
  previous_current_run_id: string;
  kind: "move_to_state" | "rerun";
};
