# Decisions to resolve before implementation

See [current state](current-state.md) for the merged implementation and verification boundaries. The dated Draft references below preserve decisions at their original slices. For working status, follow the current issue, contract, or ADR.

## Decision lookup by capability

| Capability | Start here |
|---|---|
| Platform and migrations | [Effective DEC-01 and Foundation choices](#foundation-and-access), [proposed DEC-01 amendment](#proposed-dec-01-amendment--foundation-105), [PostgreSQL contract](contracts/postgresql.md) |
| Identity and public access | [Enrollment release decisions](#identity-enrollment-release-decisions), [owning Identity contract](contracts/identity.md), [security architecture decisions](#security-architecture-decisions), [bounded passkey possession decision](#atomic-passkey-possession-verification-v36-critical-issue-78) |
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

## DEC-ID-PASSWORD-AUTHN — stacked Draft PR #39

A candidate **internal, non-HTTP, non-session** password verifier now reads minimum credentials from `mamony_identity_authenticator_runtime` (new v8 column-scoped SELECT-only role) rather than broadening registration or email verifier privileges. A valid syntactic unknown alias runs the same Argon2id v1 work class using a dummy verifier, cannot produce a subject and receives the identical generic denied result as wrong/inactive/unverified accounts. Only a real correct password plus current ACTIVE status and verified email yields an internal subject candidate, **not a user session**. SQL/KDF/role failures are sanitized unavailable. Existing roles cannot access password-verifier material, and authenticator cannot read security outbox, email challenges, admission keys or update state.

Before any public login route: design/admit a durable *pre-KDF* global+per-client+per-alias HMAC rate gate including nonexistent emails, uniform denial/timing response, Argon2id throughput limits, session creation/revalidation that prevents status-check→session-issuance races, anti-credential-stuffing monitoring and privacy, MFA/step-up and session family/revocation architecture. Production role provisioning is external; tests only provision a synthetic role. No login HTTP route or cookie is enabled by this PR.

## DEC-ID-LOGIN-ADMISSION — stacked Draft PR #40

A candidate `PasswordLoginFlow` now enforces global/client/email **durable HMAC-keyed admission before any credential lookup or Argon2id**. Database migration v9 adds only the `LOGIN` action and `GLOBAL_MINUTE` scope to the existing atomic bucket table; the action uses a static five-bucket policy (80/global minute, 8/client minute, 80/client hour, 5/email hour, 20/email day) and the original restricted registration admission DB role. Unknown valid email addresses spend the same admission resources as existing ones, even before the dummy KDF from ADR-0004. Storage, context, or untrusted peer failure prevents the KDF entirely. Candidate limits are **not approved for rollout**.

**Open before any public login:** calibrate capacity/NAT/business traffic, deploy **independent perimeter load-shedding** (denied bucket transactions roll back increments and otherwise allow database DoS), decide global hot-row scaling, trusted client IP/proxy and TLS policy, HMAC secret lifecycle/rotation, monitoring and privacy-preserving credential-stuffing response, public 429/unavailable timing/enumeration contract, accessible retry UX, safe step-up/MFA policy, and atomic session creation with status/verified-contact recheck, expiry and revocation. All candidate routes remain unmounted. See `docs/adr/0005-login-admission-before-argon2id.md`.

## DEC-ID-OPAQUE-SESSIONS — stacked Draft PR #41

An internal candidate session authority now issues **256-bit opaque** tokens only after the #40 pre-KDF admission/password decision **and an atomic DB-side recheck** of ACTIVE status + verified current contact. The session role has EXECUTE on three bounded SECURITY DEFINER functions but no table access; one account status-transition trigger increments session_epoch, preventing freeze/thaw token resurrection. PostgreSQL stores domain-separated digest only; the secret is released only after confirmed commit. DB-clock expiry is 12h absolute, revoke is idempotent with one security event, and bounded retention is maintenance-only after expiry+24h. Public HTTP routes and cookie transport remain disabled.

Security-sensitive limitation: direct compromise of the session DB role could mint tokens for known eligible subjects; lock down infrastructure and provisioning before any routing. Open: real login/session cookie contract (Secure, HttpOnly, SameSite, __Host- prefix, CSRF), session rotation/idle/refresh policy, logout route, device/account/session limits, MFA/step-up, role credential rotation, request-time resource authorization, password/contact-change epoch revocation, monitoring and rate-edge capacity. A status check and session resolution do not authorize financial actions by themselves. See `docs/adr/0006-opaque-session-authority.md`.

## DEC-ID-SESSION-RACE-AND-HTTP — stacked Draft PR #42

The session issuance/freeze race is **now tested with real PostgreSQL interleavings** in `TestIdentityZSessionFreezeIssueInterleavings`: freeze-first denies minting after its commit; issue-first permits a committed digest only before a waiting freeze, which then invalidates it by an epoch bump; explicit rollback leaves no digest or outbox event. Tests use a superuser-only observable database lock barrier against disposable `mamony_test_*` and are enforced by the required PostgreSQL testgate. These are integration evidence for the v10 SQL locking behavior, not public route authorization.

ADR-0007 proposes a first-party **cookie-based** login/session/logout/CSRF HTTP contract, with no handler or route implemented. Proposed cookie attributes: `__Host-mamony_sid`, `Secure`, `HttpOnly`, `SameSite=Strict`, `Path=/`, no Domain, 12h max age, DB expiry authoritative. Proposed logout requires exact Origin, bound CSRF token and DB revoke. Before activation: choose pre-login CSRF, signed/session-bound token derivation and rotation, reverse-proxy TLS model, session-role isolation, public timing/errors/429, edge/NAT quotas, account recovery/credential reset invalidation, finance authorization, and actual browser acceptance tests. A safe lock-interleaving result does not prove protection of later actions when the account changes after session resolution.

## DEC-ID-BROWSER-SESSION-CANDIDATE — stacked Draft PR #43

The proposed ADR-0007 four-operation browser contract has a **candidate but unmounted** Go `BrowserSessionHTTP` implementing same-origin direct-TLS login, cookie-only session read, 15-minute per-session/per-origin HMAC CSRF issuance, and CSRF-protected POST logout with DB revocation and cookie deletion. Duplicate cookies, authorization/forwarded header and Origin confusion, malformed or duplicate JSON, ambiguous commit and stale/expired CSRF are tested with `httptest`; default routers still return 404 and no released OpenAPI operation is added. [ADR-0008](adr/0008-unmounted-browser-session-http-candidate.md) owns design/test evidence.

**Open before launch:** privileged `mamony_identity_session_runtime` may issue tokens for eligible subjects through direct SQL EXECUTE without password proof, so add network/credential isolation and decide whether a stronger binding of password authentication and DB issuance is required. Also build real browser session/cookie/CSRF E2E, configuration/rotation for per-environment 32-byte CSRF key, TLS proxy/Host/Origin topology and security headers, credential stuffing/load/WAF limits, MFA and reauthentication, password/contact-change session epoch invalidation, and financial action-level authorization. Only the reviewer may approve a production route; green CI is insufficient.

## DEC-19 — Session issuer DB-capability abuse (Draft PR #45; PROPOSED)

**Current verified boundary:** Identity v10 grants `mamony_identity_session_runtime` direct EXECUTE on `issue_opaque_session(subject,digest,event)` without an Argon2id or authentication grant prerequisite. Merely knowing a synthetic ACTIVE/verified subject and possessing the DB role can create a session. This is demonstrated in `TestIdentityZDirectSessionMintThreatFixture`; that test intentionally passes while the weakness exists and is not mitigation.

**Proposed G2 decision:** accept a narrowly scoped Profile A objective to defeat compromise of the *session SQL credential alone*, using a restricted, one-time, epoch- and purpose-bound grant created only after the trusted Go password check. The grant writer remains a separate dangerous capability; unless its runtime/credentials are genuinely separated, this does **not** protect against full Identity runtime RCE or administrator compromise. Alternatives, required negative tests, SQL v11 migration/role design and residual risks are in [ADR-0009](adr/0009-authentication-to-session-issuance-boundary.md).

**Not yet accepted/implemented:** no migration v11, new grant role, changed session API, public login, production deployment or independent authentication issuer. G3 must approve role boundaries, short grant TTL and atomic consume semantics; G4 must replace the positive attack demonstration with a real SQL EXECUTE-denial test after the old issuer is removed. Keep Draft and 404 route isolation.

## Draft PR #46 — candidate implementation of DEC-19

Identity migration v11, Go grant staging/consumption, and a dedicated synthetic `mamony_identity_auth_grant_runtime` role are now **code candidates** in Draft PR #46. The old subject-only issuer is dropped; SQL session minting takes a one-use grant reference, not a freely selected subject. The #45 direct-mint fixture is changed into a deny regression and separate tests exercise expiry, replay, rollback and concurrent consume. This does not constitute owner acceptance of DEC-19, evidence of independent runtime isolation, or permission to publish login; see [ADR-0009](adr/0009-authentication-to-session-issuance-boundary.md).

## Draft #47 — server-side session inactivity timeout (proposed, not approved)

Identity v12 introduces a **candidate 30-minute server-side idle timeout** while keeping the v11 12-hour absolute lifetime and grant-issuance security boundary unchanged. A successful session resolution updates `opaque_sessions.last_seen_at` inside the PostgreSQL transaction; expired, revoked, frozen and stale-epoch sessions cannot refresh or resurrect. Backfilling `last_seen_at=issued_at` intentionally causes old sessions that have already been inactive for 30 minutes to fail closed on migration. A new required disposable-PostgreSQL integration test covers refresh, idle denial and absolute expiry. See [ADR-0010](adr/0010-server-side-session-idle-expiry.md).

**Unresolved:** owner acceptance of 30-minute policy, traffic/load and subject-lock contention, compromised-workload isolation under DEC-19, browser UX, key lifecycle, MFA/recovery and financial resource/tenant authorization. A `GET` used as a heartbeat would count as server activity, but cannot exceed the absolute 12-hour cutoff. The candidate browser routes remain 404 in application routers; this PR cannot be merged or deployed solely from green CI.

## Draft #48 — per-subject five-active-session cap (proposed)

Identity migration v13 proposes **at most five currently usable browser sessions per eligible subject** as an atomic grant-consume/session-issue rule. The sixth valid grant-bound login revokes the oldest currently live digest (created time, digest tie-breaker) and inserts a separate `SESSION_REVOKED` outbox record in the same transaction as its replacement session and `SESSION_CREATED` evidence. Sessions expired by DB-clock idle/absolute rules, already revoked, or from a previous account epoch do not occupy a slot. Existing legacy accounts with **more than five live sessions** must be reviewed: further issuance fails closed rather than performing an unbounded bulk revocation. See [ADR-0011](adr/0011-atomic-five-session-cap.md).

**Open:** owner approval of five-session policy and old-device eviction UX, login/resolve/logout subject-lock contention and throughput, operational over-cap remediation, production workload/SQL role credential isolation, password/contact-change revocation, MFA/step-up, remote device management, and financial action-level authorization. This does not protect against compromise of the full Profile A Identity process and does not mount public routes.

## Draft #49 — credential/contact epoch invalidation and stale-KDF grant fence

Identity v14 adds database triggers that increment `subjects.session_epoch` transactionally on real password-verifier updates/deletion and email-alias changes/deletion; an exact no-op verifier update does not invalidate a session. Password-verifier lookup also reads `session_epoch` **in the same SQL snapshot** as the Argon2id verifier. A successful password comparison can stage a one-time grant only if the epoch is still equal to that captured before the KDF; the unversioned SQL grant function is dropped. An in-flight old password proof is rejected even if the account remains ACTIVE and the new verifier differs. See [ADR-0012](adr/0012-auth-material-epoch-grant-fence.md).

**Unresolved:** This does not create an authorized password/email mutation endpoint or a notification event, protect against compromised privileged grant writer, implement recovery/MFA/reauthentication, nor prove workload isolation. Future mutation transactions must lock subject first and supply mandatory audit/notification evidence; direct test-admin SQL updates are synthetic. Owner approval, UI and production load remain outstanding. Keep 404 public route isolation and Draft stack.

## Draft #50 — authenticated internal password change, purpose-bound grant

Proposed v15 introduces a separate **PASSWORD_CHANGE** one-use authentication grant and **mamony_identity_password_change_runtime** SQL role. The trusted Go usecase requires an active opaque session, pre-KDF admission and real old-password Argon2id comparison for that same subject, then mandatory new-password policy/blocklist, fresh salt/hash and a short-lived epoch-bound change grant. Its single SQL transaction checks current session, purpose/epoch and status after locks, rotates the verifier, increments epoch through v14 trigger and inserts one identifier-only `PASSWORD_CHANGED` outbox event. The old bearer and any prior grants are invalid immediately after commit. See [ADR-0013](adr/0013-authenticated-password-change.md).

**Open:** owner approval of allowed password-change proof factors and old-session UI handling; real production compromised-password blocklist; MFA/recovery/step-up and password history; CSRF/Origin/TLS for a future unmounted route; outbox delivery and user notification; lock/contention measurements; secret/role workload isolation; and financial authorization. None are fulfilled by passing CI. No merge or route activation.

## Draft #51 — durable password-change notification queue (no live send)

Proposed v16 atomically enqueues an email **obligation** from trusted `PASSWORD_CHANGED` events in a separate queue, with original DB session-user provenance to block verifier-role event forgery. A restricted worker credential can only claim/finalize leased notices, with five bounded attempts, recipient-alias recheck, 24h eligibility, and seven-day PII erasure. The identifier-only audit row survives. Existing mail verification delivery is not reused for password notices, preventing token-bearing challenge/ACTIVE-account mixups. See [ADR-0014](adr/0014-password-change-security-notices.md).

**Still open:** actual provider transport (the new SecurityNoticeSender is mock-only), opt-in standalone process/bootstrap, sending-domain policy, bounces/complaints and provider retry-after, delivery monitoring and escalation for stale contact/dead letters, backup/WAL erasure, suppression policy, PKI/secret/workload isolation, and owner approval. Passing SQL and Go CI does not mean any email is sent. No merge/public activation.

## Draft #52 — token-free Mailgun notice sender and explicit worker

ADR-0015 adds a region-pinned Mailgun sender for `PASSWORD_CHANGED` notices, separate from encrypted verification tokens, and a standalone opt-in worker with owner-only credential files and `mamony_identity_security_notice_runtime` SQL role. A real PostgreSQL + local HTTP mock test proves retry from 429 to 200 and correct event audit retention. Green CI does **not** claim a user received an email: the binary has not been deployed, enabled or given Mailgun credentials.

**Open:** verified Mailgun sending domain and regional DPA, SPF/DKIM/DMARC, dedicated domain sending key and rotation, operational worker deployment, provider bounce/complaint/receipt webhook authenticity, monitored DEAD or suppressed notice escalation, provider quotas, production PostgreSQL TLS/load, network egress and secret isolation, no-link warning UX wording review, owner release approval, MFA/recovery and protected user-facing password-change route. See [ADR-0015](adr/0015-mailgun-password-change-notice-worker.md). Keep Draft; no public endpoint activation.

## Draft #53 — provider response correlation, signed webhook receipt and operator health

Proposed Identity v17 saves **only SHA-256 digests** of normalized Mailgun message IDs, signing tokens and event identities. The unmounted receiver validates Mailgun webhook HMAC with its **distinct** signing key, validates origin domain and 9h replay window, deduplicates receipts, and reconciles callback-before-sender-ack races. A separate webhook SQL role has one EXECUTE-only receipt function, and maintenance can read PII-free counts for due/dead/not-confirmed/SMTP-delivered/provider-failed/unmatched notices. No sender HTTP route or production webhook route was activated.

**Open:** production HTTPS webhook ingress and trusted network path, signing key loading and rotation, Mailgun account/subaccount domain verification, anti-abuse and monitoring, bounce/complaint suppression, provider API reconciliation when webhooks are missed, SLA alert thresholds/on-call ownership, exactly-once limitations, legacy ACCEPTED records without message-id, ambiguous send responses, provider security/privacy review and production permission approvals. See [ADR-0016](adr/0016-mailgun-security-notice-webhook-reconciliation.md). Do not merge, launch external services or enable public routes.

## Draft #54 — count-only monitoring with a dedicated read-only credential

Identity v18 adds a separate `mamony_identity_notice_monitor_runtime` role (schema USAGE + EXECUTE aggregate only; no purge, notice delivery, webhook writes or direct PII queries) and an explicit one-shot `mamony-identity-notice-health` CLI. Mandatory per-metric thresholds produce a redacted JSON `OK`/exit 0, `ALERT`/exit 2 or `ERROR`/exit 1. CI exercises real role-separated PostgreSQL and deterministic synthetic threshold tests. This does **not** activate monitoring, cron, paging, inbound/outbound Mailgun, recovery or public Identity HTTP.

**Open:** operator-approved thresholds and initial baseline, false-positive policy for due notices, scheduler/orchestration, monitoring sink, on-call ownership/escalation and incident response, secret rotation/least-privilege deployment, reconciliation of missed Mailgun events, and public ingress release approval. See [ADR-0017](adr/0017-read-only-notice-health-probe.md).

## Draft #55 — first-party browser password-change candidate (unmounted)

An isolated handler now exercises the existing `AuthenticatedPasswordChange` operation behind exact same-origin HTTPS browser requirements, session-bound HMAC CSRF, a strict 4096-byte JSON body and canonical alias. The form cannot choose a subject: the active cookie's owner must match the old-password's authenticated subject/epoch. Certain commits clear the browser cookie and require a separate login; denied password/policy/quota outcomes use the same generic HTTP response, and ambiguous SQL commits cannot claim success. No public/private router registration, migration or deployed listener was changed. See [ADR-0018](adr/0018-first-party-password-change-http-candidate.md).

**Unresolved before enabling:** real Chromium + disposable PostgreSQL end-to-end change-password test (including lost response), owner UX/consent and lockout guidance, account-specific password policy feedback without enumeration, approved breached-password blocklist, independent client/edge abuse controls, MFA/step-up/recovery, browser/CSRF secret rotation, production ingress TLS/proxy and workload isolation, notification deliverability and operational escalation, and financial action authorization. Keep the PR Draft/unmerged.

## Draft #56 — real browser/password-change atomicity and lost response

A disposable `httptest.NewTLSServer` + Playwright Chromium + restricted PostgreSQL role fixture now covers the **unmounted** password-change HTTP candidate end-to-end. Both successful and dropped-after-commit responses are exercised; a lost response cannot restore an epoch-stale cookie or make an old password valid. SQL evidence checks precisely one `PASSWORD_CHANGED` event and event-linked pending notice per synthetic subject. The required real-browser workflow now includes the new test alongside existing email verification and session E2E. See [ADR-0019](adr/0019-browser-password-change-postgres-e2e.md).

**Still open:** real production ingress/proxy/TLS validation; a production frontend with clear uncertain-outcome and session-logout messaging; safe lockout/recovery UX, breached-password policy, MFA/step-up, deployment role and key isolation, live Mailgun sender/webhook/alerting, monitoring/SLA, financial resource authorization and owner approval. Browser regression and disposable PostgreSQL tests are not permission to mount the endpoint or merge the draft stack.

## Draft #57 — one-shot browser password-change UX (unmounted)

A first-party, strict-CSP HTML/JavaScript candidate now renders a single-use change-password form only after verifying a live session and obtaining a session-bound CSRF. It clears DOM password inputs and locks the form after one mutation. HTTP 204 indicates a confirmed change; generic 401 indicates a denied change; response loss/503/other results never trigger an automatic retry and instead show an explicit **unknown result**. Browser/SQL E2E must prove denial and lost-after-commit behavior. See [ADR-0020](adr/0020-unmounted-password-change-recovery-page.md).

**Still unresolved:** officially owned login/recovery/support routes, user-facing security copy/localization/accessibility approval, MFA and step-up, production password blocklist, full operational identity role/ingress/CSRF key isolation, real notice deliverability, and release orchestration. None of the page or password-change routes is installed in the default public/private listeners. Keep every stacked PR draft and unmerged.

## Draft #58 — forgot-password recovery proof and threat model (no reset capability)

ADR-0021 introduces only a 256-bit CSPRNG recovery token primitive, strict base64url parsing, domain-separated SHA-256 digest and defensive redaction for ordinary logs/serialization. Password recovery will require **different authorization** from v15's session+old-password `PASSWORD_CHANGE` grant. It must have dedicated pre-lookup admission, uniform external responses for known/unknown/suppressed accounts, a verified-contact and epoch fence, expiry/attempt/replay controls, encrypted proof delivery, an atomic single-use verifier/session-epoch rotation, and a distinct provenance-checked `PASSWORD_RECOVERED` security event and notification.

**Unresolved before the next functional slice:** acceptable self-service recovery assurance (including MFA account classes), verified recovery channels and contact-change handling, SQL function/role/issuance tables, separate notification and delivery worker, request limits and retention, recovery browser link/fragment policy and race/commit-loss tests. No recovery endpoint, email, reset operation, DB migration or production route exists in this PR. Keep the full draft stack unmerged. See [ADR-0021](adr/0021-password-recovery-authority-foundation.md).

## Draft #59 — internal recovery issuance with a sealed, non-deliverable queue

Identity v19 introduces action-isolated HMAC-keyed `RECOVERY` admission and an EXECUTE-only issuer role. The trusted Go usecase charges the anonymous budget before generating and sealing the recovery bearer for **all** admitted syntactically valid aliases; it does not disclose account eligibility. The PostgreSQL owner checks current verified ACTIVE contact, captures subject/contact epoch, applies subject-level 60s/3h/5day issuance quotas and atomically stores a 15-minute single-live challenge digest plus sealed PENDING delivery. A late transaction failure preserves the previous valid challenge. No existing worker has authority to claim the new queue. See [ADR-0022](adr/0022-password-recovery-issuance-outbox.md).

**Open:** independently reviewed production AEAD sealer and keys; dedicated delivery claim/finalize and provider; encrypted-envelope retirement and PII/backup erasure; strong account MFA/recovery assurance; reset-token one-use consume SQL transaction, status/contact/epoch recheck and per-token attempts; full-session invalidation plus a distinctly authorized `PASSWORD_RECOVERED` event/notice; real public anti-enumeration timing controls and perimeter abuse gate; incident recovery escalation. No public recovery URL or password reset is ready. Keep Draft/unmerged.

## Draft #60 — encrypted password recovery delivery (not launched)

Identity v20 adds a dedicated EXECUTE-only recovery delivery runtime with `SKIP LOCKED` 45s claim leases, subject/current-verified-contact/epoch/expiry recheck, quarantine of stale encrypted jobs and five-attempt bounded finalization. `RecoveryKeyring` uses a **distinct recovery-purpose AES-GCM AAD and keyring**, and `RecoveryMailSender` builds only a configured HTTPS reset-link fragment. Unit, real PostgreSQL and local Mailgun HTTP mock tests cover 429 → 200 retry, stable proof digest, no plaintext SQL and no password/session mutation. No live worker CLI, server boot hook, public endpoint, landing page, email or reset has been activated. See [ADR-0023](adr/0023-password-recovery-delivery-worker.md).

**Outstanding:** deployable but independently approved encryption/PG/provider credential loading and key rotation; reconciliation of duplicate/stale/failed Mailgun submits, verified contact changes between lease and external send, bounce suppression, scheduled monitoring and expired ciphertext erasure; user-owned MFA/assurance and recovery-channel policy, recovery page and atomic one-use reset + session epoch invalidation, separate `PASSWORD_RECOVERED` audit/notice. Passing CI is not authorization to send links to non-existent landing pages.

## Draft #61 — unmounted, assurance-gated recovery redemption

Identity v21 provides one separate `redeem_password_recovery` SQL function (EXECUTE-only role), with canonical email/token equality, DB-time expiry, subject/contact/epoch locking, one-time consumption of a 15-minute challenge **and a distinct default-DENY assurance approval**, Argon2id v1 credential rotation, session-epoch invalidation and provenance-checked token-free `PASSWORD_RECOVERED` event/notice all in one atomic transaction. No ordinary runtime role can mint an assurance grant, and no public HTTP route or live email is enabled. Race/replay and post-commit-uncertain outcomes cannot be safely retried automatically. See [ADR-0024](adr/0024-atomic-password-recovery-redemption.md).

**Still required before production:** independent assurance/MFA authority and approval method for recovery grants; signed recovery proof page with fragment scrubbing and no external assets; strict public anti-enumeration timing and global abuse; provider-independent security notice worker and operational alerting; per-attempt retention/abuse metrics, old-password reuse prevention; key/role deployment, verified contact changes after mail claim, breach checking and live KDF capacity. Keep PR stack Draft/unmerged.

## Draft #62 — token-free PASSWORD_RECOVERED alert worker, monitoring and retention

Identity v22 introduces an independent, unlaunched, EXECUTE-only recovery alert sender, current-recipient/freshness recheck, 45-second lease, bounded 1/2/4/8-minute retries, and a purpose-specific fixed Mailgun text message with **no token, link or password**. Mailgun acknowledgements persist a SHA-256 message-ID digest, not raw provider IDs or delivered state. A separate count-only monitoring credential reads five bounded counters; maintenance-only purge erases old recipient-bearing alert queue records without deleting immutable `security_outbox` events. See [ADR-0025](adr/0025-password-recovery-security-notice-worker.md).

Before launch: independent high-assurance approval authority/MFA; signed recovery-notice webhook reconciliation to distinguish submitted from inbox delivery, provider bounce/suppression/on-call alerts, retry/retention operational review, first-party recovery page and HTTP API, and production wiring/secrets. **PR remains Draft/unmerged; no real email is sent.**

## Draft #63 — recovery-specific signed Mailgun callback and digest reconciliation

Identity v23 isolates signed `PASSWORD_RECOVERED` Mailgun delivery receipts in a dedicated EXECUTE-only webhook role and new digest-only, replay-deduplicated table, separate from v17 `PASSWORD_CHANGED` receipts. The unmounted recovery handler uses verified HMAC-SHA256 signing keys, bounded event payloads, timestamp and configured-domain checks, never persists raw provider bodies or PII, handles callback-before-ack, and monotonically escalates bounce, complaint and unsubscribe without being overwritten by late delivered events. Monitoring reads only counts for delivered, provider failures and unmatched signed receipts; 7-day receipt cleanup does not erase audit. See [ADR-0026](adr/0026-signed-recovery-notice-webhook.md).

**Release blockers remain:** production ingress with TLS, trusted independent webhook signing key distribution and rotation, account-level Mailgun event configuration, operational on-call alerts/suppression, recipient erasure/backups, **independent v21 MFA/assurance grant issuance**, recovery UI/HTTP and real provider launch. No route, process or reset authority has been mounted. All stacked PRs stay Draft/unmerged.

## Draft #64 — two independently sourced recovery evidences, challenge-bound grant

Identity v24 uses the existing 15-minute email challenge **plus one previously enrolled 256-bit saved offline recovery code** to create an atomically audited, single-use assurance grant no longer than five minutes and bound to that exact challenge. Approval and password rotation use **different SQL roles** with EXECUTE-only functions; an invalid/missing/replayed code, changed contact/subject epoch, or an approval for another recovery challenge must never authorize a reset. Existing unbound v21 approvals cause a deliberate fail-closed upgrade gate requiring operator review. See [ADR-0027](adr/0027-offline-recovery-code-assurance.md).

**Still not launched:** secure high-assurance code enrollment/reissuance/revocation UX, proof of device/channel independence, recovery AAL/MFA policy, account notification when backup codes change, per-attempt abuse/admission budgets for any future HTTP handler, incident responses and independent protected verifier trust roots. Real users cannot enroll a code through any normal runtime role. All stacked PRs remain Draft/unmerged.

## Draft #65 — logged-in saved recovery-code lifecycle (Identity v25)

The internal v25 candidate handles a **five-code, 256-bit saved offline recovery set** with current opaque session, durable-admission current-password Argon2id proof and distinct short-lived `RECOVERY_CODE_ROTATION` grant. **First enrollment ALSO requires a separately issued one-time independent approval that no production runtime can currently mint**: session/password by themselves cannot bootstrap account-recovery credentials. Rotation requires one unused older offline code and burns the prior set. A single SQL transaction checks current subject/contact/epoch/session/grant, writes digest-only new codes, revokes outstanding emailed password recovery proofs and approvals, and creates identifier-only audit plus a **separate, token-free, unlaunched notification obligation**. See [ADR-0028](adr/0028-recovery-code-enrollment-rotation.md).

**Still pending and blocking real users:** independent MFA/AAL enrollment ceremony and approval issuer, secure trusted one-time code display/reissue and lost-all-codes policy, rate-limited first-party HTTP with rigorous session+CSRF/Origin binding, recovery-code notice delivery and signed provider reconciliation, retention/operational monitoring, Profile A privileged-owner risk review and production rollout signoff. No public recovery or code provisioning is enabled and all stacked PRs remain Draft/unmerged.

## Draft #66 — dedicated, unmounted recovery-code security notice worker (Identity v26)

The v25 code-set change queue now has a separate sender-only SQL role, isolated Mailgun created/replaced templates, `SKIP LOCKED` claim, 45-second lease, max-five-attempt retry, current verified-recipient/provenance recheck, bounded fixed-body provider request, and SHA-256 Message-ID storage. A different count-only monitor sees pending/dead/submitted-unconfirmed/retrying/expired metrics, not identities; maintenance alone can erase seven-day-old recipient-bearing rows while preserving security audit. See [ADR-0029](adr/0029-recovery-code-security-notice-worker.md).

**Still default-DENY and unlaunched:** independent initial enrollment/MFA approval issuer and AAL policy; secure one-time backup-code display; live sender supervision/keys, signed Mailgun provider webhook reconciliation for this new queue, complaints/suppression/on-call routing and retention signoff; public authenticated HTTP, abuse control, secure recovery UI and platform review. A provider ACK is not mailbox delivery. All stacked PRs remain Draft/unmerged.

## Draft #67 — signed recovery-code notice provider callbacks (Identity v27)

A new independent unmounted Mailgun webhook verifies HMAC/timestamp/domain and records only SHA-256 token, event and normalized Message-ID digests, provider state/time, using an EXECUTE-only v27 webhook SQL principal. The v26 sender can correlate a valid callback that arrived **before** provider ACK; later callbacks reconcile only code-notice receipts. Replay and out-of-order events do not overstate delivery: delivered, permanent failure, complaint and unsubscribe are monotonic. The v26 five-column monitor remains compatible; a separate count-only provider-health function reports delivered/failure/unmatched statuses. Seven-day digest retention is maintenance-only. See [ADR-0030](adr/0030-recovery-code-notice-signed-webhook.md).

**Unresolved before release:** independent MFA/assurance bootstrap issuer; secure one-time code display/rotation; provider webhook key custody and live ingress, bounced-address suppression and incident routing, alert thresholds, end-user abuse/rate-limited recovery HTTP, operational replay windows, privileged database-owner risk, human release signoff. Provider status is not proof of human mailbox access. Nothing in the stack is merged or deployed.

## Draft #68 — independently bound Ed25519 challenge for first recovery-code enrollment (Identity v28)

The candidate v28 foundation adds independent proof *only for accounts with public keys already securely provisioned outside the application*. Its two EXECUTE-only issuer/verifier roles cannot register a public key, and neither can perform the other's SQL capabilities. A trusted Go verifier checks an Ed25519 signature over a 256-bit nonce + subject + epoch + challenge ID + purpose, then persists one-use, five-minute v25 approval and identifier-only audit. Reauthentication and session gates remain mandatory, signature failures count toward a per-challenge limit, and previous recovery-code enrollment cannot be repeated. See [ADR-0031](adr/0031-independent-factor-signed-enrollment.md).

**Highest-priority release blockers:** actual independently trusted authenticator registration and revocation ceremony (a compromised login/session must not self-provision keys), preferably WebAuthn with RP origin and user verification policy; attestation/platform assumptions and AAL threat review; browser session+CSRF/Origin/abuse controls; secure user consent, one-time code display, lost-device and lost-all-codes UX; role/key custody, production rollback and incident routing. No public routes or production factor enrollment are enabled. The stack remains Draft/unmerged.

### Recovery-code factor revocation fence — stacked Draft #69 / Identity v29

Identity v29 closes the sign/approval-to-enrollment gap: the approval now carries the verified signer public key; a later v25 bootstrap commit locks the active key and refuses revoked, changed or epoch/contact-stale bindings. Legacy NULL-key approvals are invalid. Existing code rotation is unaffected. This is a SQL commit-time containment guard only; signed approval does not establish ongoing independent user consent. No external endpoint or SQL permission expansion. See [ADR-0032](adr/0032-recovery-factor-revocation-fence.md).

**Still required:** genuinely independent authenticator registration and revocation authority (without password/session-only self-binding), loss/compromise response for code sets already enrolled, trusted consent and display, user verification/RP-origin-aware WebAuthn if selected, key provenance/operational approval, rate-limited CSRF/Origin-bound HTTP, signed provider delivery and monitoring, and Profile-A owner compromise review. Keep all stacked PRs Draft/unmerged.

### WebAuthn RP / independent binding candidate — stacked Draft #70 / ADR-0033

The first passkey RP config is pinned to exact `https://<rp-id>`, deliberately denying subdomains, wildcards, Related Origin Requests, localhost/IP, explicit ports and origin manipulation. Strict runtime config and HTTP 404 tests are **not** a WebAuthn registration/verifier implementation. Choose a maintained verifier library, a domain-ownership and origin deployment policy, durable challenge/session/epoch binding, UV and authenticator/attestation policy, key provenance and independent registration approval, code revocation vs compromise controls, public UX and real-browser plus PostgreSQL adversarial evidence. A fresh password and bearer session cannot self-assert higher assurance.

**Release:** no WebAuthn credential storage/registration, key self-provisioning, passkey login, AAL claim or public route is enabled. The existing v28/v29 signer still requires prior secure provisioning. Keep all PRs Draft/unmerged. See [ADR-0033](adr/0033-webauthn-relying-party-pre-enrollment.md).

### Durable WebAuthn registration challenge (v30 / stacked Draft #71)

ADR-0034 stages exactly one 3-minute pending passkey **registration intent** per verified active subject, coupled to a live server-side session, subject epoch, contact, and pinned RP config. New dedicated issuer SQL principal has only EXECUTE permission; a second concurrent Begin, stale/unknown session, expired or revoked session and epoch mismatch are denied. Go returns no challenge if database issuance is ambiguous. The **finish function and credential authority do not exist**; a pending challenge is not proof of WebAuthn identity, UV or trusted independent factor. Required next: maintained verifier library and protocol evidence, separate binding admission/provenance (never session/password-only independent recovery bootstrap), one-time transactional consume, audit + mandatory notice, browser CSRF/Origin and cross-account/replay/UV-negative tests. No public deployment or merger authorization.

### Server-trusted WebAuthn registration evidence (v31 / Draft #72)

A new separately scoped role can only read the 3-minute v30 nonce for the original live session, verified subject/contact/epoch and exact intent. It cannot issue, consume, edit or bind any credential. The Go wrapper checks current RP configuration and purpose generation. A wrong/stale/expired/cross-session proof has no expected challenge, but a **valid read is NOT authenticator proof** or approval. The maintained library candidate is go-webauthn v0.18.2, with actual verification, attestation trust policy, UV, credential provenance, independent registration admission, atomic finish/audit/notice and browser exposure still unresolved. Keep all PRs Draft/unmerged and public routes unmounted. See [ADR-0035](adr/0035-webauthn-registration-evidence-reader.md).

### Actual WebAuthn protocol verification (stacked Draft #73)

The v31 source-of-truth nonce is now connected to a pinned maintained library adapter (go-webauthn v0.18.2) that validates the complete creation response, exact origin/RP hash and required UP/UV; synthetic `fmt:none` ES256 positive/negative fixtures enforce tampering rejection. **Verification has zero credential or factor binding authority**: no public route, no passkey table, no challenge consume, no registration grant. Open gates: exact creation-options/SessionData congruence, attestation trust policy, independent fresh reauthentication/consent provenance, one-time transactionally fenced finish including credential global uniqueness, mandatory security audit/notice, browser protections and release operational controls. See [ADR-0036](adr/0036-webauthn-registration-protocol-verifier.md).

### Passkey registration reauth admission (v32 / Draft #74)

Password-backed **ordinary account passkey registration** now has an internal durable admission primitive: a fresh current-password verification using shared pre-KDF login budget creates a single-purpose 30s grant; a distinct SQL role consumes it for exactly the original live session and one nonexpired current v30 challenge. Admission persists in a separately keyed table with immutable `PASSWORD_REAUTH_ONLY` provenance and cannot be silently inherited by a superseding challenge. It is **not** authorization to bind a credential or grant independent recovery-code bootstrap eligibility. Remaining required decisions: higher-assurance prior factor/step-up and explicit consent, creation-options congruence, a transactional credential finish (challenge+admission one-use fences, uniqueness, notices), compromise hold, browser Origin/CSRF and staged deployment. See [ADR-0037](adr/0037-passkey-registration-password-reauth-admission.md).

### Atomic pending WebAuthn credential stage (v33 / Draft #75)

A new internal staging transaction can consume a v30 nonce plus v32 password reauth admission exactly once against the **same v31 nonce used for maintained WebAuthn protocol validation**, store a globally unique credential ID/COSE key as **PENDING_ASSERTION**, and atomically write an audit event plus a PENDING security-notice row. It does **not** establish a usable login credential, private-key possession, trusted attestation, independently approved recovery factor or delivered email. Hard gates remain: an independently challenged and signature-verified activation assertion, prior device or high-assurance step-up policy, full Begin/Finish option congruence, notice sender+monitoring+retention, threat-tested browser CSRF/Origin protections and staged launch. See [ADR-0038](adr/0038-webauthn-atomic-provisional-registration.md).

### Pending passkey proof-of-possession challenge (v34 / Draft #76)

The internal v34 challenge issuer creates a unique 3-minute (and no later than 10 minutes post-staging) activation assertion nonce only for an original live v33 PENDING credential tied to the exact registration session, spent v30 nonce and spent v32 password-backed admission; a distinct reader returns the public COSE credential and flags after checking the same fences. Only one active challenge per subject is allowed. The stored credential **remains nonactive and cannot log in**: next reviews must implement a real signed `webauthn.get` assertion verification using retained SessionData, a one-use SQL activation transaction, counter/backup policy, higher-assurance approval, browser protections, rate limiting, and operational notice delivery. See [ADR-0039](adr/0039-passkey-activation-challenge-evidence.md).

### Signed WebAuthn passkey activation proof (v35 / Draft #77)

V35 verifies a **real** ES256-signed `webauthn.get` assertion against the v34 original session-bound nonce and DB-stored COSE key with maintained go-webauthn v0.18.2; challenge/type/origin/RP hash/UP+UV, exact credential/user handle, backup flags, and explicit clone/nonprogressing-counter handling are checked. Fixture tests include real cryptography and negative cases; real PostgreSQL integration uses the same v34 EXECUTE-only reader. **The read-only result is replayable while the challenge remains unconsumed and is NOT an authorization decision.** Next review must implement independently fenced v36 atomic one-time activation, signed-evidence/counter binding, user consent/risk policy and a real security-notice sender. Credential remains `PENDING_ASSERTION`. See [ADR-0040](adr/0040-passkey-activation-signed-assertion-verifier.md).

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
