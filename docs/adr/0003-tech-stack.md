# ADR 0003 — Go / React / PostgreSQL technology stack

- Status: Accepted
- Date: 2026-11-23

## Context

We need a backend strong at concurrency and long-running workers, a typed
frontend for the dashboard, and a reliable relational store for the source of
truth. The project also serves as a portfolio demonstration, so the stack should
be mainstream and legible.

## Decision

- **Backend: Go.** Good concurrency model for the future ingestion worker,
  static binaries, strong standard library (`net/http`, `log/slog`). We use the
  standard-library router (Go 1.22+ method patterns) to avoid an unnecessary web
  framework dependency.
- **Database: PostgreSQL** via `pgx/v5` + `pgxpool`. PostgreSQL is the single
  source of truth; `jsonb` suits raw webhook payloads.
- **Redis: optional.** Used only for short-lived dedup/queue/rate-limit needs,
  never as the source of truth. Disabled when `REDIS_URL` is unset.
- **Frontend: React + TypeScript (Vite).** Typed, mainstream, fast dev loop.
- **CI: GitHub Actions**, because the surrounding tooling here is GitHub. (The
  *product* integrates with GitLab CI as a target feature; that is unrelated to
  where this repository's own CI runs.)

## Consequences

- Minimal backend dependency surface (pgx plus test libraries).
- The GitLab API client, classifier and worker will be added behind interfaces
  in later milestones without changing this foundation.
- Developers need Go 1.27+, Node 22+, and Docker for the local stack.
