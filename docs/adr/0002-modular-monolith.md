# ADR 0002 — Modular monolith for the first release

- Status: Accepted
- Date: 2026-11-23

## Context

PipelineSherlock has several logical stages (ingestion, normalization,
classification, fingerprinting, API, UI). It is tempting to split these into
microservices up front, but we have no measured scaling or ownership pressure
that would justify the operational cost.

## Decision

Ship the first release as a **modular monolith**: one deployable Go service with
clear internal boundaries (`domain`, `application`, `ports`, `adapters`,
`transport`). Business logic stays independent of HTTP, the database driver and
external GitLab clients, and integration boundaries are expressed as small
interfaces with fakes for tests. We do not introduce a separate service until a
measurement proves it necessary.

## Consequences

- Simple local development and deployment; one process, one binary.
- Boundaries are enforced in code (package structure + interfaces), so a later
  extraction into a separate service is mechanical rather than a rewrite.
- We must resist adding infrastructure (message brokers, extra services) that a
  measurement has not justified.

See the component boundaries in
[`docs/architecture.md` §2](../architecture.md#2-milestone-0-component-view-built-m0).
