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
| Session issuance and authority | [Session login and authority](../../internal/modules/identity/internal/usecase/opaque_session.go) and the [PostgreSQL session store](../../internal/modules/identity/internal/repository/postgres/opaque_session.go) use a one-use grant for issuance. Migration v11 removed the earlier subject-only issuer. [ADR-0009](../adr/0009-authentication-to-session-issuance-boundary.md) describes the proposed trust boundary and the limits of role separation in Profile A. |
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

## Internal authenticated password change — stacked Draft #50

Candidate v15 adds `AuthenticatedPasswordChange.Change`, which requires a currently usable opaque session **and** fresh old-password verification through mandatory distributed pre-KDF login admission + Argon2id, bound to the same subject and epoch. The new secret is NFC-normalized, checked against the required configured password blocklist, denied if equal to the old normalized secret, and rehashed with a new 128-bit salt. A purpose-bound one-use `PASSWORD_CHANGE` grant (30s) cannot be consumed by the login session issuer. A new EXECUTE-only `mamony_identity_password_change_runtime` can rotate the verifier only with both session and change proof; mutation, v14 `session_epoch` increment and identifier-only `PASSWORD_CHANGED` security event commit atomically. No replacement bearer is issued. See [ADR-0013](../adr/0013-authenticated-password-change.md).

No change-password public endpoint, session cookie/CSRF mutation handler, notification delivery or approved production blocklist is included. The grant stager remains a privileged assertion oracle under Profile A. Owner/security/UX approvals, MFA/step-up, trusted client network and workload secret isolation remain blockers.

## Password-change notification obligation — stacked Draft #51

The v15 trusted `PASSWORD_CHANGED` security outbox event now causes a **v16 same-transaction enqueue** in `password_change_notices`; a failed enqueue rolls back the password change. The identifier-only security outbox remains free of email addresses. Only events written under the original `mamony_identity_password_change_runtime` PostgreSQL login can trigger a notice; earlier verifier role outbox INSERT privileges cannot forge an alert. The separate queue captures the currently verified email at the time of mutation; it contains only event ID, subject, recipient and delivery metadata, never a bearer/password/reset URL.

The isolated `mamony_identity_security_notice_runtime` may EXECUTE only claim/finalize functions (not read raw tables, alter Identity state, mint grants or issue sessions). Due notices are claimed with SKIP LOCKED and 45-second fences; retry is capped at five submissions with 1/2/4/8 minute backoff. A contact change or unverified/DELETED recipient before claim marks the notice DEAD to prevent sending to an unbound address. Provider ACCEPTED never means inbox delivered; duplicate sends may occur after worker crashes. Seven-day fixed retention deletes recipient rows while preserving the security event. Refer to [ADR-0014](../adr/0014-password-change-security-notices.md).

No live sender/worker command is activated and no public password-change route exists. Sender is a mockable interface only in this candidate; no user notification can yet be claimed. Remaining policy includes operational delivery monitoring, a trusted provider sender, bounce handling and escalation when stale notices are suppressed.

## Password-change Mailgun security notice sender — stacked Draft #52

The internal `SecurityNoticeSender` abstraction from Draft #51 now has a real Mailgun Messages API implementation limited to the configured US/EU/AU region and verified sending domain. It submits a **fixed text-only password-changed warning**, not a verification token, session, grant, reset URL or user-selected text. No links or dynamic metadata; open/click tracking disabled and Mailgun recipient TLS required. HTTP response/error bodies are never exposed; a successful bounded 200 JSON response means **accepted by provider**, not delivered or read. HTTP 429/420/408/5xx and uncertain outcomes retry under the v16 PostgreSQL bounded lease/backoff; duplicates remain possible after an ambiguous acceptance.

A new dedicated `RunSecurityNoticeDelivery` facade and standalone `cmd/mamony-identity-security-notice-worker` use **only** the `mamony_identity_security_notice_runtime` role; they do not reuse the verification email sender, token envelope keyring or worker DSN. Real email sending needs both `--send-live-security-mailgun` and the exact operator approval, external owner-only secret files, non-CI runtime and verified-full PostgreSQL TLS. **The command is not scheduled/deployed, no real credentials have been connected and no live email has been sent by this implementation.** See [ADR-0015](../adr/0015-mailgun-password-change-notice-worker.md).

## Signed Mailgun notice callbacks and operator counts — stacked Draft #53

Identity v17 extends the password-change notification queue with **SHA-256 of the normalized Mailgun Message-ID** returned only after a successful Mailgun send response. The sender no longer calls an uncorrelated three-argument SQL finalizer: the worker role may only execute the new four-argument `finalize_password_change_notice_v2`. `ACCEPTED` means submitted to provider; independent `provider_state` tracks `SUBMITTED`, `TEMPORARY_FAILURE`, `DELIVERED` (recipient SMTP server only), `PERMANENT_FAILURE`, `COMPLAINED`, and `UNSUBSCRIBED`.

The **unmounted** HMAC-verifying Mailgun webhook candidate accepts only signed JSON event callbacks from the configured sending domain, fixed failure types and a verified Mailgun message-ID. It uses a signing key **different from the API sending key**. A separate `mamony_identity_notice_webhook_runtime` role may only EXECUTE a receipt function; it cannot read tables, access credentials, issue sessions, or alter password verifiers. Replay tokens and Mailgun event IDs are deduplicated in PostgreSQL using digests; receipt payloads store no email/PII. Out-of-order events are durably reconciled, and a later bounce/complaint cannot be hidden by an older delivery callback. A count-only health function is exposed to the isolated maintenance operator role, not the HTTP runtime.

