import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { StateTimeline } from "../StateTimeline";
import type { StateRun } from "../types";

function makeRun(id: string, stateName: string, displayName = ""): StateRun {
  return { id, state_name: stateName, state_display_name: displayName, started_at: "2024-01-01T00:00:00Z" };
}

describe("StateTimeline", () => {
  it("renders nothing when runs is empty", () => {
    const { container } = render(<StateTimeline runs={[]} selectedRunId="" onSelectRun={vi.fn()} />);
    expect(container.firstChild).toBeNull();
  });

  it("renders a button for each run", () => {
    const runs = [makeRun("r1", "investigate"), makeRun("r2", "implementation")];
    render(<StateTimeline runs={runs} selectedRunId="" onSelectRun={vi.fn()} />);
    expect(screen.getAllByRole("button")).toHaveLength(2);
  });

  it("uses state_display_name as the button label when present", () => {
    const runs = [makeRun("r1", "investigate", "Investigation")];
    render(<StateTimeline runs={runs} selectedRunId="" onSelectRun={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Investigation" })).toBeInTheDocument();
  });

  it("falls back to state_name when display_name is absent", () => {
    const runs = [makeRun("r1", "investigate")];
    render(<StateTimeline runs={runs} selectedRunId="" onSelectRun={vi.fn()} />);
    expect(screen.getByRole("button", { name: "investigate" })).toBeInTheDocument();
  });

  it("marks the selected run with the active class", () => {
    const runs = [makeRun("r1", "investigate"), makeRun("r2", "implementation")];
    render(<StateTimeline runs={runs} selectedRunId="r2" onSelectRun={vi.fn()} />);
    const buttons = screen.getAllByRole("button");
    expect(buttons[0]).not.toHaveClass("active");
    expect(buttons[1]).toHaveClass("active");
  });

  it("calls onSelectRun with the correct run id when clicked", async () => {
    const onSelectRun = vi.fn();
    const runs = [makeRun("r1", "investigate"), makeRun("r2", "implementation")];
    render(<StateTimeline runs={runs} selectedRunId="" onSelectRun={onSelectRun} />);
    await userEvent.click(screen.getAllByRole("button")[1]);
    expect(onSelectRun).toHaveBeenCalledWith("r2");
  });

  it("renders separators between runs but not after the last one", () => {
    const runs = [makeRun("r1", "s1"), makeRun("r2", "s2"), makeRun("r3", "s3")];
    const { container } = render(<StateTimeline runs={runs} selectedRunId="" onSelectRun={vi.fn()} />);
    const separators = container.querySelectorAll(".timeline-separator");
    expect(separators).toHaveLength(2);
  });
});
