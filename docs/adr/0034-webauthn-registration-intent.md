# ADR 0034 — Durable, session-bound WebAuthn registration intents (Identity v30)

Status: **PROPOSED / stacked Draft PR #71** based on Draft #70. This is an **unmounted Begin-only candidate**, not a functional WebAuthn registration.

## Problem

ADR-0033 pins the relying-party origin and warns that password/session-only key registration cannot become an independent recovery authenticator. Future browser ceremonies need durable challenges with subject, current session and RP-context binding; process-memory or caller-supplied `subject_id` is insufficient. Issuing a challenge, however, is not proof of authenticator possession, user presence/verification, user consent, credential provenance, or independent identity assurance.

## Decision

- Introduce Identity migration **v30** with `mamony_identity.passkey_registration_intents`. One row per subject bounds outstanding and historical storage. Columns bind random 32-byte challenge and unique `wrg_` ID to subject, **digest-only opaque session**, current subject epoch, verified contact version, canonical RP ID + exact HTTPS origin, fixed policy generation 1, explicit `PASSKEY_REGISTRATION` purpose, DB-clock issuance/expiry (maximum **3 minutes**), and reserved consumed timestamp.
- Add a separate `mamony_identity_passkey_registration_issuer_runtime` with **EXECUTE-only** on a narrow SECURITY DEFINER issuer. Its role-specific `session_user` guard refuses unknown callers; it cannot read or mutate any base table, consume intent, approve factors, insert credentials, mint a login session, register recovery-code factor keys or call cross-purpose authorities.
- Begin resolves the authenticated session via the existing session authority. In the issuer's transaction the DB independently finds the **subject via session digest**—not a Go-provided ID—locks the subject+verified alias then exact session, and checks status, epoch, revocation, absolute expiry and 30-minute idle timeout with the DB clock **after locks**. A zero active session, stale account, expired session or invalid RP input returns uniform denial.
- Issuance may **not** overwrite an unexpired outstanding intent, across sessions or concurrent requests. Old expired/consumed intents may be atomically replaced using one subject row; nonce and ID never repeat. An ambiguous COMMIT response returns no ceremony material and must not automatically retry.
- The RP parameters flow from a fail-closed immutable Go `PasskeyRelyingParty`, not from an HTTP caller. SQL also checks exact HTTPS origin and canonical DNS form. The future finisher MUST recheck RP configuration and policy generation against current trusted deployment configuration; this issuer's SQL role is **not** an RP trust authority.
- **No** finish function, credential table, credential registration/attestation parser, WebAuthn library, independent binding grant, security assurance promotion, or HTTP route is present. A stolen live session could request a challenge but cannot enroll a credential in this revision. Future Begin may need fresh reauthentication or step-up depending on the approved account policy.

## Locking, state and rate limits

Subject/alias -> session -> per-subject pending intent. Existing session resolution may update activity immediately before Begin; the issuer independently rechecks expiry/state and never treats that preliminary result as authority. The one-outstanding-per-subject rule limits issuance to one successful fresh challenge per 3-minute window; it is **not** a replacement for network/global abuse protection, TLS/origin/CSRF policy, or controlled authenticated registration UI.

An intent is **public ceremony data**, not a secret approval. Persisting a **session digest** is a sensitive account binding; do not return it through a public API or log raw bearer secrets. No direct production SELECT/INSERT/UPDATE/DELETE privileges on intent data. Historical intents per account are overwritten when expired; account deletion/retention and total subject count need an explicit policy before release.

## Verification

- Unit tests: immutable challenge and RP material, invalid/no session, missing RP config, unavailable store/entropy and lost-response fail-closed behavior.
- Disposable PostgreSQL integration: active account + live session -> one committed 32-byte challenge; stored exact subject/session digest/epoch/RP/purpose; concurrent duplicate issue denied; unknown bearer denied; expired intent superseded; stale epoch denied; no recovery-code approval.
- Role tests: dedicated issuer can only invoke one function and cannot see challenge table, account tables, keys, grants or security outbox. Unrelated runtime roles cannot issue; the `finish_passkey_registration_intent` function **does not exist**. Migration counts and fingerprint updated to v30, both CI/testgate and local disposable fixture pinned.
- Full CI 5/5 must pass on **exact PR head**; no production operations until formal approval.

## Required next cut

1. Evaluate and pin a **maintained WebAuthn library** with independent test fixtures and exact RP/origin/policy verification, challenge type, UV/UP, authenticator data, credential ID/public key and attestation trust decisions.
2. Define trustworthy registration **admission and credential provenance**. Session-only begin or password/email alone must **not** mint an independent bootstrap factor for recovery codes. Bind fresh reauth/step-up, consent and exception handling to the same subject/epoch/intent.
3. Implement separate least-privileged verifier and transactional **finish**: verify protocol first, lock/recheck session, account, challenge and RP-policy generation, single-use consume, globally unique credential ID, write trusted provenance, immutable security audit and mandatory notice; test wrong origin, wrong RP, replay, UV absent, wrong account, stale epoch, role compromise, concurrency and notification failure.
4. Complete device loss versus suspected compromise, recovery, client browser CSRF/Origin/CSP/abuse defenses, deployment DNS/TLS ownership, controlled rollout and user-facing UX.

v28's standalone Ed25519 signature remains **not WebAuthn**. This PR claims no WebAuthn conformance, phishing resistance, NIST AAL2/AAL3, or live passkey enrollment.

## References

- [ADR-0033](0033-webauthn-relying-party-pre-enrollment.md) — RP/origin policy and independent binding boundary.
- [ADR-0032](0032-recovery-factor-revocation-fence.md) — prebound signer revocation fence for first offline recovery-code enrollment.
