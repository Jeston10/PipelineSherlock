// API client for the incidents read API. The parser validates the server's
// JSON shape at the boundary (never trust external data) and maps the snake_case
// wire format to camelCase domain types used by the UI.

export interface Incident {
  fingerprint: string;
  category: string;
  normalizedSignature: string;
  occurrenceCount: number;
  algorithmVersion: number;
  firstSeenAt: string;
  lastSeenAt: string;
}

// IncidentsError carries the HTTP status so the UI can distinguish an
// unregistered project (404) from a transport or server failure.
export class IncidentsError extends Error {
  readonly status: number;
  constructor(message: string, status: number) {
    super(message);
    this.name = "IncidentsError";
    this.status = status;
  }
}

function asRecord(value: unknown, context: string): Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new IncidentsError(`malformed incident: expected object (${context})`, 0);
  }
  return value as Record<string, unknown>;
}

function readString(obj: Record<string, unknown>, key: string): string {
  const v = obj[key];
  if (typeof v !== "string") {
    throw new IncidentsError(`malformed incident: ${key} must be a string`, 0);
  }
  return v;
}

function readNumber(obj: Record<string, unknown>, key: string): number {
  const v = obj[key];
  if (typeof v !== "number" || Number.isNaN(v)) {
    throw new IncidentsError(`malformed incident: ${key} must be a number`, 0);
  }
  return v;
}

function parseIncident(value: unknown): Incident {
  const obj = asRecord(value, "incident");
  return {
    fingerprint: readString(obj, "fingerprint"),
    category: readString(obj, "category"),
    normalizedSignature: readString(obj, "normalized_signature"),
    occurrenceCount: readNumber(obj, "occurrence_count"),
    algorithmVersion: readNumber(obj, "algorithm_version"),
    firstSeenAt: readString(obj, "first_seen_at"),
    lastSeenAt: readString(obj, "last_seen_at"),
  };
}

// parseIncidents validates and maps the list response. An empty array is valid
// (a project with no recorded failures yet).
export function parseIncidents(data: unknown): Incident[] {
  if (!Array.isArray(data)) {
    throw new IncidentsError("malformed response: expected an array of incidents", 0);
  }
  return data.map(parseIncident);
}

export interface FetchIncidentsOptions {
  limit?: number;
  signal?: AbortSignal;
}

// fetchIncidents loads a project's incidents from the backend. It throws an
// IncidentsError on a non-OK response or a malformed body; the caller maps that
// to UI state. A caught AbortError (from `signal`) is re-thrown unchanged so the
// caller can ignore it.
export async function fetchIncidents(
  gitlabProjectId: number,
  options: FetchIncidentsOptions = {},
): Promise<Incident[]> {
  const params = new URLSearchParams();
  if (options.limit && options.limit > 0) {
    params.set("limit", String(options.limit));
  }
  const query = params.toString();
  const url = `/api/v1/projects/${gitlabProjectId}/incidents${query ? `?${query}` : ""}`;

  const res = await fetch(url, {
    headers: { Accept: "application/json" },
    signal: options.signal,
  });

  if (!res.ok) {
    const message =
      res.status === 404
        ? "Project is not registered."
        : `Request failed with status ${res.status}.`;
    throw new IncidentsError(message, res.status);
  }

  const body: unknown = await res.json();
  return parseIncidents(body);
}

// --- Incident detail -------------------------------------------------------

export interface IncidentOccurrence {
  observedAt: string;
  ref: string;
  sha: string;
  jobName: string;
  stage: string;
  gitlabJobId: number;
  gitlabPipelineId: number;
  evidenceExcerpt: string;
}

export interface RefSummary {
  ref: string;
  occurrenceCount: number;
  distinctShas: number;
}

export interface IncidentSignals {
  distinctRefs: number;
  distinctShas: number;
  repeatedOnSameCommit: boolean;
  spansMultipleCommits: boolean;
  flakySuspected: boolean;
  regressionSuspected: boolean;
}

export interface IncidentDetail extends Incident {
  occurrences: IncidentOccurrence[];
  affectedRefs: RefSummary[];
  signals: IncidentSignals;
}

