# PROJECT_STATUS

Living status for PipelineSherlock. Updated as milestones progress.

Last updated: 2026-11-24 (M3 slice 2)

## Milestone 0 — Foundation ✅ complete

### Done

- Go module + repository structure (modular monolith: `domain`, `platform`,
  `transport`, `cmd`).
- `config` package: env loading with fail-fast, aggregated validation; duration
  parsing (Go-duration or integer seconds); credential-redacting `Redacted()`.
  Unit-tested, including a credential-leak guard.
- `domain.Project`: first domain entity with validated invariants (positive
  GitLab id, non-empty/bounded name, GitLab-style namespace that rejects
  path-traversal segments). Immutable value semantics. Unit-tested
  (table-driven, `errors.Is` matching).
- `platform/postgres`: pgx connection pool with bounded ping; small embedded
  migration runner (transactional up/rollback, `schema_migrations` ledger).
- `transport/http`: std-lib router with method patterns; panic-recovery,
  structured access-logging (no secrets) and request-id middleware;
  `/healthz` liveness and `/readyz` readiness (DB ping). Unit-tested, including
  an error-leak guard.
- `cmd/server`: wiring + lifecycle (`serve` / `migrate` / `rollback`), graceful
  shutdown on SIGINT/SIGTERM.
- Core schema migration `0001_init` (projects, webhook_events, pipelines, jobs,
  failure_fingerprints, failure_occurrences, comments), reversible.
- Docker Compose (Postgres + Redis), Dockerfile (distroless), Makefile
  (`make dev` one-command path), `.env.example`, `.gitignore`.
- GitHub Actions CI: gofmt check, `go vet`, staticcheck, build, `go test -race`
  with a Postgres service; separate frontend build job.
- Minimal Vite React + TS frontend showing backend health.
- Docs: `docs/architecture.md` with a workflow diagram + walkthrough per
  mechanism; ADRs 0001–0004.

### Verified locally

- `go build ./...` — clean.
- `go vet ./...` — clean.
- `go test ./...` — all packages pass (race detector run in CI; local gcc is
  32-bit so `-race` cannot compile here).

### Not yet verified locally

- Live database migration + `/readyz` against a running Postgres (requires
  `docker compose up db`). The migrator and readiness check have unit coverage
  but have not been exercised end-to-end on this machine yet.
- Frontend `npm install` / build.

## Milestone 1 — GitLab ingestion ✅ complete

### Slice 1 — webhook ingestion ✅ done (this slice)

- Migration `0002`: `projects.webhook_secret_hash` (reversible).
- `platform/token`: generate 256-bit secret, SHA-256 hash, constant-time
  verify. Unit-tested.
- `domain`: `Project` extended with webhook secret (hash only, immutable);
  `WebhookEvent` inbox entity with status enum + validation. Unit-tested.
- `application`: `RegisterProject` (mints secret, stores hash, returns plaintext
  once) and `IngestWebhook` (project lookup, token verify, idempotent inbox
  insert) use cases, behind repository ports. Unit-tested with fakes.
- `adapters/postgres`: `ProjectRepo` (unique-violation → already-exists) and
  `WebhookEventRepo` (ON CONFLICT DO NOTHING → dedup).
- `transport/http`: `POST/GET /api/v1/projects`, `POST /api/v1/webhooks/gitlab`
  (size-bounded body, `X-Gitlab-Token` validation, delivery-key dedup,
  202/400/401/404). Router refactored to `Deps`. Unit-tested end-to-end.

Verified locally: `go build`, `go vet`, `gofmt -s`, `go test ./...` all clean.

### Slice 2 — processing ✅ done (this slice)

- Migration `0003`: `webhook_events.attempts` (reversible).
- `config`: `GITLAB_BASE_URL` (default `https://gitlab.com`) and optional
  `GITLAB_TOKEN`; `GitLabConfigured()` gates reconciliation; the token is
  redaction-guarded in logs. Unit-tested.
- `domain`: `Pipeline` + `Job` entities (immutable timing via `WithTiming`);
  `WebhookEvent` gains an `Attempts` counter. Unit-tested.
- `application`: `ProcessEventsService` (claim → parse → reconcile → upsert →
  mark processed, with bounded requeue/fail) and a `Worker` loop that drains the
  inbox and idles when empty. Unit-tested with in-memory fakes.
- `adapters/postgres`: `WebhookEventRepo` gains `ClaimNext`
  (`FOR UPDATE SKIP LOCKED`), `MarkProcessed`, `Requeue`, `MarkFailed`;
  new `PipelineRepo.Upsert` (pipeline + jobs in one tx, idempotent).
- `adapters/gitlab`: REST client implementing `application.GitLabClient` with
  per-request timeout, pagination (`X-Next-Page`) and bounded retries with full
  jitter. Unit-tested against `httptest`.
- `cmd/server`: worker wired into the lifecycle, started on `serve` and stopped
  on shutdown; GitLab reconciliation enabled only when a token is configured.
- Live-DB integration test (`-tags=integration`) exercising migrate → register →
  webhook insert + dedup → claim → idempotent upsert → mark processed; wired
  into CI against the Postgres service.

Verified locally: `go build`, `go build -tags=integration`, `go vet`,
`gofmt -s`, `go test ./...` all clean. The race detector and the live-DB
integration test run in CI (local gcc is 32-bit, so `-race` cannot compile
here; no local Postgres for the integration test).

## Milestone 2 — failure analysis ✅ complete

### Slice 1 — the deterministic analysis engine ✅ done (this slice)

