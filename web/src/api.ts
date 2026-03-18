import type { AcceptedJob, EventItem, Job, RepositoryListResponse, ServerEvent, TicketDetails, TicketSummary } from "./types";

const API_BASE = (import.meta.env.VITE_API_BASE_URL as string | undefined) ?? "";

function apiUrl(path: string): string {
  return `${API_BASE}${path}`;
}

async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(apiUrl(path), init);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    const msg = typeof data?.error === "string" ? data.error : `HTTP ${res.status}`;
    throw new Error(msg);
  }
  return data as T;
}

export async function listTickets(repoPath?: string): Promise<TicketSummary[]> {
  const query = repoPath ? `?repo_path=${encodeURIComponent(repoPath)}` : "";
  const data = await requestJSON<{ tickets: TicketSummary[] }>(`/api/tickets${query}`);
  return data.tickets ?? [];
}

export async function listRepositories(): Promise<string[]> {
  const data = await requestJSON<RepositoryListResponse>("/api/repositories");
  return data.repositories ?? [];
}

export async function getTicket(repoPath: string, ticket: string): Promise<TicketDetails> {
  return requestJSON<TicketDetails>(
    `/api/tickets/${encodeURIComponent(ticket)}?repo_path=${encodeURIComponent(repoPath)}`
  );
}

export async function getEvents(repoPath: string, ticket: string): Promise<EventItem[]> {
  const data = await requestJSON<{ events: EventItem[] }>(
    `/api/tickets/${encodeURIComponent(ticket)}/events?repo_path=${encodeURIComponent(repoPath)}`
  );
  return data.events ?? [];
}

export async function getArtifact(repoPath: string, ticket: string, name: string): Promise<string> {
  const data = await requestJSON<{ content: string }>(
    `/api/tickets/${encodeURIComponent(ticket)}/artifacts/${encodeURIComponent(name)}?repo_path=${encodeURIComponent(repoPath)}`
  );
  return data.content ?? "";
}

export async function getJob(jobId: string): Promise<Job> {
  return requestJSON<Job>(`/api/jobs/${encodeURIComponent(jobId)}`);
}

export function connectEvents(
  onEvent: (event: ServerEvent) => void,
  onError?: () => void,
  onOpen?: () => void
): EventSource {
  const es = new EventSource(apiUrl("/api/events"));
  const handler = (evt: MessageEvent<string>) => {
    try {
      const parsed = JSON.parse(evt.data) as ServerEvent;
      onEvent(parsed);
    } catch {
      // ignore invalid events
    }
  };
  es.addEventListener("job", handler as EventListener);
  es.addEventListener("ticket_updated", handler as EventListener);
  es.addEventListener("ticket_deleted", handler as EventListener);
  es.addEventListener("repo_tickets_synced", handler as EventListener);
  if (onError) {
    es.onerror = onError;
  }
  if (onOpen) {
    es.onopen = onOpen;
  }
  return es;
}

function postAccepted<TBody>(path: string, body: TBody): Promise<AcceptedJob> {
  return requestJSON<AcceptedJob>(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body)
  });
}

export function runTicket(repoPath: string, ticket: string): Promise<AcceptedJob> {
  return postAccepted(`/api/tickets/${encodeURIComponent(ticket)}/run`, { repo_path: repoPath });
}

export function resumeTicket(repoPath: string, ticket: string): Promise<AcceptedJob> {
  return postAccepted(`/api/tickets/${encodeURIComponent(ticket)}/resume`, { repo_path: repoPath });
}

export function approveTicket(repoPath: string, ticket: string): Promise<AcceptedJob> {
  return postAccepted(`/api/tickets/${encodeURIComponent(ticket)}/approve`, { repo_path: repoPath });
}

export function rejectTicket(repoPath: string, ticket: string): Promise<AcceptedJob> {
  return postAccepted(`/api/tickets/${encodeURIComponent(ticket)}/reject`, { repo_path: repoPath });
}

export function createPR(repoPath: string, ticket: string): Promise<AcceptedJob> {
  return postAccepted(`/api/tickets/${encodeURIComponent(ticket)}/pr`, { repo_path: repoPath });
}

export function applyPRComments(repoPath: string, ticket: string): Promise<AcceptedJob> {
  return postAccepted(`/api/tickets/${encodeURIComponent(ticket)}/apply-pr-comments`, { repo_path: repoPath });
}

export function cleanupTicket(repoPath: string, ticket: string): Promise<AcceptedJob> {
  return postAccepted(`/api/tickets/${encodeURIComponent(ticket)}/cleanup`, { repo_path: repoPath });
}

export function feedbackTicket(repoPath: string, ticket: string, message: string): Promise<AcceptedJob> {
  return postAccepted(`/api/tickets/${encodeURIComponent(ticket)}/feedback`, {
    repo_path: repoPath,
    message
  });
}

export function cleanupDone(repoPath: string): Promise<AcceptedJob> {
  return postAccepted("/api/cleanup", { repo_path: repoPath, scope: "done" });
}

export function cleanupAll(repoPath: string): Promise<AcceptedJob> {
  return postAccepted("/api/cleanup", { repo_path: repoPath, scope: "all" });
}
