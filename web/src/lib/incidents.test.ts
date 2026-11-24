import { describe, it, expect, vi, afterEach } from "vitest";
import {
  parseIncidents,
  fetchIncidents,
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
