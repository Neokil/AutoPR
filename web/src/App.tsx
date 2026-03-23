import { useEffect, useMemo, useRef, useState } from "react";
import {
  approveTicket,
  applyPRComments,
  cleanupAll,
  cleanupDone,
  cleanupTicket,
  connectEvents,
  createPR,
  feedbackTicket,
  getArtifact,
  getJob,
  getTicket,
  listTickets,
  listRepositories,
  rejectTicket,
  resumeTicket,
  runTicket
} from "./api";
import { MarkdownView } from "./MarkdownView";
import { TicketList } from "./TicketList";
import type { Job, ServerEvent, TicketDetails, TicketSummary } from "./types";

function ticketKey(t: TicketSummary): string {
  return `${t.repo_id}::${t.ticket_number}`;
}

function pendingTicketKey(repoPath: string, ticketNumber: string): string {
  return `${repoPath}::${ticketNumber}`;
}

type Action = "run" | "resume" | "approve" | "reject" | "pr" | "apply_pr_comments" | "cleanup";

function allowedActions(status: string): Action[] {
  switch (status) {
    case "queued":
      return ["run", "cleanup"];
    case "proposal_ready":
    case "waiting_for_human":
      return ["approve", "reject"];
    case "failed":
      return ["resume", "cleanup"];
    case "pr_ready":
      return ["pr", "apply_pr_comments", "cleanup"];
    case "done":
      return ["apply_pr_comments", "cleanup"];
    case "investigating":
    case "implementing":
    case "validating":
    default:
      return ["resume", "cleanup"];
  }
}

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

function firstLinkedItem(ticket: Record<string, unknown> | null, keys: string[]): Record<string, unknown> | null {
  if (!ticket) {
    return null;
  }
  for (const key of keys) {
    const candidate = asRecord(ticket[key]);
    if (candidate) {
      return candidate;
    }
  }
  return null;
}

