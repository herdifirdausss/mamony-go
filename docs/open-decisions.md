# Decisions to resolve before implementation

See [current state](current-state.md) for the merged implementation and verification boundaries. The dated Draft references below preserve decisions at their original slices. For working status, follow the current issue, contract, or ADR.

## Decision lookup by capability

| Capability | Start here |
|---|---|
| Platform and migrations | [Effective DEC-01 and Foundation choices](#foundation-and-access), [proposed DEC-01 amendment](#proposed-dec-01-amendment--foundation-105), [PostgreSQL contract](contracts/postgresql.md) |
| Observability and telemetry | [Critical proposal #117](proposals/observability-foundation.md) (owner-approved candidate; independent technical review pending; no effective policy adoption) |
| Identity and public access | [Enrollment release decisions](#identity-enrollment-release-decisions), [login and session release decisions](#identity-login-and-session-release-decisions), [password-change release decisions](#identity-password-change-release-decisions), [owning Identity contract](contracts/identity.md), [security architecture decisions](#security-architecture-decisions), [bounded passkey possession decision](#atomic-passkey-possession-verification-v36-critical-issue-78) |
| Account recovery and passkeys | [Account-recovery release decisions](#identity-account-recovery-release-decisions), [passkey release decisions](#identity-passkey-release-decisions) |
| Platform recovery and Identity recovery sessions | [Platform recovery issue #14](https://github.com/herdifirdausss/mamony-go/issues/14), [recovery-fence evidence and E2 status](proposals/identity-recovery-fence.md#current-rf--experiment-disposition-supersedes-earlier-dependency-labels), [separate recovery-session prerequisites](proposals/identity-recovery-session-capability.md#changed-blocker-disposition) |
| Ledger, money flows, and the pilot | [Money and recovery choices](#money-and-recovery), [Ledger implementation sequence](roadmap.md#ledger-implementation-sequence), [Ledger ownership decision](adr/0002-ledger-and-data-ownership.md) |
| Governance and system design | [Branch protection and review issue #13](https://github.com/herdifirdausss/mamony-go/issues/13), [architecture decision index](adr/README.md) |

Consult the linked record for the exact decision scope. Independent technical acceptance, implementation verification, merge, and operational approval each have their own evidence.

## Foundation and access

**DEC-01 ACCEPTED by the repository owner on 2026-11-21:** PostgreSQL 18.7, Go 1.27 (CI toolchain 1.27.4), native pgx v5, and SQL-only golang-migrate v4 with database-committed SHA-256 fingerprint verification and separate owner/migrator/runtime roles. [Research and trade-offs](proposals/postgresql-foundation.md#dec-01--proposed-runtimedatabase-implementation-decision-g1g3) remain available for context. Acceptance of the design is not equivalent to passing migration tests or deploying production credentials; see [migration runbook](runbooks/platform-migrations.md). The release image digest and production secret source must be pinned/verified at deployment.

| ID | Choice | Decision evidence |
|---|---|---|
| DEC-01 | **ACCEPTED** — PostgreSQL 18.7, Go 1.27, native pgx v5, golang-migrate v4, applied-file SHA-256 verification and separate credentials. | Verify fresh/upgrade migration, dirty state, runtime grants, and restore; implementation evidence remains separately tracked. |
| DEC-02 | Owner selected Go repository/direct parameterized SQL and Ledger-local ACID as design direction; exact isolation, lock graph, constraints, role matrix, replay/fact encoding and authority remain open. | Same-key contention, floor race, close and post race, and lost response after commit. |
| DEC-03 | Choose Go password and factor storage, recovery, credential rotation, and revocation timing. | Threat review and tests for operator second factor, revocation, and recovery. |
| DEC-04 | Choose a screening matcher and rule version policy. Choose list age and review limits. | Synthetic false-positive and miss fixtures. A missing or stale list always causes hold. |
| DEC-05 | Approve a job role only when its access cannot use an existing service boundary. Define its exact grants. | Runtime catalog test, least privilege, and no access from a request handler. |

### Proposed DEC-01 amendment — Foundation #105

The owner accepted C1–C6 as the **detailed design candidate for review**, not as an effective replacement of DEC-01. See [ADR-0042](adr/0042-context-aware-migration-executor.md), [compatibility and fault-test proposal](proposals/native-pgx-migration-executor.md), and the [proposed migration contract](contracts/postgresql.md#proposed-native-migration-contract-not-effective). The current golang-migrate v4 engine, existing SQL/history, roles, administrative entry point and active DEC-01 remain authoritative. Native pgx implementation, actual fault execution, engine selection/cutover and merge each require separate authorization. Independent technical acceptance DEFERRED; production readiness NOT ACCEPTED. All new native-executor evidence is NOT RUN.

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

## Identity enrollment release decisions

Use the [Identity issue #20](https://github.com/herdifirdausss/mamony-go/issues/20) for active work and the [Identity contract](contracts/identity.md#enrollment-and-contact-verification) for requirements and the merged internal scope. The [narrow Phase 1 approval](#owner-approval-record--narrow-identity-phase-1-direction) covers controlled internal work. The production decisions below still require evidence and approval.

| Decision still open | Detail and evidence |
|---|---|
| Public ingress, abuse admission, and password capacity | Approve the TLS and trusted proxy model, Origin and request limits, global abuse and provider budgets, behavior under shared addresses, response timing and measured Argon2id capacity. Review blocklist source and maintenance. See [security architecture decisions](#security-architecture-decisions) and [enrollment HTTP history](history/identity-enrollment.md#dec-id-http-enrollment--draft-pr-31-never-mounted). |
| Provider, credentials, and delivery operations | Decide the live provider and sender configuration, key custody and rotation, bounce and complaint handling, quota, retries, monitoring and rollback. The internal Mailgun adapter was verified with a local mock. See the [delivery runbook](runbooks/identity-delivery.md). |
| Retention and backlog operations | Decide account, contact, audit, and backup retention. Review eligible and stale enrollment handling, scheduling, incident response and capacity limits. See the [retention decision history](history/identity-enrollment.md#dec-id-retention--draft-requires-privacylegaloperations-approval) and [reconciliation runbook](runbooks/identity-reconciliation.md). |
| Verification UX and release evidence | Review browser compatibility and accessibility, mail scanners and link tracking, and handling of ambiguous confirmation responses without exposing the proof or automatically retrying it. Evidence from Chromium and disposable PostgreSQL covers the synthetic flow. See the [browser verification limits](runbooks/identity-browser-e2e.md#known-limits--no-production-readiness-claim). |

### Enrollment decision history links

The original anchors below lead to dated decision records covering PRs #25–#38. Read the current decision scope above before using an older candidate record.

#### DEC-ID-DELIVERY (draft — operational approval required)

[Read the original slice record](history/identity-enrollment.md#dec-id-delivery-draft--operational-approval-required).

#### DEC-ID-DELIVERY-RUNTIME (draft, production enablement not approved)

[Read the original slice record](history/identity-enrollment.md#dec-id-delivery-runtime-draft-production-enablement-not-approved).

#### DEC-ID-EMAIL-ADMISSION (Draft, internal-only policy, external approval pending)

[Read the original slice record](history/identity-enrollment.md#dec-id-email-admission-draft-internal-only-policy-external-approval-pending).

#### DEC-ID-ANONYMOUS-ADMISSION (Draft, operational trust not approved)

[Read the original slice record](history/identity-enrollment.md#dec-id-anonymous-admission-draft-operational-trust-not-approved).

#### DEC-ID-RETENTION — Draft, requires privacy/legal/operations approval

[Read the original slice record](history/identity-enrollment.md#dec-id-retention--draft-requires-privacylegaloperations-approval).

#### DEC-ID-HTTP-ENROLLMENT — Draft PR #31 (never mounted)

[Read the original slice record](history/identity-enrollment.md#dec-id-http-enrollment--draft-pr-31-never-mounted).

#### DEC-ID-RECONCILIATION — Draft PR #32 (not deployed)

[Read the original slice record](history/identity-enrollment.md#dec-id-reconciliation--draft-pr-32-not-deployed).

#### DEC-ID-RECONCILE-OPS — Draft PR #33 (no automatic deployment)

[Read the original slice record](history/identity-enrollment.md#dec-id-reconcile-ops--draft-pr-33-no-automatic-deployment).

#### DEC-ID-EMAIL-CONFIRM-HTTP — Draft PR #34 (not mounted)

[Read the original slice record](history/identity-enrollment.md#dec-id-email-confirm-http--draft-pr-34-not-mounted).

#### DEC-ID-CONFIRM-ADMISSION — Draft PR #35 (internal candidate only)

[Read the original slice record](history/identity-enrollment.md#dec-id-confirm-admission--draft-pr-35-internal-candidate-only).

#### DEC-ID-CONFIRM-FAILURE-CLASSIFICATION — Draft PR #36

[Read the original slice record](history/identity-enrollment.md#dec-id-confirm-failure-classification--draft-pr-36).

#### DEC-ID-VERIFICATION-PAGE — Draft PR #37 (unmounted candidate)

[Read the original slice record](history/identity-enrollment.md#dec-id-verification-page--draft-pr-37-unmounted-candidate).

#### DEC-ID-BROWSER-E2E — stacked Draft PR #38

[Read the original slice record](history/identity-enrollment.md#dec-id-browser-e2e--stacked-draft-pr-38).

## Identity login and session release decisions

Use the [Identity contract](contracts/identity.md#password-login-and-browser-sessions) for the merged internal scope and [issue #20](https://github.com/herdifirdausss/mamony-go/issues/20) for active work. Candidate settings and fixture results apply within their recorded scope. Resolve the following decisions before public activation.

| Decision still open | Evidence and scope |
|---|---|
| Login capacity and abuse policy | Measure with deployment resource limits and decide edge load shedding, global and shared-network limits, response timing, retry UX, blocklist operations, and privacy-safe monitoring. Start with the [login performance baseline](runbooks/identity-login-performance.md) and [ADR-0005](adr/0005-login-admission-before-argon2id.md). |
| Session issuer and credential isolation | Review DEC-19 against actual workload and credential isolation. Separate grant and session roles have synthetic denial evidence. Profile A still trusts the Identity runtime and its authoritative database. Owner and security acceptance of the proposed design and production workload proof are outstanding in [ADR-0009](adr/0009-authentication-to-session-issuance-boundary.md). |
| Browser boundary | Approve the TLS and trusted proxy model, Origin checks, the pre-login CSRF policy, key handling, cookie settings, and ambiguous logout UX. Check the applicable deployed edge and browsers. [Browser session candidate](adr/0008-unmounted-browser-session-http-candidate.md) |
| Session lifecycle and access | Review idle and absolute limits, session cap and eviction UX, role and key lifecycle, session-load contention, MFA and reauthentication, recovery, and resource and financial authorization. [Identity contract controls](contracts/identity.md#security-state-and-recovery-threat-controls) · [Session cap design](adr/0011-atomic-five-session-cap.md) |

### Login and session decision history links

The original anchors below lead to the decisions at their original implementation slices.

#### DEC-ID-PASSWORD-AUTHN — stacked Draft PR #39

[Read the original slice record](history/identity-login-and-sessions.md#dec-id-password-authn--stacked-draft-pr-39).

#### DEC-ID-LOGIN-ADMISSION — stacked Draft PR #40

[Read the original slice record](history/identity-login-and-sessions.md#dec-id-login-admission--stacked-draft-pr-40).

#### DEC-ID-OPAQUE-SESSIONS — stacked Draft PR #41

[Read the original slice record](history/identity-login-and-sessions.md#dec-id-opaque-sessions--stacked-draft-pr-41).

#### DEC-ID-SESSION-RACE-AND-HTTP — stacked Draft PR #42

[Read the original slice record](history/identity-login-and-sessions.md#dec-id-session-race-and-http--stacked-draft-pr-42).

#### DEC-ID-BROWSER-SESSION-CANDIDATE — stacked Draft PR #43

[Read the original slice record](history/identity-login-and-sessions.md#dec-id-browser-session-candidate--stacked-draft-pr-43).

#### DEC-19 — Session issuer DB-capability abuse (Draft PR #45; PROPOSED)

[Read the original slice record](history/identity-login-and-sessions.md#dec-19--session-issuer-db-capability-abuse-draft-pr-45-proposed).

#### Draft PR #46 — candidate implementation of DEC-19

[Read the original slice record](history/identity-login-and-sessions.md#draft-pr-46--candidate-implementation-of-dec-19).

#### Draft #47 — server-side session inactivity timeout (proposed, not approved)

[Read the original slice record](history/identity-login-and-sessions.md#draft-47--server-side-session-inactivity-timeout-proposed-not-approved).

#### Draft #48 — per-subject five-active-session cap (proposed)

[Read the original slice record](history/identity-login-and-sessions.md#draft-48--per-subject-five-active-session-cap-proposed).

#### Draft #49 — credential/contact epoch invalidation and stale-KDF grant fence

[Read the original slice record](history/identity-login-and-sessions.md#draft-49--credentialcontact-epoch-invalidation-and-stale-kdf-grant-fence).

## Identity password-change release decisions

Read the [Identity contract](contracts/identity.md#authenticated-password-change-and-security-notices) for the merged synthetic flow and [issue #20](https://github.com/herdifirdausss/mamony-go/issues/20) for active work. The following decisions remain open before public activation or live notification.

| Decision still open | Evidence and scope |
|---|---|
| Reauthentication and runtime isolation | Review the password blocklist and capacity, MFA or step-up, CSRF key rotation, TLS and trusted proxy configuration, and deployment role isolation. Apply the relevant financial and resource authorization separately. [Authenticated password-change design](adr/0013-authenticated-password-change.md) · [HTTP candidate](adr/0018-first-party-password-change-http-candidate.md) |
| Customer guidance and recovery handoff | Approve consent, localization, accessibility, uncertain-outcome guidance, and the trusted sign-in, recovery, and support path. The [one-shot page design](adr/0020-unmounted-password-change-recovery-page.md) and [browser evidence](runbooks/identity-browser-e2e.md#password-change-browser-evidence) describe the candidate scope. |
| Live notice delivery and privacy | Approve the sending domain and provider account, egress, separate credentials and rotation, retention across operational data and backups, and bounce or complaint policy. Review callback ingress, missing-event reconciliation, and ambiguous provider responses. [Sender design](adr/0015-mailgun-password-change-notice-worker.md) · [Callback design](adr/0016-mailgun-security-notice-webhook-reconciliation.md) |
| Operator monitoring and incident response | Set thresholds and monitoring cadence, assign ownership for unresolved or failed notices, and review alerting and recovery procedures. The [health probe design](adr/0017-read-only-notice-health-probe.md) defines the count-only scope. |

### Password-change decision history links

The original anchors below lead to the decisions at their original implementation slices.

#### Draft #50 — authenticated internal password change, purpose-bound grant

[Read the original slice record](history/identity-password-change.md#draft-50--authenticated-internal-password-change-purpose-bound-grant).

#### Draft #51 — durable password-change notification queue (no live send)

[Read the original slice record](history/identity-password-change.md#draft-51--durable-password-change-notification-queue-no-live-send).

#### Draft #52 — token-free Mailgun notice sender and explicit worker

[Read the original slice record](history/identity-password-change.md#draft-52--token-free-mailgun-notice-sender-and-explicit-worker).

#### Draft #53 — provider response correlation, signed webhook receipt and operator health

[Read the original slice record](history/identity-password-change.md#draft-53--provider-response-correlation-signed-webhook-receipt-and-operator-health).

#### Draft #54 — count-only monitoring with a dedicated read-only credential

[Read the original slice record](history/identity-password-change.md#draft-54--count-only-monitoring-with-a-dedicated-read-only-credential).

#### Draft #55 — first-party browser password-change candidate (unmounted)

[Read the original slice record](history/identity-password-change.md#draft-55--first-party-browser-password-change-candidate-unmounted).

#### Draft #56 — real browser/password-change atomicity and lost response

[Read the original slice record](history/identity-password-change.md#draft-56--real-browserpassword-change-atomicity-and-lost-response).

#### Draft #57 — one-shot browser password-change UX (unmounted)

[Read the original slice record](history/identity-password-change.md#draft-57--one-shot-browser-password-change-ux-unmounted).

## Identity account-recovery release decisions

[Issue #20](https://github.com/herdifirdausss/mamony-go/issues/20) tracks Identity Phase 1. The [account-recovery contract](contracts/identity.md#account-recovery-and-saved-codes) links the merged internal paths and their evidence. Public activation still requires decisions on the following scope.

| Decision still open | Evidence and scope |
|---|---|
| Recovery assurance and saved-code enrollment | Approve the assurance policy, independent factor provisioning and revocation, and the response to lost codes or authenticators. The [saved-code design](adr/0028-recovery-code-enrollment-rotation.md) and [pre-provisioned factor design](adr/0031-independent-factor-signed-enrollment.md) record the synthetic scope. |
| Public recovery workflow | Approve ingress abuse controls, response timing, the reset and code-display experience, safe retry and support guidance, and the breached-password policy. The [redemption design](adr/0024-atomic-password-recovery-redemption.md) records the internal transaction and remaining gates. |
| Delivery and monitoring | Review key and role custody, provider setup, signed callback ingress, suppression handling, alert ownership, and retention including backups. The [recovery delivery design](adr/0023-password-recovery-delivery-worker.md) and [notice callback design](adr/0026-signed-recovery-notice-webhook.md) describe local evidence. |
| Credential and session state after a restore | Review the separate [recovery fence](proposals/identity-recovery-fence.md) and [recovery-session design](proposals/identity-recovery-session-capability.md). [E1 #106](https://github.com/herdifirdausss/mamony-go/pull/106) merged a symbolic model. Integrated RF-01–08 evidence remains NOT RUN, RH-03 remains PARTIAL, and E2 remains BLOCKED. |

## Identity passkey release decisions

The [bounded possession contract](contracts/identity.md#bounded-synthetic-passkey-possession) and [ADR-0041](adr/0041-passkey-atomic-activation.md) define the accepted synthetic scope. Review the remaining decisions before any production use.

| Decision still open | Evidence and scope |
|---|---|
| Credential eligibility and trust | Decide whether separately approved login or independent recovery can use a credential, including attestation, provenance, authenticator loss, revocation, and consent policy. Existing password-backed staging and possession evidence retain their bounded scope. |
| Browser and deployment scope | Review complete registration and assertion option binding, RP and Origin deployment, CSRF and abuse controls, and operating role custody. See the [passkey design and acceptance matrix](proposals/identity-passkey-atomic-activation.md). |
| Recovery, notices, and release | Close the recovery and delivery blockers in [ADR-0041](adr/0041-passkey-atomic-activation.md). Production readiness remains NOT ACCEPTED. |

### Atomic passkey possession verification (v36; Critical issue #78)

The owner accepted the [bounded synthetic direction](https://github.com/herdifirdausss/mamony-go/issues/78#issuecomment-7150570178). The design, Go handoff and SQL finisher merged in #79–#81 under bounded synthetic authorizations. [ADR-0041](adr/0041-passkey-atomic-activation.md) records the decision and evidence. The operation remains internal and unmounted. Independent technical acceptance is DEFERRED. Production readiness is NOT ACCEPTED.

| Decision | Status and consequence |
|---|---|
| D1 — non-login possession semantics | **ACCEPTED (bounded)**: possession evidence only; no login, session, independent recovery or authorization. |
| D2 — activation security audit/notice | **ACCEPTED (bounded)**: mandatory atomic audit and pending notice; delivery/retry/retention/incident policy remains a release blocker. |
| D3 — privileged finisher signature-claim oracle | **ACCEPTED (bounded)**: trusted Profile A with explicit residual risks; no independent authority claim or live credentials. |
| D4 — expiration and operations | **ACCEPTED (bounded)**: existing three-minute challenge, ten-minute staging limit; no automatic cleanup or recovery release. Platform #14 owns recovery foundation; Identity owns semantics and obligations. Production recovery and ingress remain release blockers. |

**Unverified release-sensitive scenarios** remain distinct from achieved CI checks: cross-role
provisioning outside disposable fixtures, arbitrary network partitions during COMMIT
(disposable PostgreSQL now passes **pre-COMMIT-request disconnect**,
**full COMMIT TCP write followed by disconnect before any response**
with either atomic outcome accepted, **post-server-CommandComplete
ACK disconnect**, and application-side acknowledgment loss; arbitrary
partitions during server processing, node crash **during** COMMIT,
standby failover and **stale-backup recovery** remain unproven;
a **separate after-test, one-node SIGKILL/restart** and a full **post-commit
logical dump restored on a second disposable PostgreSQL instance** passed
with 9 linked credentials/audits/notices; logical restore intentionally
omits ownership, grants and secrets),
counter/backup handling on real production authenticators (synthetic real-ES256
PostgreSQL counter/BE/BS fixtures now exist), genuine
registration-intent/admission replacement during concurrent writes, rollback
and recovery after a database restore, and non-synthetic production notification
delivery. See #78 for the exact-head evidence and acceptance checklist.

## Account-recovery decision history links

The original anchors below lead to the decisions at their original implementation slices.

### Draft #58 — forgot-password recovery proof and threat model (no reset capability)

[Read the original slice record](history/identity-account-recovery.md#draft-58--forgot-password-recovery-proof-and-threat-model-no-reset-capability).

### Draft #59 — internal recovery issuance with a sealed, non-deliverable queue

[Read the original slice record](history/identity-account-recovery.md#draft-59--internal-recovery-issuance-with-a-sealed-non-deliverable-queue).

### Draft #60 — encrypted password recovery delivery (not launched)

[Read the original slice record](history/identity-account-recovery.md#draft-60--encrypted-password-recovery-delivery-not-launched).

### Draft #61 — unmounted, assurance-gated recovery redemption

[Read the original slice record](history/identity-account-recovery.md#draft-61--unmounted-assurance-gated-recovery-redemption).

### Draft #62 — token-free PASSWORD_RECOVERED alert worker, monitoring and retention

[Read the original slice record](history/identity-account-recovery.md#draft-62--token-free-password_recovered-alert-worker-monitoring-and-retention).

### Draft #63 — recovery-specific signed Mailgun callback and digest reconciliation

[Read the original slice record](history/identity-account-recovery.md#draft-63--recovery-specific-signed-mailgun-callback-and-digest-reconciliation).

### Draft #64 — two independently sourced recovery evidences, challenge-bound grant

[Read the original slice record](history/identity-account-recovery.md#draft-64--two-independently-sourced-recovery-evidences-challenge-bound-grant).

### Draft #65 — logged-in saved recovery-code lifecycle (Identity v25)

[Read the original slice record](history/identity-account-recovery.md#draft-65--logged-in-saved-recovery-code-lifecycle-identity-v25).

### Draft #66 — dedicated, unmounted recovery-code security notice worker (Identity v26)

[Read the original slice record](history/identity-account-recovery.md#draft-66--dedicated-unmounted-recovery-code-security-notice-worker-identity-v26).

### Draft #67 — signed recovery-code notice provider callbacks (Identity v27)

[Read the original slice record](history/identity-account-recovery.md#draft-67--signed-recovery-code-notice-provider-callbacks-identity-v27).

### Draft #68 — independently bound Ed25519 challenge for first recovery-code enrollment (Identity v28)

[Read the original slice record](history/identity-account-recovery.md#draft-68--independently-bound-ed25519-challenge-for-first-recovery-code-enrollment-identity-v28).

### Recovery-code factor revocation fence — stacked Draft #69 / Identity v29

[Read the original slice record](history/identity-account-recovery.md#recovery-code-factor-revocation-fence--stacked-draft-69--identity-v29).

## Passkey registration decision history links

The original anchors below lead to the decisions at their original implementation slices.

### WebAuthn RP / independent binding candidate — stacked Draft #70 / ADR-0033

[Read the original slice record](history/identity-passkey.md#webauthn-rp--independent-binding-candidate--stacked-draft-70--adr-0033).

### Durable WebAuthn registration challenge (v30 / stacked Draft #71)

[Read the original slice record](history/identity-passkey.md#durable-webauthn-registration-challenge-v30--stacked-draft-71).

### Server-trusted WebAuthn registration evidence (v31 / Draft #72)

[Read the original slice record](history/identity-passkey.md#server-trusted-webauthn-registration-evidence-v31--draft-72).

### Actual WebAuthn protocol verification (stacked Draft #73)

[Read the original slice record](history/identity-passkey.md#actual-webauthn-protocol-verification-stacked-draft-73).

### Passkey registration reauth admission (v32 / Draft #74)

[Read the original slice record](history/identity-passkey.md#passkey-registration-reauth-admission-v32--draft-74).

### Atomic pending WebAuthn credential stage (v33 / Draft #75)

[Read the original slice record](history/identity-passkey.md#atomic-pending-webauthn-credential-stage-v33--draft-75).

### Pending passkey proof-of-possession challenge (v34 / Draft #76)

[Read the original slice record](history/identity-passkey.md#pending-passkey-proof-of-possession-challenge-v34--draft-76).

### Signed WebAuthn passkey activation proof (v35 / Draft #77)

[Read the original slice record](history/identity-passkey.md#signed-webauthn-passkey-activation-proof-v35--draft-77).
