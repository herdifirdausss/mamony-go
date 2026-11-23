# ADR 0035 — Isolated registration challenge evidence reader (Identity v31)

Status: **PROPOSED / stacked Draft PR #72** based on Draft #71. No WebAuthn attestation or assertion verifier, no public Identity route, and no credential-bound session or registration authority.

## Context

ADR-0034 stores a random 32-byte WebAuthn registration challenge in a DB-owned, 3-minute, session/subject/epoch/contact/RP-bound pending intent. Only an EXECUTE-only issuer may stage it. A future WebAuthn verifier needs the **server-expected** challenge and RP context, rather than accepting a client-selected value or user/subject identity. The issuer principal must **not** gain broader READ/FINISH privileges.

The initial research/design requires standard WebAuthn verification for RP ID, origin, server-issued challenge, credential subject binding and user verification [CIAM Stage 2 R-AU04 / V06, Stage 3 S08]. Ordinary user-agent WebAuthn proof is not trusted financial transaction display and does not automatically enroll an independent offline recovery authenticator.

## Decision

1. Add Identity migration **v31** `read_passkey_registration_intent(session_digest, intent_id)`, returning only server-derived expected subject, epoch, nonce, RP ID, origin and policy generation; no secret session bearer or password data. The session digest is authenticated by an independent session authority in Go, then independently checked by PostgreSQL. The database resolves the subject **from the session**, not from an untrusted request body.
2. Restrict EXECUTE to **`mamony_identity_passkey_registration_reader_runtime`** and check the original `session_user` in SECURITY DEFINER code. There is no direct SELECT on `passkey_registration_intents`, `subjects`, `email_aliases`, `opaque_sessions`, passkeys (no table exists) or audit/recovery data. The v30 issuer cannot read, the v31 reader cannot issue.
3. Lock in the order **subject+verified contact -> exact live session -> matching intent**; check status ACTIVE, contact v1 verified, current epoch, session revocation and absolute/30-min idle expiry, intent subject/session digest/purpose/policy generation, nonconsumed status and DB-clock 3-minute TTL **after the locks**. Expired, superseded, mismatched, inactive or unknown intent yields zero rows without disclosing account state.
4. The Go `PasskeyRegistrationEvidenceReader` independently resolves the bearer session, derives its SHA256 domain-separated digest, reads through the restricted v31 store and requires **exact** same subject plus configured RP ID/origin/policy generation 1. It returns a typed, internal expected-challenge record, **not** proof of any submitted credential. A DB error or ambiguous result remains fail-closed.
5. **Read is intentionally repeatable and has no credential binding side effects.** A future verifier may use this only as input to a maintained WebAuthn implementation, and a separately privileged **atomic finish** must recheck the account/session/purpose/RP generation/challenge, consume only once, verify authorization for binding, enforce global credential ID uniqueness and atomically write durable audit and mandatory notice. There is no alternate password/email self-provisioning of the independent recovery factor.
6. The vetted maintained library candidate is `github.com/go-webauthn/webauthn v0.18.2` (release **2026-09-19**, with explicit improvements to registration challenge verification, UV authorization and attestation constraints). This PR **does not add the dependency** or claim protocol-conformant verification; version pin, library initialization, exact challenge/session behavior, attestation trust policy, vendor lock review and test fixtures belong to the next separate implementation.

## Verification and negative cases

- Disposable PostgreSQL with real opaque sessions: fresh v30 intent can be read by the same bearer; wrong bearer/intent returns no data; a different configured RP policy is denied; expired and superseded IDs are denied; epoch changes deny both new issuance and read.
- Privilege tests: reader can call only `read_passkey_registration_intent`; can neither read tables nor call v30 issuer, session resolver, factor verifier, recovery-code management, or elevated role. All other runtime roles (including v30 issuer) get SQLSTATE 42501 on the read function.
- Unit tests: enforce exact caller subject, RP ID, origin, policy generation and nonzero challenge, plus default denial on missing/malformed input, unavailable DB and lost-session states.
- Migration-integrity fingerprint and expected file count progress to **31**, and local/GitHub disposable role fixtures + the required PostgreSQL testgate include the reader DSN.

## Remaining release blockers

Actual WebAuthn registration response parsing / verification through a maintained library and independent attacker-generated fixtures; origin and RP verification, challenge/clientDataJSON type, user presence and **UV policy**, credential COSE key, attestation format and trust, clientData/extension policies; a distinct reviewed binding admission authority; transactional read+consume+insert+security notice; account recovery and compromise workflows; browser HTTP security, production key/role deployment and RP ownership/operational rollout.

No WebAuthn credentials can be registered, attested or logged in with this PR; no phishing-resistance or NIST assurance claim is made.

## References

- [ADR-0033](0033-webauthn-relying-party-pre-enrollment.md) — strict RP/origin profile.
- [ADR-0034](0034-webauthn-registration-intent.md) — v30 session-bound Begin-only issuance.
- [go-webauthn v0.18.2](https://github.com/go-webauthn/webauthn/releases/tag/v0.18.2) — maintained library candidate, **not** imported.
- [W3C WebAuthn Level 3](https://www.w3.org/TR/webauthn-3/) — authoritative WebAuthn protocol verification requirements.
