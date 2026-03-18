export type JobStatus = "queued" | "running" | "done" | "failed";

export interface TicketSummary {
  repo_id: string;
  repo_path: string;
  ticket_number: string;
  title?: string;
  status: string;
  busy: boolean;
  approved: boolean;
  updated_at: string;
  pr_url?: string;
  jobs?: Job[];
}

export interface TicketState {
  ticket_number: string;
  branch_name: string;
  worktree_path: string;
  status: string;
  approved: boolean;
  fix_attempts: number;
  last_error?: string;
  last_feedback?: string;
  created_at: string;
  updated_at: string;
  proposal_path: string;
  final_solution_path: string;
  log_path: string;
  pr_path: string;
  checks_log_path: string;
  ticket_json_path: string;
  provider_dir_path: string;
  pr_url?: string;
}

export interface TicketDetails {
  repo_id: string;
  repo_path: string;
  ticket_number: string;
  state: TicketState;
  ticket?: Record<string, unknown> & {
    title?: string;
  };
  next_steps?: string;
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
  status?: string;
  job_id?: string;
  action?: string;
  scope?: string;
  error?: string;
}

export interface RepositoryListResponse {
  repositories: string[];
}