A signed callback cannot be treated as full-body cryptographic attestation: Mailgun signs `timestamp+token`, not the entire JSON. Transport/TLS, domain binding, role isolation and real provider-key ownership must be reviewed before enabling a live endpoint. **No route is registered, no webhook receiver is deployed, no real Mailgun callback is processed and no on-call alert is configured.** Prior uncorrelated ACCEPTED notices cannot be automatically backfilled; ambiguous sender timeouts may produce unmatched provider receipts and duplicate messages. See [ADR-0016](../adr/0016-mailgun-security-notice-webhook-reconciliation.md).

## Read-only Mailgun notice health probe — stacked Draft #54

Identity v18 introduces the isolated `mamony_identity_notice_monitor_runtime` PostgreSQL LOGIN role, with **only** schema USAGE and EXECUTE on the count-only `security_notice_health()` function. It has no retention purge, mail delivery, webhook receipt, contacts, session, password or table permissions. `ReadSecurityNoticeHealthAsMonitor` exclusively accepts this role; the earlier maintenance-only facade remains backwards-compatible.

The unmounted, one-shot `mamony-identity-notice-health` command requires a private DSN file, strict full TLS verification, `--check-security-notices`, five mandatory numeric thresholds, and a non-CI operator environment. It writes only count-only JSON and exits 0 OK, 2 ALERT (configured threshold exceeded), or 1 ERROR (misconfiguration/DB unavailable). No default production thresholds, on-call routing, scheduler or public HTTP health endpoint exist. Counts can report suspected problems, not verify human inbox receipt. See [ADR-0017](../adr/0017-read-only-notice-health-probe.md).

## Unmounted browser password-change boundary — stacked Draft #55

The candidate `BrowserPasswordChangeHTTP` provides a separately constructed but **unmounted** `POST /v1/identity/change-password`. It reuses the existing concrete browser-session handler's exact HTTPS Origin/Host, direct-TLS, no-forwarded-identity, trusted peer, host-only cookie and 15-minute session-bound CSRF requirements. Bounded, duplicate-key-rejecting JSON has exactly `email`, `currentPassword`, `nextPassword`; the email must be canonical, but is **not** trusted as the account authority. A concrete `AuthenticatedPasswordChange` usecase independently verifies the old secret against the session's subject and epoch before the SQL atomic rotation.

On certain success, the response is 204 with the old cookie cleared; v14 SQL epoch invalidation has already revoked all previous sessions, and there is **no replacement session**. Wrong secret, account mismatch, throttle or new-secret policy denial collapse to a generic 401. Unknown commit outcome returns 503 and never claims success or clears the cookie, even if PostgreSQL might have committed. Response headers use no-store and no password or session material is emitted.

This is **not an enabled public or private route**: application routers and `main` remain unchanged, and no real-browser + PostgreSQL test of this new operation has yet passed. MFA/step-up, production password blocklist, live email sender/webhook, trusted ingress, account recovery, user-facing UX and finance authorization remain open. See [ADR-0018](../adr/0018-first-party-password-change-http-candidate.md).

## Actual Chromium password-change proof — stacked Draft #56

The previously unmounted `BrowserPasswordChangeHTTP` candidate is now exercised with real Chromium cookie handling, actual Argon2id, isolated PostgreSQL runtime roles and v15–v18 transactional state in `TestIdentityZBrowserPasswordChangeE2E` (tags `integration,browser_e2e`). Two independent browser sessions on the same account prove a successful password change invalidates both, while the initiator's host-only cookie is cleared and only the new credential can create a fresh session. Denied CSRF, oversized/duplicate JSON, sibling/cross-site requests, and a different account's valid password do not authorize the rotation.

A second scenario deliberately drops the HTTP response **after** PostgreSQL commit: Chromium retains the old cookie but server-side session resolution rejects it; a fresh login needs the new password. The test also proves one epoch increment and exactly one `PASSWORD_CHANGED` event plus pending notice per account. It does not retry ambiguous credential changes or send email. The test-only TLS bridge does not establish deployed ingress or CORS proof. The actual production application routers still return 404 on `POST /v1/identity/change-password`; public deployment, real browser UX and MFA/recovery approvals are open. See [ADR-0019](../adr/0019-browser-password-change-postgres-e2e.md).

## Candidate one-shot browser password-change experience — stacked Draft #57

An explicitly unmounted `PasswordChangePage` now renders static first-party `GET /account/change-password` using the **same** direct-TLS origin and browser session boundary as `BrowserPasswordChangeHTTP`. Its script/style are embedded with exact CSP hashes, no third-party resources or persistent storage. A real active session and session-bound CSRF must be fetched before the form becomes visible; the confirmation password remains client-only. A single deliberate click or keyboard form submit sends the existing strict `email`/`currentPassword`/`nextPassword` JSON request, clears all password input elements and locks the UI against automatic resend.

The page distinguishes confirmed `204` (successful; all sessions invalidated, sign in again), generic `401` (denied, reason undisclosed), and **any uncertain response or network exception** (might have committed; do not repeat the password change; use established trusted sign-in or recovery). No new login/recovery URL is invented or exposed. Real Chromium + PostgreSQL tests cover signed-out state, matching confirmation, certain commit, denial, and HTTP response lost **after** commit. Default production listeners still return 404 for both the candidate page and mutation. See [ADR-0020](../adr/0020-unmounted-password-change-recovery-page.md).

