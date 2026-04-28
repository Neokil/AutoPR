import { describe, it, expect } from "vitest";
import type { TicketDetails, TicketSummary, StateRun, ServerEvent } from "../types";
import {
  ticketKey,
  pendingTicketKey,
  knownRepoPaths,
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

describe("ticketKey", () => {
  it("combines repo_id and ticket_number with ::", () => {
    expect(ticketKey(makeSummary({ repo_id: "r1", ticket_number: "42" }))).toBe("r1::42");
  });
});

describe("pendingTicketKey", () => {
  it("combines repo path and ticket number with ::", () => {
    expect(pendingTicketKey("/my/repo", "123")).toBe("/my/repo::123");
  });
});

describe("knownRepoPaths", () => {
  it("merges repository options and ticket paths without duplicates", () => {
    const tickets = [makeSummary({ repo_path: "/b" }), makeSummary({ repo_path: "/c" })];
    expect(knownRepoPaths(["/a", "/b"], tickets)).toEqual(["/a", "/b", "/c"]);
  });

  it("preserves the order: options first, then ticket paths", () => {
    const tickets = [makeSummary({ repo_path: "/c" })];
    expect(knownRepoPaths(["/a", "/b"], tickets)).toEqual(["/a", "/b", "/c"]);
  });

  it("returns empty array when both inputs are empty", () => {
    expect(knownRepoPaths([], [])).toEqual([]);
  });
});

describe("runDisplayLabel", () => {
  it("returns the display name when only one run for that state", () => {
    const run = makeRun({ state_display_name: "Investigation" });
    expect(runDisplayLabel(run, [run])).toBe("Investigation");
  });

  it("falls back to state_name when display_name is absent", () => {
    const run = makeRun({ state_name: "investigate", state_display_name: "" });
    expect(runDisplayLabel(run, [run])).toBe("investigate");
  });

  it("appends a 1-based index when multiple runs share the same state", () => {
    const run1 = makeRun({ id: "a", state_name: "investigate", state_display_name: "Investigate" });
    const run2 = makeRun({ id: "b", state_name: "investigate", state_display_name: "Investigate" });
    const runs = [run1, run2];
    expect(runDisplayLabel(run1, runs)).toBe("Investigate 1");
    expect(runDisplayLabel(run2, runs)).toBe("Investigate 2");
  });
});

describe("selectTicketKey", () => {
  it("returns an empty string when there are no tickets", () => {
    expect(selectTicketKey("r1::1", [])).toBe("");
  });

  it("keeps the current key when it is still in the list", () => {
    const t = makeSummary({ repo_id: "r1", ticket_number: "1" });
    expect(selectTicketKey("r1::1", [t])).toBe("r1::1");
  });

  it("falls back to the first ticket when the current key is gone", () => {
    const t = makeSummary({ repo_id: "r1", ticket_number: "2" });
    expect(selectTicketKey("r1::99", [t])).toBe("r1::2");
  });

  it("selects the first ticket when current is empty", () => {
    const t = makeSummary({ repo_id: "r1", ticket_number: "1" });
    expect(selectTicketKey("", [t])).toBe("r1::1");
  });
});

describe("getFeedbackAction", () => {
  it("returns the provide_feedback action when the ticket is waiting", () => {
    const summary = makeSummary({ status: "waiting" });
    const details = makeDetails([
      { label: "Provide Feedback", type: "provide_feedback" },
      { label: "Approve", type: "move_to_state" },
    ]);
    expect(getFeedbackAction(details, summary)).toEqual({ label: "Provide Feedback", type: "provide_feedback" });
  });

  it("returns undefined when the ticket is not waiting", () => {
    const summary = makeSummary({ status: "running" });
    expect(getFeedbackAction(null, summary)).toBeUndefined();
  });

  it("returns undefined when there is no provide_feedback action", () => {
    const summary = makeSummary({ status: "waiting" });
    const details = makeDetails([{ label: "Approve", type: "move_to_state" }]);
    expect(getFeedbackAction(details, summary)).toBeUndefined();
  });
});

describe("getNonFeedbackActions", () => {
  it("returns only non-provide_feedback actions when waiting", () => {
    const summary = makeSummary({ status: "waiting" });
    const details = makeDetails([
      { label: "Provide Feedback", type: "provide_feedback" },
      { label: "Approve", type: "move_to_state" },
    ]);
    expect(getNonFeedbackActions(details, summary)).toEqual([{ label: "Approve", type: "move_to_state" }]);
  });

  it("returns empty array when the ticket is not waiting", () => {
    const summary = makeSummary({ status: "running" });
    expect(getNonFeedbackActions(null, summary)).toEqual([]);
  });
});

describe("applyTicketEvent", () => {
  const ticket = makeSummary({ repo_id: "r1", ticket_number: "42", status: "waiting", title: "Old Title" });

  it("removes the ticket on ticket_deleted", () => {
    const evt: ServerEvent = { type: "ticket_deleted", repo_id: "r1", ticket_number: "42" };
    const result = applyTicketEvent([ticket], evt);
    expect(result.tickets).toHaveLength(0);
    expect(result.needsFullRefresh).toBe(false);
  });

  it("updates title and status on ticket_updated", () => {
    const evt: ServerEvent = { type: "ticket_updated", repo_id: "r1", ticket_number: "42", title: "New Title", status: "done" };
    const result = applyTicketEvent([ticket], evt);
    expect(result.tickets[0].title).toBe("New Title");
    expect(result.tickets[0].status).toBe("done");
    expect(result.needsFullRefresh).toBe(false);
  });

  it("signals needsFullRefresh when an update arrives for an unknown ticket", () => {
    const evt: ServerEvent = { type: "ticket_updated", repo_id: "r1", ticket_number: "unknown" };
    const result = applyTicketEvent([ticket], evt);
    expect(result.needsFullRefresh).toBe(true);
  });

  it("appends a new job and marks the ticket as busy", () => {
    const evt: ServerEvent = { type: "job", repo_id: "r1", ticket_number: "42", job_id: "j1", status: "running", action: "run_ticket" };
    const result = applyTicketEvent([ticket], evt);
    expect(result.tickets[0].busy).toBe(true);
    expect(result.tickets[0].jobs).toHaveLength(1);
    expect(result.tickets[0].jobs![0].id).toBe("j1");
  });

  it("leaves other tickets unchanged", () => {
    const other = makeSummary({ repo_id: "r1", ticket_number: "99" });
    const evt: ServerEvent = { type: "ticket_deleted", repo_id: "r1", ticket_number: "42" };
    const result = applyTicketEvent([ticket, other], evt);
    expect(result.tickets).toHaveLength(1);
    expect(result.tickets[0].ticket_number).toBe("99");
  });

  it("is a no-op when repo_id or ticket_number is missing from the event", () => {
    const evt: ServerEvent = { type: "ticket_updated" };
    const result = applyTicketEvent([ticket], evt);
    expect(result.tickets).toEqual([ticket]);
    expect(result.needsFullRefresh).toBe(false);
  });
});
