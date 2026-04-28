import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AddTicketDialog } from "../AddTicketDialog";

const defaultProps = {
  knownRepoPaths: [],
  repoPath: "",
  ticketNumber: "",
  error: "",
  onRepoPathChange: vi.fn(),
  onTicketNumberChange: vi.fn(),
  onSubmit: vi.fn(),
  onClose: vi.fn(),
};

beforeEach(() => {
  vi.clearAllMocks();
});

describe("AddTicketDialog", () => {
  it("renders the repo path and ticket number inputs", () => {
    render(<AddTicketDialog {...defaultProps} />);
    expect(screen.getByLabelText("Repository Folder")).toBeInTheDocument();
    expect(screen.getByLabelText("Ticket Number")).toBeInTheDocument();
  });

  it("shows an error banner when the error prop is set", () => {
    render(<AddTicketDialog {...defaultProps} error="Something went wrong" />);
    expect(screen.getByText("Something went wrong")).toBeInTheDocument();
  });

  it("does not show an error banner when error is empty", () => {
    render(<AddTicketDialog {...defaultProps} error="" />);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("calls onSubmit when Schedule Run is clicked", async () => {
    const onSubmit = vi.fn();
    render(<AddTicketDialog {...defaultProps} onSubmit={onSubmit} />);
    await userEvent.click(screen.getByRole("button", { name: "Schedule Run" }));
    expect(onSubmit).toHaveBeenCalledOnce();
  });

  it("calls onClose when Cancel is clicked", async () => {
    const onClose = vi.fn();
    render(<AddTicketDialog {...defaultProps} onClose={onClose} />);
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("calls onClose when the backdrop is clicked", () => {
    const onClose = vi.fn();
    const { container } = render(<AddTicketDialog {...defaultProps} onClose={onClose} />);
    fireEvent.click(container.querySelector(".modal-backdrop")!);
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("does not call onClose when the modal body is clicked", () => {
    const onClose = vi.fn();
    const { container } = render(<AddTicketDialog {...defaultProps} onClose={onClose} />);
    fireEvent.click(container.querySelector(".modal")!);
    expect(onClose).not.toHaveBeenCalled();
  });

  it("calls onSubmit on Enter key in the ticket number field", async () => {
    const onSubmit = vi.fn();
    render(<AddTicketDialog {...defaultProps} onSubmit={onSubmit} />);
    await userEvent.type(screen.getByLabelText("Ticket Number"), "{Enter}");
    expect(onSubmit).toHaveBeenCalledOnce();
  });

  it("populates the datalist with known repo paths", () => {
    const { container } = render(<AddTicketDialog {...defaultProps} knownRepoPaths={["/repo/a", "/repo/b"]} />);
    const options = container.querySelectorAll("datalist option");
    expect(options).toHaveLength(2);
    expect(options[0]).toHaveValue("/repo/a");
    expect(options[1]).toHaveValue("/repo/b");
  });
});