export function App() {
  const [tickets, setTickets] = useState<TicketSummary[]>([]);
  const [selectedKey, setSelectedKey] = useState<string>("");
  const [details, setDetails] = useState<TicketDetails | null>(null);
  const [proposal, setProposal] = useState("");
  const [logText, setLogText] = useState("");
  const [activeTab, setActiveTab] = useState<"details" | "proposal" | "logs">("details");
  const [feedbackMessage, setFeedbackMessage] = useState("");
  const [activeJobId, setActiveJobId] = useState<string>("");
  const [activeJob, setActiveJob] = useState<Job | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>("");
  const [repositoryOptions, setRepositoryOptions] = useState<string[]>([]);
  const [showAddTicketDialog, setShowAddTicketDialog] = useState(false);
  const [newTicketRepoPath, setNewTicketRepoPath] = useState("");
  const [newTicketNumber, setNewTicketNumber] = useState("");
  const [pendingAddedTickets, setPendingAddedTickets] = useState<string[]>([]);
  const [addTicketError, setAddTicketError] = useState("");

  const selectedSummary = useMemo(() => tickets.find((t) => ticketKey(t) === selectedKey) ?? null, [tickets, selectedKey]);
  const knownRepoPaths = useMemo(() => {
    const seen = new Set<string>();
    const paths: string[] = [...repositoryOptions];
    for (const p of repositoryOptions) {
      seen.add(p);
    }
    for (const t of tickets) {
      if (!seen.has(t.repo_path)) {
        seen.add(t.repo_path);
        paths.push(t.repo_path);
      }
    }
    return paths;
  }, [repositoryOptions, tickets]);
  const ticketRecord = asRecord(details?.ticket);
  const ticketTitle = readString(ticketRecord, "title") || selectedSummary?.title || "(no title)";
  const ticketURL = readString(ticketRecord, "url");
  const ticketDescription = readString(ticketRecord, "description");
  const acceptanceCriteria = readString(ticketRecord, "acceptance_criteria");
  const priority = readString(ticketRecord, "priority");
  const parentTicket = firstLinkedItem(ticketRecord, ["parent_ticket"]);
  const epicTicket = firstLinkedItem(ticketRecord, ["epic", "epic_ticket", "parent_epic", "epic_story"]);
  const selectedSummaryRef = useRef<TicketSummary | null>(null);
  const activeJobIdRef = useRef<string>("");
  const fullRefreshScheduledRef = useRef(false);
  const reconnectErrorMessage = "event stream connection lost; reconnecting";

  useEffect(() => {
    selectedSummaryRef.current = selectedSummary;
  }, [selectedSummary]);

  useEffect(() => {
    activeJobIdRef.current = activeJobId;
  }, [activeJobId]);

  useEffect(() => {
    void refreshTickets();
    void refreshRepositories();
    const stream = connectEvents(
      (evt) => {
        void handleServerEvent(evt);
      },
      () => {
        setError(reconnectErrorMessage);
      },
      () => {
        setError((current) => (current === reconnectErrorMessage ? "" : current));
      }
    );
    return () => stream.close();
  }, []);

  useEffect(() => {
    if (!selectedSummary) {
      setDetails(null);
      setProposal("");
      setLogText("");
      return;
    }
    void refreshTicketDetails(selectedSummary.repo_path, selectedSummary.ticket_number);
    setActiveTab("details");
  }, [selectedSummary?.repo_path, selectedSummary?.ticket_number]);

  async function refreshTickets(showLoader = true) {
    if (showLoader) {
      setLoading(true);
    }
    try {
      const data = await listTickets();
      setTickets(data);
      setSelectedKey((current) => {
        if (data.length === 0) {
          return "";
        }
        if (!current || !data.some((t) => ticketKey(t) === current)) {
          return ticketKey(data[0]);
        }
        return current;
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to load tickets");
    } finally {
      if (showLoader) {
        setLoading(false);
      }
    }
  }

  async function refreshRepositories() {
    try {
      const repos = await listRepositories();
      setRepositoryOptions(repos);
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to load repositories");
    }
  }

  async function refreshTicketDetails(repoPath: string, ticket: string, showLoader = true) {
    if (showLoader) {
      setLoading(true);
    }
    setError("");
    try {
      const [ticketDetails, proposalText, logs] = await Promise.all([
        getTicket(repoPath, ticket),
        getArtifact(repoPath, ticket, "proposal"),
        getArtifact(repoPath, ticket, "log")
      ]);
      setDetails(ticketDetails);
      setProposal(proposalText);
      setLogText(logs);
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to load ticket details");
      setDetails(null);
      setProposal("");
      setLogText("");
    } finally {
      if (showLoader) {
        setLoading(false);
      }
    }
  }

  async function handleServerEvent(evt: ServerEvent) {
    const selected = selectedSummaryRef.current;
    const trackedJobID = activeJobIdRef.current;
    if (evt.type === "job" && evt.job_id && trackedJobID && evt.job_id === trackedJobID) {
      const status = evt.status ?? "";
      if (status === "failed") {
        try {
          const job = await getJob(evt.job_id);
          setActiveJob(job);
        } catch (err) {
          setError(err instanceof Error ? err.message : "failed to refresh job");
        } finally {
          setActiveJobId("");
        }
      } else if (status === "done") {
        setActiveJob(null);
        setActiveJobId("");
      } else if (status === "queued" || status === "running") {
        setActiveJob((current) => {
          if (!current) {
            return current;
          }
          return { ...current, status };
        });
      }
    }

    applyTicketEvent(evt);

    if (evt.type === "repo_tickets_synced") {
      scheduleFullRefresh();
    }

    if (
      selected &&
      evt.type === "ticket_updated" &&
      evt.repo_path === selected.repo_path &&
      evt.ticket_number === selected.ticket_number
    ) {
      await refreshTicketDetails(selected.repo_path, selected.ticket_number, false);
    }
  }

  function applyTicketEvent(evt: ServerEvent) {
    if (!evt.repo_id || !evt.ticket_number) {
      return;
    }
    const key = `${evt.repo_id}::${evt.ticket_number}`;
    if (evt.type === "ticket_deleted") {
      setTickets((current) => current.filter((t) => ticketKey(t) !== key));
      return;
    }
    setTickets((current) => {
      let found = false;
      const next = current.map((t) => {
        if (ticketKey(t) !== key) {
          return t;
        }
        found = true;
        if (evt.type === "job") {
          const nextJobs = [...(t.jobs ?? [])];
          const jobIndex = evt.job_id ? nextJobs.findIndex((job) => job.id === evt.job_id) : -1;
          const nextJob: Job = jobIndex >= 0
            ? {
                ...nextJobs[jobIndex],
                status: evt.status === "queued" || evt.status === "running" || evt.status === "done" || evt.status === "failed"
                  ? evt.status
                  : nextJobs[jobIndex].status,
                action: evt.action ?? nextJobs[jobIndex].action,
                error: evt.error ?? nextJobs[jobIndex].error
              }
            : {
                id: evt.job_id ?? "",
                action: evt.action ?? "",
                repo_id: evt.repo_id ?? t.repo_id,
                repo_path: evt.repo_path ?? t.repo_path,
                ticket_number: evt.ticket_number ?? t.ticket_number,
                status:
                  evt.status === "queued" || evt.status === "running" || evt.status === "done" || evt.status === "failed"
                    ? evt.status
                    : "queued",
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
          return { ...t, busy: isBusy, jobs: nextJobs };
        }
        if (evt.type === "ticket_updated") {
          return {
            ...t,
            title: evt.title ?? t.title,
            status: evt.status ?? t.status,
            last_error: evt.error ?? t.last_error,
            updated_at: new Date().toISOString()
          };
        }
        return t;
      });
      if (!found && evt.type === "ticket_updated") {
        scheduleFullRefresh();
      }
      return next;
    });
  }

  function scheduleFullRefresh() {
    if (fullRefreshScheduledRef.current) {
      return;
    }
    fullRefreshScheduledRef.current = true;
    window.setTimeout(() => {
      fullRefreshScheduledRef.current = false;
      void refreshTickets(false);
    }, 250);
  }

  async function queueAction(fn: () => Promise<{ job_id: string }>): Promise<boolean> {
    setError("");
    try {
      const accepted = await fn();
      setActiveJobId(accepted.job_id);
      setActiveJob(null);
      return true;
    } catch (err) {
      setError(err instanceof Error ? err.message : "action failed");
      return false;
    }
  }

  async function submitAddTicket() {
    const repoPath = newTicketRepoPath.trim();
    const ticketNumber = newTicketNumber.trim();
    setAddTicketError("");
    if (!repoPath || !ticketNumber) {
      setAddTicketError("repo folder path and ticket number are required");
      return;
    }
    const pendingKey = pendingTicketKey(repoPath, ticketNumber);
    if (pendingAddedTickets.includes(pendingKey)) {
      setAddTicketError(`ticket ${ticketNumber} is already being added to AutoPR for this repository`);
      return;
    }
    try {
      const repoTickets = await listTickets(repoPath);
      const existing = repoTickets.find((t) => t.ticket_number === ticketNumber);
      if (existing) {
        setAddTicketError(`ticket ${ticketNumber} is already added to AutoPR for this repository`);
        return;
      }
    } catch (err) {
      setAddTicketError(err instanceof Error ? err.message : "failed to validate ticket");
      return;
    }
    setPendingAddedTickets((current) => [...current, pendingKey]);
    const ok = await queueAction(() => runTicket(repoPath, ticketNumber));
    setPendingAddedTickets((current) => current.filter((key) => key !== pendingKey));
    if (ok) {
      setShowAddTicketDialog(false);
      setNewTicketRepoPath("");
      setNewTicketNumber("");
      scheduleFullRefresh();
    }
  }

  return (
    <div className="app">
      <header className="header">
        <h1 className="brand">
          <img src="/autopr-logo-with-text.png" alt="AutoPR" className="brand-logo-text" />
        </h1>
        <div className="button-row">
          <button onClick={() => void refreshTickets()} disabled={loading}>
            Refresh All Tickets
          </button>
          <button
            onClick={() => selectedSummary && void queueAction(() => cleanupDone(selectedSummary.repo_path))}
            disabled={!selectedSummary}
          >
            Cleanup Done
          </button>
          <button
            onClick={() => selectedSummary && void queueAction(() => cleanupAll(selectedSummary.repo_path))}
            disabled={!selectedSummary}
          >
            Cleanup All
          </button>
        </div>
      </header>

      {error ? <div className="banner error">{error}</div> : null}
      {activeJob ? (
        <div className={`banner ${activeJob.status === "failed" ? "error" : "info"}`}>
          Job `{activeJob.id}`: {activeJob.action} ({activeJob.status})
          {activeJob.error ? ` - ${activeJob.error}` : ""}
        </div>
      ) : null}

      <main className="main">
        <TicketList
          tickets={tickets}
          selectedKey={selectedKey}
          onSelectTicket={setSelectedKey}
          onAddTicket={() => {
            setError("");
            setAddTicketError("");
            setNewTicketRepoPath(selectedSummary?.repo_path ?? "");
            setNewTicketNumber("");
            setShowAddTicketDialog(true);
            void refreshRepositories();
          }}
        />

        <section className="panel right">
          {selectedSummary ? (
            <>
              <div className="detail-top-row">
                <h2 className="detail-main-title">
                  {ticketURL ? (
                    <a href={ticketURL} target="_blank" rel="noreferrer">
                      {selectedSummary.ticket_number} - {ticketTitle} ({selectedSummary.status})
                    </a>
                  ) : (
                    <>
                      {selectedSummary.ticket_number} - {ticketTitle} ({selectedSummary.status})
                    </>
                  )}
                </h2>
                <div className="button-row detail-actions">
                {allowedActions(selectedSummary.status).includes("run") ? (
                  <button onClick={() => void queueAction(() => runTicket(selectedSummary.repo_path, selectedSummary.ticket_number))}>
                    Run
                  </button>
                ) : null}
                {allowedActions(selectedSummary.status).includes("resume") ? (
                  <button onClick={() => void queueAction(() => resumeTicket(selectedSummary.repo_path, selectedSummary.ticket_number))}>
                    Resume
                  </button>
                ) : null}
                {allowedActions(selectedSummary.status).includes("approve") ? (
                  <button onClick={() => void queueAction(() => approveTicket(selectedSummary.repo_path, selectedSummary.ticket_number))}>
                    Approve
                  </button>
                ) : null}
                {allowedActions(selectedSummary.status).includes("reject") ? (
                  <button onClick={() => void queueAction(() => rejectTicket(selectedSummary.repo_path, selectedSummary.ticket_number))}>
                    Reject
                  </button>
                ) : null}
                {allowedActions(selectedSummary.status).includes("pr") ? (
                  <button onClick={() => void queueAction(() => createPR(selectedSummary.repo_path, selectedSummary.ticket_number))}>
                    Create PR
                  </button>
                ) : null}
                {allowedActions(selectedSummary.status).includes("apply_pr_comments") ? (
                  <button
                    onClick={() =>
                      void queueAction(() => applyPRComments(selectedSummary.repo_path, selectedSummary.ticket_number))
                    }
                    disabled={!selectedSummary.pr_url}
                  >
                    Apply PR Comments
                  </button>
                ) : null}
                {allowedActions(selectedSummary.status).includes("cleanup") ? (
                  <button onClick={() => void queueAction(() => cleanupTicket(selectedSummary.repo_path, selectedSummary.ticket_number))}>
                    Cleanup
                  </button>
                ) : null}
                </div>
              </div>

              <div className="tabs">
                <button className={activeTab === "details" ? "tab active" : "tab"} onClick={() => setActiveTab("details")}>
                  Details
                </button>
                <button className={activeTab === "proposal" ? "tab active" : "tab"} onClick={() => setActiveTab("proposal")}>
                  Proposal
                </button>
                <button className={activeTab === "logs" ? "tab active" : "tab"} onClick={() => setActiveTab("logs")}>
                  Logs
                </button>
              </div>

              {activeTab === "details" ? (
                <article className="card">
                  <div className="ticket-fields">
                    <section className="ticket-section">
                      <span className="field-label">Epic</span>
                      {epicTicket && readString(epicTicket, "url") ? (
                        <a href={readString(epicTicket, "url")} target="_blank" rel="noreferrer">
                          {readString(epicTicket, "title") || "(no title)"}
                        </a>
                      ) : (
                        <span className="meta">No epic ticket</span>
                      )}
                    </section>

                    <section className="ticket-section">
                      <span className="field-label">Parent Ticket</span>
                      {parentTicket && readString(parentTicket, "url") ? (
                        <a href={readString(parentTicket, "url")} target="_blank" rel="noreferrer">
                          {readString(parentTicket, "title") || "(no title)"}
                        </a>
                      ) : (
                        <span className="meta">No parent ticket</span>
                      )}
                    </section>

                    <section className="ticket-section">
                      <span className="field-label">Description</span>
                      <MarkdownView content={ticketDescription} emptyText="No description." />
                    </section>

                    <section className="ticket-section">
                      <span className="field-label">Acceptance Criteria</span>
                      <MarkdownView content={acceptanceCriteria} emptyText="No acceptance criteria." />
                    </section>

                    <section className="ticket-section">
                      <span className="field-label">Priority</span>
                      <span>{priority || "-"}</span>
                    </section>

                    <section className="ticket-section">
                      <span className="field-label">Last Error</span>
                      <MarkdownView content={details?.state.last_error ?? ""} emptyText="No error recorded." />
                    </section>

                    <section className="ticket-section">
                      <span className="field-label">Next Steps</span>
                      <MarkdownView content={details?.next_steps ?? ""} emptyText="No next steps available." />
                    </section>
                  </div>
                </article>
              ) : null}

              {activeTab === "proposal" ? (
                <>
                  {selectedSummary.status === "proposal_ready" || selectedSummary.status === "waiting_for_human" ? (
                    <form
                      className="feedback-form"
                      onSubmit={(e) => {
                        e.preventDefault();
                        if (!feedbackMessage.trim()) {
                          return;
                        }
                        void queueAction(() =>
                          feedbackTicket(selectedSummary.repo_path, selectedSummary.ticket_number, feedbackMessage)
                        ).then(() => setFeedbackMessage(""));
                      }}
                    >
                      <input
                        value={feedbackMessage}
                        onChange={(e) => setFeedbackMessage(e.target.value)}
                        placeholder="Send feedback on proposal"
                      />
                      <button type="submit">Send Feedback</button>
                    </form>
                  ) : (
                    <p className="meta">Feedback is available in proposal phase only.</p>
                  )}
                  <article className="card">
                    <h4>Proposal</h4>
                    <MarkdownView content={proposal} emptyText="No proposal available." />
                  </article>
                </>
              ) : null}

              {activeTab === "logs" ? (
                <article className="card">
                  <h4>Logs</h4>
                  <MarkdownView content={logText} emptyText="No logs available." />
                </article>
              ) : null}
            </>
          ) : (
            <p>Select a ticket to continue.</p>
          )}
        </section>
      </main>

      {showAddTicketDialog ? (
        <div className="modal-backdrop" onClick={() => setShowAddTicketDialog(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h3>Add Ticket</h3>
            <p className="meta">Schedule a ticket run for a repository.</p>
            {addTicketError ? <div className="banner error">{addTicketError}</div> : null}

            <label className="field-label" htmlFor="repo-path-input">
              Repository Folder
            </label>
            <input
              id="repo-path-input"
              list="repo-path-options"
              value={newTicketRepoPath}
              onChange={(e) => {
                setNewTicketRepoPath(e.target.value);
                setAddTicketError("");
              }}
              placeholder="/absolute/path/to/repo"
            />
            <datalist id="repo-path-options">
              {knownRepoPaths.map((p) => (
                <option key={p} value={p} />
              ))}
            </datalist>

            <label className="field-label" htmlFor="ticket-number-input">
              Ticket Number
            </label>
            <input
              id="ticket-number-input"
              value={newTicketNumber}
              onChange={(e) => {
                setNewTicketNumber(e.target.value);
                setAddTicketError("");
              }}
              placeholder="e.g. 66825"
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  void submitAddTicket();
                }
              }}
            />

            <div className="button-row modal-actions">
              <button
                type="button"
                className="secondary"
                onClick={() => {
                  setAddTicketError("");
                  setShowAddTicketDialog(false);
                }}
              >
                Cancel
              </button>
              <button type="button" onClick={() => void submitAddTicket()}>
                Schedule Run
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}
