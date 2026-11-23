# Identity email delivery — runbook and security boundary

**Status:** internal proposed infrastructure (stacked Draft PRs #25–#30); a Mailgun HTTP API adapter and an opt-in worker command exist, but deployment secrets are not connected, the process is not deployed, and all HTTP integration tests use local mocks. No public Identity endpoint or real outbound email is configured. No real delivery claim may be made.

## Ownership and secrets

- The registration connection inserts a pending subject and identifier-only obligation. It cannot mark an account verified.
- The verifier connection atomically issues or consumes a challenge. At issuance it must persist the challenge, a **separate AES-256-GCM envelope**, and the event obligation in one transaction. Only a SHA-256 digest of the bearer is used for token validation; an encrypted bearer is retained solely for bounded email delivery.
- The **delivery** DB credential may read narrowly scoped challenge/contact state and encrypted jobs, claim a job, and update delivery status. It cannot read Argon2id verifiers, change contact/subject state, issue new challenges, or alter migration history.
- Inject an active 256-bit key and eligible read-only old keys from a trusted external secret source (KMS/secret manager decision still pending). The application has no baked-in key, no default secret, and no environment variable that automatically activates a worker. Production key owners and role grants need review.
- Rotate keys only while old key IDs remain decryptable until outstanding jobs are processed/expired; reject unknown key IDs, bad AAD, invalid nonce/tag and corrupt ciphertext without delivering a token. Never write bearer, ciphertext plaintext, key material, provider errors or signed URL to audit/logs.

## Retry, crash and provider semantics

1. The verifier inserts the delivery row together with the challenge/event. Failure anywhere rolls back the entire issuance. The event records an **obligation**, not a send.
2. The worker, running with a separate DB role, claims a due job using `FOR UPDATE SKIP LOCKED`, a random `lse_` fencing ID, and a 45-second lease. It skips expired, consumed, revoked, already-active or contact-mismatched proofs.
3. The worker decrypts the envelope in memory, calls a future EmailSender with a 10-second timeout and stable challenge ID as the idempotency key, then conditionally marks `ACCEPTED`, reschedules `TEMPORARY`, or marks `DEAD`. No provider error text is persisted or exposed.
4. Temporary failures back off 1, 2, 4 and 8 minutes; the fifth attempt is terminal. A process death releases work after the lease expires. A worker with an expired/stolen lease cannot finalize another worker's claim.
5. A provider can accept a message and time out before the client records that acceptance. Reclaim/retry may send a duplicate. **Exactly once is not guaranteed**, and provider idempotency cannot be assumed. Review a real provider's capabilities before declaring deduplication. `ACCEPTED` is submission acknowledgement, not inbox delivery/read proof.
6. If a newer proof was issued during processing, the older token is invalid at the verifier even if a delayed email is sent. Delivery may be confusing but cannot activate a user via a stale token.
7. Use separate periodic cleanup to erase encrypted envelopes and old contact records according to an approved privacy/retention policy. The new one-shot retention tool remains unconfigured and undeployed.

## Preflight and verification

Use only the disposable loopback PostgreSQL fixture:

```sh
make check
make race
make integration-local
```

CI specifically requires `TestPostgresIdentityDeliveryRole` and `TestIdentityProofDeliveryOutbox` in addition to earlier platform, registration and proof tests. `make check` verifies migration fingerprints but not running PostgreSQL roles; a green Go unit test alone cannot establish durable delivery.

Before public enablement, resolve and test: actual sender provider and domain authentication; key lifecycle/isolated access; email template and HTTPS origin allowlist; per-subject and per-IP rate budgets (including non-existent accounts); request timeout, bounce/complaint feedback and provider quotas; time drift and expiry; message suppression on reissue; privacy and deletion; end-to-end HTTP error normalization, Origin/CSRF; verified-account session and object authorization.

## Deployment sequencing and rollback

Apply immutable Identity v1 through **v7**, with the offline `mamony-identity-migrate` binary and a dedicated migrator credential. Provision the separate nonprivileged delivery role before v3. The v3 verifier implementation fails closed if the encrypted delivery table is missing; do not deploy the changed verifier against a v2-only database. Keep the worker paused until both the persisted envelope and external key source are verified. A code rollback must account for queue rows produced by the new version, and must not re-enable public endpoints.

## Mailgun adapter (stacked Draft PR #26) — mock-first contract

Mailgun supports explicit US/EU/AU data regions. The adapter uses only the official `POST /v3/{domain}/messages` host for the configured region, **HTTP Basic Auth** (`api` + a purpose-limited Domain Sending Key), and documented `multipart/form-data` fields. Production code does not accept a caller-supplied provider endpoint; HTTP tests inject a custom local mock transport without DNS/network access to Mailgun.

Required deployment inputs (all mandatory, no defaults): region (`US`, `EU`, `AU`), verified sending domain, a `From` address on that exact domain, Domain Sending Key from a trusted secret source, and a verified HTTPS origin for the future frontend's `/verify-email` page. The secret manager and actual runner remain **unwired**. Sender configuration is validated before a request; no keys, response bodies or verification tokens may be returned in errors.

The plain-text message contains a link with a fragment `#token=...`; browser fragments are not sent with an ordinary GET, but the future frontend must remove the fragment from browser history and explicitly POST the token to a protected verification endpoint. Treat the fragment as a bearer secret nonetheless: no analytics, click redirects, URL logging, link previews, support screenshots or client-side third-party scripts on this page. Until that frontend exists, this contract is internal only.

Mailgun request options disable tracking opens/clicks and require TLS. `o:deliver-within=5m` limits Mailgun's own retry window but **does not guarantee arrival before the challenge's 20-minute expiry**. Mailgun's Messages API does **not** provide generic exactly-once idempotency for normal sends. The separate `o:inbox-placement-test-key` relates to inbox-placement testing, not deduplication of user email. A retried ambiguous send may arrive twice; the token stays one-use.

Tests: `sender_test.go` runs request/auth/region/config/validation/429/420/408/5xx/4xx/timeouts/redirect tests against an `httptest` HTTP server, not Mailgun. A real PostgreSQL integration test `TestIdentityMailgunMockPipeline` tests the path through the registrar, v3 encrypted outbox, claim, mock 429, retry, mock 200, acceptance, and activation. **No live Mailgun sandbox, API account, DNS or email is accessed by CI**. Provider webhook signature verification, bounce/complaint handling and inbound event reconciliation are separate future work.

## Explicit Mailgun worker runtime (stacked Draft PR #27)

A separate binary `cmd/mamony-identity-mail-worker` invokes the owning Identity facade and keeps the HTTP application unchanged. One process performs **at most one send at a time** with a fixed 5-second polling interval; multiple processes rely on the existing PostgreSQL lease/fencing/claim checks. It is not scheduled or deployed by this repository and is not automatically started when an API server starts.

Starting this binary requires **both** the exact CLI flag `--send-live-mailgun` and `MAMONY_IDENTITY_MAIL_LIVE_APPROVAL=I_ACKNOWLEDGE_REAL_MAILGUN_EMAIL_SENDS`. It also refuses a `CI` environment or a disposable `mamony_test_*` database. Without both explicit approvals, it exits before opening database connections or sending any HTTP request. This is an operational brake, **not a substitute** for provider IAM quotas, host egress policy or authentic authorization of the operator.

All three sensitive values are read from externally provisioned **absolute, non-symlink, owner-private files** (mode `0400` or `0600`); no raw Mailgun API key or envelope key is accepted directly as an environment variable:

- `MAMONY_IDENTITY_DELIVERY_DSN_FILE`: complete PostgreSQL URL for **only** `mamony_identity_delivery_runtime`, `sslmode=verify-full`. Must NOT point at the disposable test DB.
- `MAMONY_IDENTITY_MAILGUN_KEY_FILE`: Domain Sending Key only. Scope and rotation are owned by external secret provisioning.
- `MAMONY_IDENTITY_ENVELOPE_KEYS_FILE`: bounded keyring JSON: `{"active":"key-version","keys":{"key-version":"<base64-encoded 32-byte key>"}}`. Maximum eight keys with prior keys kept while retry envelopes exist. The same external key management authority must also provision the issuer's keyring; the worker cannot issue new challenges.

Other required nonsecret configuration: `MAMONY_IDENTITY_MAILGUN_REGION`, `MAMONY_IDENTITY_MAILGUN_DOMAIN`, `MAMONY_IDENTITY_MAIL_FROM`, `MAMONY_IDENTITY_MAIL_VERIFY_URL`. The Mailgun adapter validates these, including the region-specific fixed API endpoint and HTTPS verification URL. Example **dry review only** (DO NOT execute without an approved live environment):

```sh
# All /run/secrets/* files must be provisioned by an external secret manager.
# Never store secret file contents in Git, workflow logs, shell command history,
# or checked-in .env files.
MAMONY_IDENTITY_DELIVERY_DSN_FILE=/run/secrets/identity_delivery_dsn \
MAMONY_IDENTITY_MAILGUN_KEY_FILE=/run/secrets/mailgun_domain_key \
MAMONY_IDENTITY_ENVELOPE_KEYS_FILE=/run/secrets/identity_envelope_keys \
MAMONY_IDENTITY_MAILGUN_REGION=EU \
MAMONY_IDENTITY_MAILGUN_DOMAIN=mg.example.com \
MAMONY_IDENTITY_MAIL_FROM=security@mg.example.com \
MAMONY_IDENTITY_MAIL_VERIFY_URL=https://app.example.com/verify-email \
MAMONY_IDENTITY_MAIL_LIVE_APPROVAL=I_ACKNOWLEDGE_REAL_MAILGUN_EMAIL_SENDS \
go run ./cmd/mamony-identity-mail-worker --send-live-mailgun
```

The command logs only coarse startup/shutdown messages; errors returned through the facade are generic and cannot include provider bodies or email bearer URLs. SIGTERM/SIGINT cancels active network work, stops subsequent claims, and releases pool connections. A send canceled during shutdown can leave its 45-second lease for crash recovery; an ambiguous provider response may still cause a duplicate email. **Never claim exactly once or durable inbox delivery.** The process intentionally exits on unexpected DB errors rather than silently looping on an unknown storage state.

**Mock-only automated verification:** `TestDeliveryRunnerPollsSeriallyAndStopsOnShutdown` and `cmd/mamony-identity-mail-worker` tests cover cancellation, unsafe startup, CI guard, symlinks/permissions/invalid keyring. The named `TestIdentityMailgunMockPipeline` also exercises `RunDelivery` using an isolated real PostgreSQL fixture and **local HTTP mock transport**. No CI job supplies the live approval flag or external Mailgun credential. It cannot accidentally send real email.

**Still pending before deploying live:** operator approval for account/domain/region; externally managed secret and rotation policy; audited egress restriction and provider quota; abuse/resend limits at the registration API; complaint/bounce/webhook signature verification; encrypted-envelope retention cleanup; frontend safe token handling; deploy rollback, metrics, readiness and alerting. No user-facing register/verify endpoint is exposed.

## Internal resend admission budgets — stacked Draft PR #28

The **verifier**, not the provider worker or the HTTP app, enforces the following fixed policy for the synthetic first-party Identity profile: **one new proof per 60 seconds, three per rolling hour, five per rolling 24 hours per subject**. This applies to the first issuance as well as any resend. A historical challenge counts whether its proof was revoked, expired or mailed; otherwise repeatedly rotating tokens would bypass the budget. All thresholds use an authoritative PostgreSQL timestamp and are evaluated **after** locking the current subject/alias but **before** revoking the existing proof, inserting a new digest, encrypted delivery or security event. An over-budget attempt fails closed without changing any persistent state.

`EmailVerification.Resend` returns no bearer, only a trusted-internal outcome. The public API is absent and must NEVER return a distinct account-existence/rejected-by-quota response without a reviewed anti-enumeration strategy. The privilege-restricted verifier role has SELECT on historical challenges for this query; no new role permissions are granted in v4. The additive v4 migration adds `email_verify_budget_by_subject` to support bounded history scans (and keeps the earlier v1–v3 fingerprints unchanged).

**Important incompleteness:** This is **not** general rate limiting for anonymous callers or all accounts. Network/IP budgets, trusted proxy policy, abuse of nonexistent accounts and bulk account creation, global Mailgun quota, resend URL authorization, expiry cleanup, and request timing/response normalization remain unimplemented. Never equate this internally enforced per-subject guard with production readiness. Required tagged test: `TestIdentityResendAdmission` (hour/day boundary, 10 parallel reissues, old-token preservation and denied-outbox rollback), in addition to the existing contact/worker checks.

## Anonymous admission and proxy trust (stacked Draft PR #29)

Before any future registration/resend endpoint is deployed, it must invoke the unmounted Identity `AnonymousAdmission` gate **before** account lookup or credential KDF work. The gate uses HMAC-identified account/client budgets in PostgreSQL (v5), with four atomic fixed-window charges per action; it does **not** trust any default forwarded-header origin. Only a narrowly allowlisted private/loopback proxy sending one rewritten canonical XFF value may supply a forwarded client address. All else uses the TCP peer or fails closed; at deployment, explicitly prevent a client from reaching the backend behind an otherwise trusted proxy. No API registration/resend route or provider delivery has been enabled by this branch.

Static demo values: 8 requests/min and 80/hour per client IPv4 or IPv6 /64, 3/hour and 5/day per canonical email; `REGISTER` and `RESEND` use distinct budgets. Fixed UTC-aligned bins are not true rolling windows; boundary bursts, shared IPs, botnets, global provider quotas and timing differences still need defense in depth. A sanitized 202 JSON response helper is identical for allowed/limited/known/unknown in test fixtures, but no account existence authorization or complete HTTP flow is claimed. **Outstanding before public release:** durable asynchronous admission command handling, IP trust deployment validation, HMAC key rotation, TTL/retention cleanup, timed response normalization, trusted proxies and multi-region limits, global abuse controls, CSRF/Origin and actual registration/resend authorization.

## Draft v6 retention maintenance — stacked PR #30

Two no-argument PostgreSQL SECURITY DEFINER functions erase sensitive data only after fixed server-clock eligibility, **100 records max per function per call**, with `FOR UPDATE SKIP LOCKED`. Expired proof envelopes are cleared (NULL `sealed_token` plus `purged_at`) at least **24 hours after challenge expiry** and outside a live lease; delivery metadata, contact verification digest and security events remain. Old anonymous HMAC admission buckets are deleted **48 hours after fixed-window start**, exceeding the longest 24-hour quota. The delivery claim explicitly rejects null ciphertext. Neither operation allows the caller to supply identifiers, SQL clauses, or an alternative retention period.

Only `mamony_identity_maintenance_runtime` can EXECUTE these definer functions. PUBLIC EXECUTE is revoked. The role cannot directly read/update/delete Identity tables, and cannot assume migrator rights. The one-shot facade calls both in one transaction: failure rolls back both. It processes **one batch** only, and does not schedule repeat runs or activate public endpoints.

The independent `cmd/mamony-identity-maintain` requires both `--purge-expired` and `MAMONY_IDENTITY_MAINTENANCE_APPROVAL=I_ACKNOWLEDGE_IDENTITY_RETENTION_PURGE`, with an absolute, nonsymlink, owner-private `MAMONY_IDENTITY_MAINTENANCE_DSN_FILE` holding a `mamony_identity_maintenance_runtime` PostgreSQL DSN with `sslmode=verify-full`. The binary denies CI and `mamony_test_*` databases. Approval flags are operational brakes, **not operator IAM**. A possible operator-managed command, **not authorized for execution against real data**, is:

```sh
MAMONY_IDENTITY_MAINTENANCE_APPROVAL=I_ACKNOWLEDGE_IDENTITY_RETENTION_PURGE \
MAMONY_IDENTITY_MAINTENANCE_DSN_FILE=/run/secrets/identity_maintenance_dsn \
go run ./cmd/mamony-identity-maintain --purge-expired
```

**Release requirements still open:** privacy/compliance approval of 24h/48h periods, operator auditing, scheduling, backup/WAL/PITR/replica lifecycle, key destruction, document and contact erasure, legal holds, security audit retention and deletion SLAs. Setting a database column to NULL is not physical or forensic erasure from backups. Real PostgreSQL tests `TestPostgresIdentityMaintenanceRole` and `TestIdentityRetentionPurge` are mandatory.

## Missing-first-proof reconciliation candidate (stacked Draft PR #32)

`identity.ReconcileEnrollmentOnce` is a bounded internal facade, not a command, schedule, or HTTP handler. It opens only `mamony_identity_verifier_runtime`, with an operator-injected out-of-DB AES-GCM keyring, to find eligible UNVERIFIED registrations whose initial proof never committed. Query age is limited to 2 minutes–24 hours and the scan returns at most 25 subjects per call. Existing/historical proof records are excluded: only the separately admitted resend flow can authorize another proof. The first-only proof issuer rechecks no prior challenge inside the locked issuance transaction, in addition to cooldown and rolling budgets and creates a durable encrypted queue row, never a direct Mailgun send. Its result reports only aggregate counts. Partial results followed by error require operator review before retry; no raw SQL/provider/token data is exposed.

**No production scheduler, metrics exporter, secret files/CLI, paging, backlog thresholds, or real Mailgun connection are included.** This is an additional recovery primitive and real-PostgreSQL CI test, not evidence of operational reconciliation. Operators must not manually invoke it with live credentials absent change approval, protected logs, bounded frequency, KDF/provider quota checks and a reviewed production runner. Post-admission HTTP 202 responses remain opaque and can still mask failure. Reconciliation must not send to stale or new destinations without contact authority.

## Operator-only reconciliation observation (stacked Draft PR #33)

`cmd/mamony-identity-reconcile` exposes a **private, non-scheduled CLI**, not an Identity HTTP route. The no-key `--inspect` mode reads only a verifier-role DSN, reports capped eligible first-proof backlog and a stale-account boolean, and exits 2 when an operator should investigate. The explicitly approved `--repair-once` mode requires separately provisioned keyring secrets, calls the first-only reconciliation exactly once, and re-inspects. Neither mode sends Mailgun messages. The CLI itself has **no pager, webhook, Prometheus exporter, deployment scheduler or missing-run detector**; operational approval is required before using it on real identities. Full contract and failure policy: [Identity reconciliation runbook](identity-reconciliation.md).

## Candidate proof-consumption HTTP boundary (stacked Draft PR #34)

The verifier-only `EmailVerification.Confirm` primitive now has an **unmounted** strict HTTP candidate at `POST /v1/identity/verify-email`. It accepts only the 43-character token in bounded JSON, never in URL/query/cookie/Authorization, and has no ability to send Mailgun email or mint a login session. Valid first consumption returns 204; expired/replay/unknown proof responses are identical generic 400. Real PostgreSQL integration proves exact-contact, unverified-status, expiry, replay and one-winner concurrent behavior without any real provider call.

**Browser and operations are not yet built:** the Mailgun link fragment must be stripped from the browser before third-party analytics/scripts and explicitly POSTed by first-party JS. Direct TLS in Go remains mandatory to the candidate; reverse-proxy TLS offload is not approved. A real rate-limiter implementation (constructor injection only here), client UX for expired/replayed links, security metrics, provider quotas and live deployment are blocking. Do not mount this handler or claim email verification is publicly usable on the strength of these internal tests.

## Confirmation admission implementation (stacked Draft PR #35)

The unmounted `POST /v1/identity/verify-email` handler from PR #34 has a new **database-backed candidate confirmation gate**. It uses the existing nonprivileged registration/admission role and HMAC-only anonymous accounting table with a new `CONFIRM` action (Identity migration **v7**). Two fixed UTC-aligned client windows, 8/minute and 80/hour, are charged transactionally **before** verifier token lookups. IPv6 users share a /64 prefix counter; neither IP nor proof bearer is stored in the budget table. The v6 private cleanup function already removes old rows, and existing verifier/delivery roles have no read access to counters. Integration tests include 24 concurrent requests, two independent gate instances and two HTTP handler instances with exactly eight operations admitted; the handler returns 429 after exhaustion. An admission store outage fails closed as 503. No automatic route registration or production credentials are provided.

A per-client fixed-window quota is not global abuse protection; server-side provider quotas, edge bot defense, key rotation, NAT/shared-address fairness, timed load tests and an authenticated TLS proxy policy still need review. The verifier currently may collapse DB faults to invalid proof (400), which still requires a separate fix before release. Rollout of this migration must precede running any confirmation gate; old code remains compatible with the extra action constraint.

## Email confirmation 400 / 503 classification (stacked Draft PR #36)

The currently **unmounted** `POST /v1/identity/verify-email` candidate distinguishes failed proof ownership from failure to consult Identity's verifier role. A valid single-use proof returns 204 and is consumed atomically with `ACTIVE` + `CONTACT_VERIFIED` event. Unknown, expired, revoked and replayed proofs return the same generic 400 `INVALID_OR_EXPIRED_PROOF` body. Database unavailability, missing verifier privileges, failed mandatory event insert, unexpected transaction result or timeout returns a generic 503 `UNAVAILABLE` body; raw SQL, token and contact fields are never forwarded to HTTP. The 429 emitted for a denied admission budget remains independent.

An operator **must not infer that a 503 means the proof was definitely not consumed**: a lost response or ambiguous database commit can leave a valid consumed proof. Do not blindly reissue or rotate a challenge; first inspect authorized internal evidence and expose a separate safe customer follow-up UX. An active account must not receive a new enrollment proof. Missing-run, outage and rollback alerts are external deployment prerequisites, not installed by this PR. Regression tests verify a registration-only database role yields 503 without consuming a token and a late security-event insert failure rolls back state and reports unavailable. The actual health-only router remains unchanged.

## First-party verification-link page (stacked Draft PR #37)

The Mailgun mock adapter already emits `https://app.example.test/verify-email#token=<bearer>`; the new static `EmailVerificationPage` candidate serves the matching `GET /verify-email` route **only when directly constructed in a test**, not in the production public/private routers. The `#token` fragment is a browser-only value: the HTTP GET will contain `/verify-email` **without the token**. The early inline script removes the fragment from visible browser history before DOM-ready work; if cleanup fails, no POST is allowed. A deliberate click (not a mail-scanner preview GET) consumes the token through the existing unmounted `POST /v1/identity/verify-email` candidate, with credentials omitted and redirects forbidden. A 503 or network error **may indicate a committed verification with a lost response**: no automatic retry, resend or account activation assertion. The page has no login or session creation.

Test with `go test ./internal/modules/identity/...` (static page headers/routes/fragment guard) and `node --test tools/browsercheck/email_verification_page.test.cjs` (exact embedded JavaScript in a deterministic simulated DOM+Fetch). CI pins Node.js **22.16.0** in `.github/workflows/identity-browser-contract.yml` and requires these JavaScript behavior tests in addition to existing Go/race/PostgreSQL gates. The Node VM test does not prove a real-browser CSP or Mailgun deliverability; test those with an approved real-browser and mock end-to-end environment before any route is mounted. No new identity SQL migration, role, port, npm package or production secret is required by the page.

Before public activation, the product/security owner must review browser compatibility, scanner click automation, CSP and secure hosting, no-token-retaining analytics, high-NAT quotas, lost-response follow-up UX, accessible status, live alerting, secret storage and role privilege separation. Production `httpedge` remains **health-only**; a blank/404 verification URL in the running app is currently expected.

## Real Chromium, local Mailgun and PostgreSQL E2E — stacked Draft PR #38

The new test-only `TestIdentityZBrowserVerificationE2E` and dedicated `Identity real browser E2E` GitHub Actions workflow validate the internal registration → encrypted delivery → Mailgun mock → actual Chromium verification page → actual restricted PostgreSQL verifier path in a disposable loopback fixture. A local TLS bridge preserves the browser origin without adding an unsafe production Host/Origin relaxation; no public/private router gains Identity routes. The browser must not auto-consume during a GET preview, must remove URL fragments, must send a one-shot POST after explicit click, and must distinguish 204 success, expired 400 and **ambiguous network failure after a committed 204**. Go then checks exactly-once activation and the `CONTACT_VERIFIED` security event. Test command and constraints: `docs/runbooks/identity-browser-e2e.md`. No real Mailgun credentials, external email, non-disposable database, or production browser frontend is involved.

## Password authentication boundary — stacked Draft PR #39

The existing registration and email-proof flows now have a separately owned **internal** password-checking usecase. A new, restricted database authenticator credential can SELECT only the columns needed to validate Argon2id v1 and eligibility; the role is **not** able to send email, write status, read proof material, or mint a session. The new migration v8 is required before constructing this repository adapter. Database tests use only `TEST_IDENTITY_AUTHENTICATOR_URL` pointing at a disposable `mamony_test_*` loopback database and verify a real Argon2id password **after** atomic proof activation, generic denial for unknown/incorrect/inactive accounts, and SQL privilege refusal for old/wrong roles. Never provision this test password on a real database or broaden the email verifier role to read credential digests.

This is **not** a public login service. Before enablement, design a pre-KDF, distributed account-independent login rate limiter, deployment capacity, standardized error/timing semantics, sensitive-operation step-up/MFA, and a durable session issuer that atomically rechecks account eligibility before committing session state. For precise risk/authorization boundaries see `docs/adr/0004-password-authentication-foundation.md`.

## Pre-KDF login gate — stacked Draft PR #40

Candidate-only login admission is now separate from email-delivery, proof-confirmation and registration actions. The `PasswordLoginFlow` must invoke the `LoginAdmission` port, backed by the existing restricted `mamony_identity_runtime` PostgreSQL admission table, **before** a separate read-only authenticator role looks up credential rows or spends Argon2id capacity. The migration v9 is fingerprinted and does not create a session or route. Run `make integration` against a guarded disposable DB to verify exact LOGIN action/scope counters and across-pool per-network/per-email limits. Ordinary Go unit/race and specialized architecture tests verify HMAC privacy, no KDF on refusal/outage, and independent old REGISTER/RESEND/CONFIRM budgets. Do not put HMAC secrets, raw emails, bearer tokens or IPs in logs.

**No public login activation yet.** The limiter's global 80/minute and per-email 5/hour are uncalibrated, the fixed-window boundary permits bursts, and denied attempts still cost DB work; deploy perimeter controls and metric/alerting, select a safe client-IP/proxy policy and credential-stuffing response first. Sessions, MFA, secure cookies, atomic eligibility rechecks and revocation are still absent. Review `docs/adr/0005-login-admission-before-argon2id.md` before any deployment.

## Opaque session authority and retention — stacked Draft PR #41

The candidate-only internal `SessionLogin` now passes **durable pre-KDF admission → Argon2id password authentication → PostgreSQL atomic ACTIVE+verified recheck → 256-bit opaque session issuance**. Only a digest is written, with fixed 12h DB-clock expiry and a `SESSION_CREATED` outbox event; a canceled/failed/ambiguous COMMIT must never return the bearer. `resolve_opaque_session` checks revocation, expiry, current status/verified contact and session_epoch; status changes increment epoch, preventing any frozen/thawed token revival. `revoke_opaque_session` is idempotent and logs exactly one event per first revocation. Do not log or JSON-serialize the bearer. All session SQL is behind a new session-runtime role with EXECUTE-only functions; password reader, registration and proof roles have **no** session mint access.

The existing operator-only retention facade additionally runs `purge_expired_opaque_sessions()`: at most 100 expired +24h session digests per call, within the same transaction as the 100-envelope and 100-admission-counter cleanup. `RetentionResult.SessionsPurged` reports the count; audit outbox rows are preserved. Testing uses only `TEST_IDENTITY_SESSION_URL` with the synthetic `mamony_identity_session_runtime` on loopback `mamony_test_*`; never copy these credentials into a deployment. A production session runtime must be isolated because direct access to its approved issuer function is itself a token-minting capability. No public HTTP login/session/logout routes, cookie, CSRF guard, MFA or tenant authorization are enabled. See `docs/adr/0006-opaque-session-authority.md`.

## Deterministic session SQL race test — Draft PR #42

`make integration` now requires `TestIdentityZSessionFreezeIssueInterleavings` in addition to the existing full session lifecycle test. This dedicated test uses **only** explicitly validated disposable test database URLs and a synthetic superuser observer to confirm the session issuer/freezer have genuinely reached `pg_stat_activity.wait_event_type='Lock'` before one transaction releases its row lock. It proves both freeze-first/issue-first serialization orders, no minted event on denied issue, epoch invalidation after freeze, no token resurrection after thaw, and rollback of digest plus security event. A test timeout is a failure, not evidence of safety. Production roles must never have access to `pg_stat_activity` superuser inspection via this test path.

A future cookie-based HTTP login/session/logout contract is **documented but not implemented** in `docs/adr/0007-first-party-browser-session-http-contract.md`; do not interpret the tests as approval to register routes or return the session bearer through JSON. Real cookie security, Origin/CSRF protection and browser acceptance tests remain pending, as do production session role isolation and action-level authorization.

## Candidate browser login/session/csrf/logout HTTP boundary — Draft PR #43

Unexported runtime wiring remains **deliberately absent**. The Identity handler package offers `NewBrowserSessionHTTP` requiring concrete `SessionLogin`, opaque session authority, exact HTTPS Origin, direct peer resolver **without proxy trust** and a separate 32-byte CSRF HMAC key; it supports four **unmounted** HTTP candidate paths. Both default public/private listeners must continue returning 404 for every candidate route (`TestListenerRouteIsolation`). `httptest` covers strict origin/host/direct TLS/forwarded-header, admission-before-KDF, max-2048 JSON duplicate-key protection, 12h host-only HttpOnly Secure SameSite=Strict cookie, signed per-session CSRF token (15min), cookie-only session check, and revoke-before-cookie-clear logout. All errors are generic, no-store and fail closed. No production secret or route exists; never reuse synthetic test HMAC keys in deployment. See `docs/adr/0008-unmounted-browser-session-http-candidate.md` for all release blockers, particularly the **high-risk session database role's direct token minting privilege**.

## Draft #51 — password-change security notices (separate from verification tokens)

Identity v16 adds `password_change_notices` for the `PASSWORD_CHANGED` event. Unlike `email_proof_deliveries`, these jobs contain no encrypted bearer or verification URL and belong to ACTIVE account credentials. A trigger accepts the event only from the **original authenticated password-change SQL runtime** and snapshots the verified email into a separate PII-limited table atomically with v15 password rotation. Compromised verifier SQL permissions cannot forge notices.

`SecurityNoticeWorker` and `SecurityNoticeStore` expose one-event delivery with 45-second leases, five max attempts, 1/2/4/8-minute backoff and strict stale lease fencing; multiple workers can coexist. Recipients that change before claim are quarantined rather than emailed; 24-hour eligible window and seven-day bounded PII purge must be monitored. The immutable outbox audit remains after recipient erasure. **No actual password-change Mailgun sender or live worker command is deployed or enabled by this PR.** Test evidence uses a mock `SecurityNoticeSender`, plus real role-separated PostgreSQL and the earlier real-Argon2id path.

Required `TestIdentityZSecurityNoticeOutbox` and `TestPostgresIdentityZSecurityNoticeRole` must run as part of the disposable database gate. Delivery acceptance never means inbox arrival; crash-after-provider-acceptance can cause retries and duplicates. A production rollout requires recipient/privacy policy review, bounce/escalation, worker operational monitoring, DPA/retention review and credential isolation. See [ADR-0014](../adr/0014-password-change-security-notices.md).
