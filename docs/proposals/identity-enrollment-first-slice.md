# Identity enrollment: first implementation slice

**Status:** DRAFT / stacked PRs #22–#24; SQL Identity storage and internal email-verification implementation now exist. **No public HTTP Identity route, email sender, session or login.** Tracking: [Identity issue #20](https://github.com/herdifirdausss/mamony-go/issues/20).

The initial PR implements a pure Identity domain policy, credential adapter, and an atomic enrollment port. The stacked storage PR adds a least-privileged Identity PostgreSQL repository, schema, migrator and real transaction tests. Neither PR exposes registration or login as public HTTP routes. Email proof and notification dispatch are still disabled.

## Accepted direction vs remaining decisions

Directionally agreed in chat: prioritize first-party Profile A Identity before ledger, modular-monolith Go implementation, email verification before active login. NIST SP 800-63B-4 and OWASP Password Storage guidance were checked on 2026-11-21; The owner explicitly approved continuing with the reviewed Profile A direction on 2026-11-21. Project P01 uses NFC before hashing (the earlier no-NFC candidate is superseded for new credentials); existing verifier compatibility must be preserved in any future versioned migration. KDF work factor, blocklist source, operational delivery and HTTP admission remain release gates.

The `64 MiB, t=3, p=1` Argon2id parameters are fixed in this first version to prevent stored-record cost inflation, **but they are only a candidate**. Run `go test -run ^$ -bench BenchmarkPasswordHashCandidate ./internal/modules/identity/internal/repository/credential` on the supported machine and approve admission limits before exposing an endpoint. Concurrency is explicitly limited to 1..4 configured slots; this is **not** a distributed/rate-limit budget. A vetted common/breached/contextual password blocklist is required; `PasswordPolicy` intentionally fails closed if its rejector is absent.

The first synthetic identity profile uses ASCII email addresses with normalized application aliases, not EAI/IDN or provider-specific dot/plus mapping. It stores only a pending state and an *event identifier* at this interface; verification challenges, notification payload security, UI/HTTP admission and real store rollback are still pending.

## Next implementation gates

1. **Schema and atomic persistence:** separate Identity migrator/runtime credentials, SQL migration with fingerprint history, unique normalized alias, pending subject, verifier columns, durable event/outbox obligation. Prove actual PostgreSQL duplicate-race, obligation rollback, role denials and migrations. The `RegistrationStore` operation must be one transaction; fake-store unit tests do not prove this.
2. **Email proof:** current email+version+subject+purpose-bound one-time token with hashed verifier and protected delivery obligation; verify token consumption atomically; no token in public response, log, or unprotected DB event.
3. **Browser admission/login:** explicit Origin+CSRF controls, duplicate-key rejection, bounded rate/KDF budget, opaque sessions, revocation, no public operator access. Real HTTP+SQL negative tests required before enabling.
4. **Resource authorization:** default-deny subject/action/resource/tenant policy and BOLA tests. Profile A does not withstand arbitrary writes to the authoritative identity DB; Profile B independently trusted current heads remain deferred.

Run `make check` and `make race` under Go 1.27.4 and record exact evidence per requirement. Current expected identity DB integration state: **NOT RUN**. Do not mark G5 complete or merge as release-ready based solely on unit tests.

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