## Password recovery proof foundation — stacked Draft #58

Forgot-password recovery is **not** the already implemented authenticated password-change operation. A caller without a valid session and old-password proof must never be granted the existing v15 `PASSWORD_CHANGE` capability. A new isolated `PasswordRecoveryProof` primitive generates 32 CSPRNG bytes (43-character canonical base64url) and a purpose-specific SHA-256 digest with domain `mamony:identity:password-recovery-proof:v1\0`. Strict parsing rejects malformed, padded and noncanonical values; regular formatting and serialization conceal or reject proof material. A proof digest alone never authenticates, identifies an account, or permits a password reset.

**Nothing issues or consumes this proof in production.** No recovery request handler, token email, recovery database role/table/SQL function, reset UI, update-password operation, or application route has been added. The future protocol needs independent distributed admission, generic anti-enumeration responses, current verified-contact epoch checks, durable encrypted outbox delivery, expiry/attempt limits, single-use transactional redemption, all-session epoch invalidation, a separately provenance-checked `PASSWORD_RECOVERED` event/notice and MFA/assurance policy. The existing v16 `PASSWORD_CHANGED` trigger must not be bypassed. See [ADR-0021](../adr/0021-password-recovery-authority-foundation.md).

## Internal password recovery issuance — stacked Draft #59 / Identity v19

The `RECOVERY` admission action uses purpose-isolated HMAC buckets (global 60/min; client 5/min and 40/hour; email 2/hour and 4/day), charged in PostgreSQL before proof generation or account lookup. For **every admitted canonical email, including unknown aliases**, `RecoveryRequest` generates a fresh 256-bit purpose-separated bearer and invokes a trusted *injected* recovery envelope sealer. The unmounted usecase never returns a subject, challenge ID, eligibility flag or token to an external caller. No application HTTP listener exposes recovery endpoints.

The dedicated `mamony_identity_recovery_issuer_runtime` credential has only EXECUTE on v19's `issue_password_recovery` function (no direct table read/write). The function rechecks subject ACTIVE/current verified alias and its version/epoch under row locks, enforces per-subject DB-time cooldown and hourly/daily quotas, and atomically revokes the previous live challenge, stores only a digest with 15-minute expiry and enqueues its matching sealed recovery token. An absent/ineligible/quota-denied subject produces internal false without changing any challenge. Failed commits cannot leave partial queue writes or revoked old proofs.

The ciphertext queue deliberately has **no delivery worker, decrypt role, Mailgun adapter or live secret** yet. Test envelopes are mock bytes; actual production AEAD recovery-purpose AAD and key isolation remain unimplemented. Challenge redemption, Argon2id rotation, sessions invalidation, new `PASSWORD_RECOVERED` event and notification, MFA/assurance and anonymous uniform HTTP status/timing are future work. See [ADR-0022](../adr/0022-password-recovery-issuance-outbox.md).

## Recovery-purpose AEAD delivery — stacked Draft #60 / Identity v20

Recovery issuance from Identity v19 now has an explicitly constructed, **unlaunched** recovery delivery path. `RecoveryKeyring` supports a distinct active recovery key plus bounded read-old keys, sealing the canonical alias, `rcv_` challenge and purpose in AES-256-GCM AAD; it is not the EMAIL_VERIFY envelope. `RecoveryMailSender` accepts a pinned HTTPS `/account/recover-password` destination and sends only a text multipart message containing the recovery bearer in a URL fragment; disabled tracking, no redirects, no Host-derived reset URL, and sanitized provider errors. The reset page path is not mounted: this candidate **cannot be used for live emails or user recovery** yet.

Identity v20's `mamony_identity_recovery_delivery_runtime` holds only EXECUTE on `claim_password_recovery_delivery` and `finalize_password_recovery_delivery`, **no table access**. Claim locks and leases one due job, rechecks ACTIVE status, current verified email/version and subject epoch, remaining TTL, and unrevoked/unused proof; stale jobs become DEAD without revealing recipient or ciphertext to Go. Finalize fences by live 45-second lease with bounded retries 1/2/4/8 minutes and a five-attempt cap. Mailgun ACCEPTED is only provider acceptance, not delivery. Worker can never issue or redeem a proof or rotate password. Real PostgreSQL plus a local Mailgun HTTP mock validates 429 → 200 with one stable bearer; previous default routers remain 404.

See [ADR-0023](../adr/0023-password-recovery-delivery-worker.md). **Unresolved:** account recovery assurance/MFA, production encryption secrets, a safe reset page and atomic one-use redemption, session revocation, token-free `PASSWORD_RECOVERED` alert, provider operations, contact-change race after claim, PII retention, and public anti-enumeration. Do not merge or activate.

## Atomic account-recovery redemption — stacked Draft #61 / Identity v21

`PasswordRecoveryRedemption` accepts only a strict canonical purpose-specific 256-bit bearer, a canonical email for both contextual new-password policy and SQL **equality binding**, and a new password approved by the injected breach/common-password rejector; it hashes using the bounded Argon2id v1 adapter. No subject ID is caller-selected and no public handler, login or new session exists. Unknown/used/revoked/stale/expired challenge or assurance failure yields internal denial, not an explanation of account existence. A missing or failed KDF and uncertain COMMIT yield generic unavailable, never automatic retry.

