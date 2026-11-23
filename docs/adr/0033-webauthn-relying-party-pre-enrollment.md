# ADR 0033 — Strict WebAuthn relying-party policy before trusted authenticator enrollment

Status: **PROPOSED / stacked Draft PR #70** on Draft #69. Scope: unmounted **RP/origin configuration and negative routing tests only**. No WebAuthn ceremony or authenticator registration is implemented, no SQL schema/role is added, and no public endpoint is enabled.

## Problem and boundaries

Mamony v28 has an Ed25519 challenge signed by a **previously independently bound** key and v29 fences that approval against key revocation before first saved-code enrollment. It cannot establish how the key was enrolled. An attacker able to register their own public key using only a stolen session/password would undermine the independent possession claim; attaching a WebAuthn label or an extra signature step does not repair unsafe binding.

WebAuthn is a separate protocol with explicit RP ID, origin, challenge, credential ownership and authenticator data verification. The v28 custom Ed25519 signature **must not be treated as a WebAuthn assertion**, converted into a passkey credential, or be assigned phishing-resistant/AAL2/AAL3 status.

## Decision for the first RP profile

1. Initial RP policy is **exactly one canonical lower-case ASCII DNS host, and exactly `https://<rp-id>`**. No HTTP, wildcard, parent-domain RP scope, subdomain origins, explicit ports, trailing slash, query/fragment, userinfo, localhost, IP-literal origins, Related Origin Requests or cross-origin iframe. Related origins may be an audited separate ADR later, never inferred from suffix matching.
2. `NewPasskeyRelyingParty` fails closed and exposes an immutable RP ID/origin pair for a **future maintained WebAuthn protocol library**. It does not validate site ownership or public-suffix registrability, DNS/TLS/certificates, passkey attestations or user intent. Deployment review must establish ownership, secure HTTPS termination and independently governed origin configuration. The code is intentionally NOT a WebAuthn verifier.
3. A future `BeginRegistration` must bind a fresh server-generated challenge to a canonical subject, live session, subject/auth epoch, current contact version, RP config/policy generation, purpose and short expiry in **trusted transactional state**. `FinishRegistration` must consume it once, recheck those bindings and account status under transaction locks, validate attestation through a maintained library (including client challenge, exact origin, RP ID hash, ceremony type and user-verification policy), and atomically record credential ownership/provenance plus security audit/mandatory notification obligation. No direct caller-supplied `subject_id` or `verified=true` authority.
4. **Binding assurance is separate from protocol validity.** A passkey created after session+password+verified email can be useful for regular Profile-A authentication, but must **not automatically qualify** as the independent prior authenticator needed to bootstrap v28/v25 saved recovery codes. Recovery-approval eligibility needs previously verified independent factor or a separately reviewed high-assurance enrollment/recovery ceremony, tracked in authoritative provenance. No such ceremony exists yet; default DENY.
5. Normal signer revocation (e.g., lost device) prevents future use of that authenticator. It does **not automatically erase separate offline recovery codes**, which could be the legitimate user's only remaining recovery path. Suspected account/factor **compromise** is a different incident-response decision: freeze affected permissions, invalidate relevant sessions/approvals, evaluate recovery-code exposure, and issue durable security notifications, all under independent operator policy. The future atomic incident command and unblock/recovery process remain open.
6. Do not hand-implement WebAuthn parsing, attestation, signature counters, origin checks, or clientDataJSON validation. Evaluate maintained `github.com/go-webauthn/webauthn` with pinned versions and adversarial fixtures. User-verification flags, backup/sync state, authenticator loss, credential ID uniqueness, account recovery, phishing and client compromise require dedicated tests and explicit assurance claims, not a package import alone.

## This PR implements and verifies

- `internal/modules/identity/internal/usecase/passkey_rp_policy.go`: immutable exact-host, HTTPS-only RP configuration with a fail-closed zero value.
- Table-driven unit tests for origin mismatch, wildcards, userinfo, ports, subdomains, paths, query/fragment, invalid labels, IP/localhost and Unicode.
- Public **and private** listener negative tests preserve HTTP 404 for future registration and authentication paths, using real intended HTTP methods.
- No new credentials, passwords, raw secrets, sessions, SQL privileges, public routes, or runtime processes. No dependency on a WebAuthn library yet because no verification ceremony has been built.

## Next evidence needed before binding a real passkey

- Exact maintained library/version selection and conformance scope; RP config ownership; session-coupled registration challenge with single-use DB-clock expiry.
- A typed **independent binding authority** (not a caller-provided boolean), plus secure UI/cross-origin/CSRF/abuse and phishing-resistant step-up policy.
- Real browser + disposable PostgreSQL registration and assertion tests, including replay, wrong origin/RP, UV absent, cross-subject credential, concurrent finish, stale epoch, attestation policy, compromised role and lost-response behavior.
- Secure account recovery/revocation, audit and mandatory security notification dispatch, operational rollout, compromise containment and explicit owner review.

No NIST AAL, WebAuthn-conformant implementation, production readiness or delegated identity assurance claim is made.

## References

- [W3C Web Authentication Level 3 Recommendation, 25 August 2026](https://www.w3.org/TR/2026/REC-webauthn-3-20260825/) — RP/challenge/origin verification and authenticator semantics.
- [NIST SP 800-63B-4](https://pages.nist.gov/800-63-4/sp800-63b.html) — authenticator binding, loss/compromise and recovery requirements; applicability and AAL claims remain separate.
- [OWASP Multifactor Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Multifactor_Authentication_Cheat_Sheet.html) — factor replacement and high-risk recovery.
- [go-webauthn library](https://github.com/go-webauthn/webauthn) — library candidate only, not yet adopted.
