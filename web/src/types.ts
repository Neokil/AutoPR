export type JobStatus = "queued" | "running" | "done" | "failed";

export type FlowStatus = "pending" | "running" | "waiting" | "done" | "failed" | "cancelled";

export interface TicketSummary {
  repo_id: string;
  repo_path: string;
  ticket_number: string;
  title?: string;
  status: FlowStatus;
  busy: boolean;
  last_error?: string;
  updated_at: string;
  pr_url?: string;
  jobs?: Job[];
}

export interface TicketState {
  ticket_number: string;
  flow_status: FlowStatus;
  current_state: string;
  current_run_id?: string;
  branch_name: string;
  worktree_path: string;
  last_error?: string;
  pr_url?: string;
  state_history?: StateRun[];
  created_at: string;
  updated_at: string;
}

export interface StateRun {
  id: string;
  state_name: string;
  state_display_name?: string;
  started_at: string;
  artifact_ref?: string;
  log_ref?: string;
}

export interface ActionInfo {
  label: string;
  type: string;
}

export interface TicketDetails {
  repo_id: string;
  repo_path: string;
  ticket_number: string;
  github_blob_base?: string;
  state: TicketState;
  ticket?: Record<string, unknown> & {
    title?: string;
  };
  next_steps?: string;
  available_actions: ActionInfo[];
}

export interface EventItem {
  title: string;
  timestamp: string;
  body: string;
}

export interface ExecutionLog {
  run_id: string;
  state: string;
  state_display_name?: string;
  timestamp: string;
  path: string;
  content: string;
}

export interface Job {
  id: string;
  action: string;
  repo_id: string;
  repo_path: string;
  ticket_number?: string;
  status: JobStatus;
  scope?: string;
  error?: string;
  created_at: string;
  started_at?: string;
  finished_at?: string;
}

export interface AcceptedJob {
  status: "accepted";
  job_id: string;
  action: string;
  repo_id: string;
  repo_path: string;
  ticket_number?: string;
}

export interface ServerEvent {
  type: string;
  repo_id?: string;
  repo_path?: string;
  ticket_number?: string;
  title?: string;
  status?: string;
  job_id?: string;
  action?: string;
  scope?: string;
  error?: string;
  pr_url?: string;
}

export interface RepositoryListResponse {
  repositories: string[];
}
