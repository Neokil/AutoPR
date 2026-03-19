import type { Job, TicketSummary } from "./types";

function ticketKey(t: TicketSummary): string {
  return `${t.repo_id}::${t.ticket_number}`;
}

function summarizeJobAction(action: string): string {
  switch (action) {
    case "apply_pr_comments":
      return "apply comments";
    case "cleanup_ticket":
      return "cleanup";
    case "cleanup_done":
      return "cleanup done";
    case "cleanup_all":
      return "cleanup all";
    default:
      return action.split("_").join(" ");
  }
}

type TicketListProps = {
  tickets: TicketSummary[];
  selectedKey: string;
  onSelectTicket: (key: string) => void;
  onAddTicket: () => void;
};

function renderJobChip(job: Job) {
  return (
    <span key={job.id} className={`job-chip job-${job.status}`}>
      {summarizeJobAction(job.action)} · {job.status}
    </span>
  );
}

export function TicketList({ tickets, selectedKey, onSelectTicket, onAddTicket }: TicketListProps) {
  return (
    <section className="panel left">
      <div className="panel-header">
        <h2>Tickets (All Repos)</h2>
      </div>
      <ul className="ticket-list">
        {tickets.map((ticket) => (
          <li key={ticketKey(ticket)}>
            <button
              className={selectedKey === ticketKey(ticket) ? "ticket-item active" : "ticket-item"}
              onClick={() => onSelectTicket(ticketKey(ticket))}
            >
              <strong>{ticket.ticket_number}</strong>
              <span>{ticket.title || "(no title)"}</span>
              <div className="ticket-status-row">
                {ticket.busy ? <span className="spinner" title="Worker is running" aria-label="Worker running" /> : null}
                <span className="meta">
                  {ticket.status} {ticket.approved ? "· approved" : ""}
                </span>
              </div>
              {ticket.jobs && ticket.jobs.length > 0 ? <div className="ticket-jobs-row">{ticket.jobs.map(renderJobChip)}</div> : null}
              <span className="meta">{ticket.repo_path}</span>
            </button>
          </li>
        ))}
      </ul>
      <div className="ticket-list-footer">
        <button onClick={onAddTicket}>Add Ticket</button>
      </div>
    </section>
  );
}
