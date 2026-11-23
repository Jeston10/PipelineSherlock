# PROJECT_STATUS

Living status for PipelineSherlock. Updated as milestones progress.

Last updated: 2026-11-23 (M1 slice 2)

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

## Current milestone: M2 — failure analysis 🚧 in progress

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

### Slice 2 — persistence & wiring (next)

- Plumb job ids out of `PipelineRepo.Upsert` (or a lookup) so occurrences can
  reference the `jobs` row.
- `FingerprintRepo`: upsert into `failure_fingerprints` (increment
  `occurrence_count`, update `last_seen_at`) and append to
  `failure_occurrences` with evidence.
- Wire the engine into the worker: for failed jobs, fetch the trace, `Analyze`,
  and persist the diagnosis. Live-DB integration coverage.

## Exact local run commands

```bash
cp .env.example .env
docker compose up -d db redis
make migrate
make run         # serve on :8080
make seed        # optional demo data
make test
```
