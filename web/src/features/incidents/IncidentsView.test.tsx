import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IncidentsView } from "./IncidentsView";

const wireIncident = {
  fingerprint: "fp-abc",
  category: "test_failure",
  normalized_signature: "--- FAIL: TestLogin",
  occurrence_count: 4,
  algorithm_version: 1,
  first_seen_at: "2026-11-01T10:00:00Z",
  last_seen_at: "2026-11-20T12:30:00Z",
};

function stubFetch(impl: typeof fetch): void {
  vi.stubGlobal("fetch", vi.fn(impl));
}

describe("IncidentsView", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("starts idle with a prompt to enter a project id", () => {
    render(<IncidentsView />);
    expect(screen.getByText(/enter a registered gitlab project id/i)).toBeInTheDocument();
  });

  it("loads and renders incidents for the submitted project id", async () => {
    let calledUrl = "";
    stubFetch((async (input: RequestInfo | URL) => {
      calledUrl = String(input);
      return new Response(JSON.stringify([wireIncident]), { status: 200 });
    }) as typeof fetch);

    render(<IncidentsView />);
    await userEvent.type(screen.getByLabelText(/gitlab project id/i), "1001");
    await userEvent.click(screen.getByRole("button", { name: /load/i }));

    expect(await screen.findByText("--- FAIL: TestLogin")).toBeInTheDocument();
    expect(screen.getByText("4")).toBeInTheDocument();
    expect(calledUrl).toBe("/api/v1/projects/1001/incidents");
  });

  it("shows an empty-state message when the project has no incidents", async () => {
    stubFetch((async () => new Response("[]", { status: 200 })) as typeof fetch);

    render(<IncidentsView />);
    await userEvent.type(screen.getByLabelText(/gitlab project id/i), "1001");
    await userEvent.click(screen.getByRole("button", { name: /load/i }));

    expect(await screen.findByText(/no failure incidents recorded/i)).toBeInTheDocument();
  });

  it("shows a friendly error when the project is not registered", async () => {
    stubFetch((async () => new Response("{}", { status: 404 })) as typeof fetch);

    render(<IncidentsView />);
    await userEvent.type(screen.getByLabelText(/gitlab project id/i), "9999");
    await userEvent.click(screen.getByRole("button", { name: /load/i }));

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/project is not registered/i);
  });

  it("rejects a non-positive project id without calling the API", async () => {
    const fetchSpy = vi.fn();
    stubFetch(fetchSpy as unknown as typeof fetch);

    render(<IncidentsView />);
    await userEvent.type(screen.getByLabelText(/gitlab project id/i), "0");
    await userEvent.click(screen.getByRole("button", { name: /load/i }));

    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent(/positive gitlab project id/i),
    );
    expect(fetchSpy).not.toHaveBeenCalled();
  });
});
