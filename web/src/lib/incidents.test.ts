import { describe, it, expect, vi, afterEach } from "vitest";
import {
  parseIncidents,
  fetchIncidents,
  parseIncidentDetail,
  fetchIncidentDetail,
  IncidentsError,
  type Incident,
} from "./incidents";

const wireIncident = {
  fingerprint: "fp-abc",
  category: "test_failure",
  normalized_signature: "--- FAIL: TestLogin",
  occurrence_count: 4,
  algorithm_version: 1,
  first_seen_at: "2026-11-01T10:00:00Z",
  last_seen_at: "2026-11-20T12:30:00Z",
};

describe("parseIncidents", () => {
  it("maps snake_case wire fields to camelCase domain fields", () => {
    const [inc] = parseIncidents([wireIncident]);
    const expected: Incident = {
      fingerprint: "fp-abc",
      category: "test_failure",
      normalizedSignature: "--- FAIL: TestLogin",
      occurrenceCount: 4,
      algorithmVersion: 1,
      firstSeenAt: "2026-11-01T10:00:00Z",
      lastSeenAt: "2026-11-20T12:30:00Z",
    };
    expect(inc).toEqual(expected);
  });

  it("accepts an empty array (project with no failures yet)", () => {
    expect(parseIncidents([])).toEqual([]);
  });

  it("throws when the response is not an array", () => {
    expect(() => parseIncidents({})).toThrow(IncidentsError);
  });

  it("throws when a numeric field has the wrong type", () => {
    const bad = { ...wireIncident, occurrence_count: "4" };
    expect(() => parseIncidents([bad])).toThrow(/occurrence_count must be a number/);
  });

  it("throws when a required string field is missing", () => {
    const { fingerprint: _omit, ...bad } = wireIncident;
    expect(() => parseIncidents([bad])).toThrow(/fingerprint must be a string/);
  });
});

describe("fetchIncidents", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  function stubFetch(impl: typeof fetch): void {
    vi.stubGlobal("fetch", vi.fn(impl));
  }

  it("requests the project's incidents and returns the parsed list", async () => {
    let calledUrl = "";
    stubFetch((async (input: RequestInfo | URL) => {
      calledUrl = String(input);
      return new Response(JSON.stringify([wireIncident]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }) as typeof fetch);

    const incidents = await fetchIncidents(1001, { limit: 25 });

    expect(calledUrl).toBe("/api/v1/projects/1001/incidents?limit=25");
    expect(incidents).toHaveLength(1);
    expect(incidents[0].fingerprint).toBe("fp-abc");
  });

  it("omits the limit query when not provided", async () => {
    let calledUrl = "";
    stubFetch((async (input: RequestInfo | URL) => {
      calledUrl = String(input);
      return new Response("[]", { status: 200 });
    }) as typeof fetch);

    await fetchIncidents(7);

    expect(calledUrl).toBe("/api/v1/projects/7/incidents");
  });

  it("throws a friendly IncidentsError on 404", async () => {
    stubFetch((async () => new Response("{}", { status: 404 })) as typeof fetch);

    await expect(fetchIncidents(9999)).rejects.toMatchObject({
      status: 404,
      message: "Project is not registered.",
    });
  });

  it("throws with the status on a 500", async () => {
    stubFetch((async () => new Response("{}", { status: 500 })) as typeof fetch);

    await expect(fetchIncidents(1001)).rejects.toMatchObject({ status: 500 });
  });
});

const wireDetail = {
  ...wireIncident,
  occurrences: [
    {
      observed_at: "2026-11-20T12:30:00Z",
      ref: "main",
      sha: "deadbeef",
      job_name: "test",
      stage: "test",
      gitlab_job_id: 9,
      gitlab_pipeline_id: 55,
      evidence_excerpt: "--- FAIL: TestLogin",
    },
  ],
  affected_refs: [{ ref: "main", occurrence_count: 4, distinct_shas: 2 }],
  signals: {
    distinct_refs: 1,
    distinct_shas: 2,
    repeated_on_same_commit: true,
    spans_multiple_commits: true,
    flaky_suspected: true,
    regression_suspected: true,
  },
};

describe("parseIncidentDetail", () => {
  it("maps the header, occurrences, affected refs and signals", () => {
    const detail = parseIncidentDetail(wireDetail);
    expect(detail.fingerprint).toBe("fp-abc");
    expect(detail.occurrences).toHaveLength(1);
    expect(detail.occurrences[0]).toEqual({
      observedAt: "2026-11-20T12:30:00Z",
      ref: "main",
      sha: "deadbeef",
      jobName: "test",
      stage: "test",
      gitlabJobId: 9,
      gitlabPipelineId: 55,
      evidenceExcerpt: "--- FAIL: TestLogin",
    });
    expect(detail.affectedRefs[0]).toEqual({ ref: "main", occurrenceCount: 4, distinctShas: 2 });
    expect(detail.signals.flakySuspected).toBe(true);
    expect(detail.signals.regressionSuspected).toBe(true);
  });

  it("throws when occurrences is not an array", () => {
    const bad = { ...wireDetail, occurrences: {} };
    expect(() => parseIncidentDetail(bad)).toThrow(/occurrences must be an array/);
  });

  it("throws when a signal flag is not a boolean", () => {
    const bad = { ...wireDetail, signals: { ...wireDetail.signals, flaky_suspected: "yes" } };
    expect(() => parseIncidentDetail(bad)).toThrow(/flaky_suspected must be a boolean/);
  });
});

describe("fetchIncidentDetail", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  function stubFetch(impl: typeof fetch): void {
    vi.stubGlobal("fetch", vi.fn(impl));
  }

  it("requests the incident by fingerprint and returns the parsed detail", async () => {
    let calledUrl = "";
    stubFetch((async (input: RequestInfo | URL) => {
      calledUrl = String(input);
      return new Response(JSON.stringify(wireDetail), { status: 200 });
    }) as typeof fetch);

    const detail = await fetchIncidentDetail(1001, "fp-abc");

    expect(calledUrl).toBe("/api/v1/projects/1001/incidents/fp-abc");
    expect(detail.occurrences).toHaveLength(1);
  });

  it("url-encodes a fingerprint with unsafe characters", async () => {
    let calledUrl = "";
    stubFetch((async (input: RequestInfo | URL) => {
      calledUrl = String(input);
      return new Response(JSON.stringify(wireDetail), { status: 200 });
    }) as typeof fetch);

    await fetchIncidentDetail(1001, "a/b c");

    expect(calledUrl).toBe("/api/v1/projects/1001/incidents/a%2Fb%20c");
  });

  it("surfaces the server's 404 error text as the message", async () => {
    stubFetch((async () =>
      new Response(JSON.stringify({ error: "incident not found" }), {
        status: 404,
      })) as typeof fetch);

    await expect(fetchIncidentDetail(1001, "missing")).rejects.toMatchObject({
      status: 404,
      message: "Incident not found.",
    });
  });

  it("falls back to a generic 404 message when the body has no error field", async () => {
    stubFetch((async () => new Response("", { status: 404 })) as typeof fetch);

    await expect(fetchIncidentDetail(1001, "missing")).rejects.toMatchObject({
      status: 404,
      message: "Incident not found.",
    });
  });
});
