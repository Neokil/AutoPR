import { MarkdownView } from "./MarkdownView";
import { StateTimeline } from "./StateTimeline";
import { TicketMenu } from "./TicketMenu";
import { getNonFeedbackActions, runDisplayLabel, ticketTitle, ticketURL } from "./tickets";
import type { ActionInfo, DisplayStateRun, Job, TicketDetails, TicketSummary } from "./types";

type TicketDetailPanelProps = {
  selectedSummary: TicketSummary | null;
  details: TicketDetails | null;
  stateRuns: DisplayStateRun[];
  selectedRunId: string;
  selectedArtifactContent: string;
  artifactLoading: boolean;
  statusLabel: string;
  feedbackAction?: ActionInfo;
  openQuestions: string[];
  questionAnswers: Record<string, string>;
  generalFeedback: string;
  actionsDisabled: boolean;
  feedbackDisabled: boolean;
  cleanupDisabled: boolean;
  moveDisabled: boolean;
  rerunDisabled: boolean;
  jobFailure: Job | null;
  onSelectRun: (runId: string) => void;
  onQuestionAnswerChange: (index: number, value: string) => void;
  onGeneralFeedbackChange: (value: string) => void;
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
  statusLabel,
  feedbackAction,
  openQuestions,
  questionAnswers,
  generalFeedback,
  actionsDisabled,
  feedbackDisabled,
  cleanupDisabled,
  moveDisabled,
  rerunDisabled,
  jobFailure,
  onSelectRun,
  onQuestionAnswerChange,
  onGeneralFeedbackChange,
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
  const selectedRunLabel = selectedRun ? runDisplayLabel(selectedRun, stateRuns) : "";

  return (
    <section className="panel right">
      <div className="detail-top-row">
        <h2 className="detail-main-title">
          {url ? (
            <a href={url} target="_blank" rel="noreferrer">
              {selectedSummary.ticket_number} - {title} ({statusLabel})
            </a>
          ) : (
            <>
              {selectedSummary.ticket_number} - {title} ({statusLabel})
            </>
          )}
        </h2>
        <div className="detail-actions-wrap">
          <div className="button-row detail-actions">
            {actionButtons.map((action) => (
              <button key={action.label} onClick={() => onApplyAction(action.label)} disabled={actionsDisabled}>
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
            rerunDisabled={rerunDisabled}
            cleanupDisabled={cleanupDisabled}
            moveDisabled={moveDisabled}
          />
        </div>
      </div>

      {jobFailure?.error ? (
        <div className="banner error">
          Job `{jobFailure.id}`: {jobFailure.action} ({jobFailure.status}) - {jobFailure.error}
        </div>
      ) : null}
      {details?.state.last_error ? (
        <div className="banner error">{details.state.last_error}</div>
      ) : null}

      <article className="card">
        <span className="field-label">Timeline</span>
        <StateTimeline runs={stateRuns} selectedRunId={selectedRunId} onSelectRun={onSelectRun} />
        {selectedRun ? (
          <div className="timeline-content">
            <div className="timeline-content-header">
              <h4>{selectedRunLabel}</h4>
              <span className="meta">{new Date(selectedRun.started_at).toLocaleString()}</span>
            </div>
            {selectedRun.synthetic ? (
              <div className="timeline-running-placeholder">
                <div className="timeline-running-header">
                  <span className="spinner" aria-hidden="true" />
                  <strong>Running {selectedRunLabel}</strong>
                </div>
                <p className="meta">Waiting for server confirmation.</p>
              </div>
            ) : (
              <>
                <p className="meta artifact-path">{selectedRun.artifact_ref || selectedRun.log_ref || "No artifact path available."}</p>
                {artifactLoading ? <p className="meta">Loading artifact...</p> : null}
                <MarkdownView
                  content={selectedArtifactContent}
                  emptyText="No run artifact available."
                  githubBlobBase={details?.github_blob_base}
                  repoPath={details?.repo_path}
                  worktreePath={details?.state.worktree_path}
                />
              </>
            )}
          </div>
        ) : (
          <p className="meta">No workflow runs available yet.</p>
        )}
        {feedbackAction ? (
          <form
            className="feedback-form"
            onSubmit={(event) => {
              event.preventDefault();
              onSubmitFeedback();
            }}
          >
            {openQuestions.length > 0 ? (
              <>
                {openQuestions.map((question, index) => (
                  <label key={`${index}-${question}`} className="feedback-field">
                    <span className="field-label">Open Question {index + 1}</span>
                    <p className="feedback-question">{question}</p>
                    <textarea
                      value={questionAnswers[String(index)] ?? ""}
                      onChange={(event) => onQuestionAnswerChange(index, event.target.value)}
                      placeholder={`Answer open question ${index + 1}`}
                      rows={4}
                    />
                  </label>
                ))}
                <label className="feedback-field">
                  <span className="field-label">Additional Feedback</span>
                  <textarea
                    value={generalFeedback}
                    onChange={(event) => onGeneralFeedbackChange(event.target.value)}
                    placeholder="Add any additional context"
                    rows={4}
                  />
                </label>
              </>
            ) : (
              <textarea
                value={generalFeedback}
                onChange={(event) => onGeneralFeedbackChange(event.target.value)}
                placeholder={`Send feedback (${feedbackAction.label})`}
                rows={3}
              />
            )}
            <div className="feedback-submit">
              <button type="submit" disabled={feedbackDisabled}>{feedbackAction.label}</button>
            </div>
          </form>
        ) : null}
      </article>
    </section>
  );
}
