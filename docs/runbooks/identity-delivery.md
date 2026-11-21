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

Apply immutable Identity v1 through **v6**, with the offline `mamony-identity-migrate` binary and a dedicated migrator credential. Provision the separate nonprivileged delivery role before v3. The v3 verifier implementation fails closed if the encrypted delivery table is missing; do not deploy the changed verifier against a v2-only database. Keep the worker paused until both the persisted envelope and external key source are verified. A code rollback must account for queue rows produced by the new version, and must not re-enable public endpoints.

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
