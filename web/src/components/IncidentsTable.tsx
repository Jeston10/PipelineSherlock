import type { Incident } from "../lib/incidents";

interface IncidentsTableProps {
  incidents: readonly Incident[];
  onSelect?: (fingerprint: string) => void;
}

// Presentational: renders the incident rows. No data fetching, no state — the
// container owns those. Each row is keyed by fingerprint (stable and unique).
export function IncidentsTable({ incidents, onSelect }: IncidentsTableProps) {
  return (
    <table className="incidents">
      <caption className="sr-only">Failure incidents, most recently seen first</caption>
      <thead>
        <tr>
          <th scope="col">Category</th>
          <th scope="col">Signature</th>
          <th scope="col" className="num">
            Occurrences
          </th>
          <th scope="col">Last seen</th>
        </tr>
      </thead>
      <tbody>
        {incidents.map((inc) => (
          <IncidentRow key={inc.fingerprint} incident={inc} onSelect={onSelect} />
        ))}
      </tbody>
    </table>
  );
}

interface IncidentRowProps {
  incident: Incident;
  onSelect?: (fingerprint: string) => void;
}

function IncidentRow({ incident, onSelect }: IncidentRowProps) {
  const handleClick = (): void => onSelect?.(incident.fingerprint);

  return (
    <tr className="incidents__row">
      <td>
        <span className={`tag tag--${categoryTone(incident.category)}`}>{incident.category}</span>
      </td>
      <td className="incidents__sig">
        {onSelect ? (
          <button type="button" className="linklike" onClick={handleClick}>
            <code>{incident.normalizedSignature}</code>
          </button>
        ) : (
          <code>{incident.normalizedSignature}</code>
        )}
      </td>
      <td className="num">{incident.occurrenceCount}</td>
      <td>
        <time dateTime={incident.lastSeenAt}>{formatTimestamp(incident.lastSeenAt)}</time>
      </td>
    </tr>
  );
}

// categoryTone maps a failure category to a visual tone bucket. Unknown
// categories fall back to a neutral tone rather than throwing.
function categoryTone(category: string): string {
  switch (category) {
    case "oom":
    case "timeout":
    case "infra_error":
      return "infra";
    case "dependency_error":
    case "compile_error":
      return "build";
    case "test_failure":
    case "lint_error":
      return "code";
    default:
      return "neutral";
  }
}

// formatTimestamp renders an ISO timestamp in the viewer's locale, falling back
// to the raw string if it cannot be parsed (never crash the row on bad data).
function formatTimestamp(iso: string): string {
  const parsed = new Date(iso);
  if (Number.isNaN(parsed.getTime())) {
    return iso;
  }
  return parsed.toLocaleString(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  });
}
