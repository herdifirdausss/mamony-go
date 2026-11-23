# ADR 0036 — Unmounted WebAuthn registration protocol verifier

Status: **PROPOSED**, stacked Draft PR #73 based on Draft #72. No HTTP route, credential persistence, attestation trust assertion, account-binding authority, factor enrollment, passkey login, or migration.

## Problem

Identity v30 stores a DB-clock 3-minute nonce under a live session, subject epoch, and RP origin. Identity v31 offers expected challenge evidence to an isolated, EXECUTE-only reader. Those controls prevent caller-controlled subjects and nonce substitution, but **do not verify the WebAuthn client and authenticator response**. This PR adds a maintained library boundary without exposing a credential-registration capability.

## Library selection and exact contract

Pin `github.com/go-webauthn/webauthn v0.18.2` (2026-09-19 release, verified vendor tag). Its `protocol.ParseCredentialCreationResponseBytes` parses the complete browser response, then `WebAuthn.CreateCredential` validates WebAuthn challenge, `webauthn.create` type, origin, RP ID hash, authenticator flags including **UP + UV**, supported algorithm, attested credential ID equality and attestation structure. The library's error messages remain internal and are not returned to callers.

A specifically scoped adapter at `internal/modules/identity/internal/repository/webauthn` holds a strictly configured RP ID and single HTTPS origin (ADR-0033) and permits **no related/opaque/cross-origin origin scope**. The adapter only accepts `PasskeyRegistrationEvidence` returned by v31 (subject, epoch, nonce, RP ID, origin, generation). No caller may dynamically override RP policy. `SessionData` is explicitly reconstructed from those server-owned values using the **registered subject as a 26-byte opaque user handle**, UV required, default mediation, no requested extension outputs, and `CredentialParametersRecommendedL3` (EdDSA, ES256, RS256); it does not accept a browser-supplied challenge, user ID, or origin as authority.

**CRITICAL compatibility caveat:** this is a deliberately limited, reconstructable subset of `SessionData`, **not** the full `SessionData` returned by `BeginRegistration`. V30 does not generate full PublicKeyCredentialCreationOptions or extensions. Before enabling HTTP in future, the Begin creation options **must exactly match** the verifier's subject/user handle, nonce, RP/UV, algorithm, extensions, mediation and timeout policy. If more options or extension checks are needed, persist and reread the full immutable library `SessionData` instead; do not infer defaults. The adapter never sets an in-memory expiration later than DB-authorized validity; v31 checks the DB clock at read, and the future atomic finish must **recheck** before credential insertion. A successful read/verify may race expiry and must never itself authorize registration.

A `PasskeyRegistrationProofVerifier` port/orchestrator checks request body length (128KiB max), calls the existing session-bound v31 evidence reader and then delegates to the library adapter. The returned *candidate* includes an ID, COSE key and metadata solely for future trusted processing; it is **not a registration record, independent recovery factor or authorization to bind**. No `finish` function exists and neither verifier nor repository gets a credential-writing role.

## Evidence and limitations

- Unit and independent synthetic `fmt:none`/ES256 CBOR fixture tests: a well-formed creation response passes expected nonce/RP/origin/UP/UV; mutated nonce, origin, RP hash, `webauthn.get` type, cross-origin iframe, missing UP/UV, ID disagreement, invalid JSON/CBOR, oversized body, missing or stale DB evidence fail closed.
- `fmt:none` demonstrates protocol *structural verification* and does **not** prove hardware manufacturer, authenticator provenance, independent identity, possession of a private key over an assertion, or NIST AAL. A local malicious client can generate a self-signed/attestation-none credential; trustworthy admission is a **separate** authorization decision.
- Protocol verification occurs entirely in memory and is intentionally **repeatable** until the DB nonce expires; no consumption, replay ban, global credential uniqueness, audit notice or session commitment occurs here.
- Do not claim phishing resistance, trusted device provisioning, live passkeys, independent MFA or financial transaction approval simply because this library passes these tests.
- Dependency versions/checksums are generated under the exact pinned repo Go toolchain. The temporary module-lock workflow is deleted before merge. Existing security/architecture, PostgreSQL and browser route gates remain mandatory.

## Next separately reviewed change

1. Approve the **registration admission policy**: fresh reauthentication and explicit consent, step-up requirements/provenance, lost-device versus stolen-session handling. A password/session/email-only new passkey **cannot** become a trusted independent recovery-code factor.
2. Enforce full **Begin/Finish option congruence** for UV, RP origin, challenge/user ID, algorithms, timeouts and extension policy. Additional attestation metadata/device trust requirements must be an explicit product/security policy; unauthenticated claims are insufficient.
3. Build an independent least-privileged **transactional finish authority**: reread DB nonce under subject/session/intent locks, recheck expiry/epoch/RP generation/explicit binding admission, consume only once, enforce globally unique credential IDs, persist the complete security-relevant verified credential and atomically produce audit plus mandatory security notice. Test concurrency and failed notice atomicity.
4. Implement HTTP Origin/CSRF/CSP, browser client flow, rate limiting, key lifecycles, device revocation and staged operational rollout.

## References

- [ADR-0033](0033-webauthn-relying-party-pre-enrollment.md) — RP ID/origin pinning.
- [ADR-0034](0034-webauthn-registration-intent.md) — v30 nonce issuance.
- [ADR-0035](0035-webauthn-registration-evidence-reader.md) — v31 expected-challenge reader.
- [Maintained library release v0.18.2](https://github.com/go-webauthn/webauthn/releases/tag/v0.18.2).
- [W3C WebAuthn Level 3](https://www.w3.org/TR/webauthn-3/).
