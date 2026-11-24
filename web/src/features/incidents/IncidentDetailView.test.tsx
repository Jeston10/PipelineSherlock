import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IncidentDetailView } from "./IncidentDetailView";

const wireDetail = {
  fingerprint: "fp-abc",
  category: "test_failure",
  normalized_signature: "--- FAIL: TestLogin",
  occurrence_count: 3,
  algorithm_version: 1,
  first_seen_at: "2026-11-01T10:00:00Z",
  last_seen_at: "2026-11-20T12:30:00Z",
  occurrences: [
    {
      observed_at: "2026-11-20T12:30:00Z",
      ref: "main",
      sha: "deadbeef",
      job_name: "unit",
      stage: "test",
      gitlab_job_id: 9,
      gitlab_pipeline_id: 55,
      evidence_excerpt: "--- FAIL: TestLogin",
    },
  ],
  affected_refs: [{ ref: "main", occurrence_count: 3, distinct_shas: 2 }],
  signals: {
    distinct_refs: 1,
    distinct_shas: 2,
    repeated_on_same_commit: true,
    spans_multiple_commits: true,
    flaky_suspected: true,
    regression_suspected: true,
  },
};

function stubFetch(impl: typeof fetch): void {
  vi.stubGlobal("fetch", vi.fn(impl));
}

describe("IncidentDetailView", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("loads and renders the incident detail for the fingerprint", async () => {
    let calledUrl = "";
    stubFetch((async (input: RequestInfo | URL) => {
      calledUrl = String(input);
      return new Response(JSON.stringify(wireDetail), { status: 200 });
    }) as typeof fetch);

    render(<IncidentDetailView gitlabProjectId={1001} fingerprint="fp-abc" onBack={() => {}} />);

    expect(await screen.findByText("Flaky suspected")).toBeInTheDocument();
    expect(calledUrl).toBe("/api/v1/projects/1001/incidents/fp-abc");
  });

  it("calls onBack when the back control is clicked", async () => {
    stubFetch((async () => new Response(JSON.stringify(wireDetail), { status: 200 })) as typeof fetch);
    const onBack = vi.fn();

    render(<IncidentDetailView gitlabProjectId={1001} fingerprint="fp-abc" onBack={onBack} />);
    await userEvent.click(screen.getByRole("button", { name: /back to incidents/i }));

    expect(onBack).toHaveBeenCalledOnce();
  });

  it("shows the server's error message when the incident is missing", async () => {
    stubFetch((async () =>
      new Response(JSON.stringify({ error: "incident not found" }), {
        status: 404,
      })) as typeof fetch);

    render(<IncidentDetailView gitlabProjectId={1001} fingerprint="missing" onBack={() => {}} />);

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/incident not found/i);
  });
});
