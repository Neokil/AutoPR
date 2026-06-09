import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "../App";

const apiMocks = vi.hoisted(() => ({
  applyAction: vi.fn(),
  cleanupAll: vi.fn(),
  cleanupDone: vi.fn(),
  cleanupTicket: vi.fn(),
  connectEvents: vi.fn(),
  discoverTickets: vi.fn(),
  getArtifact: vi.fn(),
  getExecutionLogs: vi.fn(),
  getHealth: vi.fn(),
  getJob: vi.fn(),
  getTicket: vi.fn(),
  listRepositories: vi.fn(),
  listTickets: vi.fn(),
  moveToState: vi.fn(),
  runTicket: vi.fn()
}));

vi.mock("../api", () => apiMocks);

let eventHandler: ((evt: Record<string, unknown>) => void) | null = null;

function makeSummary(overrides: Record<string, unknown> = {}) {
  return {
    repo_id: "repo1",
    repo_path: "/repo1",
    ticket_number: "GH-5",
    title: "Structured feedback",
    status: "waiting",
    busy: false,
    approved: false,
    updated_at: "2024-01-01T00:00:00Z",
    ...overrides
  };
}

function makeDetails(ticketNumber = "GH-5", title = "Structured feedback", overrides: Record<string, unknown> = {}) {
  return {
    repo_id: "repo1",
    repo_path: "/repo1",
    ticket_number: ticketNumber,
    ticket: {
      title,
      url: `https://github.com/Neokil/AutoPR/issues/${ticketNumber.replace("GH-", "")}`
    },
    workflow_states: [
      { name: "implementation", display_name: "Implementation" },
      { name: "cancelled", display_name: "Cancelled" },
      { name: "review", display_name: "Review" }
    ],
    available_actions: [
      { label: "Provide Feedback", type: "provide_feedback" },
      { label: "Approve", type: "move_to_state", target: "implementation" },
      { label: "Cancel", type: "move_to_state", target: "cancelled" }
    ],
    state: {
      ticket_number: ticketNumber,
      current_state: "investigation",
      current_run_id: "run-2",
      flow_status: "waiting",
      branch_name: "",
      worktree_path: "",
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
      state_history: [
        {
          id: "run-1",
          state_name: "investigation",
          state_display_name: "Investigation",
          artifact_ref: "runs/run-1/artifacts/investigation.md",
          started_at: "2024-01-01T00:00:00Z"
        },
        {
          id: "run-2",
          state_name: "investigation",
          state_display_name: "Investigation",
          artifact_ref: "runs/run-2/artifacts/investigation.md",
          started_at: "2024-01-02T00:00:00Z"
        }
      ]
    },
    ...overrides
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  eventHandler = null;
  apiMocks.connectEvents.mockImplementation((onEvent: (evt: Record<string, unknown>) => void) => {
    eventHandler = onEvent;
    return { close: vi.fn() };
  });
  apiMocks.listTickets.mockResolvedValue([makeSummary()]);
  apiMocks.listRepositories.mockResolvedValue(["/repo1"]);
  apiMocks.getHealth.mockResolvedValue({ discover_tickets_configured: false });
  apiMocks.getExecutionLogs.mockResolvedValue([]);
  apiMocks.getTicket.mockImplementation(async (_repoPath: string, ticketNumber: string) =>
    makeDetails(ticketNumber, ticketNumber === "GH-6" ? "Second ticket" : "Structured feedback")
  );
  apiMocks.getArtifact.mockImplementation(async (_repoPath: string, _ticket: string, artifactRef: string) => {
    if (artifactRef.includes("run-1")) {
      return `## Open Questions

1. Old question?
`;
    }
    return `## Open Questions

1. Current question?
`;
  });
  apiMocks.applyAction.mockResolvedValue({
    status: "accepted",
    job_id: "job-1",
    action: "action",
    repo_id: "repo1",
    repo_path: "/repo1",
    ticket_number: "GH-5"
  });
  apiMocks.cleanupDone.mockResolvedValue({
    status: "accepted",
    job_id: "job-cleanup-done",
    action: "cleanup_done",
    repo_id: "repo1",
    repo_path: "/repo1"
  });
  apiMocks.cleanupAll.mockResolvedValue({
    status: "accepted",
    job_id: "job-cleanup-all",
    action: "cleanup_all",
    repo_id: "repo1",
    repo_path: "/repo1"
  });
  apiMocks.moveToState.mockResolvedValue({
    status: "accepted",
    job_id: "job-2",
    action: "move_to_state",
    repo_id: "repo1",
    repo_path: "/repo1",
    ticket_number: "GH-5"
  });
  apiMocks.runTicket.mockResolvedValue({
    status: "accepted",
    job_id: "job-3",
    action: "run",
    repo_id: "repo1",
    repo_path: "/repo1",
    ticket_number: "GH-5"
  });
  apiMocks.getJob.mockResolvedValue({
    id: "job-1",
    action: "action",
    repo_id: "repo1",
    repo_path: "/repo1",
    ticket_number: "GH-5",
    status: "failed",
    error: "job failed",
    created_at: "2024-01-02T00:00:00Z"
  });
});

