import { useEffect, useState } from "react";
import { fetchIncidentDetail, type IncidentDetail } from "../../lib/incidents";
import { IncidentDetail as IncidentDetailCard } from "../../components/IncidentDetail";

interface IncidentDetailViewProps {
  gitlabProjectId: number;
  fingerprint: string;
  onBack: () => void;
}

type DetailState =
  | { status: "loading" }
  | { status: "error"; message: string }
  | { status: "loaded"; detail: IncidentDetail };

// IncidentDetailView is the container for a single incident: it owns the detail
// load lifecycle for the given (project, fingerprint) and renders the back
// control plus the detail card. The fetch cancels in flight on change/unmount.
export function IncidentDetailView({
  gitlabProjectId,
  fingerprint,
  onBack,
}: IncidentDetailViewProps) {
  const [state, setState] = useState<DetailState>({ status: "loading" });

  useEffect(() => {
    const controller = new AbortController();
    setState({ status: "loading" });

    fetchIncidentDetail(gitlabProjectId, fingerprint, { signal: controller.signal })
      .then((detail) => {
        if (!controller.signal.aborted) {
          setState({ status: "loaded", detail });
        }
      })
      .catch((err: unknown) => {
        if (controller.signal.aborted) {
          return;
        }
        setState({ status: "error", message: errorMessage(err) });
      });

    return () => controller.abort();
  }, [gitlabProjectId, fingerprint]);

  return (
    <div className="detail-view">
      <button type="button" className="back" onClick={onBack}>
        ← Back to incidents
      </button>
      <DetailResult state={state} />
    </div>
  );
}

function DetailResult({ state }: { state: DetailState }) {
  switch (state.status) {
    case "loading":
      return (
        <p className="hint" role="status">
          Loading incident…
        </p>
      );
    case "error":
      return (
        <p className="hint hint--error" role="alert">
          {state.message}
        </p>
      );
    case "loaded":
      return <IncidentDetailCard detail={state.detail} />;
  }
}

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return "Unexpected error loading the incident.";
}