Identity v21 introduces independent `mamony_identity_recovery_redeemer_runtime` with only EXECUTE on `redeem_password_recovery`. The owner-controlled SQL transaction locks current subject/verified alias, token, separate **default-DENY epoch-bound assurance grant** and verifier, revalidates DB-clock expiry and contact epoch, consumes challenge/grant exactly once, rotates salt+digest, invokes the existing v14 session epoch invalidator and records `PASSWORD_RECOVERED` audit plus token-free notice in one atomic commit. A separate trigger requires the original DB session user of the dedicated redeemer role for recovery notices; existing `PASSWORD_CHANGED` notice provenance stays restricted. Not-yet-sent proof deliveries are quarantined. All former sessions and grants are invalidated by epoch; fresh login and MFA/step-up rules remain mandatory.

**There is no trusted production writer for the assurance grant**, which is inserted **only by disposable test admin fixtures**. There is no functional forgot-password flow, recovery event notice worker, provider activation, reset UI, token redemption HTTP endpoint, production MFA approval or anti-enumeration envelope. CI tests use disposable PostgreSQL and locally generated Argon2id; green test evidence does not authorize activating recovery. See [ADR-0024](../adr/0024-atomic-password-recovery-redemption.md).

## PASSWORD_RECOVERED security alert — stacked Draft #62 / Identity v22

The v21 atomic password recovery transaction enqueues a distinct `password_recovery_notices` row. Identity v22 adds independent `RecoverySecurityNoticeWorker`, a fixed text-only `SendPasswordRecovered` Mailgun adapter, and strictly scoped SQL role `mamony_identity_recovery_notice_runtime`. The sender can only EXECUTE the claim/finalize functions (45-second lease, verified recipient recheck, 24-hour event freshness, max 5 attempts with 1/2/4/8-minute backoff). Every notice is **token-free and link-free**, tracking is disabled, and Mailgun acceptance stores only a SHA-256 provider message-ID digest. The password-change and recovery-proof sender roles remain separate. Only the original v21 `session_user` of the recovery redeemer can create a `PASSWORD_RECOVERED` notification through the audited trigger.

`mamony_identity_recovery_notice_monitor_runtime` is distinct and sees only five bounded counters: pending-due, dead, submitted-unconfirmed, retrying and expired-pending; never personal, account, bearer or provider-event details. A maintenance-only procedure purges up to 100 recipient-bearing notices older than seven days per invocation. Neither monitoring nor worker grants can modify account, verifier or challenge rows.

**No public activation**: this PR provides only an explicitly constructed, unlaunched Go worker; the recovery assurance approval issuer is intentionally unavailable in production. Mailgun accepted means provider submission, NOT inbox delivery. Signed recovery-specific delivery callbacks, operational suppression/bounce monitoring, human-approved assurance policy and reset UI remain separate release gates. See [ADR-0025](../adr/0025-password-recovery-security-notice-worker.md).

## Signed PASSWORD_RECOVERED Mailgun receipt reconciliation — stacked Draft #63 / Identity v23

`MailgunRecoveryNoticeWebhook` is an explicit **unmounted** handler that reuses the HMAC-SHA256 authentication, configured-domain check, strict timestamp limits, bounded JSON parsing and delivery-state normalization of the previous Mailgun notice verifier, but calls a **different** `RecoveryNoticeWebhookStore` backed only by `mamony_identity_recovery_notice_webhook_runtime`. This role has EXECUTE solely on `record_mailgun_recovery_notice_receipt`, with an original-session-user fence. The old `PASSWORD_CHANGED` webhook cannot use that function, and the recovery webhook cannot use the old function, read an address, or write a verifier. Only SHA-256 digests of signed token, provider event and canonical Message-ID plus a bounded state/timestamp are committed; event body and PII are never stored.

Receipt replay is deduplicated, including concurrent copies. The v23 revision to recovery-notice finalize reconciles early signed callback arrivals before a provider 200 acknowledgement, and later callbacks only advance the matching accepted recovery notice. Severity cannot regress: a late delivered callback never hides a permanent bounce or complaint. Unlike the v22 submitted-only state, `provider_state` can now be DELIVERED, TEMPORARY_FAILURE, PERMANENT_FAILURE, COMPLAINED or UNSUBSCRIBED. **These states are provider reports, not independent evidence of inbox access.** The legacy v17 queue/receipts are unchanged and remain inaccessible to the recovery-only callback.

The count-only monitor retains its original five-field function but corrects submitted-unconfirmed to exclude confirmed or failed states. A second function adds delivered, provider-failures and unmatched-receipts counts without exposing addresses or event identifiers. Maintenance-only cleanup removes at most 100 receipt digests older than seven days per call and preserves identifier-only audit. No webhook route, real Mailgun account, recovery approval authority or public forgot-password endpoint is enabled. See [ADR-0026](../adr/0026-signed-recovery-notice-webhook.md).

## Saved offline recovery codes and challenge-bound grant — Draft #64 / Identity v24

A saved recovery code is 256 random bits, generated via OS CSPRNG and encoded as exactly 43 base64url characters, with only a domain-separated SHA-256 digest stored. It must have been saved *before* an email recovery challenge was issued, under a separately reviewed authenticated enrollment ceremony. **No real enrollment ceremony or enrollment runtime role exists yet**; only disposable privileged fixture tests write offline code digests. The Go primitive redacts formatting, JSON and logging. An emailed token is **not** the offline code.

