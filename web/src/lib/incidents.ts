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
