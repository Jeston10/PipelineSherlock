import { useEffect, useState } from "react";
import { fetchIncidents, type Incident } from "../../lib/incidents";
import { IncidentsTable } from "../../components/IncidentsTable";

type LoadState =
  | { status: "idle" }
  | { status: "loading" }
  | { status: "error"; message: string }
  | { status: "loaded"; incidents: Incident[] };

// IncidentsView is the container: it owns the selected GitLab project id and the
// load lifecycle, and delegates rendering of the rows to IncidentsTable. The
// fetch is keyed on the submitted id and cancels in flight on change/unmount.
export function IncidentsView() {
  const [projectInput, setProjectInput] = useState("");
  const [submittedId, setSubmittedId] = useState<number | null>(null);
  const [state, setState] = useState<LoadState>({ status: "idle" });

  useEffect(() => {
    if (submittedId === null) {
      return;
    }

    const controller = new AbortController();
    setState({ status: "loading" });

    fetchIncidents(submittedId, { signal: controller.signal })
      .then((incidents) => {
        if (!controller.signal.aborted) {
          setState({ status: "loaded", incidents });
        }
      })
      .catch((err: unknown) => {
        if (controller.signal.aborted) {
          return; // expected on cancel; not an error to surface
        }
        setState({ status: "error", message: errorMessage(err) });
      });

    return () => controller.abort();
  }, [submittedId]);

  const handleSubmit = (event: React.FormEvent): void => {
    event.preventDefault();
    const parsed = Number.parseInt(projectInput, 10);
    if (!Number.isInteger(parsed) || parsed <= 0) {
      setState({ status: "error", message: "Enter a positive GitLab project id." });
      return;
    }
    setSubmittedId(parsed);
  };

  return (
    <section className="incidents-view" aria-labelledby="incidents-heading">
      <div className="incidents-view__head">
        <h2 id="incidents-heading">Failure incidents</h2>
        <form className="project-picker" onSubmit={handleSubmit}>
          <label htmlFor="project-id">GitLab project id</label>
          <input
            id="project-id"
            name="project-id"
            inputMode="numeric"
            placeholder="e.g. 1001"
            value={projectInput}
            onChange={(e) => setProjectInput(e.target.value)}
          />
          <button type="submit">Load</button>
        </form>
      </div>

      <IncidentsResult state={state} />
    </section>
  );
}

function IncidentsResult({ state }: { state: LoadState }) {
  switch (state.status) {
    case "idle":
      return <p className="hint">Enter a registered GitLab project id to view its incidents.</p>;
    case "loading":
      return (
        <p className="hint" role="status">
          Loading incidents…
        </p>
      );
    case "error":
      return (
        <p className="hint hint--error" role="alert">
          {state.message}
        </p>
      );
    case "loaded":
      if (state.incidents.length === 0) {
        return <p className="hint">No failure incidents recorded for this project yet.</p>;
      }
      return <IncidentsTable incidents={state.incidents} />;
  }
}

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return "Unexpected error loading incidents.";
}
