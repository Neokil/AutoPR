import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { AddTicketDialog } from "../AddTicketDialog";

const defaultProps = {
  knownRepoPaths: [],
  repoPath: "",
  ticketNumber: "",
  baseBranch: "",
  error: "",
  onRepoPathChange: vi.fn(),
  onTicketNumberChange: vi.fn(),
  onBaseBranchChange: vi.fn(),
  onSubmit: vi.fn(),
  onClose: vi.fn(),
};

beforeEach(() => vi.clearAllMocks());

describe("AddTicketDialog", () => {
  it("shows an error banner when the error prop is set", () => {
    render(<AddTicketDialog {...defaultProps} error="Something went wrong" />);
    expect(screen.getByText("Something went wrong")).toBeInTheDocument();
  });

  // Clicking the semi-transparent backdrop should close the modal.
  it("calls onClose when the backdrop is clicked", () => {
    const onClose = vi.fn();
    const { container } = render(<AddTicketDialog {...defaultProps} onClose={onClose} />);
    fireEvent.click(container.querySelector(".modal-backdrop")!);
    expect(onClose).toHaveBeenCalledOnce();
  });

  // Clicking inside the modal body must NOT bubble up and close the dialog.
  it("does not call onClose when the modal body is clicked", () => {
    const onClose = vi.fn();
    const { container } = render(<AddTicketDialog {...defaultProps} onClose={onClose} />);
    fireEvent.click(container.querySelector(".modal")!);
    expect(onClose).not.toHaveBeenCalled();
  });

  it("renders the base branch input and forwards changes", () => {
    const onBaseBranchChange = vi.fn();
    render(<AddTicketDialog {...defaultProps} onBaseBranchChange={onBaseBranchChange} baseBranch="main" />);

    const input = screen.getByLabelText("Base Branch");
    expect(input).toHaveValue("main");

    fireEvent.change(input, { target: { value: "release/1.2" } });
    expect(onBaseBranchChange).toHaveBeenCalledWith("release/1.2");
  });
});
