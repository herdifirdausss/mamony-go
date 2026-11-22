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
| [ADR-0010 — Server-side session idle expiry](0010-server-side-session-idle-expiry.md) | PROPOSED: v12 30-minute DB-clock inactivity expiry; 12-hour absolute unaffected, no public routing. |
| [ADR-0011 — Atomic five-session cap](0011-atomic-five-session-cap.md) | PROPOSED: v13 grant-bound mint atomically evicts oldest live session when five slots are occupied; consistent SQL lock order, no public routing. |
| [ADR-0012 — Auth material epoch and grant fence](0012-auth-material-epoch-grant-fence.md) | PROPOSED: v14 invalidates sessions on credential/contact change and requires exact pre-KDF epoch for grant staging; no public mutations. |
| [ADR-0013 — Authenticated password change](0013-authenticated-password-change.md) | PROPOSED: v15 separate purpose-bound password-change grant plus valid session; atomic verifier/epoch/audit update, no public route. |
| [ADR-0014 — Password-change security notices](0014-password-change-security-notices.md) | PROPOSED: v16 durable event-bound notification queue with original SQL login-role provenance, leases/retry, and recipient retention; mock-only delivery. |
| [ADR-0015 — Mailgun password-change notice worker](0015-mailgun-password-change-notice-worker.md) | PROPOSED: dedicated Mailgun token-free sender and opt-in security-notice worker, with local mock + PostgreSQL evidence; no live deployment. |
| [ADR-0016 — Mailgun notice webhook reconciliation](0016-mailgun-security-notice-webhook-reconciliation.md) | PROPOSED: v17 digest-only provider message correlation, signed unmounted webhook, replay dedup, count-only failure monitoring; no live ingress. |
| [ADR-0017 — Read-only notice health probe](0017-read-only-notice-health-probe.md) | PROPOSED: v18 least-privilege aggregate-only monitoring role and explicit thresholded one-shot JSON/exit-code probe; no scheduler or deployment. |

A baseline defines what an implementation must preserve. A proposed choice needs owner review. Use the [task-specific documentation index](../index.md) to find the tests and implementation status.
