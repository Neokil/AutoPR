import { useEffect, useMemo, useRef, useState } from "react";
import {
  applyAction,
  cleanupAll,
  cleanupDone,
  cleanupTicket,
  connectEvents,
  getArtifact,
  getExecutionLogs,
  getJob,
  getTicket,
  listRepositories,
  listTickets,
  runTicket
} from "./api";
import { ExecutionLogsModal } from "./ExecutionLogsModal";
import { MarkdownView } from "./MarkdownView";
import { StateTimeline } from "./StateTimeline";
import { TicketList } from "./TicketList";
import { TicketMenu } from "./TicketMenu";
import type { ExecutionLog, Job, ServerEvent, StateRun, TicketDetails, TicketSummary } from "./types";

function ticketKey(t: TicketSummary): string {
  return `${t.repo_id}::${t.ticket_number}`;
}

function pendingTicketKey(repoPath: string, ticketNumber: string): string {
  return `${repoPath}::${ticketNumber}`;
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

function normalizeStateRuns(details: TicketDetails | null): StateRun[] {
  const history = details?.state.state_history ?? [];
  if (history.length > 0) {
    return history;
  }
  if (!details?.state.current_state) {
    return [];
  }
  return [
    {
      id: details.state.current_run_id || `current-${details.state.current_state}`,
      state_name: details.state.current_state,
      state_display_name: details.state.current_state,
      started_at: details.state.updated_at,
      artifact_ref: "",
      log_ref: ""
    }
  ];
}

function runDisplayLabel(run: StateRun, runs: StateRun[]): string {
  const base = run.state_display_name || run.state_name;
  const matching = runs.filter((candidate) => candidate.state_name === run.state_name);
  if (matching.length <= 1) {
    return base;
  }
  const index = matching.findIndex((candidate) => candidate.id === run.id);
  return `${base} ${index + 1}`;
}

export function App() {
  const [tickets, setTickets] = useState<TicketSummary[]>([]);
  const [selectedKey, setSelectedKey] = useState<string>("");
  const [details, setDetails] = useState<TicketDetails | null>(null);
  const [selectedRunId, setSelectedRunId] = useState("");
  const [selectedArtifactContent, setSelectedArtifactContent] = useState("");
  const [feedbackMessage, setFeedbackMessage] = useState("");
  const [activeJobId, setActiveJobId] = useState<string>("");
  const [activeJob, setActiveJob] = useState<Job | null>(null);
  const [loading, setLoading] = useState(false);
  const [artifactLoading, setArtifactLoading] = useState(false);
  const [error, setError] = useState<string>("");
  const [repositoryOptions, setRepositoryOptions] = useState<string[]>([]);
  const [showAddTicketDialog, setShowAddTicketDialog] = useState(false);
  const [showLogsModal, setShowLogsModal] = useState(false);
  const [executionLogs, setExecutionLogs] = useState<ExecutionLog[]>([]);
  const [executionLogsLoading, setExecutionLogsLoading] = useState(false);
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
  const showLogsModalRef = useRef(false);
  const fullRefreshScheduledRef = useRef(false);
  const reconnectErrorMessage = "event stream connection lost; reconnecting";
  const stateRuns = useMemo(() => normalizeStateRuns(details), [details]);
  const selectedRun = useMemo(
    () => stateRuns.find((run) => run.id === selectedRunId) ?? null,
    [stateRuns, selectedRunId]
  );
  const feedbackAction =
    selectedSummary?.status === "waiting"
      ? (details?.available_actions ?? []).find((action) => action.type === "provide_feedback")
      : undefined;

  useEffect(() => {
    selectedSummaryRef.current = selectedSummary;
  }, [selectedSummary]);

  useEffect(() => {
    activeJobIdRef.current = activeJobId;
  }, [activeJobId]);

  useEffect(() => {
    showLogsModalRef.current = showLogsModal;
  }, [showLogsModal]);

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
      setSelectedRunId("");
      setSelectedArtifactContent("");
      setShowLogsModal(false);
      setExecutionLogs([]);
      return;
    }
    void refreshTicketDetails(selectedSummary.repo_path, selectedSummary.ticket_number);
  }, [selectedSummary?.repo_path, selectedSummary?.ticket_number]);

  useEffect(() => {
    if (stateRuns.length === 0) {
      setSelectedRunId("");
      return;
    }
    const currentRunId = details?.state.current_run_id;
    setSelectedRunId((current) => {
      if (current && stateRuns.some((run) => run.id === current)) {
        return current;
      }
      if (currentRunId && stateRuns.some((run) => run.id === currentRunId)) {
        return currentRunId;
      }
      return stateRuns[stateRuns.length - 1]?.id ?? "";
    });
  }, [details?.state.current_run_id, stateRuns]);

  useEffect(() => {
    if (!selectedSummary || !selectedRun) {
      setSelectedArtifactContent("");
      return;
    }
    const artifactRef = selectedRun.artifact_ref || selectedRun.log_ref;
    if (!artifactRef) {
      setSelectedArtifactContent("");
      return;
    }
    let cancelled = false;
    setArtifactLoading(true);
    void getArtifact(selectedSummary.repo_path, selectedSummary.ticket_number, artifactRef)
      .then((content) => {
        if (!cancelled) {
          setSelectedArtifactContent(content);
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "failed to load artifact");
          setSelectedArtifactContent("");
        }
      })
      .finally(() => {
        if (!cancelled) {
          setArtifactLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [selectedRun?.artifact_ref, selectedRun?.id, selectedRun?.log_ref, selectedSummary?.repo_path, selectedSummary?.ticket_number]);

  useEffect(() => {
    if (!showLogsModal || !selectedSummary) {
      return;
    }
    let cancelled = false;
    setExecutionLogsLoading(true);
    void getExecutionLogs(selectedSummary.repo_path, selectedSummary.ticket_number)
      .then((logs) => {
        if (!cancelled) {
          setExecutionLogs(logs);
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "failed to load execution logs");
        }
      })
      .finally(() => {
        if (!cancelled) {
          setExecutionLogsLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [showLogsModal, selectedSummary?.repo_path, selectedSummary?.ticket_number]);

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
      const ticketDetails = await getTicket(repoPath, ticket);
      setDetails(ticketDetails);
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to load ticket details");
      setDetails(null);
      setSelectedArtifactContent("");
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
      if (showLogsModalRef.current) {
        try {
          const logs = await getExecutionLogs(selected.repo_path, selected.ticket_number);
          setExecutionLogs(logs);
        } catch (err) {
          setError(err instanceof Error ? err.message : "failed to refresh execution logs");
        }
      }
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
          const nextJob =
            jobIndex >= 0
              ? { ...nextJobs[jobIndex], status: (evt.status as Job["status"]) ?? nextJobs[jobIndex].status, error: evt.error }
              : {
                id: evt.job_id ?? "",
                action: evt.action ?? "",
                repo_id: evt.repo_id ?? t.repo_id,
                repo_path: evt.repo_path ?? t.repo_path,
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
          return { ...t, busy: isBusy, jobs: nextJobs };
        }
        if (evt.type === "ticket_updated") {
          return {
            ...t,
            title: evt.title ?? t.title,
            status: (evt.status as TicketSummary["status"]) ?? t.status,
            last_error: evt.error ?? t.last_error,
            pr_url: evt.pr_url ?? t.pr_url,
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
                <div className="detail-actions-wrap">
                  <div className="button-row detail-actions">
                    {selectedSummary.status === "waiting"
                      ? (details?.available_actions ?? [])
                        .filter((action) => action.type !== "provide_feedback")
                        .map((action) => (
                          <button
                            key={action.label}
                            onClick={() =>
                              void queueAction(() =>
                                applyAction(selectedSummary.repo_path, selectedSummary.ticket_number, action.label)
                              )
                            }
                          >
                            {action.label}
                          </button>
                        ))
                      : null}
                    {selectedSummary.pr_url ? (
                      <a href={selectedSummary.pr_url} target="_blank" rel="noreferrer">
                        <button type="button">Open PR</button>
                      </a>
                    ) : null}
                  </div>
                  <TicketMenu
                    onLogs={() => setShowLogsModal(true)}
                    onRerun={() => void queueAction(() => runTicket(selectedSummary.repo_path, selectedSummary.ticket_number))}
                    onCleanup={() => void queueAction(() => cleanupTicket(selectedSummary.repo_path, selectedSummary.ticket_number))}
                    rerunDisabled={selectedSummary.status === "running"}
                    cleanupDisabled={selectedSummary.status === "running"}
                  />
                </div>
              </div>

              <article className="card">
                <span className="field-label">Timeline</span>
                <StateTimeline runs={stateRuns} selectedRunId={selectedRunId} onSelectRun={setSelectedRunId} />
                {feedbackAction ? (
                  <form
                    className="feedback-form"
                    onSubmit={(event) => {
                      event.preventDefault();
                      if (!feedbackMessage.trim()) {
                        return;
                      }
                      void queueAction(() =>
                        applyAction(
                          selectedSummary.repo_path,
                          selectedSummary.ticket_number,
                          feedbackAction.label,
                          feedbackMessage
                        )
                      ).then(() => setFeedbackMessage(""));
                    }}
                  >
                    <input
                      value={feedbackMessage}
                      onChange={(event) => setFeedbackMessage(event.target.value)}
                      placeholder={`Send feedback (${feedbackAction.label})`}
                    />
                    <button type="submit">{feedbackAction.label}</button>
                  </form>
                ) : null}
                {selectedRun ? (
                  <div className="timeline-content">
                    <div className="timeline-content-header">
                      <h4>{runDisplayLabel(selectedRun, stateRuns)}</h4>
                      <span className="meta">{new Date(selectedRun.started_at).toLocaleString()}</span>
                    </div>
                    <p className="meta artifact-path">{selectedRun.artifact_ref || selectedRun.log_ref || "No artifact path available."}</p>
                    {artifactLoading ? <p className="meta">Loading artifact...</p> : null}
                    <MarkdownView
                      content={selectedArtifactContent}
                      emptyText="No run artifact available."
                      githubBlobBase={details?.github_blob_base}
                      repoPath={details?.repo_path}
                      worktreePath={details?.state.worktree_path}
                    />
                  </div>
                ) : (
                  <p className="meta">No workflow runs available yet.</p>
                )}
              </article>
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

      {showLogsModal ? (
        executionLogsLoading ? (
          <div className="modal-backdrop" onClick={() => setShowLogsModal(false)}>
            <div className="modal" onClick={(event) => event.stopPropagation()}>
              <h3>Execution Logs</h3>
              <p className="meta">Loading logs...</p>
            </div>
          </div>
        ) : (
          <ExecutionLogsModal
            logs={executionLogs}
            onClose={() => setShowLogsModal(false)}
            githubBlobBase={details?.github_blob_base}
            repoPath={details?.repo_path}
            worktreePath={details?.state.worktree_path}
          />
        )
      ) : null}
    </div>
  );
}
