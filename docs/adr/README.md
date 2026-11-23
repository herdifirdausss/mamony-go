# Architecture decisions

These IDs are local to Mamony Go. Read the decision for your change and the contract it links to.

| Decision | Status |
|---|---|
| [ADR-0001 — One process with capability boundaries](0001-go-modular-monolith.md) | Proposed architecture. The current bootstrap uses one process and two listeners. |
| [ADR-0002 — Ledger and data ownership](0002-ledger-and-data-ownership.md) | Contract baseline. PostgreSQL work is pending. |
| [ADR-0003 — Public HTTP contract](0003-public-http-contract.md) | Contract baseline. The wire-amount decision is open. |
| [ADR-0004 — Internal password-authentication foundation](0004-password-authentication-foundation.md) | Candidate only; read-only authenticator, no login/session route. |
| [ADR-0005 — Pre-Argon2id login admission](0005-login-admission-before-argon2id.md) | Candidate only; durable global/client/alias quotas before KDF, no login/session route. |
| [ADR-0006 — Opaque session authority](0006-opaque-session-authority.md) | Candidate only; atomic eligibility recheck, epoch-bound opaque bearer, no public routes. |
| [ADR-0007 — First-party browser session HTTP contract](0007-first-party-browser-session-http-contract.md) | Proposed only; cookie/CSRF/login/logout behavior, lock-order evidence, still unmounted. |
| [ADR-0008 — Unmounted first-party browser session handler](0008-unmounted-browser-session-http-candidate.md) | Candidate handler + HMAC CSRF and httptest; routes still unmounted, no production launch. |
| [ADR-0009 — Authentication-to-session issuance trust boundary](0009-authentication-to-session-issuance-boundary.md) | PROPOSED: v11 grant-bound issuer in stacked Draft #46, role-capability/negative tests; no production approval or route activation. |

A baseline defines what an implementation must preserve. A proposed choice needs owner review. Use the [task-specific documentation index](../index.md) to find the tests and implementation status.
