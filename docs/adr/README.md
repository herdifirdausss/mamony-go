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
| [ADR-0018 — Browser password-change HTTP candidate](0018-first-party-password-change-http-candidate.md) | PROPOSED: explicit unmounted first-party password-change handler reusing session-bound CSRF and internal credential proof; no public route. |
| [ADR-0019 — Browser password-change PostgreSQL E2E](0019-browser-password-change-postgres-e2e.md) | PROPOSED: real Chromium cookie/CSRF, Argon2id, PostgreSQL epoch/audit/notice and lost-response behavior under disposable TLS fixture; no live route. |
| [ADR-0020 — Password-change recovery page](0020-unmounted-password-change-recovery-page.md) | PROPOSED: unmounted strict-CSP one-shot browser page, with explicit uncertainty lock and no automatic retry or recovery claim. |
| [ADR-0021 — Password recovery authority foundation](0021-password-recovery-authority-foundation.md) | PROPOSED: 256-bit domain-separated recovery proof primitive and threat model; no issuance, SQL authority, public recovery route or password reset. |
| [ADR-0022 — Password recovery issuance and sealed outbox](0022-password-recovery-issuance-outbox.md) | PROPOSED: Identity v19 dedicated EXECUTE-only issuer, independent HMAC recovery budget and atomic challenge/ciphertext queue; no HTTP, worker or redemption. |
| [ADR-0023 — Recovery AEAD and fenced delivery worker](0023-password-recovery-delivery-worker.md) | PROPOSED: Identity v20 EXECUTE-only lease/finalize worker, separate AES-GCM recovery keys, bounded Mailgun adapter and local mock tests; no live send or reset. |
| [ADR-0024 — Assurance-gated atomic recovery redemption](0024-atomic-password-recovery-redemption.md) | PROPOSED: Identity v21 separate EXECUTE-only one-use recovery authority, default-deny assurance grant, global session epoch rotation and token-free recovery notice; no public reset. |
| [ADR-0025 — Token-free recovery security notice worker](0025-password-recovery-security-notice-worker.md) | PROPOSED: Identity v22 separate recovery-only alert worker, strict SQL lease, count-only monitor and seven-day recipient retention; no live send. |
| [ADR-0026 — Signed recovery notice Mailgun webhook](0026-signed-recovery-notice-webhook.md) | PROPOSED: Identity v23 signed HMAC callback, separate recovery-only receipt role/table, monotonic provider states and count-only delivery health; no mounted route. |
| [ADR-0027 — Offline saved recovery code assurance](0027-offline-recovery-code-assurance.md) | PROPOSED: Identity v24 purpose-separated 256-bit pre-enrolled saved codes, challenge-bound five-minute assurance, one-use/audited SQL and strict role separation; no live enrollment or public reset. |
| [ADR-0028 — Recovery-code enrollment and rotation](0028-recovery-code-enrollment-rotation.md) | PROPOSED: Identity v25 session+fresh password, independent default-deny bootstrap, saved-code-gated five-code rotation, purpose-specific reauth and separate token-free notice obligation; unmounted. |
| [ADR-0029 — Recovery-code security notice worker](0029-recovery-code-security-notice-worker.md) | PROPOSED: Identity v26 separate EXECUTE-only worker and count-only monitor, fenced Mailgun token-free enrollment/rotation alerts, retries and seven-day PII erasure; unmounted. |
| [ADR-0030 — Signed recovery-code notice Mailgun webhook](0030-recovery-code-notice-signed-webhook.md) | PROPOSED: Identity v27 signed unmounted HMAC callback, recovery-code-only digest receipts, early ACK reconciliation, monotonic delivery states and separate count-only delivery health; no live webhook. |

A baseline defines what an implementation must preserve. A proposed choice needs owner review. Use the [task-specific documentation index](../index.md) to find the tests and implementation status.
