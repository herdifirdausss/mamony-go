# Identity enrollment: first implementation slice

**Status:** DRAFT / stacked PRs #22–#26; SQL Identity storage, internal proof verification, and encrypted delivery queue/worker ports are implemented. **No public HTTP Identity route, deployed provider credentials/worker process, real email send, session or login.** Tracking: [Identity issue #20](https://github.com/herdifirdausss/mamony-go/issues/20).

The initial PR implements a pure Identity domain policy, credential adapter, and an atomic enrollment port. The stacked storage PR adds a least-privileged Identity PostgreSQL repository, schema, migrator and real transaction tests. Neither PR exposes registration or login as public HTTP routes. Real provider delivery, running worker and public HTTP activation are still disabled.

## Accepted direction vs remaining decisions

Directionally agreed in chat: prioritize first-party Profile A Identity before ledger, modular-monolith Go implementation, email verification before active login. NIST SP 800-63B-4 and OWASP Password Storage guidance were checked on 2026-11-21; The owner explicitly approved continuing with the reviewed Profile A direction on 2026-11-21. Project P01 uses NFC before hashing (the earlier no-NFC candidate is superseded for new credentials); existing verifier compatibility must be preserved in any future versioned migration. KDF work factor, blocklist source, operational delivery and HTTP admission remain release gates.

The `64 MiB, t=3, p=1` Argon2id parameters are fixed in this first version to prevent stored-record cost inflation, **but they are only a candidate**. Run `go test -run ^$ -bench BenchmarkPasswordHashCandidate ./internal/modules/identity/internal/repository/credential` on the supported machine and approve admission limits before exposing an endpoint. Concurrency is explicitly limited to 1..4 configured slots; this is **not** a distributed/rate-limit budget. A vetted common/breached/contextual password blocklist is required; `PasswordPolicy` intentionally fails closed if its rejector is absent.

The first synthetic identity profile uses ASCII email addresses with normalized application aliases, not EAI/IDN or provider-specific dot/plus mapping. It stores only a pending state and an *event identifier* at this interface; verification challenges, notification payload security, UI/HTTP admission and real store rollback are still pending.

## Next implementation gates

1. **Schema and atomic persistence:** separate Identity migrator/runtime credentials, SQL migration with fingerprint history, unique normalized alias, pending subject, verifier columns, durable event/outbox obligation. Prove actual PostgreSQL duplicate-race, obligation rollback, role denials and migrations. The `RegistrationStore` operation must be one transaction; fake-store unit tests do not prove this.
2. **Email proof:** current email+version+subject+purpose-bound one-time token with hashed verifier and protected delivery obligation; verify token consumption atomically; no token in public response, log, or unprotected DB event.
3. **Browser admission/login:** explicit Origin+CSRF controls, duplicate-key rejection, bounded rate/KDF budget, opaque sessions, revocation, no public operator access. Real HTTP+SQL negative tests required before enabling.
4. **Resource authorization:** default-deny subject/action/resource/tenant policy and BOLA tests. Profile A does not withstand arbitrary writes to the authoritative identity DB; Profile B independently trusted current heads remain deferred.

Run `make check` and `make race` under Go 1.27.4 and record exact evidence per requirement. Proof and delivery integration evidence must be tied to the exact PR commit; never assume PASS from earlier PRs. Do not mark G5 complete or merge as release-ready based solely on unit tests.

## Identity PostgreSQL storage (stacked draft PR)

The `mamony_identity` schema uses the same forward-only SHA-256 fingerprint-verifying migration framework as `mamony_platform`, but a distinct migrator role, advisory lock, runtime role and migration history. Run migrations **outside** the HTTP application:

```sh
MAMONY_IDENTITY_MIGRATION_DSN='<dedicated identity migrator DSN>' make migrate-identity
```

For a disposable local database with Docker and `psql`, use `make integration-local`. CI requires `TEST_IDENTITY_MIGRATION_URL` and `TEST_IDENTITY_RUNTIME_URL`, both isolated to the same `mamony_test_` PostgreSQL database as platform tests. Fixture credentials are synthetic and MUST NOT be used outside tests.

The Identity runtime role receives only `USAGE` on its schema and `INSERT` on four owned tables: `subjects`, `email_aliases`, `password_verifiers` and `security_outbox`. It cannot `SELECT` credential rows, `UPDATE` account status or contact verification, `DELETE` events or alter migration history. The unique canonical email PK and single PostgreSQL transaction enforce all-or-nothing pending registration. The outbox is identifier-only enrollment evidence; **no email challenge or notification delivery exists** and no event should be treated as human receipt.

`TestPostgresIdentityMigration` verifies dedicated role enforcement, repeatable migrations and SQL denials against real PostgreSQL. `TestIdentityRegistrationPersistence` verifies four-table atomicity, duplicate-alias races, forced-outbox failure rollback and `UNVERIFIED` results. The testgate runs these serially after provisioning a disposable database and rejects missing required named tests. Existing platform migration tests must remain unchanged and passing.

**Residual / next gate:** no production blocklist, chosen email provider, challenge table/token, worker dispatcher, account activation, session, rate-limit, HTTP admission or BOLA authorization. Audit obligations are durable within the trusted DB but not independently tamper-proof. PostgreSQL commit outcome can be ambiguous if the client disconnects; reconcile by authoritative ID before any retry policy. Identity owns global subject/email aliases in this first slice, while tenant membership and access decisions are deferred. No Profile B DB-writer resistance claim.

