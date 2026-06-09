import type { DiscoveredTicket } from "./types";

type DiscoverTicketsModalProps = {
  repoPath: string;
  tickets: DiscoveredTicket[];
  loading: boolean;
  error: string;
  pendingTicketNumbers: string[];
  onAdd: (ticketNumber: string) => void;
  onClose: () => void;
};

export function DiscoverTicketsModal({
  repoPath,
  tickets,
  loading,
  error,
  pendingTicketNumbers,
  onAdd,
  onClose
}: DiscoverTicketsModalProps) {
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal discover-modal" onClick={(event) => event.stopPropagation()}>
        <h3>Discover Tickets</h3>
        <p className="meta">Available tickets for {repoPath}.</p>
        {error ? <div className="banner error">{error}</div> : null}
        {loading ? (
          <p className="meta">Fetching Tickets...</p>
        ) : tickets.length === 0 && !error ? (
          <p className="meta">No Tickets available.</p>
        ) : (
          <ul className="discover-list">
            {tickets.map((ticket) => (
              <li key={ticket.ticket_number} className="discover-item">
                <span className="discover-ticket-id">{ticket.ticket_number}</span>
                <span className="discover-ticket-title">{ticket.title}</span>
                <button
                  type="button"
                  disabled={pendingTicketNumbers.includes(ticket.ticket_number)}
                  onClick={() => onAdd(ticket.ticket_number)}
                >
                  {pendingTicketNumbers.includes(ticket.ticket_number) ? "Adding..." : "Add"}
                </button>
              </li>
            ))}
          </ul>
        )}
        <div className="button-row modal-actions">
          <button type="button" className="secondary" onClick={onClose}>
            Close
          </button>
        </div>
      </div>
    </div>
  );
}
