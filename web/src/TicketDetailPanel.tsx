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
  openQuestions: string[];
  questionAnswers: Record<string, string>;
  generalFeedback: string;
  isRunning: boolean;
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
  feedbackAction,
  openQuestions,
  questionAnswers,
  generalFeedback,
  isRunning,
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
              <button key={action.label} onClick={() => onApplyAction(action.label)} disabled={isRunning}>
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

      {details?.state.last_error ? (
        <div className="banner error">{details.state.last_error}</div>
      ) : null}

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
              <button type="submit">{feedbackAction.label}</button>
            </div>
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
