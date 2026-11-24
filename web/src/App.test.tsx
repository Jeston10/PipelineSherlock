import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { App } from "./App";

// App polls /healthz and /readyz on an interval and hosts the incidents view.
// These smoke tests stub fetch so the health probes resolve deterministically
// and assert the key surfaces render.

function stubHealthFetch(status: string): void {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify({ status }), { status: 200 })),
  );
}

describe("App", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it("renders the masthead and the incidents view prompt", () => {
    stubHealthFetch("ok");
    render(<App />);

    expect(screen.getByRole("heading", { name: /ci failure intelligence/i })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /failure incidents/i })).toBeInTheDocument();
    expect(screen.getByText(/enter a registered gitlab project id/i)).toBeInTheDocument();
  });

  it("shows the health probes as healthy once they resolve", async () => {
    stubHealthFetch("ok");
    render(<App />);

    const healthy = await screen.findAllByText(/healthy/i);
    expect(healthy.length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("/healthz")).toBeInTheDocument();
    expect(screen.getByText("/readyz")).toBeInTheDocument();
  });
});