- `internal/analysis` (pure, no I/O / clock / randomness):
  - `Normalize` — strips ANSI and replaces volatile tokens (timestamps,
    durations, hashes, addresses, numbers) with placeholders; idempotent.
  - `Classify` — priority-ordered rule-based classifier (oom, timeout, infra,
    dependency, compile, lint, test, unknown), returning the salient signature
    line and a human-readable evidence excerpt. Root cause beats symptom.
  - `Fingerprint` / `Analyze` — stable SHA-256 over
    (project ∥ category ∥ normalized signature ∥ algorithm version) with a
    field separator; `Analyze` is the entry point returning a full `Diagnosis`.
  - Unit-tested, including the convergence property (same failure across runs →
    one fingerprint) and distinctness.
- `adapters/gitlab`: `GetJobTrace` (raw text endpoint) added, sharing the retry
  core; unit-tested against `httptest`.

Verified locally: `go build`, `go vet`, `gofmt -s`, `go test ./...` all clean.

### Slice 2 — persistence & wiring ✅ done (this slice)

- Migration `0004`: unique `(job_id, fingerprint_id)` on `failure_occurrences`
  (reversible) for idempotent occurrence recording.
- `application`: `FingerprintRepository` port + `FingerprintOccurrence`;
  `PipelineRepository.Upsert` now returns a `gitlab_job_id → jobs.id` map;
  `GitLabClient` gains `GetJobTrace`.
- `adapters/postgres`: `FingerprintRepo.RecordOccurrence` (one tx: upsert
  fingerprint, insert occurrence `ON CONFLICT DO NOTHING`, bump
  `occurrence_count` only on a new row); `PipelineRepo.Upsert` returns job ids.
- `application`: the processor fingerprints each failed job (fetch trace →
  `Analyze` → `RecordOccurrence`) when a GitLab token is configured; best-effort
  (a per-job error is logged and skipped, never failing the event). Unit-tested.
- `cmd/server`: `FingerprintRepo` wired into the processor.
- Live-DB integration test extended: idempotent fingerprint/occurrence recording
  (record twice → one occurrence, count stays 1).

Verified locally: `go build`, `go build -tags=integration`, `go vet`,
`gofmt -s`, `go test ./...` all clean.

## Milestone 3 — incidents & grouping ✅ complete

### Slice 1 — incident read path ✅ done (this slice)

- `domain`: `Incident` read-model entity (fingerprint, category, normalized
  signature, algorithm version, occurrence count, first/last seen).
- `application`: `IncidentRepository` port + `IncidentService.List` — resolves a
  GitLab project id to the registered project (`ErrProjectNotFound` if unknown),
  then lists its incidents with a clamped limit (default 50, max 200).
  Unit-tested with fakes.
- `adapters/postgres`: `IncidentRepo.ListByProject` reads `failure_fingerprints`
  (one incident per fingerprint) ordered by `last_seen_at` desc, limited.
- `transport/http`: `GET /api/v1/projects/{gitlab_project_id}/incidents`
  (`?limit=`), behind a small `IncidentLister` interface — 200 with a JSON
  array, 400 on an invalid id, 404 when the project is not registered.
  Unit-tested end-to-end.
- `cmd/server`: `IncidentRepo` + `IncidentService` wired into `Deps`.
- Live-DB integration test extended: after recording a fingerprint, the incident
  read model surfaces it as exactly one incident.

Verified locally: `go build`, `go build -tags=integration`, `go vet`,
`gofmt -s`, `go test ./...` all clean.

### Slice 2 — incident detail & signals ✅ done (this slice)

- `domain`: `IncidentOccurrence`, `RefSummary`, `IncidentSignals` and
  `IncidentDetail` read models; `AnalyzeIncident` — a pure function computing a
  per-ref breakdown and heuristic flaky/regression signals from the occurrence
  history (no clock/I/O; fully unit-tested, table-driven).
- `application`: `IncidentRepository` gains `GetByFingerprint` and
  `ListOccurrences`; `IncidentService.Detail` resolves project → fingerprint →
  occurrences and returns the assembled `IncidentDetail`. New sentinel
  `ErrIncidentNotFound`; occurrence limit clamped (default 100, max 500).
  Unit-tested with fakes.
- `adapters/postgres`: `IncidentRepo.GetByFingerprint` (most-recent row per
  fingerprint; clean not-found) and `ListOccurrences` (occurrences ⨝ jobs ⨝
  pipelines for ref/sha/stage/GitLab ids, newest first, bounded).
- `transport/http`: `GET /api/v1/projects/{gitlab_project_id}/incidents/{fingerprint}`
  behind the grown `IncidentLister`; 200 detail / 400 invalid id / 404 unknown
  project or fingerprint. Unit-tested end-to-end.
- Live-DB integration test extended: resolve a recorded fingerprint by header,
  list its enriched occurrence (ref/sha/job), and confirm unknown → not-found.

Verified locally: `go build`, `go build -tags=integration`, `go vet`,
`gofmt -s`, `go test ./...` all clean.

## Next milestone: M4 — read API & dashboard (next)

- A React + TypeScript dashboard to browse incidents, drill into an incident's
  occurrence history and evidence, and surface the flaky/regression signals.
- Any read-API gaps the dashboard needs (filtering, pagination cursors).
- Later milestones: GitLab MR write-back (comments table), auth/RBAC, rate
  limiting, observability, and richer flaky/regression detection using success
  data and timing.

## Exact local run commands

```bash
cp .env.example .env
docker compose up -d db redis
make migrate
make run         # serve on :8080
make seed        # optional demo data
make test
```
