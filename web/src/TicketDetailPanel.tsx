import { MarkdownView } from "./MarkdownView";
import { StateTimeline } from "./StateTimeline";
import { TicketMenu } from "./TicketMenu";
import { getNonFeedbackActions, runDisplayLabel, ticketTitle, ticketURL } from "./tickets";
import type { ActionInfo, StateRun, TicketDetails, TicketSummary } from "./types";

type TicketDetailPanelProps = {
  selectedSummary: TicketSummary | null;
  details: TicketDetails | null;
  stateRuns: StateRun[];
  selectedRunId: string;
  selectedArtifactContent: string;
  artifactLoading: boolean;
  feedbackAction?: ActionInfo;
  feedbackMessage: string;
  onSelectRun: (runId: string) => void;
  onFeedbackMessageChange: (value: string) => void;
  onSubmitFeedback: () => void;
  onApplyAction: (label: string) => void;
  onOpenLogs: () => void;
  onRerun: () => void;
  onCleanup: () => void;
  onMoveToState: (target: string) => void;
};

export function TicketDetailPanel({
  selectedSummary,
  details,
  stateRuns,
  selectedRunId,
  selectedArtifactContent,
  artifactLoading,
  feedbackAction,
  feedbackMessage,
  onSelectRun,
  onFeedbackMessageChange,
  onSubmitFeedback,
  onApplyAction,
  onOpenLogs,
  onRerun,
  onCleanup,
  onMoveToState
}: TicketDetailPanelProps) {
  if (!selectedSummary) {
    return (
      <section className="panel right">
        <p>Select a ticket to continue.</p>
      </section>
    );
  }

  const selectedRun = stateRuns.find((run) => run.id === selectedRunId) ?? null;
  const actionButtons = getNonFeedbackActions(details, selectedSummary);
  const url = ticketURL(details);
  const title = ticketTitle(details, selectedSummary);

  return (
    <section className="panel right">
      <div className="detail-top-row">
        <h2 className="detail-main-title">
          {url ? (
            <a href={url} target="_blank" rel="noreferrer">
              {selectedSummary.ticket_number} - {title} ({selectedSummary.status})
            </a>
          ) : (
            <>
              {selectedSummary.ticket_number} - {title} ({selectedSummary.status})
            </>
          )}
        </h2>
        <div className="detail-actions-wrap">
          <div className="button-row detail-actions">
            {actionButtons.map((action) => (
              <button key={action.label} onClick={() => onApplyAction(action.label)}>
                {action.label}
              </button>
            ))}
            {selectedSummary.pr_url ? (
              <a href={selectedSummary.pr_url} target="_blank" rel="noreferrer">
                <button type="button">Open PR</button>
              </a>
            ) : null}
          </div>
          <TicketMenu
            onLogs={onOpenLogs}
            onRerun={onRerun}
            onCleanup={onCleanup}
            onMoveToState={onMoveToState}
            workflowStates={details?.workflow_states ?? []}
            currentStateName={details?.state.current_state}
            rerunLabel={selectedSummary.status === "failed" ? "Retry" : "Rerun"}
            rerunDisabled={selectedSummary.status === "running"}
            cleanupDisabled={selectedSummary.status === "running"}
            moveDisabled={selectedSummary.status === "running"}
          />
        </div>
      </div>

      <article className="card">
        <span className="field-label">Timeline</span>
        <StateTimeline runs={stateRuns} selectedRunId={selectedRunId} onSelectRun={onSelectRun} />
        {feedbackAction ? (
          <form
            className="feedback-form"
            onSubmit={(event) => {
              event.preventDefault();
              onSubmitFeedback();
            }}
          >
            <input
              value={feedbackMessage}
              onChange={(event) => onFeedbackMessageChange(event.target.value)}
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
    </section>
  );
}
