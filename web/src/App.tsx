import { useEffect, useMemo, useRef, useState } from "react";
import {
  applyAction,
  cleanupAll,
  cleanupDone,
  cleanupTicket,
  connectEvents,
  discoverTickets,
  getArtifact,
  getExecutionLogs,
  getHealth,
  getJob,
  getTicket,
  listRepositories,
  listTickets,
  moveToState,
  runTicket
} from "./api";
import { AddTicketDialog } from "./AddTicketDialog";
import { DiscoverTicketsModal } from "./DiscoverTicketsModal";
import { ExecutionLogsModal } from "./ExecutionLogsModal";
import { TicketDetailPanel } from "./TicketDetailPanel";
import { TicketList } from "./TicketList";
import {
  applyTicketEvent,
  getFeedbackAction,
  knownRepoPaths,
  pendingTicketKey,
  selectTicketKey,
  stateRunsFromDetails,
  ticketKey
} from "./tickets";
import type { DiscoveredTicket, ExecutionLog, Job, ServerEvent, TicketDetails, TicketSummary } from "./types";

export function App() {
  const [tickets, setTickets] = useState<TicketSummary[]>([]);
  const [selectedKey, setSelectedKey] = useState("");
  const [details, setDetails] = useState<TicketDetails | null>(null);
  const [selectedRunId, setSelectedRunId] = useState("");
  const [selectedArtifactContent, setSelectedArtifactContent] = useState("");
  const [feedbackMessage, setFeedbackMessage] = useState("");
  const [activeJobId, setActiveJobId] = useState("");
  const [activeJobTicketKey, setActiveJobTicketKey] = useState("");
  const [activeJob, setActiveJob] = useState<Job | null>(null);
  const [loading, setLoading] = useState(false);
  const [artifactLoading, setArtifactLoading] = useState(false);
  const [error, setError] = useState("");
  const [repositoryOptions, setRepositoryOptions] = useState<string[]>([]);
  const [showAddTicketDialog, setShowAddTicketDialog] = useState(false);
  const [showLogsModal, setShowLogsModal] = useState(false);
  const [executionLogs, setExecutionLogs] = useState<ExecutionLog[]>([]);
  const [executionLogsLoading, setExecutionLogsLoading] = useState(false);
  const [newTicketRepoPath, setNewTicketRepoPath] = useState("");
  const [newTicketNumber, setNewTicketNumber] = useState("");
  const [pendingAddedTickets, setPendingAddedTickets] = useState<string[]>([]);
  const [addTicketError, setAddTicketError] = useState("");
  const [showDiscoverModal, setShowDiscoverModal] = useState(false);
  const [discoverRepoPath, setDiscoverRepoPath] = useState("");
  const [discoveredTickets, setDiscoveredTickets] = useState<DiscoveredTicket[]>([]);
  const [discoverLoading, setDiscoverLoading] = useState(false);
  const [discoverError, setDiscoverError] = useState("");
  const [discoverConfigured, setDiscoverConfigured] = useState(false);

  const selectedSummary = useMemo(() => tickets.find((ticket) => ticketKey(ticket) === selectedKey) ?? null, [tickets, selectedKey]);
  const availableRepoPaths = useMemo(() => knownRepoPaths(repositoryOptions, tickets), [repositoryOptions, tickets]);
  const stateRuns = useMemo(() => stateRunsFromDetails(details), [details]);
  const feedbackAction = useMemo(() => getFeedbackAction(details, selectedSummary), [details, selectedSummary]);

  const selectedSummaryRef = useRef<TicketSummary | null>(null);
  const activeJobIdRef = useRef("");
  const showLogsModalRef = useRef(false);
  const fullRefreshScheduledRef = useRef(false);
  const prevLastRunIdRef = useRef("");
  const reconnectErrorMessage = "event stream connection lost; reconnecting";

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
    void getHealth()
      .then((health) => { setDiscoverConfigured(health.discover_tickets_configured); })
      .catch(() => { setDiscoverConfigured(false); });
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
      setArtifactLoading(false);
      setShowLogsModal(false);
      setExecutionLogs([]);
      setExecutionLogsLoading(false);
      return;
    }
    void refreshTicketDetails(selectedSummary.repo_path, selectedSummary.ticket_number);
  }, [selectedSummary?.repo_path, selectedSummary?.ticket_number]);

  useEffect(() => {
    if (stateRuns.length === 0) {
      prevLastRunIdRef.current = "";
      setSelectedRunId("");
      return;
    }
    const lastRunId = stateRuns[stateRuns.length - 1].id;
    const currentRunId = details?.state.current_run_id;
    const prevLastRunId = prevLastRunIdRef.current;
    prevLastRunIdRef.current = lastRunId;

    setSelectedRunId((current) => {
      if (!current || !stateRuns.some((run) => run.id === current)) {
        return (currentRunId && stateRuns.some((run) => run.id === currentRunId)) ? currentRunId : lastRunId;
      }
      if (current === prevLastRunId) {
        return (currentRunId && stateRuns.some((run) => run.id === currentRunId)) ? currentRunId : lastRunId;
      }
      return current;
    });
  }, [details?.state.current_run_id, stateRuns]);

  useEffect(() => {
    if (!selectedSummary) {
      setArtifactLoading(false);
      return;
    }
    const selectedRun = stateRuns.find((run) => run.id === selectedRunId) ?? null;
    if (!selectedRun) {
      setSelectedArtifactContent("");
      setArtifactLoading(false);
      return;
    }
    const artifactRef = selectedRun.artifact_ref || selectedRun.log_ref;
    if (!artifactRef) {
      setSelectedArtifactContent("");
      setArtifactLoading(false);
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
  }, [selectedRunId, selectedSummary, stateRuns]);

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
      setSelectedKey((current) => selectTicketKey(current, data));
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

  async function refreshExecutionLogs(repoPath: string, ticketNumber: string) {
    try {
      const logs = await getExecutionLogs(repoPath, ticketNumber);
      setExecutionLogs(logs);
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to refresh execution logs");
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
          setActiveJobTicketKey("");
        }
      } else if (status === "done") {
        setActiveJob(null);
        setActiveJobId("");
        setActiveJobTicketKey("");
      } else if (status === "queued" || status === "running") {
        setActiveJob((current) => {
          if (!current) {
            return current;
          }
          return { ...current, status };
        });
      }
    }

    let needsFullRefresh = evt.type === "repo_tickets_synced";
    setTickets((current) => {
      const ticketUpdate = applyTicketEvent(current, evt);
      if (ticketUpdate.needsFullRefresh) {
        needsFullRefresh = true;
      }
      return ticketUpdate.tickets;
    });
    if (needsFullRefresh) {
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
        await refreshExecutionLogs(selected.repo_path, selected.ticket_number);
      }
    }
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

  async function queueAction(fn: () => Promise<{ job_id: string }>, forTicketKey = ""): Promise<boolean> {
    setError("");
    try {
      const accepted = await fn();
      setActiveJobId(accepted.job_id);
      setActiveJobTicketKey(forTicketKey);
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
      const existing = repoTickets.find((ticket) => ticket.ticket_number === ticketNumber);
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
      closeAddTicketDialog();
      scheduleFullRefresh();
    }
  }

  function openAddTicketDialog() {
    setError("");
    setAddTicketError("");
    setNewTicketRepoPath(selectedSummary?.repo_path ?? "");
    setNewTicketNumber("");
    setShowAddTicketDialog(true);
    void refreshRepositories();
  }

  function closeAddTicketDialog() {
    setAddTicketError("");
    setShowAddTicketDialog(false);
    setNewTicketRepoPath("");
    setNewTicketNumber("");
  }

  function updateAddTicketRepoPath(value: string) {
    setNewTicketRepoPath(value);
    setAddTicketError("");
  }

  function updateAddTicketNumber(value: string) {
    setNewTicketNumber(value);
    setAddTicketError("");
  }

  function submitFeedback() {
    if (!selectedSummary || !feedbackAction || !feedbackMessage.trim()) {
      return;
    }
    const key = ticketKey(selectedSummary);
    void queueAction(() =>
      applyAction(
        selectedSummary.repo_path,
        selectedSummary.ticket_number,
        feedbackAction.label,
        feedbackMessage
      ),
      key
    ).then((ok) => {
      if (ok) {
        setFeedbackMessage("");
      }
    });
  }

  function applyNamedAction(label: string) {
    if (!selectedSummary) {
      return;
    }
    void queueAction(() => applyAction(selectedSummary.repo_path, selectedSummary.ticket_number, label), ticketKey(selectedSummary));
  }

  function rerunSelectedTicket() {
    if (!selectedSummary) {
      return;
    }
    void queueAction(() => runTicket(selectedSummary.repo_path, selectedSummary.ticket_number), ticketKey(selectedSummary));
  }

  function cleanupSelectedTicket() {
    if (!selectedSummary) {
      return;
    }
    void queueAction(() => cleanupTicket(selectedSummary.repo_path, selectedSummary.ticket_number), ticketKey(selectedSummary));
  }

  function moveSelectedTicket(target: string) {
    if (!selectedSummary) {
      return;
    }
    void queueAction(() => moveToState(selectedSummary.repo_path, selectedSummary.ticket_number, target), ticketKey(selectedSummary));
  }

  function openDiscoverModal() {
    if (!discoverConfigured) {
      return;
    }
    const repoPath = selectedSummary?.repo_path ?? availableRepoPaths[0] ?? "";
    setDiscoverRepoPath(repoPath);
    setDiscoveredTickets([]);
    setDiscoverError("");
    setDiscoverLoading(true);
    setShowDiscoverModal(true);
    void discoverTickets(repoPath)
      .then((found) => { setDiscoveredTickets(found); })
      .catch((err) => { setDiscoverError(err instanceof Error ? err.message : "discovery failed"); })
      .finally(() => { setDiscoverLoading(false); });
  }

  function handleDiscoverAdd(ticketNumber: string) {
    setShowDiscoverModal(false);
    setError("");
    setAddTicketError("");
    setNewTicketRepoPath(discoverRepoPath);
    setNewTicketNumber(ticketNumber);
    setShowAddTicketDialog(true);
    void refreshRepositories();
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
          <span title={!discoverConfigured ? "not configured" : ""}>
            <button onClick={openDiscoverModal} disabled={!discoverConfigured || availableRepoPaths.length === 0}>
              Discover Tickets
            </button>
          </span>
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
        <TicketList tickets={tickets} selectedKey={selectedKey} onSelectTicket={setSelectedKey} onAddTicket={openAddTicketDialog} />
        <TicketDetailPanel
          selectedSummary={selectedSummary}
          details={details}
          stateRuns={stateRuns}
          selectedRunId={selectedRunId}
          selectedArtifactContent={selectedArtifactContent}
          artifactLoading={artifactLoading}
          feedbackAction={feedbackAction}
          feedbackMessage={feedbackMessage}
          isRunning={!!activeJobId && activeJobTicketKey === (selectedSummary ? ticketKey(selectedSummary) : "")}
          onSelectRun={setSelectedRunId}
          onFeedbackMessageChange={setFeedbackMessage}
          onSubmitFeedback={submitFeedback}
          onApplyAction={applyNamedAction}
          onOpenLogs={() => setShowLogsModal(true)}
          onRerun={rerunSelectedTicket}
          onCleanup={cleanupSelectedTicket}
          onMoveToState={moveSelectedTicket}
        />
      </main>

      {showAddTicketDialog ? (
        <AddTicketDialog
          knownRepoPaths={availableRepoPaths}
          repoPath={newTicketRepoPath}
          ticketNumber={newTicketNumber}
          error={addTicketError}
          onRepoPathChange={updateAddTicketRepoPath}
          onTicketNumberChange={updateAddTicketNumber}
          onSubmit={submitAddTicket}
          onClose={closeAddTicketDialog}
        />
      ) : null}

      {showDiscoverModal ? (
        <DiscoverTicketsModal
          repoPath={discoverRepoPath}
          tickets={discoveredTickets}
          loading={discoverLoading}
          error={discoverError}
          onAdd={handleDiscoverAdd}
          onClose={() => setShowDiscoverModal(false)}
        />
      ) : null}

      {showLogsModal ? (
        <ExecutionLogsModal
          logs={executionLogs}
          loading={executionLogsLoading}
          onClose={() => setShowLogsModal(false)}
          githubBlobBase={details?.github_blob_base}
          repoPath={details?.repo_path}
          worktreePath={details?.state.worktree_path}
        />
      ) : null}
    </div>
  );
}