**Pre-verification database fence:** the first SQL migration deliberately CHECK-restricts subjects to `UNVERIFIED`, contacts to version 1, and `verified_at IS NULL` even for the INSERT-capable runtime. Later email verification requires a new reviewed migration, distinct update permissions and negative tests; never relax the baseline migration in place after deployment.

## Email-verification internal slice (stacked draft PR #24)

- `db/migrations/identity/000002_email-verification.up.sql` extends the existing fingerprinted Identity schema **without modifying the applied v1 file**, adds challenge digest/purpose/current contact/version/expiry/consume/revoke state, and grants a dedicated `mamony_identity_verifier_runtime` only the needed SELECT, INSERT and column-level UPDATE privileges. Registration runtime remains INSERT-only for pending enrollment; it cannot activate accounts or read challenge tokens.
- `usecase/verify_email.go` issues 32-byte random URL-safe proofs. Only a domain-separated SHA-256 digest is stored. The in-memory `PreparedEmailProof` is non-JSON-serializable and redacted under normal Go formatting. It is **not** an HTTP handler and has no public escape path.
- `repository/postgres/verification.go` binds a challenge to the authoritative current subject/email/version and `EMAIL_VERIFY` purpose; serializes issuance and consumption with subject/contact locks; enforces 20-minute DB clock expiry, one-use consumption, and one transaction for `ACTIVE` + email verification + durable event. A resend revokes earlier outstanding proofs.
- Real PostgreSQL gate `TestIdentityEmailVerification` covers replay, wrong contact, expiry, duplicate consume race, outbox failure rollback, reissue/revocation, active-account refusal and credential/DDL grants. `TestPostgresIdentityVerifierRole` verifies verifier isolation against real roles.
- There is no email delivery provider, delivery worker, durable encrypted payload, resend throttle, brute-force admission or public route. A digest-only event **cannot** reproduce a lost plaintext token after a crash; a reviewable future delivery design must either reissue or use a separate appropriately protected retry envelope. Do not imply queued `EMAIL_PROOF_ISSUED` means an email was dispatched or received.
- Contact confirmation **only proves control of a registered email**, not password authentication, MFA, financial authority, customer due diligence or operator identity. Profile A trusts the authoritative Identity database.

**Migration v2 privilege hardening:** relaxing the v1 `UNVERIFIED`/NULL-only constraints requires first removing v1 table-wide INSERT privileges from the registration runtime and granting **column-specific** INSERT on non-state fields only. New status/outbox `event_type` values come from immutable server defaults for registration. The verifier role alone has guarded column UPDATE rights. The real migration gate must reject a registration-role direct `ACTIVE` or `verified_at` INSERT. Run migration v2 **before** deploying the registration code that omits those protected columns; mixing v2 code with a v1 database fails closed (no implicit default for older status column).

## Encrypted mail-delivery queue (stacked draft PR #25)

The v3 migration adds `email_proof_deliveries` with challenge-bound encrypted bearer envelopes, claim leases, retry count/next-attempt timestamps, and `PENDING → ACCEPTED / DEAD` states. `ACCEPTED` means only a provider acknowledged submission, **not** inbox delivery. Issuing a new challenge must atomically write the challenge, its encrypted delivery payload, and the security obligation, or write none of them.

An AES-256-GCM keyring is a repo-owned crypto adapter with an explicitly injected **external key source** (no hard-coded keys, DB key, default env fallback or implicit production mail provider). The envelope authenticates subject ID, challenge ID, and purpose; tokens remain random 32-byte URL-safe values with only a SHA-256 digest available to the verifier. Keys may be rotated by keeping old decrypt-only keys until the active queue is drained. A missing key fails closed; decrypted tokens must never be logged.

A dedicated delivery DB role can claim ciphertext using `FOR UPDATE SKIP LOCKED`, fence work with a 45-second lease, retry temporary provider errors after 1/2/4/8 minutes, and stop after five attempts or the 20-minute token expiry. A real worker must give its provider a **stable challenge ID** for idempotency where supported. A crash or ambiguous provider timeout can cause duplicate deliveries; exactly-once delivery is **not guaranteed**. Expired, revoked, consumed or already-verified challenges must not be claimed.

Do not enable public registration until a real provider adapter, configured execution process, secure sender domain/DKIM/SPF/DMARC and destination policy, bounded per-subject/per-IP rate limiting, provider timeout/rejection/bounce monitoring, expiry/retention cleanup, encryption key lifecycle and HTTP-level security tests exist. [Operational runbook](../runbooks/identity-delivery.md).

## Mock-first Mailgun delivery adapter (stacked Draft PR #26)

`internal/modules/identity/internal/repository/mailgun` supplies a region-pinned, HTTPS-only Mailgun Messages API adapter for the existing `EmailSender` usecase port. Its HTTP integration seam accepts a **test-provided mock transport**; production does not accept arbitrary Mailgun URLs. Mailgun Domain Sending Keys, `From` domain and HTTPS frontend origin are validated and never exposed in errors. The text-only email embeds the short-lived proof in a URL **fragment** (requiring a future safe browser handler), disables Mailgun open/click tracking, and requests TLS and a bounded provider delivery window.

All outgoing HTTP tests use local `httptest` mocks. A separate tagged real-PostgreSQL integration test exercises the persisted encrypted delivery → mock 429 → retry → mock 200 → activation path. Production provider credentials, live outbound mail, worker startup, delivery webhooks, rate admission and login/HTTP endpoints remain disabled.
