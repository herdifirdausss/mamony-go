# Identity password-change and security-notice history

This record preserves the original contract notes and decision text for PRs [#50](https://github.com/herdifirdausss/mamony-go/pull/50) through [#57](https://github.com/herdifirdausss/mamony-go/pull/57), which merged as internal synthetic work. Draft labels and evidence gaps describe the cited slice. Later slices add the local Mailgun mock, browser handler, and Chromium coverage within their recorded scopes.

Start with the [Identity contract](../contracts/identity.md#authenticated-password-change-and-security-notices) for the merged scope, the [browser runbook](../runbooks/identity-browser-e2e.md#password-change-browser-evidence) for current test coverage, and the [release decision register](../open-decisions.md#identity-password-change-release-decisions) for approval status.

## Contract slice records

### Internal authenticated password change — stacked Draft #50

Candidate v15 adds `AuthenticatedPasswordChange.Change`, which requires a currently usable opaque session **and** fresh old-password verification through mandatory distributed pre-KDF login admission + Argon2id, bound to the same subject and epoch. The new secret is NFC-normalized, checked against the required configured password blocklist, denied if equal to the old normalized secret, and rehashed with a new 128-bit salt. A purpose-bound one-use `PASSWORD_CHANGE` grant (30s) cannot be consumed by the login session issuer. A new EXECUTE-only `mamony_identity_password_change_runtime` can rotate the verifier only with both session and change proof; mutation, v14 `session_epoch` increment and identifier-only `PASSWORD_CHANGED` security event commit atomically. No replacement bearer is issued. See [ADR-0013](../adr/0013-authenticated-password-change.md).

No change-password public endpoint, session cookie/CSRF mutation handler, notification delivery or approved production blocklist is included. The grant stager remains a privileged assertion oracle under Profile A. Owner/security/UX approvals, MFA/step-up, trusted client network and workload secret isolation remain blockers.

### Password-change notification obligation — stacked Draft #51

The v15 trusted `PASSWORD_CHANGED` security outbox event now causes a **v16 same-transaction enqueue** in `password_change_notices`; a failed enqueue rolls back the password change. The identifier-only security outbox remains free of email addresses. Only events written under the original `mamony_identity_password_change_runtime` PostgreSQL login can trigger a notice; earlier verifier role outbox INSERT privileges cannot forge an alert. The separate queue captures the currently verified email at the time of mutation; it contains only event ID, subject, recipient and delivery metadata, never a bearer/password/reset URL.

The isolated `mamony_identity_security_notice_runtime` may EXECUTE only claim/finalize functions (not read raw tables, alter Identity state, mint grants or issue sessions). Due notices are claimed with SKIP LOCKED and 45-second fences; retry is capped at five submissions with 1/2/4/8 minute backoff. A contact change or unverified/DELETED recipient before claim marks the notice DEAD to prevent sending to an unbound address. Provider ACCEPTED never means inbox delivered; duplicate sends may occur after worker crashes. Seven-day fixed retention deletes recipient rows while preserving the security event. Refer to [ADR-0014](../adr/0014-password-change-security-notices.md).

No live sender/worker command is activated and no public password-change route exists. Sender is a mockable interface only in this candidate; no user notification can yet be claimed. Remaining policy includes operational delivery monitoring, a trusted provider sender, bounce handling and escalation when stale notices are suppressed.

### Password-change Mailgun security notice sender — stacked Draft #52

The internal `SecurityNoticeSender` abstraction from Draft #51 now has a real Mailgun Messages API implementation limited to the configured US/EU/AU region and verified sending domain. It submits a **fixed text-only password-changed warning**, not a verification token, session, grant, reset URL or user-selected text. No links or dynamic metadata; open/click tracking disabled and Mailgun recipient TLS required. HTTP response/error bodies are never exposed; a successful bounded 200 JSON response means **accepted by provider**, not delivered or read. HTTP 429/420/408/5xx and uncertain outcomes retry under the v16 PostgreSQL bounded lease/backoff; duplicates remain possible after an ambiguous acceptance.

A new dedicated `RunSecurityNoticeDelivery` facade and standalone `cmd/mamony-identity-security-notice-worker` use **only** the `mamony_identity_security_notice_runtime` role; they do not reuse the verification email sender, token envelope keyring or worker DSN. Real email sending needs both `--send-live-security-mailgun` and the exact operator approval, external owner-only secret files, non-CI runtime and verified-full PostgreSQL TLS. **The command is not scheduled/deployed, no real credentials have been connected and no live email has been sent by this implementation.** See [ADR-0015](../adr/0015-mailgun-password-change-notice-worker.md).

### Signed Mailgun notice callbacks and operator counts — stacked Draft #53

Identity v17 extends the password-change notification queue with **SHA-256 of the normalized Mailgun Message-ID** returned only after a successful Mailgun send response. The sender no longer calls an uncorrelated three-argument SQL finalizer: the worker role may only execute the new four-argument `finalize_password_change_notice_v2`. `ACCEPTED` means submitted to provider; independent `provider_state` tracks `SUBMITTED`, `TEMPORARY_FAILURE`, `DELIVERED` (recipient SMTP server only), `PERMANENT_FAILURE`, `COMPLAINED`, and `UNSUBSCRIBED`.

The **unmounted** HMAC-verifying Mailgun webhook candidate accepts only signed JSON event callbacks from the configured sending domain, fixed failure types and a verified Mailgun message-ID. It uses a signing key **different from the API sending key**. A separate `mamony_identity_notice_webhook_runtime` role may only EXECUTE a receipt function; it cannot read tables, access credentials, issue sessions, or alter password verifiers. Replay tokens and Mailgun event IDs are deduplicated in PostgreSQL using digests; receipt payloads store no email/PII. Out-of-order events are durably reconciled, and a later bounce/complaint cannot be hidden by an older delivery callback. A count-only health function is exposed to the isolated maintenance operator role, not the HTTP runtime.

A signed callback cannot be treated as full-body cryptographic attestation: Mailgun signs `timestamp+token`, not the entire JSON. Transport/TLS, domain binding, role isolation and real provider-key ownership must be reviewed before enabling a live endpoint. **No route is registered, no webhook receiver is deployed, no real Mailgun callback is processed and no on-call alert is configured.** Prior uncorrelated ACCEPTED notices cannot be automatically backfilled; ambiguous sender timeouts may produce unmatched provider receipts and duplicate messages. See [ADR-0016](../adr/0016-mailgun-security-notice-webhook-reconciliation.md).

### Read-only Mailgun notice health probe — stacked Draft #54

Identity v18 introduces the isolated `mamony_identity_notice_monitor_runtime` PostgreSQL LOGIN role, with **only** schema USAGE and EXECUTE on the count-only `security_notice_health()` function. It has no retention purge, mail delivery, webhook receipt, contacts, session, password or table permissions. `ReadSecurityNoticeHealthAsMonitor` exclusively accepts this role; the earlier maintenance-only facade remains backwards-compatible.

The unmounted, one-shot `mamony-identity-notice-health` command requires a private DSN file, strict full TLS verification, `--check-security-notices`, five mandatory numeric thresholds, and a non-CI operator environment. It writes only count-only JSON and exits 0 OK, 2 ALERT (configured threshold exceeded), or 1 ERROR (misconfiguration/DB unavailable). No default production thresholds, on-call routing, scheduler or public HTTP health endpoint exist. Counts can report suspected problems, not verify human inbox receipt. See [ADR-0017](../adr/0017-read-only-notice-health-probe.md).

### Unmounted browser password-change boundary — stacked Draft #55

The candidate `BrowserPasswordChangeHTTP` provides a separately constructed but **unmounted** `POST /v1/identity/change-password`. It reuses the existing concrete browser-session handler's exact HTTPS Origin/Host, direct-TLS, no-forwarded-identity, trusted peer, host-only cookie and 15-minute session-bound CSRF requirements. Bounded, duplicate-key-rejecting JSON has exactly `email`, `currentPassword`, `nextPassword`; the email must be canonical, but is **not** trusted as the account authority. A concrete `AuthenticatedPasswordChange` usecase independently verifies the old secret against the session's subject and epoch before the SQL atomic rotation.

On certain success, the response is 204 with the old cookie cleared; v14 SQL epoch invalidation has already revoked all previous sessions, and there is **no replacement session**. Wrong secret, account mismatch, throttle or new-secret policy denial collapse to a generic 401. Unknown commit outcome returns 503 and never claims success or clears the cookie, even if PostgreSQL might have committed. Response headers use no-store and no password or session material is emitted.

This is **not an enabled public or private route**: application routers and `main` remain unchanged, and no real-browser + PostgreSQL test of this new operation has yet passed. MFA/step-up, production password blocklist, live email sender/webhook, trusted ingress, account recovery, user-facing UX and finance authorization remain open. See [ADR-0018](../adr/0018-first-party-password-change-http-candidate.md).

### Actual Chromium password-change proof — stacked Draft #56

The previously unmounted `BrowserPasswordChangeHTTP` candidate is now exercised with real Chromium cookie handling, actual Argon2id, isolated PostgreSQL runtime roles and v15–v18 transactional state in `TestIdentityZBrowserPasswordChangeE2E` (tags `integration,browser_e2e`). Two independent browser sessions on the same account prove a successful password change invalidates both, while the initiator's host-only cookie is cleared and only the new credential can create a fresh session. Denied CSRF, oversized/duplicate JSON, sibling/cross-site requests, and a different account's valid password do not authorize the rotation.

A second scenario deliberately drops the HTTP response **after** PostgreSQL commit: Chromium retains the old cookie but server-side session resolution rejects it; a fresh login needs the new password. The test also proves one epoch increment and exactly one `PASSWORD_CHANGED` event plus pending notice per account. It does not retry ambiguous credential changes or send email. The test-only TLS bridge does not establish deployed ingress or CORS proof. The actual production application routers still return 404 on `POST /v1/identity/change-password`; public deployment, real browser UX and MFA/recovery approvals are open. See [ADR-0019](../adr/0019-browser-password-change-postgres-e2e.md).

### Candidate one-shot browser password-change experience — stacked Draft #57

An explicitly unmounted `PasswordChangePage` now renders static first-party `GET /account/change-password` using the **same** direct-TLS origin and browser session boundary as `BrowserPasswordChangeHTTP`. Its script/style are embedded with exact CSP hashes, no third-party resources or persistent storage. A real active session and session-bound CSRF must be fetched before the form becomes visible; the confirmation password remains client-only. A single deliberate click or keyboard form submit sends the existing strict `email`/`currentPassword`/`nextPassword` JSON request, clears all password input elements and locks the UI against automatic resend.

The page distinguishes confirmed `204` (successful; all sessions invalidated, sign in again), generic `401` (denied, reason undisclosed), and **any uncertain response or network exception** (might have committed; do not repeat the password change; use established trusted sign-in or recovery). No new login/recovery URL is invented or exposed. Real Chromium + PostgreSQL tests cover signed-out state, matching confirmation, certain commit, denial, and HTTP response lost **after** commit. Default production listeners still return 404 for both the candidate page and mutation. See [ADR-0020](../adr/0020-unmounted-password-change-recovery-page.md).

## Decision records

### Draft #50 — authenticated internal password change, purpose-bound grant

Proposed v15 introduces a separate **PASSWORD_CHANGE** one-use authentication grant and **mamony_identity_password_change_runtime** SQL role. The trusted Go usecase requires an active opaque session, pre-KDF admission and real old-password Argon2id comparison for that same subject, then mandatory new-password policy/blocklist, fresh salt/hash and a short-lived epoch-bound change grant. Its single SQL transaction checks current session, purpose/epoch and status after locks, rotates the verifier, increments epoch through v14 trigger and inserts one identifier-only `PASSWORD_CHANGED` outbox event. The old bearer and any prior grants are invalid immediately after commit. See [ADR-0013](../adr/0013-authenticated-password-change.md).

**Open:** owner approval of allowed password-change proof factors and old-session UI handling; real production compromised-password blocklist; MFA/recovery/step-up and password history; CSRF/Origin/TLS for a future unmounted route; outbox delivery and user notification; lock/contention measurements; secret/role workload isolation; and financial authorization. None are fulfilled by passing CI. No merge or route activation.

### Draft #51 — durable password-change notification queue (no live send)

Proposed v16 atomically enqueues an email **obligation** from trusted `PASSWORD_CHANGED` events in a separate queue, with original DB session-user provenance to block verifier-role event forgery. A restricted worker credential can only claim/finalize leased notices, with five bounded attempts, recipient-alias recheck, 24h eligibility, and seven-day PII erasure. The identifier-only audit row survives. Existing mail verification delivery is not reused for password notices, preventing token-bearing challenge/ACTIVE-account mixups. See [ADR-0014](../adr/0014-password-change-security-notices.md).

**Still open:** actual provider transport (the new SecurityNoticeSender is mock-only), opt-in standalone process/bootstrap, sending-domain policy, bounces/complaints and provider retry-after, delivery monitoring and escalation for stale contact/dead letters, backup/WAL erasure, suppression policy, PKI/secret/workload isolation, and owner approval. Passing SQL and Go CI does not mean any email is sent. No merge/public activation.

### Draft #52 — token-free Mailgun notice sender and explicit worker

ADR-0015 adds a region-pinned Mailgun sender for `PASSWORD_CHANGED` notices, separate from encrypted verification tokens, and a standalone opt-in worker with owner-only credential files and `mamony_identity_security_notice_runtime` SQL role. A real PostgreSQL + local HTTP mock test proves retry from 429 to 200 and correct event audit retention. Green CI does **not** claim a user received an email: the binary has not been deployed, enabled or given Mailgun credentials.

**Open:** verified Mailgun sending domain and regional DPA, SPF/DKIM/DMARC, dedicated domain sending key and rotation, operational worker deployment, provider bounce/complaint/receipt webhook authenticity, monitored DEAD or suppressed notice escalation, provider quotas, production PostgreSQL TLS/load, network egress and secret isolation, no-link warning UX wording review, owner release approval, MFA/recovery and protected user-facing password-change route. See [ADR-0015](../adr/0015-mailgun-password-change-notice-worker.md). Keep Draft; no public endpoint activation.

### Draft #53 — provider response correlation, signed webhook receipt and operator health

Proposed Identity v17 saves **only SHA-256 digests** of normalized Mailgun message IDs, signing tokens and event identities. The unmounted receiver validates Mailgun webhook HMAC with its **distinct** signing key, validates origin domain and 9h replay window, deduplicates receipts, and reconciles callback-before-sender-ack races. A separate webhook SQL role has one EXECUTE-only receipt function, and maintenance can read PII-free counts for due/dead/not-confirmed/SMTP-delivered/provider-failed/unmatched notices. No sender HTTP route or production webhook route was activated.

**Open:** production HTTPS webhook ingress and trusted network path, signing key loading and rotation, Mailgun account/subaccount domain verification, anti-abuse and monitoring, bounce/complaint suppression, provider API reconciliation when webhooks are missed, SLA alert thresholds/on-call ownership, exactly-once limitations, legacy ACCEPTED records without message-id, ambiguous send responses, provider security/privacy review and production permission approvals. See [ADR-0016](../adr/0016-mailgun-security-notice-webhook-reconciliation.md). Do not merge, launch external services or enable public routes.

### Draft #54 — count-only monitoring with a dedicated read-only credential

Identity v18 adds a separate `mamony_identity_notice_monitor_runtime` role (schema USAGE + EXECUTE aggregate only; no purge, notice delivery, webhook writes or direct PII queries) and an explicit one-shot `mamony-identity-notice-health` CLI. Mandatory per-metric thresholds produce a redacted JSON `OK`/exit 0, `ALERT`/exit 2 or `ERROR`/exit 1. CI exercises real role-separated PostgreSQL and deterministic synthetic threshold tests. This does **not** activate monitoring, cron, paging, inbound/outbound Mailgun, recovery or public Identity HTTP.

**Open:** operator-approved thresholds and initial baseline, false-positive policy for due notices, scheduler/orchestration, monitoring sink, on-call ownership/escalation and incident response, secret rotation/least-privilege deployment, reconciliation of missed Mailgun events, and public ingress release approval. See [ADR-0017](../adr/0017-read-only-notice-health-probe.md).

### Draft #55 — first-party browser password-change candidate (unmounted)

An isolated handler now exercises the existing `AuthenticatedPasswordChange` operation behind exact same-origin HTTPS browser requirements, session-bound HMAC CSRF, a strict 4096-byte JSON body and canonical alias. The form cannot choose a subject: the active cookie's owner must match the old-password's authenticated subject/epoch. Certain commits clear the browser cookie and require a separate login; denied password/policy/quota outcomes use the same generic HTTP response, and ambiguous SQL commits cannot claim success. No public/private router registration, migration or deployed listener was changed. See [ADR-0018](../adr/0018-first-party-password-change-http-candidate.md).

**Unresolved before enabling:** real Chromium + disposable PostgreSQL end-to-end change-password test (including lost response), owner UX/consent and lockout guidance, account-specific password policy feedback without enumeration, approved breached-password blocklist, independent client/edge abuse controls, MFA/step-up/recovery, browser/CSRF secret rotation, production ingress TLS/proxy and workload isolation, notification deliverability and operational escalation, and financial action authorization. Keep the PR Draft/unmerged.

### Draft #56 — real browser/password-change atomicity and lost response

A disposable `httptest.NewTLSServer` + Playwright Chromium + restricted PostgreSQL role fixture now covers the **unmounted** password-change HTTP candidate end-to-end. Both successful and dropped-after-commit responses are exercised; a lost response cannot restore an epoch-stale cookie or make an old password valid. SQL evidence checks precisely one `PASSWORD_CHANGED` event and event-linked pending notice per synthetic subject. The required real-browser workflow now includes the new test alongside existing email verification and session E2E. See [ADR-0019](../adr/0019-browser-password-change-postgres-e2e.md).

**Still open:** real production ingress/proxy/TLS validation; a production frontend with clear uncertain-outcome and session-logout messaging; safe lockout/recovery UX, breached-password policy, MFA/step-up, deployment role and key isolation, live Mailgun sender/webhook/alerting, monitoring/SLA, financial resource authorization and owner approval. Browser regression and disposable PostgreSQL tests are not permission to mount the endpoint or merge the draft stack.

### Draft #57 — one-shot browser password-change UX (unmounted)

A first-party, strict-CSP HTML/JavaScript candidate now renders a single-use change-password form only after verifying a live session and obtaining a session-bound CSRF. It clears DOM password inputs and locks the form after one mutation. HTTP 204 indicates a confirmed change; generic 401 indicates a denied change; response loss/503/other results never trigger an automatic retry and instead show an explicit **unknown result**. Browser/SQL E2E must prove denial and lost-after-commit behavior. See [ADR-0020](../adr/0020-unmounted-password-change-recovery-page.md).

**Still unresolved:** officially owned login/recovery/support routes, user-facing security copy/localization/accessibility approval, MFA and step-up, production password blocklist, full operational identity role/ingress/CSRF key isolation, real notice deliverability, and release orchestration. None of the page or password-change routes is installed in the default public/private listeners. Keep every stacked PR draft and unmerged.
