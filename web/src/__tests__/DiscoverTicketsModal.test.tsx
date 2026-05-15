import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { DiscoverTicketsModal } from "../DiscoverTicketsModal";

const defaultProps = {
  repoPath: "/repo/path",
  tickets: [],
  loading: false,
  error: "",
  onAdd: vi.fn(),
  onClose: vi.fn(),
};

beforeEach(() => vi.clearAllMocks());

describe("DiscoverTicketsModal", () => {
  it("renders generic copy without Shortcut references", () => {
    render(<DiscoverTicketsModal {...defaultProps} />);

    expect(screen.getByText("Available tickets for /repo/path.")).toBeInTheDocument();
    expect(screen.getByText("No Tickets available.")).toBeInTheDocument();
    expect(screen.queryByText(/shortcut/i)).not.toBeInTheDocument();
  });

  it("shows generic loading copy", () => {
    render(<DiscoverTicketsModal {...defaultProps} loading />);

    expect(screen.getByText("Fetching Tickets...")).toBeInTheDocument();
    expect(screen.queryByText(/shortcut/i)).not.toBeInTheDocument();
  });

  it("renders discovered tickets and calls onAdd", () => {
    const onAdd = vi.fn();
    render(
      <DiscoverTicketsModal
        {...defaultProps}
        tickets={[
          { ticket_number: "GH-4", title: "Remove Shortcut references" },
          { ticket_number: "SC-1", title: "Another ticket" }
        ]}
        onAdd={onAdd}
      />
    );

    fireEvent.click(screen.getAllByRole("button", { name: "Add" })[0]);
    expect(onAdd).toHaveBeenCalledWith("GH-4");
  });

  it("calls onClose when the backdrop is clicked", () => {
    const onClose = vi.fn();
    const { container } = render(<DiscoverTicketsModal {...defaultProps} onClose={onClose} />);

    fireEvent.click(container.querySelector(".modal-backdrop")!);
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("does not call onClose when the modal body is clicked", () => {
    const onClose = vi.fn();
    const { container } = render(<DiscoverTicketsModal {...defaultProps} onClose={onClose} />);

    fireEvent.click(container.querySelector(".modal")!);
    expect(onClose).not.toHaveBeenCalled();
  });
});