describe("App", () => {
  it("submits structured feedback from the current run even when an older run is selected", async () => {
    render(<App />);

    expect(await screen.findByPlaceholderText("Answer open question 1")).toBeInTheDocument();
    expect(screen.getAllByText("Current question?").length).toBeGreaterThan(0);

    fireEvent.click(screen.getByRole("button", { name: "Investigation 1" }));

    await waitFor(() => {
      expect(screen.getAllByText("Current question?").length).toBeGreaterThan(0);
    });

    const answerField = screen.getByPlaceholderText("Answer open question 1");
    const additionalField = screen.getByPlaceholderText("Add any additional context");

    fireEvent.change(answerField, { target: { value: "Current answer" } });
    fireEvent.change(additionalField, { target: { value: "Extra context" } });
    fireEvent.click(screen.getByRole("button", { name: "Provide Feedback" }));

    await waitFor(() => {
      expect(apiMocks.applyAction).toHaveBeenCalledWith(
        "/repo1",
        "GH-5",
        "Provide Feedback",
        expect.stringContaining("Current question?")
      );
    });
    expect(apiMocks.applyAction).toHaveBeenCalledWith(
      "/repo1",
      "GH-5",
      "Provide Feedback",
      expect.not.stringContaining("Old question?")
    );
    expect(apiMocks.applyAction).toHaveBeenCalledWith(
      "/repo1",
      "GH-5",
      "Provide Feedback",
      expect.stringContaining("## Additional Feedback")
    );
  });

  it("shows an optimistic upcoming state immediately after approve", async () => {
    render(<App />);

    expect(await screen.findByRole("button", { name: "Approve" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));

    await waitFor(() => {
      expect(screen.getByText("Running Implementation")).toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: "Implementation" })).toBeInTheDocument();
    expect(screen.getByText("GH-5 - Structured feedback (Implementation)")).toBeInTheDocument();
  });

  it("shows an optimistic rerun immediately after structured feedback submit", async () => {
    render(<App />);

    expect(await screen.findByPlaceholderText("Answer open question 1")).toBeInTheDocument();
    fireEvent.change(screen.getByPlaceholderText("Answer open question 1"), { target: { value: "Answer" } });
    fireEvent.click(screen.getByRole("button", { name: "Provide Feedback" }));

    await waitFor(() => {
      expect(screen.getByText("Running Investigation 3")).toBeInTheDocument();
    });
  });

  it("applies the same optimistic behavior to overflow move-to-state", async () => {
    render(<App />);

    expect(await screen.findByRole("button", { name: "☰" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "☰" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Implementation" }));

    await waitFor(() => {
      expect(apiMocks.moveToState).toHaveBeenCalledWith("/repo1", "GH-5", "implementation");
    });
    expect(screen.getByText("Running Implementation")).toBeInTheDocument();
  });

  it("rolls back the optimistic run when the tracked job fails", async () => {
    render(<App />);

    expect(await screen.findByRole("button", { name: "Approve" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));

    await waitFor(() => {
      expect(screen.getByText("Running Implementation")).toBeInTheDocument();
    });

    eventHandler?.({
      type: "job",
      repo_id: "repo1",
      repo_path: "/repo1",
      ticket_number: "GH-5",
      job_id: "job-1",
      status: "failed",
      action: "action",
      error: "job failed"
    });

    await waitFor(() => {
      expect(screen.queryByText("Running Implementation")).not.toBeInTheDocument();
    });
    expect(screen.getAllByText("Current question?").length).toBeGreaterThan(0);
    expect(screen.getByText(/Job `job-1`: action \(failed\) - job failed/)).toBeInTheDocument();
  });

  it("keeps another ticket actionable while one ticket has an optimistic pending job", async () => {
    apiMocks.listTickets.mockResolvedValue([
      makeSummary(),
      makeSummary({ ticket_number: "GH-6", title: "Second ticket" })
    ]);

    render(<App />);

    expect(await screen.findByRole("button", { name: "Approve" })).toBeEnabled();

    fireEvent.click(screen.getByRole("button", { name: "Approve" }));

    await waitFor(() => {
      expect(apiMocks.applyAction).toHaveBeenCalledWith("/repo1", "GH-5", "Approve");
    });
    expect(screen.getAllByLabelText("Worker running")).toHaveLength(1);

    fireEvent.click(screen.getByRole("button", { name: /GH-6/i }));

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Approve" })).toBeEnabled();
    });
  });

  it("disables same-ticket controls immediately after queueing an action", async () => {
    render(<App />);

    expect(await screen.findByRole("button", { name: "Approve" })).toBeEnabled();
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));

    await waitFor(() => {
      expect(apiMocks.applyAction).toHaveBeenCalledWith("/repo1", "GH-5", "Approve");
    });
    expect(screen.getByRole("button", { name: "Approve" })).toBeDisabled();
    expect(screen.getAllByLabelText("Worker running")).toHaveLength(1);
  });

  it("keeps repo-wide cleanup actions available while a ticket job is pending", async () => {
    render(<App />);

    expect(await screen.findByRole("button", { name: "Approve" })).toBeEnabled();
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));

    await waitFor(() => {
      expect(apiMocks.applyAction).toHaveBeenCalledWith("/repo1", "GH-5", "Approve");
    });
    expect(screen.getByRole("button", { name: "Cleanup Done" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Cleanup All" })).toBeEnabled();
  });

  it("shows failed jobs only for the affected ticket", async () => {
    apiMocks.listTickets.mockResolvedValue([
      makeSummary(),
      makeSummary({ ticket_number: "GH-6", title: "Second ticket" })
    ]);

    render(<App />);

    expect(await screen.findByRole("button", { name: "Approve" })).toBeEnabled();
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));

    await waitFor(() => {
      expect(apiMocks.applyAction).toHaveBeenCalledWith("/repo1", "GH-5", "Approve");
    });

    eventHandler?.({
      type: "job",
      repo_id: "repo1",
      repo_path: "/repo1",
      ticket_number: "GH-5",
      job_id: "job-1",
      action: "action",
      status: "failed",
      error: "job failed"
    });

    expect(await screen.findByText(/Job `job-1`: action \(failed\) - job failed/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /GH-6/i }));

    await waitFor(() => {
      expect(screen.queryByText(/Job `job-1`: action \(failed\) - job failed/)).not.toBeInTheDocument();
    });
  });
});
