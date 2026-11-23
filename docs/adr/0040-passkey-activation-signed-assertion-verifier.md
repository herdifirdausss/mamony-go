# ADR-0040 — Signed WebAuthn assertion verification for provisional passkeys

Status: **PROPOSED / stacked Draft #77**. Depends on Draft #76 v34 pending-credential issuer and reader. **There is no credential activation, login, session issuance, consumed challenge, independent recovery assurance or public HTTP route in this PR.**

## Context and threat boundary

A successful v33 `webauthn.create` ceremony stages a COSE public key under `PENDING_ASSERTION`. V34 issues a fresh, unique assertion nonce with a maximum 3-minute lifetime and binds the pending credential, subject, original registration session, epoch, RP and challenge. The v34 EXECUTE-only reader supplies the expected nonce, credential ID, public key and counter *only* for the same live session and still-pending credential.

The existence of the COSE key does **not** prove possession of its private key. This cut establishes the cryptographic proof by verifying a genuinely signed `webauthn.get` assertion with the **pinned `github.com/go-webauthn/webauthn v0.18.2` maintained library**, not a homemade signature-verification path. A successful verification alone is **not** a durable, one-use authorization.

## Implementation decisions

1. **Read-only usecase.** `PasskeyActivationAssertionVerifier.VerifyOnly(ctx, bearer, challengeID, raw)` reads v34 evidence once through the already isolated reader, then delegates the whole raw assertion to `PasskeyActivationAssertionProtocol.VerifyAssertion`. This emits only copied credential ID, 32-bit authenticator counter, actual signed-ceremony UP/UV and BE/BS flags. No session, credential writes, activation transition or user ID selected from client fields.
2. **Library validation.** The adapter invokes `protocol.ParseCredentialRequestResponseBytes` and `WebAuthn.ValidateLogin`. It builds library `SessionData` exclusively from the **original DB-v34 nonce**, RP ID, origin, subject handle, required UV, and a single **allowed credential ID**. The synthetic user contains one credential: exact server-owned ID and COSE key, expected AAGUID, saved authenticator counter and BE/BS flags. It checks full clientData type `webauthn.get`, expected challenge, cross-origin rejection, RP ID hash, UP and UV, credential ID/user-handle matching, signature over authenticatorData + SHA256(clientDataJSON) and unsolicited extensions according to library defaults. Neither an incoming origin nor COSE key can override the pinned RP/DB values.
3. **Counter / clone security.** go-webauthn's `Authenticator.UpdateCounter` raises `CloneWarning` instead of an error when a nonzero counter fails to advance. The adapter explicitly rejects this flag and rejects any nonprogressing/regressed counter whenever either stored or observed count is nonzero. Zero-stored/zero-returned is allowed (a common multicdevice passkey case) but **does not establish anti-cloning assurance**. Only the original authenticator flags from the signed assertion are reported (not the stored latched UV value). BE must match the registration record; BS may transition legally; BS=1 with BE=0 is denied.
4. **Defensive limits.** Raw JSON bounded to 128 KiB before parse, ID 16–1023 bytes, COSE 16–2048 bytes, expected v34 nonce required, epoch/RP policy generation exactly matched. All credential ID buffers are copied. Errors are deliberately mapped to generic denied/unavailable classes; vendor error details are not leaked.
5. **Explicit non-authority.** `VerifyOnly` is side-effect-free. Repeating a valid signed response while v34 challenge is still pending **MAY return success multiple times**; that is an intentional demonstration that verification is not consumption. A later *separate* SQL role and transaction must recheck and consume that nonce **exactly once**, concurrently and after all account/credential/epoch/expiry locks, atomically with any allowed status transition. Public login/recovery cannot call this API as an authorization shortcut.
6. **Browser ceremony congruence not complete.** V34 Begin only returns nonce+credential selector; it does not yet construct complete `navigator.credentials.get` assertion options for a public browser. The adapter explicitly reconstructs the relevant library SessionData: the future public Begin JSON must contain congruent challenge, RP ID, allowedCredential ID and UV policy. V34 database TTL (not SessionData.Expires) is the canonical expiry and a v36 write transaction must recheck DB clock under locks. No HTTP endpoint is mounted.

## Tests and evidence

- Direct protocol test creates a **fresh P-256 private key** using `crypto/rand`, CBOR-encodes its EC2 ES256 public COSE key, constructs authenticatorData, and generates a **real ECDSA-ASN.1 signature** over `SHA256(authenticatorData || SHA256(clientDataJSON))` as required for ES256. A real signed positive case plus zero counters and BE/BS transition must pass.
- Negative cases: tampered signature, unrelated private key, wrong challenge, origin and RP hash, `webauthn.create` instead of `webauthn.get`, cross-origin context, absent UP or UV, wrong userHandle, credential ID, COSE key, subject, policy, BE/BS inconsistency and cloned or regressed counters; malformed/oversize responses.
- Usecase unit tests assert DB-v34 evidence is passed unchanged to the verifier, handles session invalidation and data unavailability without leaking candidate material, rejects fake protocol results and produces defensive copies.
- **Real disposable PostgreSQL integration** extends the mandatory v34 challenge test: a genuine signed ES256 response against exact v34 nonce and v34 EXECUTE-only reader's COSE key, same subject+session, negative other valid session and wrong nonce. The test verifies a repeated read-only assertion is still cryptographically valid to prevent any false claims about one-use consumption. The credential stays `PENDING_ASSERTION`.
- Architecture boundary: CBOR only in `repository/webauthn` tests / integration-tagged fixtures; no app-domain vendor import and no new SQL privileges/migrations.

## Next required cut — v36

A separate **EXECUTE-only activation authority** must consume v34 nonce and bind a signature-verified candidate, with an independently reviewed trust policy for `ACTIVE`, subject/session/epoch/contact/RP/credential ID, stored COSE binding, actual counter and backup flag policy, and audit + mandatory notification in one transaction. Security notice delivery worker remains unimplemented; `PENDING` in v33 is an enqueue, not a delivery receipt. Do not expose passkey login or independent recovery authority until session/activation/operational safeguards are reviewed and demonstrated.

## Source

- [go-webauthn v0.18.2 `ValidateLogin`](https://github.com/go-webauthn/webauthn/blob/v0.18.2/webauthn/login.go)
- [go-webauthn v0.18.2 assertion parser / verification](https://github.com/go-webauthn/webauthn/blob/v0.18.2/protocol/assertion.go)
- [ADR-0038](0038-webauthn-atomic-provisional-registration.md) and [ADR-0039](0039-passkey-activation-challenge-evidence.md)