function readBoolean(obj: Record<string, unknown>, key: string): boolean {
  const v = obj[key];
  if (typeof v !== "boolean") {
    throw new IncidentsError(`malformed incident: ${key} must be a boolean`, 0);
  }
  return v;
}

function parseOccurrence(value: unknown): IncidentOccurrence {
  const obj = asRecord(value, "occurrence");
  return {
    observedAt: readString(obj, "observed_at"),
    ref: readString(obj, "ref"),
    sha: readString(obj, "sha"),
    jobName: readString(obj, "job_name"),
    stage: readString(obj, "stage"),
    gitlabJobId: readNumber(obj, "gitlab_job_id"),
    gitlabPipelineId: readNumber(obj, "gitlab_pipeline_id"),
    evidenceExcerpt: readString(obj, "evidence_excerpt"),
  };
}

function parseRefSummary(value: unknown): RefSummary {
  const obj = asRecord(value, "ref summary");
  return {
    ref: readString(obj, "ref"),
    occurrenceCount: readNumber(obj, "occurrence_count"),
    distinctShas: readNumber(obj, "distinct_shas"),
  };
}

function parseSignals(value: unknown): IncidentSignals {
  const obj = asRecord(value, "signals");
  return {
    distinctRefs: readNumber(obj, "distinct_refs"),
    distinctShas: readNumber(obj, "distinct_shas"),
    repeatedOnSameCommit: readBoolean(obj, "repeated_on_same_commit"),
    spansMultipleCommits: readBoolean(obj, "spans_multiple_commits"),
    flakySuspected: readBoolean(obj, "flaky_suspected"),
    regressionSuspected: readBoolean(obj, "regression_suspected"),
  };
}

function readArray(obj: Record<string, unknown>, key: string): unknown[] {
  const v = obj[key];
  if (!Array.isArray(v)) {
    throw new IncidentsError(`malformed incident: ${key} must be an array`, 0);
  }
  return v;
}

// parseIncidentDetail validates and maps the single-incident response: the
// header fields plus the occurrence history, per-ref breakdown and signals.
export function parseIncidentDetail(data: unknown): IncidentDetail {
  const header = parseIncident(data);
  const obj = asRecord(data, "incident detail");
  return {
    ...header,
    occurrences: readArray(obj, "occurrences").map(parseOccurrence),
    affectedRefs: readArray(obj, "affected_refs").map(parseRefSummary),
    signals: parseSignals(obj["signals"]),
  };
}

// fetchIncidentDetail loads one incident by fingerprint. On a 404 it tries to
// read the server's error text to give a precise message (unknown project vs
// unknown incident), falling back to a generic one.
export async function fetchIncidentDetail(
  gitlabProjectId: number,
  fingerprint: string,
  options: { signal?: AbortSignal } = {},
): Promise<IncidentDetail> {
  const url = `/api/v1/projects/${gitlabProjectId}/incidents/${encodeURIComponent(fingerprint)}`;

  const res = await fetch(url, {
    headers: { Accept: "application/json" },
    signal: options.signal,
  });

  if (!res.ok) {
    let message = `Request failed with status ${res.status}.`;
    if (res.status === 404) {
      message = (await readErrorText(res)) ?? "Incident not found.";
    }
    throw new IncidentsError(message, res.status);
  }

  const body: unknown = await res.json();
  return parseIncidentDetail(body);
}

// readErrorText best-effort reads a `{ "error": "..." }` body and returns it as
// a sentence. Returns undefined if the body is missing or not shaped that way.
async function readErrorText(res: Response): Promise<string | undefined> {
  try {
    const body: unknown = await res.json();
    if (typeof body === "object" && body !== null && "error" in body) {
      const err = (body as Record<string, unknown>)["error"];
      if (typeof err === "string" && err.length > 0) {
        const trimmed = err.trim();
        return `${trimmed.charAt(0).toUpperCase()}${trimmed.slice(1)}.`;
      }
    }
  } catch {
    // Non-JSON or empty body — fall back to the generic message.
  }
  return undefined;
}
