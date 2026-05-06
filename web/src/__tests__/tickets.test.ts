import { describe, it, expect } from "vitest";
import type { TicketDetails, TicketSummary, StateRun, ServerEvent } from "../types";
import {
  runDisplayLabel,
  selectTicketKey,
  applyTicketEvent,
  getFeedbackAction,
  getNonFeedbackActions,
} from "../tickets";

function makeSummary(partial: Partial<TicketSummary> = {}): TicketSummary {
  return {
    repo_id: "repo1",
    repo_path: "/repo1",
    ticket_number: "1",
    status: "waiting",
    busy: false,
    approved: false,
    updated_at: "2024-01-01T00:00:00Z",
    ...partial,
  };
}

function makeRun(partial: Partial<StateRun> = {}): StateRun {
  return {
    id: "run-1",
    state_name: "investigate",
    started_at: "2024-01-01T00:00:00Z",
    ...partial,
  };
}

function makeDetails(actions: TicketDetails["available_actions"]): TicketDetails {
  return {
    repo_id: "repo1",
    repo_path: "/repo1",
    ticket_number: "1",
    workflow_states: [],
    available_actions: actions,
    state: {
      ticket_number: "1",
      current_state: "investigate",
      flow_status: "waiting",
      branch_name: "",
      worktree_path: "",
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
    },
  };
}

// ── runDisplayLabel ────────────────────────────────────────────────────────
// The duplicate-indexing behaviour is non-obvious: when the same state is
// visited more than once, pills get a 1-based suffix so the user can tell them
// apart.
describe("runDisplayLabel", () => {
  it("returns the display name for a single-visit state", () => {
    const run = makeRun({ state_display_name: "Investigation" });
    expect(runDisplayLabel(run, [run])).toBe("Investigation");
  });

  it("appends a 1-based index when a state is visited more than once", () => {
    const run1 = makeRun({ id: "a", state_name: "investigate", state_display_name: "Investigate" });
    const run2 = makeRun({ id: "b", state_name: "investigate", state_display_name: "Investigate" });
    expect(runDisplayLabel(run1, [run1, run2])).toBe("Investigate 1");
    expect(runDisplayLabel(run2, [run1, run2])).toBe("Investigate 2");
  });
});

// ── selectTicketKey ────────────────────────────────────────────────────────
describe("selectTicketKey", () => {
  it("returns empty string when there are no tickets", () => {
    expect(selectTicketKey("r1::1", [])).toBe("");
  });

  it("falls back to the first ticket when the current key is no longer in the list", () => {
    const t = makeSummary({ repo_id: "r1", ticket_number: "2" });
    expect(selectTicketKey("r1::99", [t])).toBe("r1::2");
  });
});

// ── getFeedbackAction ──────────────────────────────────────────────────────
describe("getFeedbackAction", () => {
  it("returns the provide_feedback action when the ticket is waiting", () => {
    const details = makeDetails([
      { label: "Provide Feedback", type: "provide_feedback" },
      { label: "Approve", type: "move_to_state" },
    ]);
    expect(getFeedbackAction(details, makeSummary({ status: "waiting" }))).toEqual({
      label: "Provide Feedback",
      type: "provide_feedback",
    });
  });

  it("returns undefined when the ticket is not in waiting status", () => {
    expect(getFeedbackAction(null, makeSummary({ status: "running" }))).toBeUndefined();
  });

  it("returns undefined when there is no provide_feedback action", () => {
    const details = makeDetails([{ label: "Approve", type: "move_to_state" }]);
    expect(getFeedbackAction(details, makeSummary({ status: "waiting" }))).toBeUndefined();
  });
});

// ── getNonFeedbackActions ──────────────────────────────────────────────────
describe("getNonFeedbackActions", () => {
  it("returns only non-provide_feedback actions when ticket is waiting", () => {
    const details = makeDetails([
      { label: "Provide Feedback", type: "provide_feedback" },
      { label: "Approve", type: "move_to_state" },
    ]);
    expect(getNonFeedbackActions(details, makeSummary({ status: "waiting" }))).toEqual([
      { label: "Approve", type: "move_to_state" },
    ]);
  });

  it("returns empty array when the ticket is not waiting", () => {
    expect(getNonFeedbackActions(null, makeSummary({ status: "running" }))).toEqual([]);
  });
});

// ── applyTicketEvent ───────────────────────────────────────────────────────
// This is the heart of the real-time UI: SSE events mutate the ticket list
// without a full server round-trip.
describe("applyTicketEvent", () => {
  const ticket = makeSummary({ repo_id: "r1", ticket_number: "42", status: "waiting", title: "Old Title" });

  it("removes the ticket on ticket_deleted", () => {
    const evt: ServerEvent = { type: "ticket_deleted", repo_id: "r1", ticket_number: "42" };
    const { tickets, needsFullRefresh } = applyTicketEvent([ticket], evt);
    expect(tickets).toHaveLength(0);
    expect(needsFullRefresh).toBe(false);
  });

  it("updates title and status on ticket_updated", () => {
    const evt: ServerEvent = { type: "ticket_updated", repo_id: "r1", ticket_number: "42", title: "New Title", status: "done" };
    const { tickets } = applyTicketEvent([ticket], evt);
    expect(tickets[0].title).toBe("New Title");
    expect(tickets[0].status).toBe("done");
  });

  it("signals needsFullRefresh when an update arrives for an unknown ticket", () => {
    const evt: ServerEvent = { type: "ticket_updated", repo_id: "r1", ticket_number: "unknown" };
    expect(applyTicketEvent([ticket], evt).needsFullRefresh).toBe(true);
  });

  it("appends a new job and marks the ticket as busy", () => {
    const evt: ServerEvent = { type: "job", repo_id: "r1", ticket_number: "42", job_id: "j1", status: "running", action: "run_ticket" };
    const { tickets } = applyTicketEvent([ticket], evt);
    expect(tickets[0].busy).toBe(true);
    expect(tickets[0].jobs![0].id).toBe("j1");
  });

  it("leaves other tickets unchanged", () => {
    const other = makeSummary({ repo_id: "r1", ticket_number: "99" });
    const evt: ServerEvent = { type: "ticket_deleted", repo_id: "r1", ticket_number: "42" };
    const { tickets } = applyTicketEvent([ticket, other], evt);
    expect(tickets).toHaveLength(1);
    expect(tickets[0].ticket_number).toBe("99");
  });

  it("is a no-op when repo_id or ticket_number is absent from the event", () => {
    const evt: ServerEvent = { type: "ticket_updated" };
    const { tickets, needsFullRefresh } = applyTicketEvent([ticket], evt);
    expect(tickets).toEqual([ticket]);
    expect(needsFullRefresh).toBe(false);
  });
});
