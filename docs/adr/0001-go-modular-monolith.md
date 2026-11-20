# ADR-0001 — One process with capability boundaries

**Status:** Proposed

**Date:** 2026-11-21

## Context

Mamony Go needs clear data ownership and predictable imports. The first release can run all capabilities in one process.

## Decision

Use one Go module and one binary. Give each capability a public facade and private implementation packages. Put cross-capability adapters in the application composition root. Keep business rules in the capability that owns them.

Use separate public and private HTTP listeners. The capability owns its handler and route metadata. The application registers each route on one listener.

## Consequences

The package graph and route tests give early feedback. A separate process adds deployment and failure modes. Propose a new ADR when measured load or a required isolation boundary justifies a split.

See [package and route rules](../architecture.md) and [ledger and data ownership](0002-ledger-and-data-ownership.md).
