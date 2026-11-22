# ADR 0021 — Password recovery authority and 256-bit proof foundation

Status: **PROPOSED / stacked Draft PR #58**, based on Draft #57. This PR contains only an internal, purpose-separated cryptographic proof primitive, its negative tests and a proposed security protocol. **No request/reset endpoint, SQL migration, issue/consume repository, email sender, active queue, router, password update or production secret is introduced.**

## Problem and scope

The existing `AuthenticatedPasswordChange` requires (1) a currently valid opaque browser session and (2) a real old-password proof for the same subject and epoch. This is intentionally **not** a forgot-password flow: the account owner cannot necessarily produce either factor. Giving a forgotten-password page access to `PASSWORD_CHANGE` grants or `rotate_password_verifier` without the existing session/Argon2id proof would be an authentication bypass.

A password recovery capability will be a **separate, higher-risk account authority**, never a shortcut into login, email registration verification or financial action authorization. A recovered account must still satisfy any separately required MFA, device trust, step-up or finance transaction authorization. An email inbox by itself cannot be assumed equivalent to an independently authenticated stronger factor or to an identity proofing process. Product/assurance owners must approve which classes of account may use email-only recovery before any issuance is enabled.

Relevant external security baselines at design time:
- OWASP Forgot Password Cheat Sheet: uniform outward response, anti-enumeration and abuse budgets, random single-use expiring challenge, no Host-derived reset link, post-success session invalidation, no automatic login.
- NIST SP 800-63B-4 section 4.2: recovery method assurance and account recovery notification, including the difference between replacing a forgotten password while another authenticator still works and recovering without usable authenticators.
- OWASP Session Management guidance: invalidate server-side sessions after credential changes.

This ADR records the *required implementation contract*, not evidence that the full protocol has been built or certified.

## Proof primitive shipped in this PR

`internal/modules/identity/internal/usecase/password_recovery_proof.go` implements:

- `NewPasswordRecoveryProof()`: draws exactly 32 bytes from `crypto/rand.Reader`, produces an unpadded, canonical 43-character base64url bearer, and computes a SHA-256 digest with the immutable domain prefix `mamony:identity:password-recovery-proof:v1\0`.
- `PasswordRecoveryProofDigest(raw)`: strictly accepts only canonical 32-byte unpadded base64url tokens. It returns the same purpose-separated 32-byte digest, **not** verification of a stored challenge, subject, expiry or account.
- `PasswordRecoveryProof.ForTrustedDelivery()`: the **only** explicitly named raw-bearer accessor. A future authorized, encrypted, durable delivery component must take ownership; callers may never return this secret to an unauthenticated requester.
- `Digest()`: returns a copy; digests must not cross roles or be reused as `EmailProof`, session, login grant or `PASSWORD_CHANGE` evidence.
- General-purpose `fmt`, `slog`, JSON and text serialization hide or reject proof material. This is accident resistance, not zeroization of Go strings, not defense against an already-compromised trusted memory process, and not encryption at rest.

This primitive **does not mint an authorized recovery challenge**, and no caller can use it to rotate a password. Its digest is only a building block for a future SQL owner-controlled consume-once protocol.

## Threat model

| Attack/failure | Security requirement |
| --- | --- |
| Enumeration by account existence/status, response fields, timing, delivery or throttles | Public request response is the **same generic status/body** for well-formed known, missing, suspended and rate-suppressed aliases. Do not expose whether mail was queued; measure and mitigate timing differences. Aggregate abuse logs without raw contact or token. |
| Request flood / inbox bombing / resource exhaustion | Independent distributed global/client/email budgets **before lookup/crypto/queue**, also per-account delivery cooldown and perimeter limits. Reuse the existing HMAC/privacy pattern, but **do not silently reuse** REGISTER/RESEND/LOGIN quotas or their SQL action allowlists. Refuse service if admission infrastructure fails; never issue on unchecked limits. |
| Contact change race / unverified destination | The SQL issuance transaction locks the subject and current verified recovery contact and snapshots a version/epoch. Only an approved, currently verified address is eligible; recheck eligibility at delivery and at redemption. A changed/removed address revokes prior challenge authority. |
| Token theft, low entropy, guessing, replay and purpose confusion | 256-bit RNG bearer, one-way purpose-separated digest, explicit `PASSWORD_RECOVERY` purpose and short DB-clock TTL (initial candidate 15 minutes, pending threat review); bounded attempts, atomic consume, old-token invalidation and no ability to authenticate with an email-verification token. |
| Redirect/exfiltration via links or third-party content | Generate URLs only from a pinned configured HTTPS origin, never from `Host` or forwarded headers. Prefer fragment bearer and immediate `history.replaceState` scrub on a dedicated no-referrer, no-third-party page; never auto-consume on GET/email scanner previews. Explicit user action performs a single POST; no open redirect. |
| Reset without MFA / mailbox compromise / lost all factors | Email-only recovery must **not** silently downgrade accounts that require stronger proof. Explicit per-assurance policy, step-up, second-factor/alternative recovery and offline escalation are **unresolved release gates**. No security questions, unverified new-email replacement or identity inference from a device/IP fingerprint. |
| Compromised DB role or forged event | Dedicated request/consume DB credentials with minimal SECURITY DEFINER EXECUTE only; neither can read password verifier material, email verification proofs, active sessions or migration history. A recovery-specific outbox event and trigger must verify the *original* DB session user, not trust a client-provided event name. |
| Concurrent redemption or SQL commit acknowledgement loss | A single serializable/locked transaction atomically consumes a purpose-bound unexpired challenge, rotates the Argon2id verifier, increments the subject epoch, invalidates all prior sessions and grants, and inserts the immutable security event plus notification obligation. Unknown commit is **not a confirmed failure**; no automatic retry and no new session. |
| Lost notification / stale recipient / data retention | A dedicated encrypted recovery-proof delivery queue and a **separate token-free** recovery event notice. Provider acceptance is not inbox delivery. Expiry, bounded attempts, lease fencing, suppression/complaint reconciliation, retention and operational escalation must be verified with real PostgreSQL + local provider mock before live enablement. |