Identity v24 introduces an independent `mamony_identity_recovery_assurance_runtime` principal with only EXECUTE on `approve_password_recovery_offline_code(recovery_digest,offline_digest,audit_event)`; it may never read subject, token, offline-code, grant, notice or password tables. The SECURITY DEFINER function verifies original session_user and locks subject/alias, currently valid challenge and matching **already-enrolled, single-use, subject+contact+epoch-bound offline code**. Approval consumes the code, creates a five-minute (or challenge-expiry-bound, whichever is shorter) grant for the **exact challenge ID**, and writes `RECOVERY_ASSURANCE_APPROVED` identifier-only audit in one transaction. All denials are generic. A DB error/lost COMMIT response does not cause automatic retry.

The v21 password-recovery redemption function retains its existing arguments but now refuses grants whose `challenge_id` is not the exact locked recovery proof. This removes the ability to approve one challenge and redeem another on the same subject/epoch. Existing v21 privileged test grant fixtures now supply challenge_id explicitly. A migrated DB with old **unbound** grants must be reviewed before upgrade: v24 intentionally fails the NOT NULL FK migration instead of guessing or silently binding them.

No production code generation/enrollment, MFA ceremony, reissuance UX, strong-authenticated code rotation, public recovery route, risk-based AAL decision, HTTP rate/admission envelope, production notification delivery or independently protected DB owner exists. **Do not interpret this as AAL2/AAL3 compliance or permission to activate forgot-password.** See [ADR-0027](../adr/0027-offline-recovery-code-assurance.md).

## Saved recovery-code enrollment and rotation — stacked Draft #65 / Identity v25

Identity v25 adds **unmounted** `AuthenticatedRecoveryCodeManager` and a five-code (`5 × 256-bit`) generator. Before staging any grant, it checks an active opaque session and runs the existing admission-limited Argon2id reauthentication for the exact same current subject and epoch. The restricted `mamony_identity_auth_grant_runtime` stages a distinct **30-second one-use `RECOVERY_CODE_ROTATION`** grant, which neither creates sessions nor changes password verifiers. The isolated `mamony_identity_recovery_code_manager_runtime` has only EXECUTE on one owner-controlled code-set replacement transaction, no direct table privileges. Raw recovery codes are never persisted or logged; returned `RecoveryCodeSet` prevents accidental serialization and is exposed only through explicit trusted display following known commit.

**Bootstrap is default-DENY:** session plus current password are NOT independent MFA. The SQL transaction also requires a separate five-minute `password_recovery_code_enrollment_approvals` proof, subject/epoch bound, issued through an independent authority. **No production approval issuer, INSERT grant, or live secure browser flow exists.** Only disposable ADMIN fixtures insert synthetic approvals. Later rotation requires a still-unconsumed saved offline recovery code of the current subject/contact/epoch, plus the same valid session/password and purpose-specific grant. Replacing codes deletes the former set atomically, inserts exactly five unique digests, invalidates outstanding email recovery challenges, quarantines pending proof delivery, consumes pending recovery approvals and emits an immutable `RECOVERY_CODES_ENROLLED` or `RECOVERY_CODES_ROTATED` security event.

A dedicated `password_recovery_code_notices` outbox stores a **token-free** alert obligation for the current verified email in the same transaction, but **has no live worker or provider sender**. This is NOT proof of delivered account-security notice. An ambiguous commit withholds raw codes; there is no automatic retry or public enrollment/reset route. The policy for strong factor enrollment, lost-all-codes users, authenticated recovery-code reissue, secure one-time display and notice delivery still requires explicit owner approval. See [ADR-0028](../adr/0028-recovery-code-enrollment-rotation.md).

## Isolated offline recovery-code security notice worker — Draft #66 / Identity v26

Recovery-code enrollment and rotation produce immutable `RECOVERY_CODES_ENROLLED` and `RECOVERY_CODES_ROTATED` events under the v25 recovery-code manager transaction, plus a separate `password_recovery_code_notices` obligation. Identity v26 provides a **separately credentialed**, explicitly unmounted `RecoveryCodeSecurityNoticeWorker` that selects only those two provenance-checked event types; neither v17 `PASSWORD_CHANGED` nor v22 `PASSWORD_RECOVERED` notification worker can claim it. Its `mamony_identity_recovery_code_notice_runtime` has schema USAGE and EXECUTE only on `claim_recovery_code_security_notice` and `finalize_recovery_code_security_notice`, with `session_user` original-login check, **no direct SELECT/UPDATE on subject, account, code, session or notification tables**. A distinct `mamony_identity_recovery_code_notice_monitor_runtime` reads only `recovery_code_security_notice_health()` (five aggregate counts).

Claim rechecks current verified address, event provenance and a 24-hour freshness bound before sending. Workers use lease fencing, `SKIP LOCKED`, 45-second leases, 10-second bounded provider sends, five attempts and 1/2/4/8-minute retries. The Mailgun HTTP sender accepts only the two fixed event kinds and sends **text-only, code-free, link-free** created/replaced alerts via allowlisted region+domain, TLS, disabled redirects and tracking, fixed sender and sanitized errors. Provider 200 must include a valid Mailgun message-id; its canonical SHA-256 digest is stored, not the raw ID. **ACCEPTED/SUBMITTED_UNCONFIRMED is not proof of delivered mail**; signed delivery, bounce, complaint and unsubscribe receipts have not yet been wired for this queue. An ambiguous response could result in a duplicate token-free notice.

