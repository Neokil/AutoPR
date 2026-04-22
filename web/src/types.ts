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
  branch_name: string;
  worktree_path: string;
  last_error?: string;
  pr_url?: string;
  created_at: string;
  updated_at: string;
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
