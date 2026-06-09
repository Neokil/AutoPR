import { useEffect, useEffectEvent, useMemo, useRef, useState } from "react";
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
import { extractOpenQuestions, formatFeedbackMessage } from "./investigationFeedback";
import { TicketDetailPanel } from "./TicketDetailPanel";
import { TicketList } from "./TicketList";
import {
  applyTicketEvent,
  getFeedbackAction,
  knownRepoPaths,
  pendingTicketKey,
  projectedTicketStatusLabel,
  resolveStateDisplayName,
  selectTicketKey,
  stateRunsFromDetails,
  ticketKey
} from "./tickets";
import type {
  AcceptedJob,
  DiscoveredTicket,
  ExecutionLog,
  Job,
  OptimisticTransition,
  ServerEvent,
  TicketDetails,
  TicketSummary
} from "./types";

export function App() {
  const [tickets, setTickets] = useState<TicketSummary[]>([]);
  const [selectedKey, setSelectedKey] = useState("");
  const [details, setDetails] = useState<TicketDetails | null>(null);
  const [selectedRunId, setSelectedRunId] = useState("");
  const [selectedArtifactContent, setSelectedArtifactContent] = useState("");
  const [currentFeedbackArtifactContent, setCurrentFeedbackArtifactContent] = useState("");
  const [questionAnswers, setQuestionAnswers] = useState<Record<string, string>>({});
  const [generalFeedback, setGeneralFeedback] = useState("");
  const [optimisticTransition, setOptimisticTransition] = useState<OptimisticTransition | null>(null);
  const [pendingTicketKeys, setPendingTicketKeys] = useState<string[]>([]);
  const [jobFailuresByTicket, setJobFailuresByTicket] = useState<Record<string, Job>>({});
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
  const [newTicketBaseBranch, setNewTicketBaseBranch] = useState("");
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
  const stateRuns = useMemo(() => stateRunsFromDetails(details, optimisticTransition), [details, optimisticTransition]);
  const feedbackAction = useMemo(() => getFeedbackAction(details, selectedSummary), [details, selectedSummary]);
  const currentRunId = details?.state.current_run_id ?? "";
  const currentRun = useMemo(
    () => stateRuns.find((run) => run.id === currentRunId) ?? null,
    [currentRunId, stateRuns]
  );
  const openQuestions = useMemo(
    () => extractOpenQuestions(currentFeedbackArtifactContent),
    [currentFeedbackArtifactContent]
  );
  const selectedStatusLabel = useMemo(
    () => (selectedSummary ? projectedTicketStatusLabel(selectedSummary, optimisticTransition) : ""),
    [optimisticTransition, selectedSummary]
  );
  const selectedTicketPending = selectedSummary ? pendingTicketKeys.includes(ticketKey(selectedSummary)) : false;
  const selectedTicketDisabled = Boolean(selectedSummary && (selectedSummary.busy || selectedTicketPending));
  const selectedTicketFailure = selectedSummary ? jobFailuresByTicket[ticketKey(selectedSummary)] ?? null : null;

  const selectedSummaryRef = useRef<TicketSummary | null>(null);
  const selectedRunIdRef = useRef("");
  const showLogsModalRef = useRef(false);
  const fullRefreshScheduledRef = useRef(false);
  const prevLastRunIdRef = useRef("");
  const handleServerEventRef = useRef<(evt: ServerEvent) => Promise<void>>(async () => {});
  const reconnectErrorMessage = "event stream connection lost; reconnecting";

  function setTicketPending(key: string, pending: boolean) {
    setPendingTicketKeys((current) => {
      if (pending) {
        return current.includes(key) ? current : [...current, key];
      }
      return current.filter((candidate) => candidate !== key);
    });
  }

  function clearTicketFailure(key: string) {
    setJobFailuresByTicket((current) => {
      if (!(key in current)) {
        return current;
      }
      const next = { ...current };
      delete next[key];
      return next;
    });
  }

  function setTicketFailure(key: string, job: Job) {
    setJobFailuresByTicket((current) => ({ ...current, [key]: job }));
  }

  function optimisticTicketJob(accepted: AcceptedJob) {
    if (!accepted.ticket_number) {
      return;
    }
    const key = `${accepted.repo_id}::${accepted.ticket_number}`;
    const createdAt = new Date().toISOString();
    setTicketPending(key, true);
    clearTicketFailure(key);
    setTickets((current) => current.map((ticket) => {
      if (ticketKey(ticket) !== key) {
        return ticket;
      }
      // SSE events may arrive before the 202 response — don't downgrade an already-progressed job back to "queued".
      const existingJob = (ticket.jobs ?? []).find((job) => job.id === accepted.job_id);
      const optimisticJob = existingJob ?? {
        id: accepted.job_id,
        action: accepted.action,
        repo_id: accepted.repo_id,
        repo_path: accepted.repo_path,
        ticket_number: accepted.ticket_number,
        status: "queued" as Job["status"],
        created_at: createdAt
      };
      const nextJobs = [
        optimisticJob,
        ...(ticket.jobs ?? []).filter((job) => job.id !== accepted.job_id)
      ];
      const isBusy = nextJobs.some((job) => job.status === "queued" || job.status === "running");
      return {
        ...ticket,
        busy: isBusy,
        jobs: nextJobs
      };
    }));
  }

  async function fetchTicketFailure(key: string, evt: ServerEvent) {
    if (!evt.job_id || !evt.repo_id || !evt.repo_path || !evt.ticket_number) {
      return;
    }
    try {
      const job = await getJob(evt.job_id);
      setTicketFailure(key, job);
    } catch {
      setTicketFailure(key, {
        id: evt.job_id,
        action: evt.action ?? "",
        repo_id: evt.repo_id,
        repo_path: evt.repo_path,
        ticket_number: evt.ticket_number,
        status: "failed",
        error: evt.error,
        created_at: new Date().toISOString()
      });
    }
  }

  function makeSyntheticRunId(): string {
    return `optimistic-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  }

  function applyOptimisticTransition(next: Omit<OptimisticTransition, "synthetic_run_id">) {
    const transition = {
      ...next,
      synthetic_run_id: makeSyntheticRunId()
    };
    setOptimisticTransition(transition);
    setSelectedRunId(transition.synthetic_run_id);
  }

  function createMoveTransition(accepted: AcceptedJob, targetStateName: string): OptimisticTransition | null {
    if (!selectedSummary || !details) {
      return null;
    }
    return {
      ticket_key: ticketKey(selectedSummary),
      repo_path: selectedSummary.repo_path,
      ticket_number: selectedSummary.ticket_number,
      job_id: accepted.job_id,
      target_state_name: targetStateName,
      target_state_display_name: resolveStateDisplayName(details.workflow_states ?? [], targetStateName),
      previous_selected_run_id: selectedRunIdRef.current,
      previous_current_run_id: details.state.current_run_id ?? "",
      kind: "move_to_state",
      synthetic_run_id: ""
    };
  }

  function createRerunTransition(accepted: AcceptedJob): OptimisticTransition | null {
    if (!selectedSummary || !details) {
      return null;
    }
    const currentStateName = details.state.current_state;
    const fallbackDisplayName = currentRun?.state_display_name || currentStateName;
    return {
      ticket_key: ticketKey(selectedSummary),
      repo_path: selectedSummary.repo_path,
      ticket_number: selectedSummary.ticket_number,
      job_id: accepted.job_id,
      target_state_name: currentStateName,
      target_state_display_name: resolveStateDisplayName(details.workflow_states ?? [], currentStateName, fallbackDisplayName),
      previous_selected_run_id: selectedRunIdRef.current,
      previous_current_run_id: details.state.current_run_id ?? "",
      kind: "rerun",
      synthetic_run_id: ""
    };
  }

  function rollbackOptimisticTransition(jobId?: string) {
    setOptimisticTransition((current) => {
      if (!current || (jobId && current.job_id !== jobId)) {
        return current;
      }
      if (selectedRunIdRef.current === current.synthetic_run_id) {
        setSelectedRunId(current.previous_selected_run_id);
      }
      return null;
    });
  }

  function clearOptimisticTransitionIfConfirmed(ticketDetails: TicketDetails) {
    setOptimisticTransition((current) => {
      if (!current) {
        return current;
      }
      if (current.repo_path !== ticketDetails.repo_path || current.ticket_number !== ticketDetails.ticket_number) {
        return current;
      }

      const currentRunId = ticketDetails.state.current_run_id ?? "";
      const rerunConfirmed = current.kind === "rerun" && currentRunId !== "" && currentRunId !== current.previous_current_run_id;
      const moveConfirmed =
        current.kind === "move_to_state" &&
        ticketDetails.state.current_state === current.target_state_name &&
        currentRunId !== "" &&
        currentRunId !== current.previous_current_run_id;
      const terminalConfirmed =
        current.kind === "move_to_state" &&
        (ticketDetails.state.flow_status ?? "") === current.target_state_name;

      if (!rerunConfirmed && !moveConfirmed && !terminalConfirmed) {
        return current;
      }

      if (selectedRunIdRef.current === current.synthetic_run_id && moveConfirmed) {
        setSelectedRunId(currentRunId);
      }

      return null;
    });
  }

  async function handleServerEvent(evt: ServerEvent) {
    const selected = selectedSummaryRef.current;
    const evtTicketKey = evt.repo_id && evt.ticket_number ? `${evt.repo_id}::${evt.ticket_number}` : "";
    if (evt.type === "job" && evtTicketKey) {
      const status = evt.status ?? "";
      setTicketPending(evtTicketKey, false);
      if (status === "failed") {
        rollbackOptimisticTransition(evt.job_id);
        await fetchTicketFailure(evtTicketKey, evt);
      } else if (status === "queued" || status === "running" || status === "done") {
        clearTicketFailure(evtTicketKey);
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

    if (evt.type === "ticket_updated" && evtTicketKey && (evt.status ?? "") !== "running") {
      setTicketPending(evtTicketKey, false);
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

  useEffect(() => {
    selectedSummaryRef.current = selectedSummary;
  }, [selectedSummary]);

  useEffect(() => {
    selectedRunIdRef.current = selectedRunId;
  }, [selectedRunId]);

  useEffect(() => {
    showLogsModalRef.current = showLogsModal;
  }, [showLogsModal]);

  useEffect(() => {
    handleServerEventRef.current = handleServerEvent;
  });

  useEffect(() => {
    void refreshTickets();
    void refreshRepositories();
    void getHealth()
      .then((health) => { setDiscoverConfigured(health.discover_tickets_configured); })
      .catch(() => { setDiscoverConfigured(false); });
    const stream = connectEvents(
      (evt) => {
        void handleServerEventRef.current(evt);
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
    if (stateRuns.length === 0) {
      prevLastRunIdRef.current = "";
      setSelectedRunId("");
      return;
    }
    const lastRunId = stateRuns[stateRuns.length - 1].id;
    const nextCurrentRunId = details?.state.current_run_id;
    const prevLastRunId = prevLastRunIdRef.current;
    prevLastRunIdRef.current = lastRunId;

    setSelectedRunId((current) => {
      if (!current || !stateRuns.some((run) => run.id === current)) {
        return (nextCurrentRunId && stateRuns.some((run) => run.id === nextCurrentRunId)) ? nextCurrentRunId : lastRunId;
      }
      if (current === prevLastRunId) {
        return (nextCurrentRunId && stateRuns.some((run) => run.id === nextCurrentRunId)) ? nextCurrentRunId : lastRunId;
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
    if (selectedRun.synthetic) {
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
    if (!selectedSummary || !currentRun) {
      setCurrentFeedbackArtifactContent("");
      return;
    }
    const artifactRef = currentRun.artifact_ref || currentRun.log_ref;
    if (!artifactRef) {
      setCurrentFeedbackArtifactContent("");
      return;
    }

    let cancelled = false;
    void getArtifact(selectedSummary.repo_path, selectedSummary.ticket_number, artifactRef)
      .then((content) => {
        if (!cancelled) {
          setCurrentFeedbackArtifactContent(content);
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "failed to load feedback artifact");
          setCurrentFeedbackArtifactContent("");
        }
      });
    return () => {
      cancelled = true;
    };
  }, [currentRun, selectedSummary]);

  useEffect(() => {
    setQuestionAnswers({});
    setGeneralFeedback("");
  }, [selectedSummary?.repo_path, selectedSummary?.ticket_number, currentRunId]);

  useEffect(() => {
    setPendingTicketKeys((current) => current.filter((key) => {
      const ticket = tickets.find((candidate) => ticketKey(candidate) === key);
      return ticket ? ticket.busy : false;
    }));
  }, [tickets]);

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
  }, [showLogsModal, selectedSummary]);

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

  const refreshTicketDetails = useEffectEvent(async (repoPath: string, ticket: string, showLoader = true) => {
    if (showLoader) {
      setLoading(true);
    }
    setError("");
    try {
      const ticketDetails = await getTicket(repoPath, ticket);
      setDetails(ticketDetails);
      clearOptimisticTransitionIfConfirmed(ticketDetails);
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to load ticket details");
      setDetails(null);
      setSelectedArtifactContent("");
    } finally {
      if (showLoader) {
        setLoading(false);
      }
    }
  });

  const refreshExecutionLogs = useEffectEvent(async (repoPath: string, ticketNumber: string) => {
    try {
      const logs = await getExecutionLogs(repoPath, ticketNumber);
      setExecutionLogs(logs);
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to refresh execution logs");
    }
  });

  useEffect(() => {
    if (!selectedSummary) {
      setDetails(null);
      setSelectedRunId("");
      setSelectedArtifactContent("");
      setCurrentFeedbackArtifactContent("");
      setQuestionAnswers({});
      setGeneralFeedback("");
      setArtifactLoading(false);
      setShowLogsModal(false);
      setExecutionLogs([]);
      setExecutionLogsLoading(false);
      return;
    }
    void refreshTicketDetails(selectedSummary.repo_path, selectedSummary.ticket_number);
    // `refreshTicketDetails` is a stable effect event; this effect should only react to selection changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedSummary]);

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

  async function queueAction(fn: () => Promise<AcceptedJob>): Promise<AcceptedJob | null> {
    setError("");
    try {
      const accepted = await fn();
      optimisticTicketJob(accepted);
      return accepted;
    } catch (err) {
      setError(err instanceof Error ? err.message : "action failed");
      return null;
    }
  }

  async function submitAddTicket() {
    const repoPath = newTicketRepoPath.trim();
    const ticketNumber = newTicketNumber.trim();
    const baseBranch = newTicketBaseBranch.trim();
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
    const accepted = await queueAction(() => runTicket(repoPath, ticketNumber, baseBranch));
    setPendingAddedTickets((current) => current.filter((key) => key !== pendingKey));
    if (accepted) {
      closeAddTicketDialog();
      scheduleFullRefresh();
    }
  }

  function openAddTicketDialog() {
    setError("");
    setAddTicketError("");
    setNewTicketRepoPath(selectedSummary?.repo_path ?? "");
    setNewTicketNumber("");
    setNewTicketBaseBranch("");
    setShowAddTicketDialog(true);
    void refreshRepositories();
  }

  function closeAddTicketDialog() {
    setAddTicketError("");
    setShowAddTicketDialog(false);
    setNewTicketRepoPath("");
    setNewTicketNumber("");
    setNewTicketBaseBranch("");
  }

  function updateAddTicketRepoPath(value: string) {
    setNewTicketRepoPath(value);
    setAddTicketError("");
  }

  function updateAddTicketNumber(value: string) {
    setNewTicketNumber(value);
    setAddTicketError("");
  }

  function updateAddTicketBaseBranch(value: string) {
    setNewTicketBaseBranch(value);
    setAddTicketError("");
  }

  function submitFeedback() {
    if (!selectedSummary || !feedbackAction) {
      return;
    }
    const message = formatFeedbackMessage(openQuestions, questionAnswers, generalFeedback);
    if (!message.trim()) {
      return;
    }
    void queueAction(() =>
      applyAction(
        selectedSummary.repo_path,
        selectedSummary.ticket_number,
        feedbackAction.label,
        message
      )
    ).then((accepted) => {
      if (accepted) {
        const transition = createRerunTransition(accepted);
        if (transition) {
          applyOptimisticTransition(transition);
        }
        setQuestionAnswers({});
        setGeneralFeedback("");
      }
    });
  }

  function updateQuestionAnswer(index: number, value: string) {
    setQuestionAnswers((current) => ({ ...current, [String(index)]: value }));
  }

  function applyNamedAction(label: string) {
    if (!selectedSummary || !details) {
      return;
    }
    const action = details.available_actions.find((candidate) => candidate.label === label);
    void queueAction(() => applyAction(selectedSummary.repo_path, selectedSummary.ticket_number, label)).then((accepted) => {
      if (!accepted || !action?.target) {
        return;
      }
      const transition = createMoveTransition(accepted, action.target);
      if (transition) {
        applyOptimisticTransition(transition);
      }
    });
  }

  function rerunSelectedTicket() {
    if (!selectedSummary) {
      return;
    }
    void queueAction(() => runTicket(selectedSummary.repo_path, selectedSummary.ticket_number)).then((accepted) => {
      if (!accepted) {
        return;
      }
      const transition = createRerunTransition(accepted);
      if (transition) {
        applyOptimisticTransition(transition);
      }
    });
  }

  function cleanupSelectedTicket() {
    if (!selectedSummary) {
      return;
    }
    void queueAction(() => cleanupTicket(selectedSummary.repo_path, selectedSummary.ticket_number));
  }

  function moveSelectedTicket(target: string) {
    if (!selectedSummary) {
      return;
    }
    void queueAction(() => moveToState(selectedSummary.repo_path, selectedSummary.ticket_number, target)).then((accepted) => {
      if (!accepted) {
        return;
      }
      const transition = createMoveTransition(accepted, target);
      if (transition) {
        applyOptimisticTransition(transition);
      }
    });
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

      <main className="main">
        <TicketList
          tickets={tickets}
          optimisticTransition={optimisticTransition}
          selectedKey={selectedKey}
          onSelectTicket={setSelectedKey}
          onAddTicket={openAddTicketDialog}
        />
        <TicketDetailPanel
          selectedSummary={selectedSummary}
          details={details}
          stateRuns={stateRuns}
          selectedRunId={selectedRunId}
          selectedArtifactContent={selectedArtifactContent}
          artifactLoading={artifactLoading}
          statusLabel={selectedStatusLabel}
          feedbackAction={feedbackAction}
          openQuestions={feedbackAction ? openQuestions : []}
          questionAnswers={questionAnswers}
          generalFeedback={generalFeedback}
          actionsDisabled={selectedTicketDisabled}
          feedbackDisabled={selectedTicketDisabled}
          cleanupDisabled={selectedTicketDisabled}
          moveDisabled={selectedTicketDisabled}
          rerunDisabled={selectedTicketDisabled}
          jobFailure={selectedTicketFailure}
          onSelectRun={setSelectedRunId}
          onQuestionAnswerChange={updateQuestionAnswer}
          onGeneralFeedbackChange={setGeneralFeedback}
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
          baseBranch={newTicketBaseBranch}
          error={addTicketError}
          onRepoPathChange={updateAddTicketRepoPath}
          onTicketNumberChange={updateAddTicketNumber}
          onBaseBranchChange={updateAddTicketBaseBranch}
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
