# Identity and access

The identity module owns tenants, users, sessions, API credentials, roles, and change approvals. Capabilities own their resources. They supply the resource and tenant context for authorization.

**Implementation status:** Pending. The HTTP bootstrap has no login, sessions, credentials, recovery, or authorization enforcement. Choose the enabled client channel before treating any option below as a deployed control.

## Authentication and lifecycle

| ID | Requirement | Acceptance check |
|---|---|---|
| IAM-01 | Give each tenant an opaque ID and one optional parent. Enforce the configured depth. Apply tenant suspension to the users and credentials within its scope. | Reject a cycle or excessive depth. Suspend a tenant. Check that its sessions and credentials cannot start new work within the specified revocation window. |
| IAM-02 | Require a second factor for operator sign-in. Offer a phishing-resistant factor. Use generic sign-in errors and a bounded attempt policy. Notify the user through an existing contact after a sensitive credential change. | Test an invalid second factor, repeated sign-in failures, factor recovery, and revocation. Verify the notice contains no secret. |
| IAM-03 | Give sessions an idle limit and a total lifetime. End sessions when a user is suspended or removed. Protect stored password verifiers and factor secrets. | Advance a test clock beyond both limits. Exercise session replay after revocation. |
| IAM-04 | Show a new API secret once. Store only its hash. Give it a recognizable test prefix. A creator can grant only scopes the creator holds. | Retry credential creation with the same key. Check one credential and one secret disclosure. Reject an attempt to grant a broader scope. |
| IAM-05 | Rotate a credential with an explicit overlap. End the old secret when the overlap expires or an operator revokes it. | Exercise both secrets before and after expiry. Check audit and event records. |

Record password rules, factor storage, recovery, session lifetimes, credential overlap, and revocation latency before implementation. Review the threat model and current guidance first. Use synthetic credentials in tests.

## Authorization and approvals

1. Authenticate the caller. Resolve its roles or scopes.
2. Resolve the action, target, and tenant. Deny access when the decision is missing.
3. Allow access to another tenant only through an explicit delegated rule. Give unknown and unauthorized resources the same response where that response protects existence.
4. Start the tenant transaction only after authorization. Apply the [tenant database rules](postgresql.md#tenant-isolation).
5. Write audit records for security changes. Write identifier-only events with the state change.

**IAM-06.** Protect every registered money and operator route with an authorization check. Test missing permissions, a different tenant, suspended credentials, and a route without required metadata.

**IAM-07.** A requester files a controlled change with its action, target, and exact payload fingerprint. A different authorized person approves within the allowed window. The owner applies the approved content once.

Test self-approval, an expired approval, payload changes after approval, concurrent apply, and an unauthorized read of the approval queue. Operator interfaces follow [the back-office contract](reconciliation-and-events.md).

## Security-state and recovery threat controls

| ID | Requirement | Acceptance check |
|---|---|---|
| IAM-08 | For enabled first-party browser login, use an opaque server-managed session. Set bounded idle and absolute expiry, rotate after full authentication, and revoke on suspension, recovery or sensitive credential changes. Use host-only `Secure`, `HttpOnly` and appropriate `SameSite` cookies; protect cookie-authenticated writes against CSRF. | Test session fixation, stolen/replayed and revoked sessions, expiry with a controllable clock, CSRF/origin failure and logout across active sessions. |
| IAM-09 | If password login exists, use a supported adaptive password KDF with unique random salts and versioned, bounded parameters; evaluate Argon2id against measured memory and login rate budgets. Prevent common/breached passwords according to the selected policy. Limit costly verification concurrency. Never log plaintext verifiers, recovery codes, tokens or TOTP seeds. | Reject attacker-controlled KDF parameters and known-blocklisted passwords; exercise cost migration, brute-force budget, leakage/redaction, and bad inputs. |
| IAM-10 | Recovery starts with restricted authority, not a full session. A recovery channel or one backup code must not silently bypass stronger enrollment, financial step-up or operator controls. Verify proof, challenge purpose, expiry and single-use before factor/contact changes. | Race challenge consumption. Try replacing a factor, email/recovery swap, stale proof, second-account binding, and a recovery session attempting money or privileged actions. |
| IAM-11 | Validate security-critical subject, credential-set, role and ownership state against the owning authority, not a mutable business projection. Explicitly bound claims about database-writer resistance; a checksum or signature over a row does not prove current membership, anti-rollback, or permission to change it. | For Profile A, record arbitrary authority-DB-write compromise as residual risk. If Profile B is proposed, test copied password verifier, swapped contact/alias, hidden MFA membership, old signed-state replay and independently surviving authority. |
| IAM-12 | Bind sensitive action approval to the current subject, tenant, action, target and canonical payload. Require applicable assurance, short expiry, single-use consumption and recheck of revocation/freeze before effect. A server-side PIN following a password does not constitute an independent factor. | Reject altered target or beneficiary, changed amount/currency, expired/reused receipt, self-approval, and concurrent consume. Prove revocation ordering against the owning transaction. |
| IAM-13 | Record a durable mandatory security notification obligation when a relevant credential/contact/factor change commits. Keep the trusted notification destination and required policy separate from mutable marketing preferences. Dispatch failures cannot undo a committed security change or falsely report that a human read the notice. | Simulate dropped worker, notification preference tampering, changed destination, replay, and failed outbox persistence. Check one durable obligation, traceable attempts and authorized destination policy. |

**Authentication choices are contextual.** First-party browser sessions, API keys, passkeys/WebAuthn, TOTP, and OIDC/federation address different clients and threats. Do not implement a custom OAuth/OIDC protocol or add JWT access tokens without a client requirement. If WebAuthn is enabled, validate the relying-party ID, origin, challenge, user verification and credential ownership; test recovery and authenticator loss. If OIDC is enabled, use maintained protocol libraries and validate issuer, audience, state, nonce and PKCE as applicable.

**Authority limit:** Profile A trusts its authoritative application database. It does not withstand arbitrary mutation of that same authority. Stronger compromise containment requires [an explicit decision](../open-decisions.md#security-architecture-decisions), independent admin/key/deployment boundaries and adversarial evidence. The initial modular monolith remains the default.
