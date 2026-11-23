# ADR 0025 — Independent PASSWORD_RECOVERED security alerts, v22

Status: PROPOSED, stacked Draft PR #62 on top of Draft #61. **Not approved for merge or production.** No HTTP handler, CLI, service scheduler, live Mailgun, recovery-assurance issuer, or public recovery flow.

## Why separate from PASSWORD_CHANGED and recovery proof email?

Identity v21's `PASSWORD_RECOVERED` event is written by an atomic, authenticated SQL transaction and enqueues a token-free `password_recovery_notices` row. Reusing the v16 `password_change_notices` delivery role/queue would blur event provenance. Reusing v20 recovery-link delivery would expose an unrelated bearer with account-security alerts. Identity v22 therefore creates an independent worker and recipient-only notification channel, **never a new password-reset authority**.

### Restricted PostgreSQL send boundary

`mamony_identity_recovery_notice_runtime` gets only schema USAGE and EXECUTE on `claim_password_recovery_notice(lease_id)` and `finalize_password_recovery_notice(event_id,lease_id,outcome,provider_message_digest)`. Both SECURITY DEFINER functions check original `session_user`. No runtime access to the queue, subject, alias, password verifier, recovery tokens, assurance approvals or audit history. Other roles, including the v16 password-change sender, cannot use v22 functions.

Claim selects one due PENDING recovery alert via `FOR UPDATE OF n SKIP LOCKED`, checks still-current verified recipient/contact version, non-deleted subject and event freshness of 24 hours, and gives an exclusive 45-second lease. A stale candidate becomes DEAD and returns an empty recipient. This avoids leaking an obsolete recipient to an email sender. Finalize requires a current lease, ACCEPTED/TEMPORARY/PERMANENT outcome, and a **32-byte SHA-256 normalized provider message digest iff ACCEPTED**. Retry backoffs: 1, 2, 4, 8 minutes; fifth attempt DEAD. A lost/ambiguous response can send the same token-free notification twice; provider acceptance is **not proof of inbox delivery**.

The separate `RecoverySecurityNoticeWorker` sends only `SendPasswordRecovered` and sanitizes all provider/SQL errors. `RecoverySecurityNoticeSender` uses the existing safe Mailgun transport configuration, but a fixed recovery-specific message that has **no bearer, link, password, event ID, subject ID, tracking pixel, or recipient-supplied text**. It requires TLS, disables tracking and rejects HTTP redirects. Provider HTTP acknowledgment must include a normalized message ID; only a digest is persisted. No live process invokes the worker.

### Observability and retention

Distinct `mamony_identity_recovery_notice_monitor_runtime` can EXECUTE only `password_recovery_notice_health()`. It yields exactly five aggregate counters: `pending_due`, `dead`, `submitted_unconfirmed`, `retrying` and `expired_pending`. The monitor cannot read individual email addresses, event IDs or raw provider IDs, send mail, change passwords or purge records. Do not interpret `submitted_unconfirmed` as delivered.

Maintenance runtime alone may call `purge_old_password_recovery_notices()`: batches of up to 100 recipient-bearing rows older than seven days, ignoring current leases. The identifier-only security audit in `security_outbox` survives. Production TTL, data-subject erasure, suppression and backup retention require an independent review.

### Verification and limitations

- SQL role-matrix integration tests must reject sender/monitor direct table access and any cross-purpose EXECUTE.
- Unit tests validate success, transient/permanent provider errors, invalid acknowledgement, empty queue, storage-error sanitization and provider correlation.
- Real PostgreSQL + local HTTP Mailgun mock tests use the v19 issuer and v21 assurance-gated redemption, verify that the immutable `PASSWORD_RECOVERED` event creates a pending security alert, that a 429 retries and a 200 stores only the normalized ID digest, that the email body contains neither proof nor links, and that count-only monitoring and runtime role isolation work.
- All 5 CI workflows must succeed on the **exact last commit SHA**. Never treat an earlier green run as current.
- Still missing: signed Mailgun callback reconciliation **specifically for recovery alerts** (existing v17 callback applies only to password-change notices), consumer/HTTP API, independent MFA/assurance grant flow, production secrets and delivery operations, provider suppression/bounce/complaint processing, monitoring thresholds/alerts, first-party reset page and authorization review. No successful delivery is claimed merely because Mailgun accepted an HTTP request.

The v22 worker is an **unmounted building block**, not production-ready account recovery.
