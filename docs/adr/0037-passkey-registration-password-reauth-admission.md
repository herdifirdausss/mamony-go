# ADR 0037 — Purpose-bound password reauthentication for passkey registration

Status: **PROPOSED / stacked Draft PR #74** based on Draft #73. No public endpoint, WebAuthn credential persistence, trusted independent recovery factor, passkey login or completed registration.

## Context

ADR-0034 stages a durable, 3-minute passkey registration challenge. ADR-0035 reads it only for the issuing live opaque session. ADR-0036 uses go-webauthn v0.18.2 to verify the full creation response against that DB nonce. However, **cryptographically valid WebAuthn registration is not authorization to add a new credential**. A stolen active session could start a ceremony and produce a software authenticator; it must not unilaterally elevate account or independent-recovery assurance.

Existing Identity v11/v15/v25 patterns provide a durable, admission-limited password authentication path and distinct 30-second, one-use, purpose-tagged SQL assertion grants. Reuse that trusted composition for a narrow **ordinary account passkey admission** without claiming an independently provisioned factor.

## Decision

1. Identity migration **v32** extends `password_login_grants.purpose` by exactly `PASSKEY_REGISTRATION_REAUTH`. A new `stage_passkey_registration_reauth_grant(subject,epoch,digest)` SECURITY DEFINER is callable only by the **existing trusted `mamony_identity_auth_grant_runtime` role**, after Go's current-password `AuthenticateWithEpoch` (including the durable pre-Argon2id rate/admission). SQL checks ACTIVE subject, current epoch and verified contact; the grant expires after **30 seconds**. This stager cannot read pending registration challenges, create sessions, or approve credentials.
2. A distinct **`mamony_identity_passkey_registration_admission_runtime`**, with only schema USAGE and EXECUTE on `admit_passkey_registration_intent(session_digest,intent_id,grant_digest)`, atomically consumes the special grant. It requires the grant's subject and epoch to match the **original exact live session** (not any session of that subject), an ACTIVE account and verified contact, the same unconsumed v30 intent ID, purpose and RP policy generation, and DB-clock 30-minute session idle, absolute session expiry, 30-second grant expiry and at-most-3-minute intent expiry. Lock order: **grant → subject/verified alias → session → intent → admission**.
3. The admission record is one row per subject with a **fixed `PASSWORD_REAUTH_ONLY` provenance**, original intent ID, digest-only session identity, subject epoch, verified-contact version, approval timestamp, expiry capped to the intent's remaining life, and future `consumed_at`. It is in a **separate table**; v30 can overwrite a superseded expired challenge without copying its previous admission. A later transaction MUST require an exact current matching intent ID, session digest, epoch, contact version and unconsumed/valid admission. A duplicate live admission for the same intent cannot be extended by reauthentication.
4. The Go `AuthenticatedPasskeyRegistrationAdmission.Admit` checks canonical email and opaque session, validates the *current* password using the pre-KDF budgeted real login flow, requires **exact equal subject/epoch**, creates the short-lived purpose-specific grant only after successful reauthentication and consumes it with the v32 isolated SQL role. It does not return the raw grant or any credential. Unknown or ambiguous commits return unavailable with **no automatic retry**.
5. This is **ordinary password-backed account passkey admission**, not fresh independently trusted possession/step-up. It does **not** grant offline recovery-code enrollment, trusted high-assurance hardware key provenance, MFA assurance, or permission to enroll a passkey after suspected account compromise. Product/security review must determine whether an existing passkey/stronger factor, device binding, security hold, explicit browser consent, or manual recovery is mandatory for an actual production registration; this stage cannot independently decide it.
6. There is **no credential table, finish/consume function, account-binding SQL authority, HTTP route or security notice side effect in v32**. The latter are hard release blockers: any future `finish` must run a fully authorized atomic transaction rechecking both nonce and admission, globally unique credential ID, verified COSE/public-key material, audit event, and **mandatory security notice**. A valid `fmt:none` candidate does not certify hardware provenance.

## Security verification

- Unit: wrong password, pre-KDF budget denial, invalid original session/email/intent, different password subject, grant stage DB ambiguity and admission COMMIT ambiguity; no SQL staging before a trustworthy reauthentication.
- Real Argon2id + PostgreSQL: one legitimate same-session admission; a second valid session of **the same subject** cannot authorize another session's intent; malformed password and login-purpose grants denied; exactly one consumed `PASSKEY_REGISTRATION_REAUTH` proof; reauth duplicate/replay cannot extend live approval; epoch changes deny; no independent recovery-code approval.
- Role isolation: the admission role cannot stage a proof, SELECT/UPDATE identity/grants/challenge/admission tables, mint sessions, access recovery factor keys or switch into migrator role. Existing issuer/reader/auth-grant/other runtimes cannot invoke the admission function. The grant stager likewise only permits the trusted auth-grant role.
- v32 migration fingerprint, canary count, test DSNs, disposable role fixture and mandatory PostgreSQL integration gate updated; all required CI workflows must pass on exact PR head.

## Next cut

A **separately reviewed**, limited enrollment **finish authority** with in-transaction challenge/reauth admission revalidation, atomic one-time consume and global credential-ID uniqueness. Attestation origin/UP/UV proof should feed the finish only through a trusted Go adapter, never via caller-supplied public keys. Credential persistence needs immutable provenance (not recoverable as an independent factor by default), attestation metadata policy, complete WebAuthn session/creation-options congruence, a deliverable user notice transactionally tied to enrollment, and anti-CSRF browser controls.

## References

- [ADR-0034](0034-webauthn-registration-intent.md) — v30 Begin-only intent.
- [ADR-0035](0035-webauthn-registration-evidence-reader.md) — v31 isolated expected-challenge reader.
- [ADR-0036](0036-webauthn-registration-protocol-verifier.md) — maintained full WebAuthn response verifier.
- [ADR-0028](0028-recovery-code-enrollment-rotation.md) — independent factor required for first offline-code enrollment.
