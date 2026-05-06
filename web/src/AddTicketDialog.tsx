type AddTicketDialogProps = {
  knownRepoPaths: string[];
  repoPath: string;
  ticketNumber: string;
  error: string;
  onRepoPathChange: (value: string) => void;
  onTicketNumberChange: (value: string) => void;
  onSubmit: () => void;
  onClose: () => void;
};

export function AddTicketDialog({
  knownRepoPaths,
  repoPath,
  ticketNumber,
  error,
  onRepoPathChange,
  onTicketNumberChange,
  onSubmit,
  onClose
}: AddTicketDialogProps) {
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(event) => event.stopPropagation()}>
        <h3>Add Ticket</h3>
        <p className="meta">Schedule a ticket run for a repository.</p>
        {error ? <div className="banner error">{error}</div> : null}

        <label className="field-label" htmlFor="repo-path-input">
          Repository Folder
        </label>
        <input
          id="repo-path-input"
          list="repo-path-options"
          value={repoPath}
          onChange={(event) => onRepoPathChange(event.target.value)}
          placeholder="/absolute/path/to/repo"
        />
        <datalist id="repo-path-options">
          {knownRepoPaths.map((path) => (
            <option key={path} value={path} />
          ))}
        </datalist>

        <label className="field-label" htmlFor="ticket-number-input">
          Ticket Number
        </label>
        <input
          id="ticket-number-input"
          value={ticketNumber}
          onChange={(event) => onTicketNumberChange(event.target.value)}
          placeholder="e.g. 66825"
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault();
              onSubmit();
            }
          }}
        />

        <div className="button-row modal-actions">
          <button type="button" className="secondary" onClick={onClose}>
            Cancel
          </button>
          <button type="button" onClick={onSubmit}>
            Schedule Run
          </button>
        </div>
      </div>
    </div>
  );
}
