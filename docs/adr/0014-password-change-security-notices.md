# ADR 0014 — Password-change security-notice outbox, claim leases and privacy boundary

Status: **PROPOSED, not a live email service**. Stack: Draft PR #51 → #50 → #49 → #48 onward. No public Identity endpoints have been activated.

## Why a second durable queue

v15 atomically writes an **identifier-only** security outbox event `PASSWORD_CHANGED` after changing the Argon2id verifier. It does not schedule a user notification. The existing `email_proof_deliveries` queue contains **encrypted verification bearer URLs** and runs only for UNVERIFIED subjects, so reusing it for ACTIVE password-change notices would conflate credential-bearing challenges and token-free security notifications.

A v16 migration adds `password_change_notices` keyed by the immutable security event ID. It contains the event ID, subject ID, **canonical verified recipient snapshot**, timestamps, retry state and a 128-bit claim lease ID; it does **not** contain current/old passwords, hashes, bearer tokens, grant secrets or reset links. This recipient is contact PII and is deliberately isolated from the identifier-only `security_outbox`, with bounded retention.

## Fail-closed event provenance

Before v16, the email-verifier SQL role already had `INSERT` on `security_outbox`; v15 broadened the event-type constraint to include PASSWORD_CHANGED. Triggering an alert based solely on the string event type would permit a compromised verifier to forge notices. v16's SECURITY DEFINER `enqueue_password_change_notice` trigger therefore rejects PASSWORD_CHANGED unless the original authenticated PostgreSQL `session_user` is exactly **mamony_identity_password_change_runtime**. That role has **no direct table INSERT privilege** and can produce this event only via v15's grant/session-bound `rotate_password_verifier` function.

The trigger reads the CURRENT verified alias and snapshots its canonical email into the separate delivery queue **inside the same password-rotation transaction**. A failed snapshot/insert aborts password change, session epoch bump, grant consumption and audit event together. This is not evidence of a provider submission or inbox arrival. A role migration that changes PostgreSQL login identity or uses SET ROLE/proxy impersonation must re-review this original-login invariant rather than disabling the guard.

## Restricted worker and delivery semantics

New role `mamony_identity_security_notice_runtime` has schema USAGE and EXECUTE **only** on:
- `claim_password_change_notice(lease_id)`, selecting one due row with FOR UPDATE SKIP LOCKED, 45-second lease and a verified-current-contact check; and
- `finalize_password_change_notice(event_id,lease_id,outcome)`, which enforces the latest unexpired fencing token before updating attempt count and status.

It cannot SELECT the queue, email alias, verifier, session, security audit, migration history or call authentication functions. The Go adapter returns a minimal event ID + recipient + lease to `SecurityNoticeWorker`. The worker calls an injected `SecurityNoticeSender.SendPasswordChanged` with 10-second deadline and generic error redaction, and it reuses the existing `DeliveryRunner` lifecycle. A missing worker, canceled request or failure to finalize denies success; retry outcomes are ACCEPTED, TEMPORARY and PERMANENT.

- Temporary send failures back off **1, 2, 4 and 8 minutes**. Attempt five is terminal DEAD. Permanent failures do not retry.
- An expired lease can be claimed by another worker after crash/restart. A stale worker cannot finalize, though a provider may have already accepted its previous send; **at-least-once submissions / possible duplicates** are expected. No exactly-once promise.
- PENDING notices are eligible for at most 24 hours. A changed/unverified recipient alias or deleted subject leads to a DEAD job rather than emailing an unbound address; this prioritizes privacy but **may leave a user unnotified**, requiring operator monitoring and approved escalation handling. Frozen/suspended subjects may still receive the alert at the unchanged verified address.
- A separate, fixed-policy maintenance function erases 100 notice recipient rows at a time after seven days, while preserving the identifier-only `PASSWORD_CHANGED` event. Backup/WAL erasure is not claimed.
- ACCEPTED means **provider acceptance only**, not inbox delivery. There is no downstream delivery receipt, bounce/complaint handling, retry-after from provider or real Mailgun transport in this PR.

## Test and release policy

Mandatory `TestIdentityZSecurityNoticeOutbox` on disposable role-separated PostgreSQL proves: trusted v15 rotation atomically enqueues exactly one immutable snapshot, falsified PASSWORD_CHANGED events by verifier/admin fail with 42501, EXECUTE-only claim/finalize, stale lease denial, temporary retry/backoff, fifth-attempt terminal, stale alias quarantine and bounded PII purge without deleting security audit. Existing real-Argon2id password-change test now requires its event to enqueue a PENDING notice in the same transaction, and quarantines its fixture so tests remain isolated. Unit tests verify mock sender outcome classification and provider error redaction.

**No public password-change route; no live worker command, real external sending, sending-domain configuration, outbox monitor/dashboard, production KMS/email key, account recovery, MFA, financial AuthZ, or verified operational secret isolation.** This change only makes the notification obligation durable and dispatchable through internal mockable interfaces. Do not claim users receive an email, launch a process or merge without separate review. CI success is a necessary but insufficient release gate.
