# ADR 0001 — Record architecture decisions

- Status: Accepted
- Date: 2026-11-23

## Context

We want a lightweight, durable record of *why* the architecture is the way it
is, so reviewers and future contributors can see the trade-offs rather than
guessing. A portfolio project especially benefits from visible reasoning.

## Decision

We record significant architecture decisions as short ADRs in `docs/adr/`,
numbered sequentially. Each ADR states context, the decision, and its
consequences. Diagrams for the mechanisms these decisions produce live in
[`docs/architecture.md`](../architecture.md).

## Consequences

- Decisions are reviewable in isolation and in version control history.
- The overhead is one small markdown file per real decision — not per change.
- Superseded ADRs are kept and marked, never deleted.
