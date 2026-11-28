# Identity and access

The identity module owns tenants, users, sessions, API credentials, roles, and change approvals. Capabilities own their resources. They supply the resource and tenant context for authorization.

**Implementation status:** Internal Profile A enrollment, password authentication, opaque sessions, password change and recovery, security notices and WebAuthn verification are merged. [#81](https://github.com/herdifirdausss/mamony-go/pull/81) completed the bounded synthetic one-use passkey possession slice. The default HTTP process mounts health endpoints only. Full Phase 1, production delivery, passkey login, and tenant/resource and financial authorization remain open. See [current state](../current-state.md) and [#20](https://github.com/herdifirdausss/mamony-go/issues/20).

The sections below record successive implementation slices. Earlier statements describe the scope at that slice, not the latest cumulative capability; later sections supersede earlier limitations where explicitly demonstrated. Historical Draft headings are retained to preserve existing deep links; all referenced PRs #22–#77 are merged.

## Enrollment and contact verification

Read the [narrow Phase 1 owner approval](../open-decisions.md#owner-approval-record--narrow-identity-phase-1-direction) for the accepted synthetic scope. The [Phase 1 proposal](../proposals/identity-enrollment-auth.md) records the original candidate requirements, including items awaiting separate acceptance.

### Internal contact verification — accepted narrow Phase 1 scope (IDF-02)

For synthetic first-party users only: the email alias is tied to one stable subject and contact version. The verification record stores a SHA-256 domain-separated digest of the 32-byte CSPRNG-generated, URL-safe secret with subject, exact canonical email, version, purpose `EMAIL_VERIFY`, and a database-enforced expiry of 20 minutes. Issuing a new secret atomically revokes the earlier live proof. Confirmation succeeds once only: it checks the **current** unverified subject and contact/version, marks the challenge consumed, verifies the contact, changes subject status to `ACTIVE` and writes an identifier-only security event in **one PostgreSQL transaction**. Failed outbox insert rolls back activation. Confirming email **does not mint a session**.

Separate registration and verification runtime credentials are intentionally used; the verifier may select its owning contact and update only the columns required for activation. The verifier database authority is trusted under Profile A and cannot claim arbitrary DB-writer containment. Verification tokens must never enter logs, HTTP responses or persistent plaintext storage. The internal implementation returns a short-lived in-memory delivery instruction. The merged internal implementation also atomically persists a purpose/subject/challenge-bound AES-256-GCM **encrypted delivery envelope** with the challenge; the DB still keeps only the token's hash for verification. A separate worker role and lease/fencing/retry state can recover the envelope after a process crash **when its external key source remains available**. The synthetic implementation includes an opt-in worker command and unmounted browser handlers. Provider verification uses a local mock. A reviewed global abuse policy is still required before public activation. Use separate delivery evidence to establish provider acceptance or delivery. Do not enable a public registration/verification route until reliable dispatch and security admission are separately verified.

Acceptance evidence must include real database expiry, wrong-contact/version, replay, concurrent consume (one winner), failed-outbox rollback, reissue revocation, encrypted delivery rollback, claim/lease fencing, retry ceiling, provider error redaction, and database role denials. A positive mail delivery claim requires separate delivery evidence.

See the [Identity delivery runbook](../runbooks/identity-delivery.md) for operational constraints, encryption key rotation, retry semantics, and missing deployment gates.

### Find the implementation and verification

| Part of enrollment | Where to read |
|---|---|
| Pending registration and first proof | [Enrollment flow](../../internal/modules/identity/internal/usecase/enrollment_flow.go) and [first implementation slice](../proposals/identity-enrollment-first-slice.md) |
| Proof issuance, resend, and activation | [Email verification use case](../../internal/modules/identity/internal/usecase/verify_email.go) and [PostgreSQL proof store](../../internal/modules/identity/internal/repository/postgres/verification.go) |
| Delivery, retention, and missing first proofs | [Email delivery runbook](../runbooks/identity-delivery.md) and [reconciliation runbook](../runbooks/identity-reconciliation.md) |
| Browser confirmation and evidence | [Browser test chain and evidence limits](../runbooks/identity-browser-e2e.md#tested-transaction-chain) |
| Decisions before public activation | [Enrollment release decisions](../open-decisions.md#identity-enrollment-release-decisions) |

The [enrollment history](../history/identity-enrollment.md) records the exact limits, tests, and qualifiers for the individual PRs.

## Password login and browser sessions

[Authentication and lifecycle](#authentication-and-lifecycle) and [security-state controls](#security-state-and-recovery-threat-controls) define IAM-03, IAM-08, IAM-09, and IAM-11. The scope below describes merged synthetic behavior. The default HTTP application serves health endpoints, and public Identity activation remains pending.

| Part of the flow | Merged synthetic scope and source |
|---|---|
| Password admission and verification | [Password login flow](../../internal/modules/identity/internal/usecase/login_flow.go) checks admission before credential lookup and Argon2id. Read the [login performance runbook](../runbooks/identity-login-performance.md) for fixture assumptions and measured capacity. |
| Session issuance and authority | [Session login flow](../../internal/modules/identity/internal/usecase/session_login.go) combines password authentication with one-use grant issuance. The [opaque session authority](../../internal/modules/identity/internal/usecase/opaque_session.go) issues, resolves, and revokes sessions; the [PostgreSQL session store](../../internal/modules/identity/internal/repository/postgres/opaque_session.go) persists them. Migration v11 removed the earlier subject-only issuer. [ADR-0009](../adr/0009-authentication-to-session-issuance-boundary.md) describes the proposed trust boundary and the limits of role separation in Profile A. |
| Expiry, session cap, and revocation | The internal candidate applies a 30-minute idle limit, a 12-hour absolute limit, and a cap of five usable sessions per subject. Credential or contact changes invalidate prior sessions and stale grants. See [ADR-0010](../adr/0010-server-side-session-idle-expiry.md), [ADR-0011](../adr/0011-atomic-five-session-cap.md), and [ADR-0012](../adr/0012-auth-material-epoch-grant-fence.md) for exact semantics and scope. |
| Browser session candidate and evidence | The [browser session handler](../../internal/modules/identity/internal/handler/browser_session_http.go) remains unmounted. See [Chromium and disposable PostgreSQL evidence](../runbooks/identity-browser-e2e.md#stacked-draft-pr-44--real-chromium-first-party-sessions), then follow the later fixture revisions for grant-bound issuance and lifecycle checks. |

Use the [login and session release decisions](../open-decisions.md#identity-login-and-session-release-decisions) for approval and evidence gaps. Read the [slice history](../history/identity-login-and-sessions.md) for original limits, checks, and earlier behavior.

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

## Authenticated password change and security notices

Use [IAM-08, IAM-09, IAM-11, and IAM-13](#security-state-and-recovery-threat-controls) for the applicable requirements. This scope describes the merged synthetic flow for a signed-in user with the current password. The default HTTP listeners serve health endpoints. Password recovery has a separate proof and authority.

| Part of the flow | Implementation and evidence |
|---|---|
| Change the current password | The [password-change use case](../../internal/modules/identity/internal/usecase/change_password.go) checks the live session and current password. The [PostgreSQL store](../../internal/modules/identity/internal/repository/postgres/password_change.go) consumes the change grant and writes the verifier, epoch update, and audit event in the same transaction. See [the scoped design](../adr/0013-authenticated-password-change.md) and [database integration test](../../internal/modules/identity/internal/integration/authenticated_password_change_integration_test.go). |
| Record and dispatch the security notice | The password-change transaction queues a token-free obligation linked to the audit event. The [notice store](../../internal/modules/identity/internal/repository/postgres/security_notice.go) and [standalone worker](../../cmd/mamony-identity-security-notice-worker/) handle dispatch. See [the durable notice design](../adr/0014-password-change-security-notices.md) and [delivery evidence](../runbooks/identity-delivery.md#password-change-notice-scope). |
| Reconcile provider results and observe backlog | Use the [callback design](../adr/0016-mailgun-security-notice-webhook-reconciliation.md) and the [read-only health probe](../adr/0017-read-only-notice-health-probe.md) to understand local evidence, unconfirmed outcomes, and the role limits. The callback handler remains unmounted. |
| Handle a lost browser response | The unmounted [password-change page](../../internal/modules/identity/internal/handler/password_change_page.go) reports an uncertain outcome and locks submission after one attempt. See [Chromium and disposable PostgreSQL evidence](../runbooks/identity-browser-e2e.md#password-change-browser-evidence). |

Use the [release decision register](../open-decisions.md#identity-password-change-release-decisions) for open approval and evidence requirements. See the [password-change history](../history/identity-password-change.md) for the original limits, checks, and candidate status in each slice.

## Account recovery and saved codes

Read [IAM-10, IAM-11, and IAM-13](#security-state-and-recovery-threat-controls) for recovery authority, current security state, and notice requirements. The merged paths below are internal synthetic building blocks. Public recovery remains pending.

| Part of the flow | Source and scope |
|---|---|
| Recovery proof and redemption | The [proof issuer store](../../internal/modules/identity/internal/repository/postgres/password_recovery_issuance.go) and [redemption use case](../../internal/modules/identity/internal/usecase/redeem_password_recovery.go) keep proof delivery and redemption separate. Redemption requires a current challenge-bound assurance grant. The [PostgreSQL redemption test](../../internal/modules/identity/internal/integration/password_recovery_redemption_integration_test.go) covers the bounded transaction, including credential change, session invalidation, and the event-linked notice. |
| Saved-code enrollment and rotation | The [code manager](../../internal/modules/identity/internal/usecase/recovery_code_manager.go) keeps first enrollment gated on separate approval and later rotation gated on an existing eligible code. The [factor revocation decision](../adr/0032-recovery-factor-revocation-fence.md) records the current-binding check. See the [disposable database test](../../internal/modules/identity/internal/integration/recovery_code_enrollment_integration_test.go). |
| Security notices | Password recovery and saved-code changes have separate event and delivery obligations. See the [recovered-password notice design](../adr/0025-password-recovery-security-notice-worker.md), [saved-code notice design](../adr/0029-recovery-code-security-notice-worker.md), and [suppressed notice procedure](../runbooks/recovery.md#recover-a-suppressed-mandatory-security-notice). |
| Recovery after a database restore | This scope uses the [operator recovery procedure](../runbooks/recovery.md#recover-compromised-identity-or-approval-state), [Identity recovery-fence proposal](../proposals/identity-recovery-fence.md), and [recovery-session proposal](../proposals/identity-recovery-session-capability.md). Account-recovery tests establish only their own evidence boundary. |

Use the [account-recovery release decisions](../open-decisions.md#identity-account-recovery-release-decisions) for open approval and evidence requirements. See the [account-recovery history](../history/identity-account-recovery.md) for the original limits, checks, and candidate status in each slice.

## Passkey registration and possession

[Staging](../../internal/modules/identity/internal/usecase/passkey_registration_stage.go) records a pending credential after the registration checks. The [assertion verifier](../../internal/modules/identity/internal/usecase/passkey_activation_assertion.go) checks possession evidence. The [possession committer](../../internal/modules/identity/internal/usecase/passkey_activation_commit.go) hands that evidence to the separately authorized finisher for the one-use transition.

The [bounded possession requirements](#bounded-synthetic-passkey-possession) define the accepted scope. The transition records possession. Login, new sessions, independent recovery, and financial authorization each require their own approval and evidence. See the [passkey decision record](../open-decisions.md#identity-passkey-release-decisions) and [registration history](../history/identity-passkey.md).

## Bounded synthetic passkey possession

**Decision:** The owner accepted [D1–D4](https://github.com/herdifirdausss/mamony-go/issues/78#issuecomment-7150570178) for bounded synthetic use. **Implementation:** [#81](https://github.com/herdifirdausss/mamony-go/pull/81) merged Identity slice v36. It uses SQL migration `000035` because Identity slice v35 added Go signature verification without a SQL migration. **Activation:** The operation remains internal and unmounted. Independent technical acceptance is DEFERRED. Production readiness is NOT ACCEPTED.

The permitted state transition establishes possession evidence only. It confers no login, session issuance, independent recovery, authorization, or financial approval authority. The existing three-minute assertion challenge and ten-minute staging limit remain unchanged. Profile A trusts the owning application and database; stronger independent authority guarantees are out of scope.

| ID | Accepted bounded requirement | Required verification |
|---|---|---|
| PKA-01 | Bind verified possession to the exact original server-owned evidence. | Positive and negative signed-assertion and binding tests. |
| PKA-02 | Allow at most one durable completion for the original challenge, including concurrent attempts. | Real PostgreSQL concurrency and duplicate-denial evidence. |
| PKA-03 | Recheck current subject, session, credential, policy, provenance and expiry before effect. | Real database stale-state and expiry tests. |
| PKA-04 | Enforce the approved WebAuthn verification, counter and backup-state policy. | Signed fixture and persistence checks; no general clone-detection claim. |
| PKA-05 | Persist credential change, challenge consumption, identifier-only audit and pending notice atomically. | Commit and rollback evidence; pending notice is not delivered. |
| PKA-06 | Restrict completion to the separately authorized finisher role; deny unauthorized direct access. | Database privilege and isolation tests. |
| PKA-07 | Remain unmounted; do not authorize login, sessions, independent recovery or live providers. | Route, runtime, deployment and negative activation review. |
| PKA-08 | Fail closed on unexpected storage or uncertain commit outcomes; sanitize errors and do not automatically retry. | Unit and integration error-handling evidence. |

The [detailed proposal](../proposals/identity-passkey-atomic-activation.md) retains the complete accepted requirement definitions and test matrix. These summaries do not relax that matrix, establish blanket PASS, or certify recovery after stale restore. Platform #14 owns shared recovery foundation; Identity #78 owns credential semantics and security obligations. Production notification delivery, recovery continuity, role custody, ingress, independent review and separate release approval remain blockers. No automatic cleanup or recovery release is authorized by this slice.

## Enrollment history links

The original anchors below lead to the corresponding archived sections.

### Internal email-proof admission — Draft PR #28

[Read the original slice record](../history/identity-enrollment.md#internal-email-proof-admission--draft-pr-28).

### Anonymous pre-registration/resend admission — stacked Draft PR #29

[Read the original slice record](../history/identity-enrollment.md#anonymous-pre-registrationresend-admission--stacked-draft-pr-29).

### Bounded retention — stacked Draft PR #30

[Read the original slice record](../history/identity-enrollment.md#bounded-retention--stacked-draft-pr-30).

### Unmounted HTTP registration and resend — stacked Draft PR #31

[Read the original slice record](../history/identity-enrollment.md#unmounted-http-registration-and-resend--stacked-draft-pr-31).

### Bounded missing-first-proof reconciliation — stacked Draft PR #32

[Read the original slice record](../history/identity-enrollment.md#bounded-missing-first-proof-reconciliation--stacked-draft-pr-32).

### Operator-only first-proof backlog inspection — stacked Draft PR #33

[Read the original slice record](../history/identity-enrollment.md#operator-only-first-proof-backlog-inspection--stacked-draft-pr-33).

### Unmounted HTTP email proof consumption — stacked Draft PR #34

[Read the original slice record](../history/identity-enrollment.md#unmounted-http-email-proof-consumption--stacked-draft-pr-34).

### Independent HTTP confirmation admission — stacked Draft PR #35

[Read the original slice record](../history/identity-enrollment.md#independent-http-confirmation-admission--stacked-draft-pr-35).

### Email confirmation failure classification — stacked Draft PR #36

[Read the original slice record](../history/identity-enrollment.md#email-confirmation-failure-classification--stacked-draft-pr-36).

### Safe first-party email verification page — stacked Draft PR #37 (unmounted)

[Read the original slice record](../history/identity-enrollment.md#safe-first-party-email-verification-page--stacked-draft-pr-37-unmounted).

### Real Chromium + actual database browser verification gate — stacked Draft PR #38

[Read the original slice record](../history/identity-enrollment.md#real-chromium--actual-database-browser-verification-gate--stacked-draft-pr-38).

## Login and session history links

The original anchors below lead to the corresponding archived sections.

### Internal password authentication foundation — stacked Draft PR #39

[Read the original slice record](../history/identity-login-and-sessions.md#internal-password-authentication-foundation--stacked-draft-pr-39).

### Pre-Argon2id password login admission — stacked Draft PR #40

[Read the original slice record](../history/identity-login-and-sessions.md#pre-argon2id-password-login-admission--stacked-draft-pr-40).

### Internal opaque sessions — stacked Draft PR #41

[Read the original slice record](../history/identity-login-and-sessions.md#internal-opaque-sessions--stacked-draft-pr-41).

### Session freeze/issuance concurrency and browser transport proposal — Draft PR #42

[Read the original slice record](../history/identity-login-and-sessions.md#session-freezeissuance-concurrency-and-browser-transport-proposal--draft-pr-42).

### Unmounted first-party session HTTP candidate — stacked Draft PR #43

[Read the original slice record](../history/identity-login-and-sessions.md#unmounted-first-party-session-http-candidate--stacked-draft-pr-43).

### Direct SQL session mint — identified boundary (Draft PR #45)

[Read the original slice record](../history/identity-login-and-sessions.md#direct-sql-session-mint--identified-boundary-draft-pr-45).

### Server-enforced session inactivity — Draft PR #47 (candidate)

[Read the original slice record](../history/identity-login-and-sessions.md#server-enforced-session-inactivity--draft-pr-47-candidate).

### Atomic concurrent-session cap — stacked Draft PR #48

[Read the original slice record](../history/identity-login-and-sessions.md#atomic-concurrent-session-cap--stacked-draft-pr-48).

### Password/contact-change session invalidation — stacked Draft #49

[Read the original slice record](../history/identity-login-and-sessions.md#passwordcontact-change-session-invalidation--stacked-draft-49).

## Password-change history links

The original anchors below lead to the corresponding archived sections.

### Internal authenticated password change — stacked Draft #50

[Read the original slice record](../history/identity-password-change.md#internal-authenticated-password-change--stacked-draft-50).

### Password-change notification obligation — stacked Draft #51

[Read the original slice record](../history/identity-password-change.md#password-change-notification-obligation--stacked-draft-51).

### Password-change Mailgun security notice sender — stacked Draft #52

[Read the original slice record](../history/identity-password-change.md#password-change-mailgun-security-notice-sender--stacked-draft-52).

### Signed Mailgun notice callbacks and operator counts — stacked Draft #53

[Read the original slice record](../history/identity-password-change.md#signed-mailgun-notice-callbacks-and-operator-counts--stacked-draft-53).

### Read-only Mailgun notice health probe — stacked Draft #54

[Read the original slice record](../history/identity-password-change.md#read-only-mailgun-notice-health-probe--stacked-draft-54).

### Unmounted browser password-change boundary — stacked Draft #55

[Read the original slice record](../history/identity-password-change.md#unmounted-browser-password-change-boundary--stacked-draft-55).

### Actual Chromium password-change proof — stacked Draft #56

[Read the original slice record](../history/identity-password-change.md#actual-chromium-password-change-proof--stacked-draft-56).

### Candidate one-shot browser password-change experience — stacked Draft #57

[Read the original slice record](../history/identity-password-change.md#candidate-one-shot-browser-password-change-experience--stacked-draft-57).

## Account-recovery history links

The original anchors below lead to the corresponding archived sections.

### Password recovery proof foundation — stacked Draft #58

[Read the original slice record](../history/identity-account-recovery.md#password-recovery-proof-foundation--stacked-draft-58).

### Internal password recovery issuance — stacked Draft #59 / Identity v19

[Read the original slice record](../history/identity-account-recovery.md#internal-password-recovery-issuance--stacked-draft-59--identity-v19).

### Recovery-purpose AEAD delivery — stacked Draft #60 / Identity v20

[Read the original slice record](../history/identity-account-recovery.md#recovery-purpose-aead-delivery--stacked-draft-60--identity-v20).

### Atomic account-recovery redemption — stacked Draft #61 / Identity v21

[Read the original slice record](../history/identity-account-recovery.md#atomic-account-recovery-redemption--stacked-draft-61--identity-v21).

### PASSWORD_RECOVERED security alert — stacked Draft #62 / Identity v22

[Read the original slice record](../history/identity-account-recovery.md#password_recovered-security-alert--stacked-draft-62--identity-v22).

### Signed PASSWORD_RECOVERED Mailgun receipt reconciliation — stacked Draft #63 / Identity v23

[Read the original slice record](../history/identity-account-recovery.md#signed-password_recovered-mailgun-receipt-reconciliation--stacked-draft-63--identity-v23).

### Saved offline recovery codes and challenge-bound grant — Draft #64 / Identity v24

[Read the original slice record](../history/identity-account-recovery.md#saved-offline-recovery-codes-and-challenge-bound-grant--draft-64--identity-v24).

### Saved recovery-code enrollment and rotation — stacked Draft #65 / Identity v25

[Read the original slice record](../history/identity-account-recovery.md#saved-recovery-code-enrollment-and-rotation--stacked-draft-65--identity-v25).

### Isolated offline recovery-code security notice worker — Draft #66 / Identity v26

[Read the original slice record](../history/identity-account-recovery.md#isolated-offline-recovery-code-security-notice-worker--draft-66--identity-v26).

### Signed recovery-code security notice webhook and provider reconciliation — stacked Draft #67 / Identity v27

[Read the original slice record](../history/identity-account-recovery.md#signed-recovery-code-security-notice-webhook-and-provider-reconciliation--stacked-draft-67--identity-v27).

### Independently pre-provisioned cryptographic factor — stacked Draft #68 / Identity v28

[Read the original slice record](../history/identity-account-recovery.md#independently-pre-provisioned-cryptographic-factor--stacked-draft-68--identity-v28).

### Initial recovery-code factor revocation fence — stacked Draft #69 / Identity v29

[Read the original slice record](../history/identity-account-recovery.md#initial-recovery-code-factor-revocation-fence--stacked-draft-69--identity-v29).

## Passkey registration history links

The original anchors below lead to the corresponding archived sections.

### WebAuthn RP configuration candidate — stacked Draft #70 / ADR-0033

[Read the original slice record](../history/identity-passkey.md#webauthn-rp-configuration-candidate--stacked-draft-70--adr-0033).

### Passkey registration Begin-only candidate — Identity v30 / stacked Draft #71

[Read the original slice record](../history/identity-passkey.md#passkey-registration-begin-only-candidate--identity-v30--stacked-draft-71).

### WebAuthn registration evidence reader — Identity v31 / stacked Draft #72

[Read the original slice record](../history/identity-passkey.md#webauthn-registration-evidence-reader--identity-v31--stacked-draft-72).

### WebAuthn protocol verification candidate — stacked Draft #73

[Read the original slice record](../history/identity-passkey.md#webauthn-protocol-verification-candidate--stacked-draft-73).

### Passkey registration password admission — Identity v32 / stacked Draft #74

[Read the original slice record](../history/identity-passkey.md#passkey-registration-password-admission--identity-v32--stacked-draft-74).

### Atomic passkey registration staging — Identity v33 / stacked Draft #75

[Read the original slice record](../history/identity-passkey.md#atomic-passkey-registration-staging--identity-v33--stacked-draft-75).

### Passkey activation challenge and expected evidence — Identity v34 / stacked Draft #76

[Read the original slice record](../history/identity-passkey.md#passkey-activation-challenge-and-expected-evidence--identity-v34--stacked-draft-76).

### Signed passkey activation assertion verifier — Identity v35 / stacked Draft #77

[Read the original slice record](../history/identity-passkey.md#signed-passkey-activation-assertion-verifier--identity-v35--stacked-draft-77).
