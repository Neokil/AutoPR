import type { ActionInfo, DisplayStateRun, Job, OptimisticTransition, ServerEvent, StateRun, TicketDetails, TicketSummary, WorkflowStateInfo } from "./types";

function asRecord(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return null;
  }
  return value as Record<string, unknown>;
}

function readString(record: Record<string, unknown> | null, key: string): string {
  if (!record) {
    return "";
  }
  const value = record[key];
  return typeof value === "string" ? value : "";
}

export function ticketKey(ticket: TicketSummary): string {
  return `${ticket.repo_id}::${ticket.ticket_number}`;
}

export function pendingTicketKey(repoPath: string, ticketNumber: string): string {
  return `${repoPath}::${ticketNumber}`;
}

export function knownRepoPaths(repositoryOptions: string[], tickets: TicketSummary[]): string[] {
  const seen = new Set<string>();
  const paths: string[] = [];
  for (const path of [...repositoryOptions, ...tickets.map((t) => t.repo_path)]) {
    if (!seen.has(path)) {
      seen.add(path);
      paths.push(path);
    }
  }
  return paths;
}

const terminalStateLabels: Record<string, string> = {
  done: "Done",
  cancelled: "Cancelled",
  failed: "Failed"
};

export function resolveStateDisplayName(workflowStates: WorkflowStateInfo[], stateName: string, fallback = ""): string {
  const workflowState = workflowStates.find((state) => state.name === stateName);
  if (workflowState?.display_name) {
    return workflowState.display_name;
  }
  if (terminalStateLabels[stateName]) {
    return terminalStateLabels[stateName];
  }
  return fallback || stateName;
}

export function stateRunsFromDetails(details: TicketDetails | null, optimistic: OptimisticTransition | null = null): DisplayStateRun[] {
  const runs = details?.state.state_history ?? [];
  if (!details || !optimistic) {
    return runs;
  }
  if (details.repo_path !== optimistic.repo_path || details.ticket_number !== optimistic.ticket_number) {
    return runs;
  }
  if (optimistic.kind === "rerun" && details.state.current_run_id && details.state.current_run_id !== optimistic.previous_current_run_id) {
    return runs;
  }
  if (
    optimistic.kind === "move_to_state" &&
    details.state.current_run_id &&
    details.state.current_run_id !== optimistic.previous_current_run_id &&
    details.state.current_state === optimistic.target_state_name
  ) {
    return runs;
  }

  return [
    ...runs,
    {
      id: optimistic.synthetic_run_id,
      state_name: optimistic.target_state_name,
      state_display_name: optimistic.target_state_display_name,
      started_at: new Date().toISOString(),
      synthetic: true,
      synthetic_status: "running"
    }
  ];
}

export function getFeedbackAction(details: TicketDetails | null, selectedSummary: TicketSummary | null): ActionInfo | undefined {
  if (selectedSummary?.status !== "waiting") {
    return undefined;
  }
  return (details?.available_actions ?? []).find((action) => action.type === "provide_feedback");
}

export function getNonFeedbackActions(details: TicketDetails | null, selectedSummary: TicketSummary | null): ActionInfo[] {
  if (selectedSummary?.status !== "waiting") {
    return [];
  }
  return (details?.available_actions ?? []).filter((action) => action.type !== "provide_feedback");
}

export function ticketTitle(details: TicketDetails | null, selectedSummary: TicketSummary | null): string {
  const ticket = asRecord(details?.ticket);
  return readString(ticket, "title") || selectedSummary?.title || "(no title)";
}

export function ticketURL(details: TicketDetails | null): string {
  return readString(asRecord(details?.ticket), "url");
}

export function runDisplayLabel(run: StateRun, runs: StateRun[]): string {
  const base = run.state_display_name || run.state_name;
  const matching = runs.filter((candidate) => candidate.state_name === run.state_name);
  if (matching.length <= 1) {
    return base;
  }
  const index = matching.findIndex((candidate) => candidate.id === run.id);
  return `${base} ${index + 1}`;
}

export function projectedTicketStatusLabel(ticket: TicketSummary, optimistic: OptimisticTransition | null): string {
  if (optimistic && optimistic.ticket_key === ticketKey(ticket)) {
    return optimistic.target_state_display_name;
  }
  return ticket.status;
}

export function projectedTicketBusy(ticket: TicketSummary, optimistic: OptimisticTransition | null): boolean {
  if (optimistic && optimistic.ticket_key === ticketKey(ticket)) {
    return true;
  }
  return ticket.busy;
}

export function selectTicketKey(current: string, tickets: TicketSummary[]): string {
  if (tickets.length === 0) {
    return "";
  }
  if (!current || !tickets.some((ticket) => ticketKey(ticket) === current)) {
    return ticketKey(tickets[0]);
  }
  return current;
}

export function applyTicketEvent(current: TicketSummary[], evt: ServerEvent): {
  tickets: TicketSummary[];
  needsFullRefresh: boolean;
} {
  if (!evt.repo_id || !evt.ticket_number) {
    return { tickets: current, needsFullRefresh: false };
  }

  const key = `${evt.repo_id}::${evt.ticket_number}`;
  if (evt.type === "ticket_deleted") {
    return {
      tickets: current.filter((ticket) => ticketKey(ticket) !== key),
      needsFullRefresh: false
    };
  }

  let found = false;
  const next = current.map((ticket) => {
    if (ticketKey(ticket) !== key) {
      return ticket;
    }
    found = true;
    if (evt.type === "job") {
      const nextJobs = [...(ticket.jobs ?? [])];
      const jobIndex = evt.job_id ? nextJobs.findIndex((job) => job.id === evt.job_id) : -1;
      const nextJob =
        jobIndex >= 0
          ? { ...nextJobs[jobIndex], status: (evt.status as Job["status"]) ?? nextJobs[jobIndex].status, error: evt.error }
          : {
            id: evt.job_id ?? "",
            action: evt.action ?? "",
            repo_id: evt.repo_id ?? ticket.repo_id,
            repo_path: evt.repo_path ?? ticket.repo_path,
            ticket_number: evt.ticket_number,
            status: (evt.status as Job["status"]) ?? "queued",
            scope: evt.scope,
            error: evt.error,
            created_at: new Date().toISOString()
          };
      if (jobIndex >= 0) {
        nextJobs[jobIndex] = nextJob;
      } else if (nextJob.id) {
        nextJobs.unshift(nextJob);
      }
      nextJobs.sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at));
      const isBusy = nextJobs.some((job) => job.status === "queued" || job.status === "running");
      return { ...ticket, busy: isBusy, jobs: nextJobs };
    }
    if (evt.type === "ticket_updated") {
      const nextStatus = (evt.status as TicketSummary["status"]) ?? ticket.status;
      return {
        ...ticket,
        title: evt.title ?? ticket.title,
        status: nextStatus,
        busy: nextStatus === "running" ? ticket.busy : false,
        last_error: evt.error ?? ticket.last_error,
        pr_url: evt.pr_url ?? ticket.pr_url,
        updated_at: new Date().toISOString()
      };
    }
    return ticket;
  });

  return {
    tickets: next,
    needsFullRefresh: !found && evt.type === "ticket_updated"
  };
}