The independent monitor exposes only `pending_due`, `dead`, `submitted_unconfirmed`, `retrying` and `expired_pending`. Maintenance-only cleanup removes up to 100 notices older than seven days per call, erasing recipients while preserving identifier-only audit. This worker **does not start automatically** and has no live provider, public endpoint, approval issuer, factor enrollment ceremony or delivery webhook. See [ADR-0029](../adr/0029-recovery-code-security-notice-worker.md).

## Signed recovery-code security notice webhook and provider reconciliation — stacked Draft #67 / Identity v27

Identity v27 adds a separate **unmounted** `MailgunRecoveryCodeNoticeWebhook` built on the previously reviewed bounded HMAC-SHA256, timestamp, domain and event-kind validator. It handles **only recovery-code ENROLLED/ROTATED notification provider receipts**, not password-change, `PASSWORD_RECOVERED`, password reset, initial enrollment approval or authentication. A distinct `mamony_identity_recovery_code_notice_webhook_runtime` SQL role has only EXECUTE on `record_mailgun_recovery_code_notice_receipt` and does **not** read or update any table directly. The original `session_user` check rejects SET ROLE origin confusion; the worker's send role and count-only monitor cannot invoke this recorder. No webhook is mounted to ingress, and no live signing keys or provider configuration are activated.

Signed events are reduced to SHA-256 **token/event/Message-ID digests**, normalized delivery state and provider event time, stored in `recovery_code_notice_mailgun_receipts` with token/event replay uniqueness. Valid callbacks may arrive **before** the v26 sender receives a Mailgun ACK: the owner-controlled sender finalize correlates a pre-ACK digest with the accepted Message-ID. Later callbacks can update **only matching accepted recovery-code notices**; delivery ranks are monotonic `SUBMITTED → TEMPORARY_FAILURE → DELIVERED → PERMANENT_FAILURE → COMPLAINED → UNSUBSCRIBED`. Out-of-order lower-rank events never mask a bounce or complaint. A provider 200 response alone is `SUBMITTED` and is **not inbox delivery**.

The v26 five-field `recovery_code_security_notice_health()` ABI remains unchanged; submitted_unconfirmed excludes fully reconciled notices. Separate `recovery_code_security_notice_delivery_health()` yields **delivered/provider_failures/unmatched_events** as aggregates only. Maintenance alone can purge up to 100 digest-only receipts older than seven days while preserving immutable security events. This feature is not launched and has no independent AAL/MFA issuer, safe user-facing backup code enrollment, delivery process/ingress wiring, suppression operations or production risk signoff. See [ADR-0030](../adr/0030-recovery-code-notice-signed-webhook.md).

## Independently pre-provisioned cryptographic factor — stacked Draft #68 / Identity v28

Identity v28 creates an **unmounted**, purpose-bound **Ed25519 signed challenge** whose only function is authorizing the **first** enrollment of five saved offline recovery codes. Unlike v25's synthetic admin-approved fixture, the v28 normal path can create a five-minute, one-use independent approval **only after proof of possession of a separately pre-provisioned Ed25519 private key**, together with an active subject-bound session, a fresh admission-limited Argon2id password verification and a dedicated v25 `RECOVERY_CODE_ROTATION` one-use grant. The isolated `mamony_identity_recovery_factor_issuer_runtime` consumes the grant under DB locks and creates a random 256-bit nonce challenge (three minutes). The separate `mamony_identity_recovery_factor_verifier_runtime` reads only current challenge+key through an owner-controlled function, checks Ed25519 signature **in trusted Go**, charges invalid attempts (five max), then commits a one-use approval digest and identifier-only `RECOVERY_FACTOR_APPROVED` audit atomically. v25 still independently rechecks session, password, subject, verified contact and epoch before consuming the approval to generate exactly five recovery codes. Standard formatting and JSON cannot reveal raw approval proofs.

**The independent public key MUST already be bound by a separate trusted process.** No production role can INSERT or change `recovery_code_factor_keys`; no key-binding ceremony, passkey/WebAuthn RP ID/origin, real hardware/user-verification assurance, external device management, browser or live endpoint is implemented. This means recovery-code first enrollment **still defaults to DENY for ordinary unprovisioned accounts**. Current tests use an intentionally privileged disposable ADMIN to provision a synthetic Ed25519 test key; this is **not** a user production registration authority or certified MFA/AAL2/AAL3 claim. The verifier credential itself is a sensitive trust root capable of approving an issued challenge if compromised; SQL grants do not substitute for isolating trusted Go signature verification. Signature payload binds purpose, challenge ID, subject ID, epoch and nonce, and cannot authorize other actions. Replay, expired and epoch-stale challenges fail. See [ADR-0031](../adr/0031-independent-factor-signed-enrollment.md).

## Initial recovery-code factor revocation fence — stacked Draft #69 / Identity v29

The one-use Ed25519 v28 approval is **not enough** at the later v25 recovery-code bootstrap commit: a key may have been revoked or rebound after signing. Identity v29 records the verified public key in the pending approval (never a private key/signature), and the existing v25 transactional code manager locks and verifies the **currently active, unrevoked, same subject/epoch/contact public key** before consuming the approval. Historical approvals without this binding fail closed; no password-only repair. The existing offline-code rotation path is unchanged. See [ADR-0032](../adr/0032-recovery-factor-revocation-fence.md). A signed approval must not be treated as a perpetual authenticator grant.

