import { afterEach, describe, expect, it, vi } from "vitest";
import { runTicket } from "../api";

describe("runTicket", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("includes base_branch when provided", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ job_id: "j1", status: "accepted", action: "run", repo_id: "r1", repo_path: "/repo" })
    });
    vi.stubGlobal("fetch", fetchMock);

    await runTicket("/repo", "GH-20", "release/1.2");

    expect(fetchMock).toHaveBeenCalledWith("/api/tickets/GH-20/run", expect.objectContaining({
      method: "POST",
      body: JSON.stringify({ repo_path: "/repo", base_branch: "release/1.2" })
    }));
  });

  it("omits base_branch when blank", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ job_id: "j1", status: "accepted", action: "run", repo_id: "r1", repo_path: "/repo" })
    });
    vi.stubGlobal("fetch", fetchMock);

    await runTicket("/repo", "GH-20", "   ");

    expect(fetchMock).toHaveBeenCalledWith("/api/tickets/GH-20/run", expect.objectContaining({
      method: "POST",
      body: JSON.stringify({ repo_path: "/repo" })
    }));
  });
});
