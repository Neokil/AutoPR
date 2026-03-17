import { useEffect, useMemo, useState } from "react";
import {
  approveTicket,
  cleanupAll,
  cleanupDone,
  cleanupTicket,
  createPR,
  feedbackTicket,
  getEvents,
  getJob,
  getTicket,
  listTickets,
  rejectTicket,
  resumeTicket,
  runTicket
} from "./api";
import type { EventItem, Job, TicketDetails, TicketSummary } from "./types";

function ticketKey(t: TicketSummary): string {
  return `${t.repo_id}::${t.ticket_number}`;
}

export function App() {
  const [tickets, setTickets] = useState<TicketSummary[]>([]);
  const [selectedKey, setSelectedKey] = useState<string>("");
  const [details, setDetails] = useState<TicketDetails | null>(null);
  const [events, setEvents] = useState<EventItem[]>([]);
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
      return;
    }
    void refreshTicketDetails(selectedSummary.repo_path, selectedSummary.ticket_number);
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
      const [ticketDetails, eventItems] = await Promise.all([getTicket(repoPath, ticket), getEvents(repoPath, ticket)]);
      setDetails(ticketDetails);
      setEvents(eventItems);
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to load ticket details");
      setDetails(null);
      setEvents([]);
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
                <button onClick={() => void queueAction(() => runTicket(selectedSummary.repo_path, selectedSummary.ticket_number))}>
                  Run
                </button>
                <button onClick={() => void queueAction(() => resumeTicket(selectedSummary.repo_path, selectedSummary.ticket_number))}>
                  Resume
                </button>
                <button onClick={() => void queueAction(() => approveTicket(selectedSummary.repo_path, selectedSummary.ticket_number))}>
                  Approve
                </button>
                <button onClick={() => void queueAction(() => rejectTicket(selectedSummary.repo_path, selectedSummary.ticket_number))}>
                  Reject
                </button>
                <button onClick={() => void queueAction(() => createPR(selectedSummary.repo_path, selectedSummary.ticket_number))}>
                  Create PR
                </button>
                <button onClick={() => void queueAction(() => cleanupTicket(selectedSummary.repo_path, selectedSummary.ticket_number))}>
                  Cleanup
                </button>
              </div>

              <div className="button-row">
                <button onClick={() => void queueAction(() => cleanupDone(selectedSummary.repo_path))}>Cleanup Done (Repo)</button>
                <button onClick={() => void queueAction(() => cleanupAll(selectedSummary.repo_path))}>Cleanup All (Repo)</button>
              </div>

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
                  placeholder="Feedback message"
                />
                <button type="submit">Send Feedback</button>
              </form>

              <article className="card">
                <h3>{details?.ticket?.title || "(no title)"}</h3>
                <p>{details?.ticket?.description || "No description"}</p>
                <p className="meta">{selectedSummary.repo_path}</p>
                {details?.next_steps ? (
                  <>
                    <h4>Next Steps</h4>
                    <pre>{details.next_steps}</pre>
                  </>
                ) : null}
              </article>

              <article className="card">
                <h4>Recent Events</h4>
                <ul className="events">
                  {events.slice(0, 10).map((ev, idx) => (
                    <li key={`${ev.timestamp}-${idx}`}>
                      <div>
                        <strong>{ev.title}</strong> <span className="meta">{ev.timestamp}</span>
                      </div>
                      <pre>{ev.body}</pre>
                    </li>
                  ))}
                </ul>
              </article>
            </>
          ) : (
            <p>Select a ticket to continue.</p>
          )}
        </section>
      </main>
    </div>
  );
}
