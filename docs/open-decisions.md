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

## DEC-ID-RETENTION — Draft, requires privacy/legal/operations approval

Stacked PR #30 proposes 24 hours past email proof expiry before logically clearing the encrypted bearer and 48 hours after anonymous admission window start before HMAC budget deletion. The one-shot role may execute only two fixed no-argument, 100-row bounded SQL functions. No schedule or live credential is provisioned. Required decisions include audit/contact/subject retention, legal holds, operator approvals, key destruction, WAL/PITR/backup/replica lifecycle, backlog alerting and deletion SLAs. Logical erasure cannot prove physical deletion.

## DEC-ID-HTTP-ENROLLMENT — Draft PR #31 (never mounted)

An HTTPS-only, same-origin, bounded-JSON handler has been implemented strictly for isolated tests. Identity registration and proof issuance remain **two separate transactions** and a crash between them leaves an undelivered pending user; a later resend can recover but there is no automatic reconciliation. The handler intentionally collapses accepted/duplicate/missing/throttled and downstream work-failure responses to 202 to limit account enumeration, but **timing differences and silent processing failures remain unapproved**. Configure and test an actual trusted edge/TLS termination, legitimate browser request/CSRF strategy, public-route capacity/quotas, approved KDF and password blocklist, account-independent outage monitoring, recovery and bounce suppression before a live deployment. Do not mount the handler based only on green unit or database tests.

## DEC-ID-RECONCILIATION — Draft PR #32 (not deployed)

The initial recovery policy allows a trusted **one-shot, max-25** verifier-role scan only for UNVERIFIED subjects created 2 minutes–24 hours ago that have **never** had a proof; it does not blindly resend for accounts with expired/revoked proofs. The scan is not an atomic continuation of the registration transaction. A dedicated first-proof-only issuer rechecks absence of any earlier challenge under the existing subject lock and budget rules, so a stale scan cannot rotate an existing token. An exception or concurrent issuer can leave a candidate for a subsequent pass, so a production supervisor must track successful/failed runs and privacy-safe backlog counts.

Operational review is still required for scheduling and operator authentication, global provider send volume, credential/key custody, delivery/bounce/complaint reconciliation, retention/deletion of 24h-unrepaired pending accounts, safe rollback, and alert thresholds. This does not authorize mounting the registration endpoints. For higher assurance, a future migration may make a first-proof job durable in the same registration transaction, with the required role/capability review; do not call the current two-transaction flow atomic.

## DEC-ID-RECONCILE-OPS — Draft PR #33 (no automatic deployment)

An operator-only `--inspect` mode will return capped counts of 2m–24h unissued first proofs and a boolean for older unissued accounts; `--repair-once` requires a separate explicit approval, restricted verifier-role TLS DSN, and externally supplied encrypted delivery keyring. It executes one at-most-25-subject repair, then inspects again. Count-only output uses exit code 2 for actionable/stale backlog, 1 for errors, 0 for a clean point-in-time observation. No subjects/contact aliases or bearer tokens are exposed. **Exit status alone does not deliver an alert.** The operator/infra must independently approve scheduling, authenticated invocations, missed-run detection, alert destination, high-cardinality-safe metrics, stale-account remediation, query-plan/capacity thresholds and secret rotation. This remains an unscheduled private candidate, not proof of production reliability or public onboarding readiness.

## DEC-ID-EMAIL-CONFIRM-HTTP — Draft PR #34 (not mounted)

A proposed first-party browser POST-only email-confirmation endpoint accepts a single canonical 32-byte random base64url bearer, not an email, subject ID, session or role. The server deliberately requires direct TLS and matching configured Origin/Host, bounds JSON at 256 bytes and rejects cookies, Authorization, query/fragment credentials, malformed bodies and duplicate/unknown properties. Existing verifier role and atomic token-consumption operation remain authoritative. A 204 denotes the one successful activation; all invalid/expired/replayed outcomes use one generic 400; 429 is a client budget failure, 503 a gate/peer failure. No automatic login follows contact verification.

**Unresolved before public activation:** supply a real, distributed, privacy-keyed confirmation limiter and global bot/abuse guard (the current constructor accepts only an injected test gate); approve edge TLS termination and forwarding/CSRF policy; implement and test a first-party fragment-consuming page that removes the bearer before any third-party script, redirects or analytics; decide legitimate retry/replay UX and user messaging; investigate proof-usecase 400-vs-503 error classification, on-call alerting, timing characteristics and denial-of-service budget; include endpoint in reviewed OpenAPI/route inventory. No frontend or deployed endpoint is claimed by this PR.

## DEC-ID-CONFIRM-ADMISSION — Draft PR #35 (internal candidate only)

The previously mandatory-but-unimplemented `ConfirmationAdmission` handler port now has a real private Identity implementation that spends two account-independent PostgreSQL admission buckets with action `CONFIRM`, HMAC-SHA256 keying, 8-per-minute and 80-per-hour fixed-window bounds by IPv4 address or IPv6 /64 prefix. Distinct action+domain isolation prevents confirmation traffic from consuming REGISTER/RESEND budgets; the existing retention function purges old HMAC rows. Identity migration v7 changes only the `action` CHECK allowlist, not role grants or public router mounting. Tests cover malformed network peers, synthetic secret errors, IPv6 prefix sharing, exact concurrent cap and HTTP 429 short circuit.

