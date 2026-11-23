# Architecture decisions

These IDs are local to Mamony Go. Read the decision for your change and the contract it links to.

| Decision | Status |
|---|---|
| [ADR-0001 — One process with capability boundaries](0001-go-modular-monolith.md) | Proposed architecture. The current bootstrap uses one process and two listeners. |
| [ADR-0002 — Ledger and data ownership](0002-ledger-and-data-ownership.md) | Contract baseline. PostgreSQL work is pending. |
| [ADR-0003 — Public HTTP contract](0003-public-http-contract.md) | Contract baseline. The wire-amount decision is open. |
| [ADR-0004 — Internal password-authentication foundation](0004-password-authentication-foundation.md) | Candidate only; read-only authenticator, no login/session route. |

A baseline defines what an implementation must preserve. A proposed choice needs owner review. Use the [task-specific documentation index](../index.md) to find the tests and implementation status.
