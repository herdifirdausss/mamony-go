# ADR 0024 — Assurance-gated, one-use atomic recovery redemption (Identity v21)

Status: **PROPOSED / stacked Draft PR #61**, based on Draft PR #60. No public recovery request, reset page, mail service, live recovery approval issuer, deployable CLI or login/session is enabled. This is a **restricted internal protocol candidate** whose reset operation is default-DENY for every real account.

## Problem and trust boundary

Authenticated password change (v15) correctly requires an existing live session plus old-password reauthentication; password recovery cannot reuse either authority or its `PASSWORD_CHANGED` notification provenance. Email proof issuance (v19) and email delivery (v20) are not evidence of independently approved account recovery assurance. A mailbox-compromised attacker must NOT receive account control solely because they intercepted a recovery link. **Identity v21 therefore refuses every redemption without a separate unexpired, epoch-bound `password_recovery_assurance_grants` row.** No normal runtime can issue or mutate this grant; only a privileged synthetic test fixture provisions one. This is a fail-closed design placeholder, **NOT an implemented MFA or independent identity-proofing method**, and cannot be enabled for production until the assurance owner approves a genuine grant-authority implementation.

## Atomic SQL procedure

The independent `mamony_identity_recovery_redeemer_runtime` role has schema USAGE and EXECUTE ONLY on `redeem_password_recovery(digest,expected_email,salt,new_digest,event_id)`. It cannot SELECT or mutate token tables, password verifiers, email aliases, sessions, assurance approvals, audit outbox, security notices or other grant functions. The SECURITY DEFINER function verifies `session_user` of the original connection; a separate role or `SET ROLE` cannot forge origin.

The caller passes a canonical, domain-separated 256-bit recovery proof digest, a canonical email **for password policy context and equality comparison only** (not subject selection), fresh Argon2id v1 salt/digest and an opaque audit event ID. A preliminary *unlocked* digest lookup discovers the immutable subject pointer; then it locks the subject and alias, locks and revalidates the candidate challenge, the separate assurance gate and the password verifier in that order. This avoids inversion with the v19 issuer, which locks subject before challenge. No subject identifier, token eligibility or assurance existence is returned to Go. All denials are a generic internal false result.

Under the locked PostgreSQL DB clock, redemption requires simultaneously:
- Currently ACTIVE subject, currently verified canonical email and fixed contact version.
- Token digest matches the recovery-only proof; challenge has not been consumed, revoked or expired; its subject epoch and contact version match the current account.
- Caller email exactly equals the authoritative token-bound and currently verified email, preventing a caller from bypassing contextual password-policy checks with a different account name.
- **A distinct, independently approved default-deny assurance row** matching subject epoch, unconsumed and unexpired with a maximum 10-minute window. No runtime is authorized to create one.
- Existing password verifier, validated Go password policy (including injected breach/common-password rejector), strict approved Argon2id v1 KDF and fresh per-password random salt.

One transaction marks challenge and assurance consumed, replaces the verifier, lets the existing v14 trigger increment `session_epoch` (invalidating all prior session and grant epochs), quarantines still-PENDING recovery-token delivery, then inserts an immutable `PASSWORD_RECOVERED` identifier-only security event. A separate trigger rejects forged recovery events unless the original SQL session is the redeemer role and creates an independent **token-free** `password_recovery_notices` obligation for the current verified address. No recovery-notice delivery worker or provider grant exists; this must be built before launch. Failed/outbox-colliding transactions roll back **all** changes, including both one-use fences, password verifier, epoch and notices. PostgreSQL COMMIT acknowledgement loss is **uncertain**, not confirmed success; no automatic retry, no new session.

`AuthenticatedPasswordChange`, existing v16 `PASSWORD_CHANGED` trigger and v20 recovery delivery role remain unchanged. The recovery role cannot issue/login/grant any new session.

## Concurrency, security and unresolved controls

Real integration tests require exactly one winner for two simultaneous redemptions of the same proof and gate. Replays and cross-purpose tokens are denied. A contact change advances subject epoch and makes older token/gate invalid. DB-time expiry during locks denies redemption. The new password is verified using production Argon2id parameters; the previous password stops authenticating and old sessions become invalid. A late audit-event collision retains the original verifier/epoch/token/gate, proving transaction rollback.

This change does **not** implement a production assurance process, MFA method, verified recovery-channel provisioning, session creation, recovery notice worker, public endpoint, anti-enumeration timing/captcha controls, per-token failed-attempt tracking, breached password corpus, old password reuse prevention, live encrypted delivery credentials, reset-page fragment scrubbing or operational lockout. A 256-bit random bearer resists feasible guessing, but request budgets and edge abuse controls remain mandatory. A recovery delivery may race with a contact/status change **after** its worker's claim; final redemption revalidates the epoch/contact, not external email delivery. Argon2id resource limits and production key rotation require capacity and operations approval. The assurance approval table may contain PII-free subject references but still requires retention and privilege review.

The protocol is **not deployable for self-service recovery** until all remaining controls and human signoffs are complete. CI green on a disposable synthetic fixture is evidence of implemented boundaries, not a release decision. Keep the entire stacked PR chain Draft/unmerged.
