import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { TicketDetailPanel } from "../TicketDetailPanel";
import type { DisplayStateRun, Job, TicketDetails, TicketSummary } from "../types";

function makeSummary(partial: Partial<TicketSummary> = {}): TicketSummary {
  return {
    repo_id: "repo1",
    repo_path: "/repo1",
    ticket_number: "GH-5",
    title: "Structured feedback",
    status: "waiting",
    busy: false,
    approved: false,
    updated_at: "2024-01-01T00:00:00Z",
    ...partial
  };
}

function makeDetails(): TicketDetails {
  return {
    repo_id: "repo1",
    repo_path: "/repo1",
    ticket_number: "GH-5",
    workflow_states: [],
    available_actions: [{ label: "Provide Feedback", type: "provide_feedback" }],
    state: {
      ticket_number: "GH-5",
      current_state: "investigation",
      current_run_id: "run-2",
      flow_status: "waiting",
      branch_name: "",
      worktree_path: "",
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
      state_history: [
        { id: "run-1", state_name: "investigation", state_display_name: "Investigation", started_at: "2024-01-01T00:00:00Z" },
        { id: "run-2", state_name: "investigation", state_display_name: "Investigation", started_at: "2024-01-02T00:00:00Z" }
      ]
    }
  };
}

function makeJob(partial: Partial<Job> = {}): Job {
  return {
    id: "job-1",
    action: "run",
    repo_id: "repo1",
    repo_path: "/repo1",
    ticket_number: "GH-5",
    status: "failed",
    error: "job failed",
    created_at: "2024-01-01T00:00:00Z",
    ...partial
  };
}

describe("TicketDetailPanel", () => {
  it("renders one textarea per open question plus a general feedback textarea", () => {
    render(
      <TicketDetailPanel
        selectedSummary={makeSummary()}
        details={makeDetails()}
        stateRuns={makeDetails().state.state_history ?? []}
        selectedRunId="run-2"
        selectedArtifactContent="artifact"
        artifactLoading={false}
        statusLabel="waiting"
        feedbackAction={{ label: "Provide Feedback", type: "provide_feedback" }}
        openQuestions={["What should happen first?", "What should happen second?"]}
        questionAnswers={{ "0": "First answer" }}
        generalFeedback="General feedback"
        actionsDisabled={false}
        feedbackDisabled={false}
        cleanupDisabled={false}
        moveDisabled={false}
        rerunDisabled={false}
        jobFailure={null}
        onSelectRun={vi.fn()}
        onQuestionAnswerChange={vi.fn()}
        onGeneralFeedbackChange={vi.fn()}
        onSubmitFeedback={vi.fn()}
        onApplyAction={vi.fn()}
        onOpenLogs={vi.fn()}
        onRerun={vi.fn()}
        onCleanup={vi.fn()}
        onMoveToState={vi.fn()}
      />
    );

    expect(screen.getByText("Open Question 1")).toBeInTheDocument();
    expect(screen.getByText("What should happen first?")).toBeInTheDocument();
    expect(screen.getByText("Open Question 2")).toBeInTheDocument();
    expect(screen.getByText("Additional Feedback")).toBeInTheDocument();
    expect(screen.getAllByRole("textbox")).toHaveLength(3);
    expect(screen.getByRole("button", { name: "Provide Feedback" })).toBeInTheDocument();
  });

  it("falls back to a single general feedback textarea when there are no open questions", () => {
    const onGeneralFeedbackChange = vi.fn();
    render(
      <TicketDetailPanel
        selectedSummary={makeSummary()}
        details={makeDetails()}
        stateRuns={makeDetails().state.state_history ?? []}
        selectedRunId="run-2"
        selectedArtifactContent="artifact"
        artifactLoading={false}
        statusLabel="waiting"
        feedbackAction={{ label: "Provide Feedback", type: "provide_feedback" }}
        openQuestions={[]}
        questionAnswers={{}}
        generalFeedback=""
        actionsDisabled={false}
        feedbackDisabled={false}
        cleanupDisabled={false}
        moveDisabled={false}
        rerunDisabled={false}
        jobFailure={null}
        onSelectRun={vi.fn()}
        onQuestionAnswerChange={vi.fn()}
        onGeneralFeedbackChange={onGeneralFeedbackChange}
        onSubmitFeedback={vi.fn()}
        onApplyAction={vi.fn()}
        onOpenLogs={vi.fn()}
        onRerun={vi.fn()}
        onCleanup={vi.fn()}
        onMoveToState={vi.fn()}
      />
    );

    const textarea = screen.getByPlaceholderText("Send feedback (Provide Feedback)");
    fireEvent.change(textarea, { target: { value: "General note" } });
    expect(onGeneralFeedbackChange).toHaveBeenCalledWith("General note");
    expect(screen.getAllByRole("textbox")).toHaveLength(1);
  });

  it("renders a running placeholder for a synthetic optimistic run", () => {
    const runs: DisplayStateRun[] = [
      ...(makeDetails().state.state_history ?? []),
      {
        id: "optimistic-run",
        state_name: "implementation",
        state_display_name: "Implementation",
        started_at: "2024-01-03T00:00:00Z",
        synthetic: true
      }
    ];

    render(
      <TicketDetailPanel
        selectedSummary={makeSummary()}
        details={makeDetails()}
        stateRuns={runs}
        selectedRunId="optimistic-run"
        selectedArtifactContent=""
        artifactLoading={false}
        statusLabel="Implementation"
        openQuestions={[]}
        questionAnswers={{}}
        generalFeedback=""
        actionsDisabled={true}
        feedbackDisabled={true}
        cleanupDisabled={true}
        moveDisabled={true}
        rerunDisabled={true}
        jobFailure={null}
        onSelectRun={vi.fn()}
        onQuestionAnswerChange={vi.fn()}
        onGeneralFeedbackChange={vi.fn()}
        onSubmitFeedback={vi.fn()}
        onApplyAction={vi.fn()}
        onOpenLogs={vi.fn()}
        onRerun={vi.fn()}
        onCleanup={vi.fn()}
        onMoveToState={vi.fn()}
      />
    );

    expect(screen.getByText("Running Implementation")).toBeInTheDocument();
    expect(screen.getByText("Waiting for server confirmation.")).toBeInTheDocument();
    expect(screen.queryByText("No artifact path available.")).not.toBeInTheDocument();
  });

  it("shows a ticket-scoped job failure banner", () => {
    render(
      <TicketDetailPanel
        selectedSummary={makeSummary()}
        details={makeDetails()}
        stateRuns={makeDetails().state.state_history ?? []}
        selectedRunId="run-2"
        selectedArtifactContent="artifact"
        artifactLoading={false}
        statusLabel="waiting"
        feedbackAction={{ label: "Provide Feedback", type: "provide_feedback" }}
        openQuestions={[]}
        questionAnswers={{}}
        generalFeedback=""
        actionsDisabled={false}
        feedbackDisabled={false}
        cleanupDisabled={false}
        moveDisabled={false}
        rerunDisabled={false}
        jobFailure={makeJob()}
        onSelectRun={vi.fn()}
        onQuestionAnswerChange={vi.fn()}
        onGeneralFeedbackChange={vi.fn()}
        onSubmitFeedback={vi.fn()}
        onApplyAction={vi.fn()}
        onOpenLogs={vi.fn()}
        onRerun={vi.fn()}
        onCleanup={vi.fn()}
        onMoveToState={vi.fn()}
      />
    );

    expect(screen.getByText(/Job `job-1`: run \(failed\) - job failed/)).toBeInTheDocument();
  });
});
