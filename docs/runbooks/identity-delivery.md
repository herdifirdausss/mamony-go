# Identity email delivery — runbook and security boundary

**Status:** internal proposed infrastructure (stacked Draft PR #25); no public endpoint, external email provider, live worker loop, or deployment secrets are wired. No real delivery claim may be made.

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
7. Use separate periodic cleanup to erase encrypted envelopes and old contact records according to an approved privacy/retention policy. No retention/cleanup worker is deployed in this PR.

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

Apply v1, then v2, then **v3**, with the offline `mamony-identity-migrate` binary and a dedicated migrator credential. Provision the separate nonprivileged delivery role before v3. The v3 verifier implementation fails closed if the encrypted delivery table is missing; do not deploy the changed verifier against a v2-only database. Keep the worker paused until both the persisted envelope and external key source are verified. A code rollback must account for queue rows produced by the new version, and must not re-enable public endpoints.
