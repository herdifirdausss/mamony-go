# ADR-0001 — Go-specific modular monolith translation

- Status: Proposed — owner decision required
- Date: 2026-11-21
- Source: Mamony original [ADR-0004](https://github.com/herdifirdausss/mamony/blob/main/docs/adr/0004-modular-monolith-first.md), [ADR-0027](https://github.com/herdifirdausss/mamony/blob/main/docs/adr/0027-first-runtime-and-http-layer.md)

## Context
Mamony's initial implementation is Rust. The Go project must preserve ownership/invariants while avoiding a premature multi-process deployment. Seev is a reference for architecture enforcement, not a specification of Mamony behavior.

## Proposed decision
One Go module; one binary; separate public/private HTTP listener; explicit app composition. Business capabilities live under `internal/modules/<capability>/` with nested `internal` private implementation packages. Go interfaces live with consumers when helpful. No module imports another; app adapters translate dependencies.

## Alternatives
Global `handlers/usecases/repositories` dilute capability ownership. Nine deployables as in Seev introduce unneeded infrastructure for initial scope.

## Consequences
Compile-time privacy and import checks provide early feedback. No claim of full architecture/runtime parity until PRD/TD and database contracts are implemented and tested.
