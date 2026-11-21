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
