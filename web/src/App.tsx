import { useEffect, useMemo, useState } from "react";
import {
  approveTicket,
  cleanupAll,
  cleanupDone,
  cleanupTicket,
  createPR,
  feedbackTicket,
  getArtifact,
  getEvents,
  getJob,
  getTicket,
  listTickets,
  rejectTicket,
  resumeTicket,
  runTicket
} from "./api";
import { MarkdownView } from "./MarkdownView";
import type { EventItem, Job, TicketDetails, TicketSummary } from "./types";

function ticketKey(t: TicketSummary): string {
  return `${t.repo_id}::${t.ticket_number}`;
}

type Action = "run" | "resume" | "approve" | "reject" | "pr" | "cleanup";

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
      return ["pr", "cleanup"];
    case "done":
      return ["cleanup"];
    case "investigating":
    case "implementing":
    case "validating":
    default:
      return ["resume", "cleanup"];
  }
}

export function App() {
  const [tickets, setTickets] = useState<TicketSummary[]>([]);
  const [selectedKey, setSelectedKey] = useState<string>("");
  const [details, setDetails] = useState<TicketDetails | null>(null);
  const [events, setEvents] = useState<EventItem[]>([]);
  const [proposal, setProposal] = useState("");
  const [logText, setLogText] = useState("");
  const [activeTab, setActiveTab] = useState<"details" | "proposal" | "logs">("details");
  const [feedbackMessage, setFeedbackMessage] = useState("");
  const [activeJobId, setActiveJobId] = useState<string>("");
  const [activeJob, setActiveJob] = useState<Job | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>("");

  const selectedSummary = useMemo(() => tickets.find((t) => ticketKey(t) === selectedKey) ?? null, [tickets, selectedKey]);

  useEffect(() => {
    void refreshTickets();
    const timer = setInterval(() => {
      void refreshTickets(false);
    }, 5000);
    return () => clearInterval(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (!selectedSummary) {
      setDetails(null);
      setEvents([]);
      setProposal("");
      setLogText("");
      return;
    }
    void refreshTicketDetails(selectedSummary.repo_path, selectedSummary.ticket_number);
    setActiveTab("details");
  }, [selectedSummary?.repo_path, selectedSummary?.ticket_number]);

  useEffect(() => {
    if (!activeJobId) {
      return;
    }
    const timer = setInterval(async () => {
      try {
        const job = await getJob(activeJobId);
        setActiveJob(job);
        if (job.status === "done" || job.status === "failed") {
          clearInterval(timer);
          setActiveJobId("");
          await refreshTickets();
          if (selectedSummary) {
            await refreshTicketDetails(selectedSummary.repo_path, selectedSummary.ticket_number);
          }
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : "failed to poll job");
      }
    }, 1000);
    return () => clearInterval(timer);
  }, [activeJobId, selectedSummary?.repo_path, selectedSummary?.ticket_number]);

  async function refreshTickets(showLoader = true) {
    if (showLoader) {
      setLoading(true);
    }
    try {
      const data = await listTickets();
      setTickets(data);
      if (data.length === 0) {
        setSelectedKey("");
        return;
      }
      if (!selectedKey || !data.some((t) => ticketKey(t) === selectedKey)) {
        setSelectedKey(ticketKey(data[0]));
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to load tickets");
    } finally {
      if (showLoader) {
        setLoading(false);
      }
    }
  }

  async function refreshTicketDetails(repoPath: string, ticket: string) {
    setLoading(true);
    setError("");
    try {
      const [ticketDetails, eventItems, proposalText, logs] = await Promise.all([
        getTicket(repoPath, ticket),
        getEvents(repoPath, ticket),
        getArtifact(repoPath, ticket, "proposal"),
        getArtifact(repoPath, ticket, "log")
      ]);
      setDetails(ticketDetails);
      setEvents(eventItems);
      setProposal(proposalText);
      setLogText(logs);
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to load ticket details");
      setDetails(null);
      setEvents([]);
      setProposal("");
      setLogText("");
    } finally {
      setLoading(false);
    }
  }

  async function queueAction(fn: () => Promise<{ job_id: string }>) {
    setError("");
    try {
      const accepted = await fn();
      setActiveJobId(accepted.job_id);
      setActiveJob(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "action failed");
    }
  }

  return (
    <div className="app">
      <header className="header">
        <h1>AI Orchestrator</h1>
        <button onClick={() => void refreshTickets()} disabled={loading}>
          Refresh All Tickets
        </button>
      </header>

      {error ? <div className="banner error">{error}</div> : null}
      {activeJob ? (
        <div className={`banner ${activeJob.status === "failed" ? "error" : "info"}`}>
          Job `{activeJob.id}`: {activeJob.action} ({activeJob.status})
          {activeJob.error ? ` - ${activeJob.error}` : ""}
        </div>
      ) : null}

      <main className="main">
        <section className="panel left">
          <div className="panel-header">
            <h2>Tickets (All Repos)</h2>
          </div>
          <ul className="ticket-list">
            {tickets.map((t) => (
              <li key={ticketKey(t)}>
                <button
                  className={selectedKey === ticketKey(t) ? "ticket-item active" : "ticket-item"}
                  onClick={() => setSelectedKey(ticketKey(t))}
                >
                  <strong>{t.ticket_number}</strong>
                  <span>{t.title || "(no title)"}</span>
                  <span className="meta">
                    {t.status} {t.approved ? "· approved" : ""}
                  </span>
                  <span className="meta">{t.repo_path}</span>
                </button>
              </li>
            ))}
          </ul>
        </section>

        <section className="panel right">
          <div className="panel-header">
            <h2>Ticket Details</h2>
            {selectedSummary ? (
              <span className="meta">
                {selectedSummary.ticket_number} · {selectedSummary.status}
              </span>
            ) : null}
          </div>

          {selectedSummary ? (
            <>
              <div className="button-row">
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
                {allowedActions(selectedSummary.status).includes("cleanup") ? (
                  <button onClick={() => void queueAction(() => cleanupTicket(selectedSummary.repo_path, selectedSummary.ticket_number))}>
                    Cleanup
                  </button>
                ) : null}
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
                  <h3>{details?.ticket?.title || "(no title)"}</h3>
                  <MarkdownView content={details?.ticket?.description ?? ""} emptyText="No description." />
                  <p className="meta">{selectedSummary.repo_path}</p>
                  {details?.next_steps ? (
                    <>
                      <h4>Next Steps</h4>
                      <MarkdownView content={details.next_steps} />
                    </>
                  ) : null}
                  <div className="button-row">
                    <button onClick={() => void queueAction(() => cleanupDone(selectedSummary.repo_path))}>Cleanup Done (Repo)</button>
                    <button onClick={() => void queueAction(() => cleanupAll(selectedSummary.repo_path))}>Cleanup All (Repo)</button>
                  </div>
                  <h4>Recent Events</h4>
                  <ul className="events">
                    {events.slice(0, 10).map((ev, idx) => (
                      <li key={`${ev.timestamp}-${idx}`}>
                        <div>
                          <strong>{ev.title}</strong> <span className="meta">{ev.timestamp}</span>
                        </div>
                        <MarkdownView content={ev.body} />
                      </li>
                    ))}
                  </ul>
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
    </div>
  );
}
