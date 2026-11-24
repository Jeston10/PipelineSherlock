import type {
  IncidentDetail as IncidentDetailModel,
  IncidentOccurrence,
} from "../lib/incidents";

interface IncidentDetailProps {
  detail: IncidentDetailModel;
}

// Presentational: renders a single incident's header, heuristic signals,
// per-ref breakdown and occurrence history. No data fetching or state.
export function IncidentDetail({ detail }: IncidentDetailProps) {
  const { signals } = detail;

  return (
    <article className="detail">
      <header className="detail__head">
        <span className={`tag tag--${categoryTone(detail.category)}`}>{detail.category}</span>
        <h3 className="detail__sig">
          <code>{detail.normalizedSignature}</code>
        </h3>
        <dl className="detail__meta">
          <Meta label="Occurrences" value={String(detail.occurrenceCount)} />
          <Meta label="First seen" value={formatTimestamp(detail.firstSeenAt)} />
          <Meta label="Last seen" value={formatTimestamp(detail.lastSeenAt)} />
          <Meta label="Fingerprint" value={detail.fingerprint} mono />
        </dl>
      </header>

      <section className="detail__signals" aria-label="Heuristic signals">
        <div className="signal-badges">
          {signals.flakySuspected ? (
            <span className="signal signal--flaky">Flaky suspected</span>
          ) : null}
          {signals.regressionSuspected ? (
            <span className="signal signal--regression">Regression suspected</span>
          ) : null}
          {!signals.flakySuspected && !signals.regressionSuspected ? (
            <span className="signal signal--none">No signals</span>
          ) : null}
          <span className="signal signal--count">{signals.distinctRefs} refs</span>
          <span className="signal signal--count">{signals.distinctShas} commits</span>
        </div>
        <p className="detail__note">
          Signals are candidates for review derived from failure history only, not
          verdicts.
        </p>
      </section>

      {detail.affectedRefs.length > 0 ? (
        <section className="detail__refs" aria-label="Affected refs">
          <h4>Affected refs</h4>
          <ul className="refs">
            {detail.affectedRefs.map((ref) => (
              <li key={ref.ref} className="refs__item">
                <code>{ref.ref}</code>
                <span className="refs__counts">
                  {ref.occurrenceCount} occ · {ref.distinctShas} commits
                </span>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      <section className="detail__history" aria-label="Occurrence history">
        <h4>Occurrence history</h4>
        {detail.occurrences.length === 0 ? (
          <p className="hint">No individual occurrences recorded.</p>
        ) : (
          <table className="occurrences">
            <thead>
              <tr>
                <th scope="col">Observed</th>
                <th scope="col">Ref</th>
                <th scope="col">Commit</th>
                <th scope="col">Job</th>
                <th scope="col" className="num">
                  Pipeline
                </th>
              </tr>
            </thead>
            <tbody>
              {detail.occurrences.map((occ) => (
                <OccurrenceRow key={occurrenceKey(occ)} occurrence={occ} />
              ))}
            </tbody>
          </table>
        )}
      </section>
    </article>
  );
}

function OccurrenceRow({ occurrence }: { occurrence: IncidentOccurrence }) {
  return (
    <tr>
      <td>
        <time dateTime={occurrence.observedAt}>{formatTimestamp(occurrence.observedAt)}</time>
      </td>
      <td>
        <code>{occurrence.ref}</code>
      </td>
      <td>
        <code>{shortSha(occurrence.sha)}</code>
      </td>
      <td>
        {occurrence.jobName}
        <span className="occurrences__stage">{occurrence.stage}</span>
      </td>
      <td className="num">{occurrence.gitlabPipelineId}</td>
    </tr>
  );
}

function Meta({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="detail__meta-item">
      <dt>{label}</dt>
      <dd className={mono ? "mono" : undefined}>{value}</dd>
    </div>
  );
}

// occurrenceKey builds a stable, unique key from the pipeline + job ids (a job
// is unique within a pipeline), falling back to the sha + timestamp.
function occurrenceKey(occ: IncidentOccurrence): string {
  if (occ.gitlabPipelineId > 0 && occ.gitlabJobId > 0) {
    return `${occ.gitlabPipelineId}:${occ.gitlabJobId}`;
  }
  return `${occ.sha}:${occ.observedAt}`;
}

function shortSha(sha: string): string {
  return sha.length > 10 ? sha.slice(0, 10) : sha;
}

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

function formatTimestamp(iso: string): string {
  const parsed = new Date(iso);
  if (Number.isNaN(parsed.getTime())) {
    return iso;
  }
  return parsed.toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });
}
