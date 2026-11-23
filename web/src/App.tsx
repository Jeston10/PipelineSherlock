import { useEffect, useState } from "react";

type Health = {
  status: string;
  checks?: Record<string, string>;
};

type Probe = {
  reachable: boolean;
  body?: Health;
};

const POLL_INTERVAL_MS = 5000;

async function probe(path: string, signal: AbortSignal): Promise<Probe> {
  try {
    const res = await fetch(path, { signal });
    const body = (await res.json()) as Health;
    return { reachable: true, body };
  } catch {
    // A network failure (API down) is an expected state to display, not a crash.
    return { reachable: false };
  }
}

export function App() {
  const [liveness, setLiveness] = useState<Probe | null>(null);
  const [readiness, setReadiness] = useState<Probe | null>(null);

  useEffect(() => {
    const controller = new AbortController();

    const refresh = async (): Promise<void> => {
      const [live, ready] = await Promise.all([
        probe("/healthz", controller.signal),
        probe("/readyz", controller.signal),
      ]);
      if (controller.signal.aborted) {
        return;
      }
      setLiveness(live);
      setReadiness(ready);
    };

    void refresh();
    const id = setInterval(() => void refresh(), POLL_INTERVAL_MS);

    return () => {
      controller.abort();
      clearInterval(id);
    };
  }, []);

  return (
    <main className="shell">
      <header className="masthead">
        <p className="eyebrow">PipelineSherlock</p>
        <h1>Service status</h1>
        <p className="lede">
          Milestone 0 foundation. This panel polls the backend health and
          readiness probes every {POLL_INTERVAL_MS / 1000}s.
        </p>
      </header>

      <section className="probes" aria-label="Health probes">
        <ProbeCard title="Liveness" endpoint="/healthz" probe={liveness} />
        <ProbeCard title="Readiness" endpoint="/readyz" probe={readiness} />
      </section>
    </main>
  );
}

type ProbeCardProps = {
  title: string;
  endpoint: string;
  probe: Probe | null;
};

function ProbeCard({ title, endpoint, probe }: ProbeCardProps) {
  const state = probeState(probe);

  return (
    <article className={`card card--${state.tone}`} aria-busy={probe === null}>
      <div className="card__row">
        <h2>{title}</h2>
        <span className={`pill pill--${state.tone}`}>{state.label}</span>
      </div>
      <code className="card__endpoint">{endpoint}</code>
      {probe?.body?.checks ? (
        <ul className="checks">
          {Object.entries(probe.body.checks).map(([name, value]) => (
            <li key={name}>
              <span>{name}</span>
              <span className={value === "ok" ? "ok" : "down"}>{value}</span>
            </li>
          ))}
        </ul>
      ) : null}
    </article>
  );
}

function probeState(probe: Probe | null): { label: string; tone: string } {
  if (probe === null) {
    return { label: "Checking…", tone: "idle" };
  }
  if (!probe.reachable) {
    return { label: "Unreachable", tone: "down" };
  }
  if (probe.body?.status === "ok") {
    return { label: "Healthy", tone: "ok" };
  }
  return { label: "Degraded", tone: "down" };
}
