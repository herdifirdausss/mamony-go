# Decisions to resolve before implementation

Use the local [architecture decision index](adr/README.md) for baseline requirements. Record each choice below when its stage starts. A candidate needs tests in Mamony Go before it becomes an implementation claim.

## Foundation and access

**DEC-01 ACCEPTED by the repository owner on 2026-11-21:** PostgreSQL 18.7, Go 1.27 (CI toolchain 1.27.4), native pgx v5, and SQL-only golang-migrate v4 with database-committed SHA-256 fingerprint verification and separate owner/migrator/runtime roles. [Research and trade-offs](proposals/postgresql-foundation.md#dec-01--proposed-runtimedatabase-implementation-decision-g1g3) remain available for context. Acceptance of the design is not equivalent to passing migration tests or deploying production credentials; see [migration runbook](runbooks/platform-migrations.md). The release image digest and production secret source must be pinned/verified at deployment.\n\n| ID | Choice | Decision evidence |
|---|---|---|
| DEC-01 | **ACCEPTED** — PostgreSQL 18.7, Go 1.27, native pgx v5, golang-migrate v4, applied-file SHA-256 verification and separate credentials. | Verify fresh/upgrade migration, dirty state, runtime grants, and restore; implementation evidence remains separately tracked. |
| DEC-02 | Choose the Go transaction and locking design for posting. Define which metadata belongs in the replay fingerprint. | Same-key contention, floor race, close and post race, and lost response after commit. |
| DEC-03 | Choose Go password and factor storage, recovery, credential rotation, and revocation timing. | Threat review and tests for operator second factor, revocation, and recovery. |
| DEC-04 | Choose a screening matcher and rule version policy. Choose list age and review limits. | Synthetic false-positive and miss fixtures. A missing or stale list always causes hold. |
| DEC-05 | Approve a job role only when its access cannot use an existing service boundary. Define its exact grants. | Runtime catalog test, least privilege, and no access from a request handler. |

## Money and recovery

| ID | Choice | Decision evidence |
|---|---|---|
| DEC-06 | Choose the [wire amount format](adr/0003-public-http-contract.md#open-decision-on-wire-amounts) before publishing an amount route. | Exact round trip at the wire and storage limits. Publish OpenAPI and compare it with the routes. |
| DEC-07 | Define when each usage reservation becomes used or released. Define the account-suspension boundary for work already in a ledger transaction. | Pay-in, pay-out, cancel, unknown, expiration, and concurrent suspension fixtures. |
| DEC-08 | Choose the FX action when a slow provider succeeds after its quote expires. | Specify the rate and approval policy. Test late success and a partial FX execution. |
| DEC-09 | Set pilot corridor, cutoffs, deadlines, return windows, and reporting grace periods. | Synthetic provider fixtures cover both flows and all valid transitions. |
| DEC-10 | Set event retention, replay access, and the complete rebuild source for accounting. | Test a replay inside retention and a rebuild older than the outbox window. |
| DEC-11 | Confirm provider ports and event fields for recipient checks, disputes, evidence, and monitoring. | Consumer contract tests cover missing data and supported event versions. |
| DEC-12 | Choose restore cadence, recovery target, and load thresholds for each gate. | Measure results in Go with the selected data size and job workload. |

## Later scope

Decide the dispute shortfall policy, protected audit checkpoint, sandbox isolation, and split-payment liability before their affected features start. Read the named contract for each decision. Keep unchosen values out of runtime defaults.

## Owner approval record — narrow Identity Phase 1 direction

**Approved 2026-11-21:** Continue the first-party browser, repository-owned Go Profile A implementation **before ledger**, require proof of control of the current email alias before subject activation, use NFC-normalized new passwords, and implement private single-use 32-byte email-verification challenges with a 20-minute database expiry, digest-only token storage, and atomic outbox-backed activation. This approval authorizes controlled internal implementation and review, **not** public customer onboarding or claims of completed IAM-01..13.

**Still pending before external activation:** vetted fail-closed breached/common/contextual blocklist and maintenance, machine-measured Argon2id/admission budgets, delivery-provider security and crash/retry design, privacy/retention and notice destination, HTTP Origin/CSRF/input/rate enforcement, dedicated session and authorization tests, Profile A residual-compromise signoff and independent audit. A digest-only issuance event does not allow a background worker to reconstruct the original token.

## Security architecture decisions

Make these decisions when their respective features become real. They extend, rather than replace, existing `DEC-01` (database roles), `DEC-03` (credentials/session/recovery), and `DEC-05` (cross-tenant jobs).

| ID | Choice | Required evidence before approval |
|---|---|---|
| DEC-13 | Declare the first client types (browser, machine API, operator) and their authentication, session and CSRF boundary. | Negative sessions, browser requests, API-credential scope and revocation tests for enabled clients. |
| DEC-14 | State attacker capabilities and surviving trust roots. Decide whether to accept Profile A authority-database compromise risk or build genuinely independent identity/approval/execution boundaries later. | DB-writer and app-compromise fixtures with actual credentials, deployment, key grants and currentness behavior; no Profile B claim from a shared root. |
| DEC-15 | Select the real ingress, trusted proxy set, request/JSON bounds, rate limits and resource budgets before adding non-health HTTP routes. | Oversized, duplicate-key, spoofed-forwarded-header, slow and concurrent-request tests at the deployed edge. |
| DEC-16 | Define which money actions need transaction-specific authorization, the exact bound intent fields, receipt consume/expiry and revocation ordering. | Wrong payee/amount/currency, reuse, concurrent consume, stale subject, and crash/unknown-outcome tests. |
| DEC-17 | Choose protected security-event obligations, trusted notification destinations, audit access and restore/retention guarantees. | Failed-outbox, suppressed-notice, unauthorized audit deletion, and restore-with-revocation fixtures. |
| DEC-18 | Select runtime and CI secret sources, purpose-limited crypto/key permissions, emergency revocation, and release provenance as deployment needs arise. | Off-purpose key operation, runtime/migration separation, logged-secret and hostile workflow checks; verify effective platform settings. |

Do not add a standalone security service, signing protocol, OAuth issuer, service mesh, SIEM or multi-region authority by default. Explain which threat, deployment guarantee and adversarial test would justify it.

## DEC-ID-DELIVERY (draft — operational approval required)

**Implemented as an internal candidate in stacked PR #25:** AES-256-GCM encrypted email-proof outbox, separate delivery database role, leased claim with `SKIP LOCKED`, bounded 5-attempt exponential retry and stable challenge-based provider idempotency key. Keys must come from an external purpose-scoped secret source; **not decided**: provider (SES/SMTP/etc.), key custody/rotation authority, trusted sender domain, bounce/complaint handling, final delivery audit retention, per-IP/subject abuse policy, email content URL policy, processing cadence and deployment rollback.

The delivery handler is intentionally not wired into a production process or public API. A green integration test proves synthetic DB/Go mechanics, not sender deliverability or exactly-once behavior.

## DEC-ID-DELIVERY-RUNTIME (draft, production enablement not approved)

A separately executed Identity Mailgun delivery worker exists as stacked PR #27. Code uses a capability facade, a PostgreSQL role with restricted updates, one-at-a-time polling, and dual live-send opt-in with private-file secrets. This is **not** authorization to deploy or send real email: the operator has not provisioned an account/domain, approved a secret manager, sender limits, egress policy, retention, bounce/complaint webhooks, or public API admission. All CI Mailgun sends are intercepted by an injected `httptest` transport. Keep the PR Draft until those decisions and end-to-end negative tests are resolved.

## DEC-ID-EMAIL-ADMISSION (Draft, internal-only policy, external approval pending)

PR #28 introduces a nonconfigurable database-enforced issuer admission baseline: minimum 60 seconds between proofs, maximum 3/hour and 5/rolling 24h per subject, including the first proof and revoked/expired proofs. The guard is evaluated under the authoritative subject/contact locks before any revocation, encrypted delivery or security-outbox insert. Test values are synthetic and not a settled business/security policy. There is **no approved public identity endpoint or trusted ingress**; remaining decisions include client identity proof for resend, uniform account-existence responses, trusted proxy and IP normalization, distributed anonymous rate budgets, privacy-preserving identifiers, provider quota/circuit breaker, CAPTCHA fallback, abuse alerting, and retention of old challenge metadata. Tracking is in the [Identity scope proposal](proposals/identity-enrollment-first-slice.md) and [delivery runbook](runbooks/identity-delivery.md).

## DEC-ID-ANONYMOUS-ADMISSION (Draft, operational trust not approved)

A bounded, privacy-keyed pre-registration gate is implemented in PR #29 but remains **unmounted**. The initial policy is 8/min and 80/hr by IPv4 or IPv6 /64, plus 3/hr and 5/day by canonical email, separated by action; fixed UTC-aligned windows and one PostgreSQL transaction make quotas consistent across instances but do not eliminate boundary bursts. A 32-byte purpose-domain-separated HMAC key supplied externally protects the stored key digests. Current `mamony_identity_runtime` has bounded counter INSERT/SELECT/UPDATE(hits) access but no DELETE; this is not a separate ingress credential or global bot/WAF. Trusted proxy identity is OFF by default; proxy allowlists can only cover narrow private/loopback networks, and accept exactly one sanitized XFF value. Decisions still required: end-user consent and uniform response latency, ingress deployment and direct-bypass blocking, durable background command behavior, HMAC key rotation and bucket cleanup, privacy and legitimate shared IP accessibility, capacity/load benchmarks, global/provider quotas, platform alerting and a separate public HTTP threat review. Do not connect route(s) until signed off.
