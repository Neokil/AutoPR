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

beforeEach(() => {
  vi.clearAllMocks();
  apiMocks.connectEvents.mockReturnValue({ close: vi.fn() });
  apiMocks.listTickets.mockResolvedValue([
    {
      repo_id: "repo1",
      repo_path: "/repo1",
      ticket_number: "GH-5",
      title: "Structured feedback",
      status: "waiting",
      busy: false,
      approved: false,
      updated_at: "2024-01-01T00:00:00Z"
    }
  ]);
  apiMocks.listRepositories.mockResolvedValue(["/repo1"]);
  apiMocks.getHealth.mockResolvedValue({ discover_tickets_configured: false });
  apiMocks.getExecutionLogs.mockResolvedValue([]);
  apiMocks.getTicket.mockResolvedValue({
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
    }
  });
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
    action: "apply_action",
    repo_id: "repo1",
    repo_path: "/repo1"
  });
});

describe("App structured feedback", () => {
  it("submits structured feedback from the current run even when an older run is selected", async () => {
    render(<App />);

    expect(await screen.findByText("Current question?")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Investigation 1" }));

    await waitFor(() => {
      expect(screen.getByText("Current question?")).toBeInTheDocument();
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
});
