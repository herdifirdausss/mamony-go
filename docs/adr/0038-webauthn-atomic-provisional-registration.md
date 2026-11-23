# ADR-0038 — Atomic provisional passkey credential staging and mandatory notice

Status: **PROPOSED / stacked Draft #75**, based on Draft #74. This creates **PENDING_ASSERTION only**, not a usable login authenticator, independently trusted recovery factor, device assurance guarantee or public HTTP endpoint.

## Existing foundations

The durable 3-minute v30 registration intent holds one session-bound challenge per subject. The v31 EXECUTE-only evidence reader returns the same nonce to a maintained WebAuthn v0.18.2 library in the v33 Go flow. The v32 30-second Argon2id-backed reauthentication grant was already consumed and recorded as a separate **PASSWORD_REAUTH_ONLY** admission for that exact subject, session and intent. A valid registration payload with `fmt:none` attestation is **not proof that the private key is possessed in a subsequent assertion** and cannot independently authorize recovery enrollment.

## Scope / decision

1. **No immediate login activation.** v33 persists a globally unique opaque credential ID, COSE public key, AAGUID, exact RP ID/origin, subject epoch/contact version, attestation format/type, UP/UV, BE/BS, sign count, and fixed **PASSWORD_REAUTH_ONLY** provenance in `mamony_identity.passkey_credentials`. A database CHECK permits exactly one state, **PENDING_ASSERTION**; there is no `ACTIVE` state, credential read API, authentication selector, assertion-verification endpoint, recovery factor elevation or credential mutation role. A successful stage is not a user-login capability.
2. **Immutable verified nonce across verification and transaction.** `PasskeyRegistrationProofVerifier.verifyForStage` returns both the exact v31 evidence and vetted candidate from a single maintained-library verification; it does not reread a potentially superseded nonce and mistake it for the verified nonce. The SQL transaction takes the session digest, intent ID, 32-byte expected nonce, expected epoch, RP ID+origin and candidate metadata. The SQL executor does **not** perform WebAuthn cryptographic verification: it is an origin-gated, privileged assertion oracle and must be accessible only to trusted internal Go after library validation.
3. **One transaction under fixed lock order.** The dedicated `mamony_identity_passkey_registration_stage_runtime` EXECUTE-only role can call **only** `stage_verified_passkey_credential`. It derives the subject from the session digest, locks active subject+verified alias then exact session then one current v30 intent then exact v32 admission. The database clock is read *after locks*. It checks session epoch/revocation/idle+absolute expiry, issued/nonexpired/unconsumed challenge ID **and nonce**, RP origin and policy generation, matching live **unconsumed** admission with fixed provenance, and admission expiry bounded by challenge expiry. There is no caller-selected subject.
4. **Atomic audit and notification obligation.** In the same transaction it writes one identifier-only `PASSKEY_REGISTRATION_STAGED` event, a mandatory verified-recipient **PENDING** `passkey_registration_notices` row via an origin-checked security_outbox trigger, one **globally unique** credential ID with PENDING_ASSERTION state and one-time `consumed_at` stamps on v30 nonce and v32 admission. A late unique-credential collision, duplicate event, notice failure or concurrent writer error aborts the *entire* SQL transaction; no grant/nonce can be partially consumed. A successful commit must preserve all four records.
5. **Scope of notification.** The notice row is only **enqueued**. v33 has **no send worker, webhook, retry/retention lifecycle, delivery receipt or external mail capability** and cannot be deployed as a production registration flow without those. An outbox event is **not evidence of mail delivery**. The trigger prohibits other SQL roles from forging `PASSKEY_REGISTRATION_STAGED` even if they hold generic outbox INSERT.
6. **Cryptography and evidence.** The maintained go-webauthn library verifies clientData, creation type, challenge, RP hash, origin, required UP+UV, attestation structure and COSE key; the adapter bounds returned data before invoking the privileged transaction. SQL separately fences the original nonce. Test-only `fmt:none` synthetic ES256 responses are **structural proof fixtures** and not a trusted manufacturer attestation, private-key possession guarantee or independent enrollment authorization. No user-chosen security event ID is exposed by the Go usecase.
7. **Deployment.** All routes remain unmounted and roles disconnected from any user-facing backend. The repo should not claim a functional passkey registration, phishing-resistant session, independent factor, transaction approval or deliverable security notification.

## Concurrency, abuse, and rollback proof

Integration must exercise the **real WebAuthn parser**, v31 reader and isolated staging role using disposable PostgreSQL, plus:
- no v32 admission; wrong challenge/client response; no partial side effect
- exact matching nonce+admission succeeds with one PENDING credential, outbox and mandatory pending notice, and both sources consumed
- replay denied; different nonce cannot reuse previous approval
- global duplicate credential ID (including a different intent) raises and **does not consume** any proof or emit a notice
- reused event ID fails atomically, followed by a successful corrected transaction
- concurrent staging attempts against one intent have **exactly one success** with one credential/event/notice
- role isolation: issuer, reader, admission, password-stager, session/runtime cannot call v33 finish; v33 role cannot SELECT/UPDATE/DELETE tables, stage auth grants, switch role or grant assurance
- v33 migration fingerprint/role DSNs/canary and named integration tests are mandatory in CI.

## Remaining hard blockers

**Next separate PR:** prove real authenticator private-key possession with a fresh assertion that is bound to this exact staged credential, RP ID/origin, subject/session/epoch, and independent challenge; require UV and authentic signature/sign-count/BE/BS policy. Review compromised-session/device-addition admission and explicit consent requirements; only then consider a separately authorized `ACTIVE` transition. Create a real security-notice sender with bounded retry/webhook monitoring/retention and operational alerts; do not silently treat a queued notice as sent. Complete Begin options/SessionData congruence (algorithms, RP, UV, discoverability and extensions), revocation, rate limiting, browser Origin/CSRF/CSP, risk scoring, and a separate approval channel for independent-recovery eligibility.

## References

- [ADR-0034](0034-webauthn-registration-intent.md) — v30 challenge issuance.
- [ADR-0035](0035-webauthn-registration-evidence-reader.md) — v31 expected nonce.
- [ADR-0036](0036-webauthn-registration-protocol-verifier.md) — v0.18.2 maintained verifier.
- [ADR-0037](0037-passkey-registration-password-reauth-admission.md) — v32 password-only admission.
- [W3C WebAuthn Level 3](https://www.w3.org/TR/webauthn-3/).