## Proposed future implementation slices (not implemented here)

### 1. Issuance and rate admission

A future database migration must add a **separate** `PASSWORD_RECOVERY` admission action with privacy-preserving HMAC-keyed global/client/alias limits. A dedicated issue transaction must atomically establish eligibility, compare verified contact epoch, store only a challenge digest plus purpose/expiry/attempts and enqueue a KMS/envelope-encrypted delivery record. The caller gets only the same generic response, including unknown or ineligible aliases, and never a bearer. A trusted worker delivers the bearer only to the current verified contact. Email issuance must not itself sign the user in, change a password or resolve a subject for the unauthenticated caller.

A new challenge may invalidate earlier outstanding recovery challenges **only on certain issuance commit** and only after current-contact eligibility checks; denial/failed enqueue must never consume an otherwise valid challenge. In-flight delivery and concurrent contact updates must be rechecked and fenced, not assumed safe because a token was sealed.

### 2. Presentation and proof redemption

The reset page is a separately unmounted, CSP-locked, no-store/no-referrer first-party page. A GET navigation is never an authority to consume proof. It must strip any token fragment before other execution, refuse unknown origins/forwarded identity and submit proof **exactly once** after a deliberate click with a policy-validated new password. The POST cannot accept arbitrary subject ID, requested alias, role, privilege or resource authorization as an authority selector; the DB challenge **alone** determines the eligible subject. A generic denied response must not expose whether the token was expired, used, or for an unknown alias.

### 3. Atomic recovery rotation and notification

The transaction is **not** the existing v15 `rotate_password_verifier`, because that function correctly demands a live session + old-password `PASSWORD_CHANGE` grant. A separate recovery SQL function must lock and validate its own proof and subject/contact version, reject canceled/expired/replayed proofs, rotate a policy-approved verifier with a fresh salt, and atomically invalidate sessions via the existing v14 epoch mechanism. It must write a new **`PASSWORD_RECOVERED`** event, not forge `PASSWORD_CHANGED`: v16's `enqueue_password_change_notice()` requires `session_user='mamony_identity_password_change_runtime'`, and recovery must never weaken that guard. A recovery-specific trigger/notice path must preserve trustworthy provenance, recipient snapshots, and failure rollback.

Recovery does **not** authenticate the account owner for additional financial functions. Issue a separate, appropriately risk-reviewed recovery notification and require a fresh login/step-up. A lost commit acknowledgement gives an **uncertain** client state, never a success message or automatic replay.

### 4. Tests and operational gates

Require real PostgreSQL race tests: concurrent redeem exactly one winner; expiry and attempt budgets; alias/contact/epoch change before issuance, delivery and consume; frozen/deleted subject; reused/incorrect/email-verification token; post-commit response loss; events and notice rollback on failures; wrong SQL-role deny matrix; KDF policy and session revocation. Require real Chromium cross-origin/cookie/fragment scrubbing and response-loss UX. Keep the five CI workflows on the exact final SHA, use disposable test credentials and mock-only provider. Further human signoff is required for assurance/MFA policy, registered recovery channel, accessibility and account lockout handling, provider delivery reliability, perimeter controls, privacy retention, deployed ingress and customer support incident procedures.

**No public Identity route, password reset, recovery email, session or financial authorization is activated in Draft #58. Keep the stacked PRs Draft/unmerged.**
