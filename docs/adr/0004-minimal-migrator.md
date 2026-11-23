# ADR 0004 — Minimal in-house migration runner for Milestone 0

- Status: Accepted
- Date: 2026-11-23

## Context

We need reversible, version-controlled schema migrations. The obvious choice is
a mature library such as `golang-migrate` or `goose`. However, for Milestone 0
we value a small, fully auditable dependency surface and code the reader can
understand end to end.

## Decision

Milestone 0 ships a **small in-house migrator** (~200 lines in
`internal/platform/postgres`) that:

- embeds `*.sql` files from `migrations/` into the binary,
- pairs `.up`/`.down` files and sorts by numeric version,
- tracks applied versions in a `schema_migrations` ledger table,
- applies each migration and its ledger insert inside a single transaction,
- supports `rollback` of the most recent migration.

It adds no dependency beyond `pgx`, which we already need.

## Consequences

- Fewer transitive dependencies; the entire migration path is reviewable.
- We own edge cases (concurrent migration runs are not coordinated; acceptable
  for single-instance dev/deploy at this stage).
- **Revisit trigger:** if we need advisory-locked concurrent migrations,
  non-transactional statements (e.g. `CREATE INDEX CONCURRENTLY`), or multiple
  database backends, replace this with `golang-migrate` behind the same
  `Migrate`/`Rollback` surface. This is recorded as a known future decision.

See the flow in
[`docs/architecture.md` §6](../architecture.md#6-database-migration-flow-built-m0).
