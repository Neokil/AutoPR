import type { ActionInfo, Job, ServerEvent, StateRun, TicketDetails, TicketSummary } from "./types";

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

export function stateRunsFromDetails(details: TicketDetails | null): StateRun[] {
  return details?.state.state_history ?? [];
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
      return {
        ...ticket,
        title: evt.title ?? ticket.title,
        status: (evt.status as TicketSummary["status"]) ?? ticket.status,
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