Before a real launch, review and approve (1) global/edge quotas plus DoS load behavior and fixed-window boundary bursts, (2) high-NAT false rejection/retry UX and accessibility, (3) HMAC key sourcing and cross-process rotation (multiple active key versions can split quotas), (4) TLS/forwarding/trusted proxy, (5) safe 400-vs-503 classification on verifier failures and monitoring, and (6) actual first-party token fragment POST flow. **No live rate limiter is wired to the default router and no deployment secret or alert integration is configured.**

## DEC-ID-CONFIRM-FAILURE-CLASSIFICATION — Draft PR #36

Candidate decision: distinguish an **authoritatively invalid proof** from an **unavailable verifying authority** without exposing identifiers or raw errors. After the existing anonymous admission check, `POST /v1/identity/verify-email` returns 204 for successful single-use activation; the identical 400 for invalid, unknown, expired, revoked and replayed proofs; 503 for unexpected SQL/transaction/commit/outbox failures. The repository maps only `pgx.ErrNoRows` on specific proof lookups / conditional challenge consumption to invalid. All other database errors and post-lock update anomalies are unavailable. The usecase and HTTP layers propagate only sanitized error sentinels. Unit and real PostgreSQL integration tests verify wrong-role database denial, outbox rollback/retry, redaction, and unchanged public router inventory.

This **resolves the implementation-level 400-vs-503 ambiguity noted in PR #34/#35** for this candidate, but not the operational question of a response lost *after* a successful commit: a retry may then correctly return 400 despite prior success. Decide the customer success/retry UX and private correlation/metrics, without reopening the bearer token or weakening single use. Still required before public launch: reliable production secrets and verifier credential/IAM, global bot protection, rate/latency load tests, trusted TLS proxy/Origin contract, secure first-party URL-fragment removal, monitoring/on-call alerts, provider/bounce policy and explicit release approval. No endpoint is enabled by this PR.

## DEC-ID-VERIFICATION-PAGE — Draft PR #37 (unmounted candidate)

The email confirmation link's `#token` fragment can be used only by a first-party browser page. A static `GET /verify-email` candidate exists as a separately constructed Go handler. It strips the fragment with `history.replaceState` in the first inline script, **before** any other page code or network request; fails closed if that fails; never loads external resources; requires an explicit button click rather than auto-confirming after GET (to reduce passive mail-scanner consumption); and POSTs the bearer exactly once with `credentials: "omit"`, `redirect: "error"`, no-store and no-referrer to the preexisting confirmation API. A strict response CSP hashes its embedded inline JS/CSS. The page cannot verify a user, issue a session, or read Identity state by itself, and remains **unmounted**.

Open before launch: run real-browser compatibility tests in Chrome/Firefox/Safari, review Mailgun click tracking and email security scanners, approve trusted TLS/Host/origin front door for both page and API, design non-enumerating login/account-status UX for 400/503/lost-response ambiguity, review keyboard and assistive-technology usability, approve global abuse protection and production key/role/metrics policy. A sandboxed Node.js VM test is not a substitute for deployed browser or provider end-to-end evidence. Do not report this page as publicly accessible or ask users to click a live link.

## DEC-ID-BROWSER-E2E — stacked Draft PR #38

The new dedicated CI workflow now exercises real Chromium against a manually wired, local TLS Identity candidate and a disposable PostgreSQL database, following registration → durable encrypted proof → Mailgun local mock → browser GET and manual-click POST → verifier atomic activation. It also simulates **post-commit response loss** (confirmed active once, browser uncertain, replay 400) and an expired proof (remains unverified). Headless Chromium evaluates actual CSP and origin/fetch behavior. To avoid weakening the production Host/Origin policy or binding privileged port 443 in CI, a **test-only origin-preserving HTTPS bridge** forwards browser requests to the ephemeral loopback TLS server. This intentionally **does not validate production TLS/DNS/proxy routing**. The default public/private routers stay unmounted.

Before any public routing or deployment: approve edge DNS/TLS and exact Origin/Host policy, real-browser Firefox/WebKit compatibility and accessibility, safe account status UX after commit ambiguity, active-click scanner behavior, Mailgun suppression/bounce/complaint feedback and delivery SLA, production Argon2id load budget, infrastructure quotas/alerting and HMAC/envelope key rotations. Use separate security review for provider/link click tracking and privacy. CI success alone does not authorize route activation.

## DEC-ID-PASSWORD-AUTHN — stacked Draft PR #39

A candidate **internal, non-HTTP, non-session** password verifier now reads minimum credentials from `mamony_identity_authenticator_runtime` (new v8 column-scoped SELECT-only role) rather than broadening registration or email verifier privileges. A valid syntactic unknown alias runs the same Argon2id v1 work class using a dummy verifier, cannot produce a subject and receives the identical generic denied result as wrong/inactive/unverified accounts. Only a real correct password plus current ACTIVE status and verified email yields an internal subject candidate, **not a user session**. SQL/KDF/role failures are sanitized unavailable. Existing roles cannot access password-verifier material, and authenticator cannot read security outbox, email challenges, admission keys or update state.

Before any public login route: design/admit a durable *pre-KDF* global+per-client+per-alias HMAC rate gate including nonexistent emails, uniform denial/timing response, Argon2id throughput limits, session creation/revalidation that prevents status-check→session-issuance races, anti-credential-stuffing monitoring and privacy, MFA/step-up and session family/revocation architecture. Production role provisioning is external; tests only provision a synthetic role. No login HTTP route or cookie is enabled by this PR.