No production authenticator registration/revocation ceremony, key custody, WebAuthn RP/origin/user verification, HTTP path, approval-device UX or operational incident policy exists. Revocation **after** committed code enrollment does not automatically invalidate saved codes. All work is Draft/unmerged and default-deny for ordinary unprovisioned users.

## WebAuthn RP configuration candidate — stacked Draft #70 / ADR-0033

A new immutable internal `PasskeyRelyingParty` candidate rejects all RP/origin configurations except a canonical lower-case ASCII DNS name and its **exact** `https://<rp-id>` origin. This conservative first-party profile does not support broader parent RP scope, subdomains, related origins, HTTP, IP/localhost, arbitrary ports or dynamic origin selection. `httpedge` regression tests ensure hypothetical registration/login endpoints remain 404 on both listeners. See [ADR-0033](../adr/0033-webauthn-relying-party-pre-enrollment.md).

This is **only configuration validation**: there is NO WebAuthn library verifier, attestation/assertion, challenge store, credential owner/UV state, trusted prior-key enrollment, HTTP endpoint, runtime enrollment permission, passkey authentication, notification, AAL or phishing-resistance claim. In particular, v28's custom Ed25519 signature is not WebAuthn and is not automatically a trusted passkey registration source. A session/password/email-only registration, even if chosen for ordinary AAL1 use later, must never be silently promoted to the independent factor required by saved recovery-code bootstrap. Ordinary factor loss and suspected full-account compromise need distinct recovery-code incident policies.

## Passkey registration Begin-only candidate — Identity v30 / stacked Draft #71

Identity v30 adds **durable** `passkey_registration_intents` (one current challenge per subject; random 32-byte nonce, exact digest-only session binding, epoch/contact, immutable RP ID/origin, fixed config generation/purpose, DB-clock three-minute TTL). The role `mamony_identity_passkey_registration_issuer_runtime` can only invoke a narrowly fenced challenge issuer; it cannot read the table, approve/consume a challenge, create passkey credentials, prebind recovery factor keys or bypass a preexisting authentic independent factor. Current HTTP routes stay unmounted. See [ADR-0034](../adr/0034-webauthn-registration-intent.md).

**SECURITY:** Begin only establishes public ceremony material; possession of a valid login session can request a challenge but **cannot register a passkey** in this PR. There is **no** WebAuthn library verifier, attestation, credential table, trusted independent binding, registered/passkey login, mandatory registration notice or NIST assurance claim. SQL does not treat the caller's RP ID as an ownership proof; future finish must revalidate trusted runtime origin/RP/policy. Enrollment of offline recovery-code independent factors remains blocked without previously trusted provisioning.

## WebAuthn registration evidence reader — Identity v31 / stacked Draft #72

Identity v31 permits a **distinct internal EXECUTE-only reader role** to retrieve server-expected registration challenge and RP policy generation only when the same original live session, subject epoch, verified contact, purpose and unexpired/nonconsumed pending intent still match. The Go reader independently checks session identity and the **current exact** configured RP ID/origin; a stale policy or mismatched RP denies. See [ADR-0035](../adr/0035-webauthn-registration-evidence-reader.md).

This is **expected context, not proof**. Read does not verify clientDataJSON, authenticator data, signature, UV, attestation or credential uniqueness, consume the challenge, create a credential or authorize trusted independent recovery enrollment. The candidate go-webauthn v0.18.2 library has been evaluated, **not imported**. A separately controlled transaction must consume+bind only after full maintained-library protocol verification and independent binding policy approval. All public routes remain 404; migration/role remains stacked Draft/unmerged.

## WebAuthn protocol verification candidate — stacked Draft #73

The Identity protocol adapter `internal/modules/identity/internal/repository/webauthn` uses **pinned go-webauthn v0.18.2** to parse a bounded registration response (max 128 KiB) and verify the original **server-owned v31 expected challenge**, exact RP ID/origin, `webauthn.create` ceremony, user presence + **required user verification**, attested credential ID and supported COSE algorithms. The usecase `PasskeyRegistrationProofVerifier.VerifyOnly` first resolves the original live session/intent/subject via v31, then delegates to this adapter. Synthetic ES256/`fmt:none` tests reject wrong nonce, wrong origin/RP hash, cross-origin, wrong ceremony, missing UP/UV, mismatched ID and malformed/oversized payload. See [ADR-0036](../adr/0036-webauthn-registration-protocol-verifier.md).

**IMPORTANT: successful protocol validation is not registration authorization.** The adapter exposes only an in-memory candidate and has **no** DB credential-writing capability, finish/consume function, audit, role privilege or HTTP route. An attestation format `none` provides no proof of trusted device provenance or independent account-binding permission; session/password-only registration MUST NOT become a trusted offline recovery factor. The future Begin options must match the verifier's explicit ES256/EdDSA/RS256, UV and extension-free session contract (or persist full library SessionData). A separate atomic finish must recheck challenge/epoch/RP policy under locks and write credential + mandatory notices only after independently approved binding admission. CI success is not a production enrollment claim.

## Passkey registration password admission — Identity v32 / stacked Draft #74

