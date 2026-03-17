import { FormEvent, useEffect, useMemo, useState } from "react";
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

const DEFAULT_REPO_PATH = "";

export function App() {
  const [repoPathInput, setRepoPathInput] = useState(
    localStorage.getItem("ai_orchestrator_repo_path") ?? DEFAULT_REPO_PATH
  );
  const [repoPath, setRepoPath] = useState(repoPathInput);
  const [tickets, setTickets] = useState<TicketSummary[]>([]);
  const [selectedTicket, setSelectedTicket] = useState<string>("");
  const [details, setDetails] = useState<TicketDetails | null>(null);
  const [events, setEvents] = useState<EventItem[]>([]);
  const [feedbackMessage, setFeedbackMessage] = useState("");
  const [activeJobId, setActiveJobId] = useState<string>("");
  const [activeJob, setActiveJob] = useState<Job | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>("");

  useEffect(() => {
    localStorage.setItem("ai_orchestrator_repo_path", repoPathInput);
  }, [repoPathInput]);

  useEffect(() => {
    if (!repoPath) {
      setTickets([]);
      setDetails(null);
      setEvents([]);
      return;
    }
    void refreshTickets(repoPath);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [repoPath]);

  useEffect(() => {
    if (!repoPath || !selectedTicket) {
      setDetails(null);
      setEvents([]);
      return;
    }
    void refreshTicketDetails(repoPath, selectedTicket);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [repoPath, selectedTicket]);

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
          await refreshTickets(repoPath);
          if (selectedTicket) {
            await refreshTicketDetails(repoPath, selectedTicket);
          }
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : "failed to poll job");
      }
    }, 1000);
    return () => clearInterval(timer);
  }, [activeJobId, repoPath, selectedTicket]);

  const selectedSummary = useMemo(
    () => tickets.find((t) => t.ticket_number === selectedTicket) ?? null,
    [tickets, selectedTicket]
  );

  async function refreshTickets(path: string) {
    if (!path) {
      return;
    }
    setLoading(true);
    setError("");
    try {
      const data = await listTickets(path);
      setTickets(data);
      if (!selectedTicket && data.length > 0) {
        setSelectedTicket(data[0].ticket_number);
      } else if (selectedTicket && !data.find((t) => t.ticket_number === selectedTicket)) {
        setSelectedTicket(data[0]?.ticket_number ?? "");
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to load tickets");
    } finally {
      setLoading(false);
    }
  }

  async function refreshTicketDetails(path: string, ticket: string) {
    if (!path || !ticket) {
      return;
    }
    setLoading(true);
    setError("");
    try {
      const [ticketDetails, eventItems] = await Promise.all([getTicket(path, ticket), getEvents(path, ticket)]);
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

  function onConnectRepo(e: FormEvent) {
    e.preventDefault();
    setRepoPath(repoPathInput.trim());
  }

  return (
    <div className="app">
      <header className="header">
        <h1>AI Orchestrator</h1>
        <form onSubmit={onConnectRepo} className="repo-form">
          <input
            value={repoPathInput}
            onChange={(e) => setRepoPathInput(e.target.value)}
            placeholder="/absolute/path/to/repo"
          />
          <button type="submit">Load Repo</button>
        </form>
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
            <h2>Tickets</h2>
            <button onClick={() => void refreshTickets(repoPath)} disabled={!repoPath || loading}>
              Refresh
            </button>
          </div>
          <div className="button-row">
            <button onClick={() => void queueAction(() => cleanupDone(repoPath))} disabled={!repoPath}>
              Cleanup Done
            </button>
            <button onClick={() => void queueAction(() => cleanupAll(repoPath))} disabled={!repoPath}>
              Cleanup All
            </button>
          </div>
          <ul className="ticket-list">
            {tickets.map((t) => (
              <li key={t.repo_id + t.ticket_number}>
                <button
                  className={selectedTicket === t.ticket_number ? "ticket-item active" : "ticket-item"}
                  onClick={() => setSelectedTicket(t.ticket_number)}
                >
                  <strong>{t.ticket_number}</strong>
                  <span>{t.title || "(no title)"}</span>
                  <span className="meta">
                    {t.status} {t.approved ? "· approved" : ""}
                  </span>
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

          {selectedTicket ? (
            <>
              <div className="button-row">
                <button onClick={() => void queueAction(() => runTicket(repoPath, selectedTicket))}>Run</button>
                <button onClick={() => void queueAction(() => resumeTicket(repoPath, selectedTicket))}>Resume</button>
                <button onClick={() => void queueAction(() => approveTicket(repoPath, selectedTicket))}>
                  Approve
                </button>
                <button onClick={() => void queueAction(() => rejectTicket(repoPath, selectedTicket))}>Reject</button>
                <button onClick={() => void queueAction(() => createPR(repoPath, selectedTicket))}>Create PR</button>
                <button onClick={() => void queueAction(() => cleanupTicket(repoPath, selectedTicket))}>Cleanup</button>
              </div>

              <form
                className="feedback-form"
                onSubmit={(e) => {
                  e.preventDefault();
                  if (!feedbackMessage.trim()) {
                    return;
                  }
                  void queueAction(() => feedbackTicket(repoPath, selectedTicket, feedbackMessage)).then(() =>
                    setFeedbackMessage("")
                  );
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
