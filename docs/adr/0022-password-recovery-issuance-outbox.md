# ADR 0022 — Internal password-recovery issuance and encrypted outbox (Identity v19)

Status: **PROPOSED / stacked Draft PR #59**, based on Draft #58. The issuer is **internal-only**: no public HTTP, reset page, redemption handler, mail provider, worker, send permissions, authentication or financial action is enabled.

## Goal and security decision

An unauthenticated requester who has forgotten their password cannot supply either the active session or the previous password required by the existing authenticated password-change path. Recovery issuance is a distinct **contact-bound capability**, never a `PASSWORD_CHANGE` grant and not a session.

A request will not expose account existence or eligibility. `RecoveryRequest.Request` charges the durable HMAC-keyed `RECOVERY` budget **before** doing any issuance work. Every syntactically valid, admitted address, including unknown accounts, generates a random 256-bit proof and invokes the injected envelope sealer. A dedicated PostgreSQL issuance function decides under subject/contact locks whether it can atomically retain the digest and the sealed proof. The Go API discards the internal boolean and exposes no identifier or secret. A future HTTP layer must **also** provide uniform outward status/body and measurable response-time treatment; no such public layer exists yet.

## Durable admission

The existing, tested `anonymous_admission_buckets` table is extended by an additive `RECOVERY` action in v19; the existing registration runtime retains its counter-only INSERT/SELECT/hits-UPDATE grants. Recovery uses independent HMAC domain `mamony:identity:recovery-admission:v1\0` and five fixed quotas: global 60/min, client 5/min and 40/hour, alias 2/hour and 4/day. It does not share LOGIN/REGISTER/RESEND balances or keys. The existing store validates **all five** exact scopes and refuses caller-selected weaker limits. Client IPv6 addresses share a /64, IPv4 uses exact addresses. Failed or exceeded quotas cannot proceed to proof sealing/issuing. Fixed-window double-burst risk, NAT unfairness, distributed edge control, HMAC key rotation and raw request volume capacity still require operational review.

## Identity v19: isolated issuer role, digest and sealed queue

New runtime role: `mamony_identity_recovery_issuer_runtime`, synthetic fixture only. Migration v19 grants only schema USAGE and EXECUTE on `issue_password_recovery(text,text,bytea,text,bytea)`; it does not grant table SELECT, UPDATE or INSERT. Neither verifier/registration/delivery/session role can call that function. The function demands the **original** PostgreSQL `session_user` to match the dedicated issuer role and does not accept a client-selected subject ID, contact version, status, password verifier, recovery channel or event type.

The v19 definer function locks current subject and alias in one database transaction and permits issuance only when status ACTIVE, original verified contact present with contact_version=1, and current subject epoch positive. Denied/absent/suspended aliases produce only an internal false result; no challenge or delivery is inserted. Per-subject DB-time issuance history is counted even for revoked/expired challenges: minimum 60-second gap, at most 3/hour and 5/day. A denied cooldown never revokes a prior valid proof.

For eligible requests only, the same transaction revokes the earlier live challenge, inserts a purpose-specific `rcv_` challenge with the domain-separated 32-byte proof digest, current subject/alias/version/epoch snapshot, 15-minute DB-time expiry and one-live-per-subject invariant, and inserts a matching PENDING **ciphertext-only** delivery obligation. Digest and envelope constraints are checked in PostgreSQL. The token is never stored in plaintext, and the account cannot be reset by this SQL function. A failed envelope insertion or duplicate challenge ID rolls back **all** changes, including revocation; ambiguous SQL commit is not retried.

The injected `RecoveryEnvelopeSealer` requires a separate, explicitly reviewed AEAD key and recovery-purpose AAD binding the *canonical alias + challenge ID* before live use. No real sealer, KMS key, decryption adapter, worker or provider is introduced in this PR. Test ciphertext blobs are synthetic, not proof that a recoverable AEAD envelope exists in deployment.

## Separate delivery and redemption (deliberately not implemented)

- The ciphertext queue is not readable or claimable by existing delivery/verification roles. Before any email can be sent, add a dedicated role-restricted lease/finalize worker, recipient/current-contact rechecks, expiration and suppression behavior, retry limits, provider idempotency/bounce reconciliation, key rotation and retention/erasure policy. No PDF/HTML reset links or email-body preview are generated here.
- A subsequent redemption flow must validate the purpose-bound 32-byte digest, 15-minute expiry, unused/unrevoked state, verified current contact and subject epoch in a single locked SQL transaction. It must enforce an approved MFA/assurance recovery policy, breached-password rejector and new Argon2id verifier; atomically consume the token, rotate credentials, invalidate **all** sessions via epoch bump and create its own provenance-checked `PASSWORD_RECOVERED` notice. The v16 `PASSWORD_CHANGED` event trigger must not be bypassed. A committed but unacknowledged outcome is uncertain, never auto-retry or implicit login.
- A future public request must use exactly the same generic response for known, unknown, frozen, suppressed and quota-denied aliases. Rate-limited internal diagnostics must not turn into email-existence oracles. A real live email provider is not a requirement for this PR's green CI and must not be configured.

## Tests and claims

Required tests: role matrix on disposable PostgreSQL v19, no verifier/session/account table access, independent RECOVERY HMAC action, eligible verified subject, missing/inactive contact, cooldown preserving old proof, exactly one ciphertext queue obligation, 15-minute TTL, and rollback of revocation after a late unique-key collision. Unit tests prove no subject lookup or plaintext response, admit-before-seal ordering, budget-denied no-op, fail-closed infrastructure/crypto errors and error redaction. The migration is fingerprinted and historical SQL is immutable. Keep prior real Chromium and PostgreSQL gates on the **exact final SHA**. No live recovery route must appear in either default HTTP listener.

**Status is Draft/unmerged; there is no functional reset, user email or account recovery yet.**