A **separate, unmounted, EXECUTE-only admission role** may mark one v30 passkey registration intent as password-reauthenticated only after Go rechecks the live bearer subject against a **fresh, pre-KDF-budgeted Argon2id current-password verification**. A 30-second, single-purpose `PASSKEY_REGISTRATION_REAUTH` grant is staged by the existing trusted auth-grant role and consumed in one PostgreSQL function under subject/alias/session/intent row locks and the DB clock. The resulting record binds the exact intent ID, session digest, subject epoch, verified contact, expiry (no later than the original three-minute nonce), and fixed **`PASSWORD_REAUTH_ONLY` provenance**. A different session of the same subject, wrong password, cross-purpose login/change grant, replay, expired challenge, stale epoch or already approved intent is denied. See [ADR-0037](../adr/0037-passkey-registration-password-reauth-admission.md).

**No passkey has been registered.** V32 does not consume the WebAuthn challenge, create any credential, passkey login, independent recovery-factor approval, MFA guarantee, security notice or public endpoint. Reauth admission is not sufficient for a financial high-assurance enrollment or a recovery-factor bootstrap. A separately reviewed and transactionally fenced `finish` must recheck both v30/v31 nonce and v32 admission alongside WebAuthn protocol evidence, global credential ID uniqueness and atomic security audit/mandatory notice before any account binding. All four PRs remain stacked Draft/unmerged.

## Atomic passkey registration staging — Identity v33 / stacked Draft #75

The internal **unmounted** `PasskeyRegistrationStager` receives exact v31 evidence and a complete verified go-webauthn registration response in the same call. A dedicated EXECUTE-only v33 role then transactionally rechecks the original nonce, session/epoch, exact RP origin, and a valid **PASSWORD_REAUTH_ONLY** v32 admission under subject/alias → session → intent → admission locks. A successful commit consumes both challenge and admission, stores a globally unique credential ID and COSE public key with fixed **PENDING_ASSERTION** status, writes an identifier-only `PASSKEY_REGISTRATION_STAGED` audit event, and enqueues **one PENDING security notice** through an origin-gated trigger. Collision or notice failure rolls back the entire commit. See [ADR-0038](../adr/0038-webauthn-atomic-provisional-registration.md).

**No usable passkey exists.** A `fmt:none` registration response does not independently prove ownership of the private key or high-assurance device provenance. V33 exposes no credential activation, login assertion, registration HTTP route, independent recovery factor, or security email sender. `PENDING` is a durable notification obligation, **not a delivered notice**. Before production, a second authenticator assertion must establish possession and appropriate admission, Begin creation-options congruence must be implemented, and a separate notice worker and browser/operations controls require review. All stacked PRs remain Draft/unmerged.

## Passkey activation challenge and expected evidence — Identity v34 / stacked Draft #76

V34 introduces an **unmounted, EXECUTE-only, one-challenge-per-subject** `PASSKEY_ACTIVATION_ASSERTION` nonce issuer and a **separate EXECUTE-only reader** for its expected authenticator assertion context. The nonce is unique, tied to the exact original live session, subject epoch, contact version, v33 `PENDING_ASSERTION` credential, its previously consumed v30 registration intent and v32 `PASSWORD_REAUTH_ONLY` admission, and its queued notice/audit event. It expires within **3 minutes** and at most **10 minutes after staging**. While unexpired, a second issue cannot silently supersede it. The reader exposes only matching server-owned nonce, credential ID, public COSE key, AAGUID, counter and backup flags with pinned RP ID/origin. A newer v30 registration intent invalidates the older pending credential's activation eligibility instead of reusing stale approval. See [ADR-0039](../adr/0039-passkey-activation-challenge-evidence.md).

**No assertion signature has been verified and no passkey is activated.** The new issuer/reader cannot consume this challenge or change the v33 state `PENDING_ASSERTION`; there is no public route or login/recovery entitlement. A separate maintained WebAuthn assertion verifier and separately reviewed atomic activation transaction are required, with browser options, rechecked session/state/nonce, mandatory notice and independently controlled trust policy. `PENDING` security notices remain queued, not delivered.

## Signed passkey activation assertion verifier — Identity v35 / stacked Draft #77

The internal **unmounted** `PasskeyActivationAssertionVerifier` retrieves the still-live v34 expected challenge and pending credential via the already isolated EXECUTE-only reader; it validates a bounded `webauthn.get` response using the pinned **go-webauthn v0.18.2** library `ParseCredentialRequestResponseBytes` and `ValidateLogin`. It reconstructs the exact expected DB challenge, RP ID/origin, subject/user handle and only allowed credential ID with the original COSE public key. Verification checks real P-256/ES256 signature (and other library-supported keys), UP, **required UV**, challenge/type/origin/RP hash, credential ID/user handle, BE/BS consistency, and explicit counter clone-warning denial. Zero/zero counters remain permissible but offer no clone detection. The verified result is a defensive copy, not a credential or authentication grant. See [ADR-0040](../adr/0040-passkey-activation-signed-assertion-verifier.md).

**No nonce consumption or passkey activation exists in v35.** `VerifyOnly` may accept replayed valid signatures while v34 challenge remains available; v36 must atomically consume the same challenge with the state/counter transition and mandatory audit/notice under fresh locks. The v33 credential remains `PENDING_ASSERTION`; there is no passkey login, independent factor or browser endpoint. A real security-notice worker remains a separate production gate.

<a name="bounded-synthetic-passkey-possession--identity-v36-unmerged-candidate-81"></a>

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
