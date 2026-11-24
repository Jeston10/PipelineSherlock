import { describe, it, expect } from "vitest";
import { render, screen, within } from "@testing-library/react";
import { IncidentDetail } from "./IncidentDetail";
import type { IncidentDetail as IncidentDetailModel } from "../lib/incidents";

function makeDetail(overrides: Partial<IncidentDetailModel> = {}): IncidentDetailModel {
  return {
    fingerprint: "fp-abc",
    category: "test_failure",
    normalizedSignature: "--- FAIL: TestLogin",
    occurrenceCount: 3,
    algorithmVersion: 1,
    firstSeenAt: "2026-11-01T10:00:00Z",
    lastSeenAt: "2026-11-20T12:30:00Z",
    occurrences: [
      {
        observedAt: "2026-11-20T12:30:00Z",
        ref: "main",
        sha: "deadbeefcafe1234",
        jobName: "unit",
        stage: "test",
        gitlabJobId: 9,
        gitlabPipelineId: 55,
        evidenceExcerpt: "--- FAIL: TestLogin",
      },
    ],
    affectedRefs: [{ ref: "main", occurrenceCount: 3, distinctShas: 2 }],
    signals: {
      distinctRefs: 1,
      distinctShas: 2,
      repeatedOnSameCommit: true,
      spansMultipleCommits: true,
      flakySuspected: true,
      regressionSuspected: true,
    },
    ...overrides,
  };
}

describe("IncidentDetail", () => {
  it("renders the header, signals and occurrence history", () => {
    render(<IncidentDetail detail={makeDetail()} />);

    expect(screen.getByRole("heading", { name: /--- FAIL: TestLogin/ })).toBeInTheDocument();
    expect(screen.getByText("Flaky suspected")).toBeInTheDocument();
    expect(screen.getByText("Regression suspected")).toBeInTheDocument();

    const history = screen.getByRole("table");
    expect(within(history).getByText("main")).toBeInTheDocument();
    expect(within(history).getByText("deadbeefca")).toBeInTheDocument(); // short sha
    expect(within(history).getByText("55")).toBeInTheDocument();
  });

  it("shows no-signals state when neither flag is set", () => {
    const detail = makeDetail({
      signals: {
        distinctRefs: 1,
        distinctShas: 1,
        repeatedOnSameCommit: false,
        spansMultipleCommits: false,
        flakySuspected: false,
        regressionSuspected: false,
      },
    });
    render(<IncidentDetail detail={detail} />);

    expect(screen.getByText("No signals")).toBeInTheDocument();
    expect(screen.queryByText("Flaky suspected")).not.toBeInTheDocument();
  });

  it("shows an empty-history message when there are no occurrences", () => {
    render(<IncidentDetail detail={makeDetail({ occurrences: [] })} />);

    expect(screen.getByText(/no individual occurrences recorded/i)).toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });
});
