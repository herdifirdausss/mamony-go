# ADR 0015 — Explicit Mailgun sender and runtime for password-change security notices

Status: **PROPOSED, stacked Draft PR #52 on Draft #51**. The presence of a sender and binary does NOT mean they are configured, deployed, or permitted to send real email. No public password-change, enrollment or login routes are activated.

## Decision

Keep the v16 `password_change_notices` SQL outbox and its **EXECUTE-only** `mamony_identity_security_notice_runtime` role from ADR-0014. Add a separate **token-free** `mailgun.SecurityNoticeSender` and a standalone operator-launched `cmd/mamony-identity-security-notice-worker`. Do not reuse the verification-token sender (which uses an encrypted `#token` URL), the email-proof SQL credential, the password mutation credential, or the main HTTP process.

The Go `identity.RunSecurityNoticeDelivery` facade validates a pinned Mailgun region/domain/from and a bounded polling interval, creates exactly one pool for the dedicated notice role, then composes the existing `SecurityNoticeWorker` and serial `DeliveryRunner`. One process does no more than one HTTP submission at a time; concurrent processes use the v16 45-second PostgreSQL lease and fencing token.

## Message and provider constraints

Only POST multipart form fields to the official pinned `https://api[.eu|.au].mailgun.net/v3/{domain}/messages` host for configured US/EU/AU region, using HTTP Basic Auth with a dedicated domain sending key. No caller-defined endpoint or subject/body, no redirect follow, no HTML, no verification token, no session/grant secret, no event ID forwarded to recipient or provider metadata, no reset URL, no password hints or account email body echo. Fixed security message advises opening the app directly and contacting support through a trusted channel.

Explicit Mailgun options are `o:tracking=no`, `o:tracking-opens=no`, `o:tracking-clicks=no`, `o:require-tls=yes` and `o:deliver-within=5m`. The sender's HTTP timeout never exceeds 9 seconds; worker call is bounded to 10 seconds. API request and return are sanitized: a 200 JSON response with a nonempty bounded message `id` means **provider accepted submission only**, NOT delivered/read. HTTP 429, 420, 408, 5xx, timeouts and ambiguous responses are retryable; other non-200 statuses are permanent. NEVER return provider response body, request error details or user-recipient/secret to logs.

Official provider references:
- https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/messages
- https://documentation.mailgun.com/docs/mailgun/user-manual/sending-messages/pass-sending-options

Mailgun has **no guaranteed generic exactly-once send** for this Messages API. A successful submission followed by worker crash/network uncertainty can cause duplicate emails. The event ID is a *local* durable correlation key, not a server-enforced idempotency key.

## Production startup is explicitly opt-in

The new binary is separate from `cmd/mamony-identity-mail-worker` (email verification). It refuses to start unless all conditions are satisfied:
- Exact command flag `--send-live-security-mailgun` **and** exact operator environment approval `MAMONY_IDENTITY_SECURITY_NOTICE_LIVE_APPROVAL=I_ACKNOWLEDGE_REAL_SECURITY_NOTICE_EMAIL_SENDS`.
- `CI` is absent. No defaults, automatic service spawn, hidden flag or startup from the HTTP application.
- `MAMONY_IDENTITY_SECURITY_NOTICE_DSN_FILE` points to an absolute regular, non-symlink, owner-only readable file. Its PostgreSQL username must be `mamony_identity_security_notice_runtime`, database must NOT be `mamony_test_*`, and `sslmode=verify-full` must be the sole query argument.
- `MAMONY_IDENTITY_SECURITY_NOTICE_MAILGUN_KEY_FILE` points to a similarly protected absolute file. Region/domain/from fields are explicitly configured; the sender validates them against the allowlisted Mailgun host and sending domain.
- Sensitive files are opened using os.Root confinement and Lstat/SameFile checks. The process never logs the credential, private path, DSN, event ID, recipient or provider error detail.

This is a **safety brake**, not authorization of a real rollout. Mailgun sending-domain SPF/DKIM/DMARC, sender reputation, region/data-processing review, quotas and dedicated keys, network egress rules, DB failover/TLS, secret rotation, notification suppression or bounce feedback require separate operational approvals.

## Test evidence and release blockers

- Mailgun local mock verifies region-specific official endpoint, Basic Auth, exact multipart fields, strict no-link/no-token/no-event-ID message, no tracking, required TLS, redirects denied, malformed inputs denied before network, provider response classification, timeout and secret/error redaction.
- CLI unit tests deny absent opt-in, CI, non-owner-readable or symlinked secret files, wrong SQL role, test DB, disabled TLS, extra libpq params, missing port and malformed key files.
- Mandatory `TestIdentityMailgunMockSecurityNoticePipeline` runs **real disposable PostgreSQL** v15 change+v16 notice queue through the dedicated facade to a **local** Mailgun mock. It proves 429 -> database retry -> 200 -> provider ACCEPTED metadata with one immutable PASSWORD_CHANGED audit event; an incorrect DB role cannot substitute. No live Mailgun network or credentials appear in CI.
- All five mandatory workflows must pass on the exact final commit SHA.

Remaining: recipient bounce/complaint/webhook verification and reconciliation, human escalation for DEAD or stale recipient, actual notification-delivery monitoring, SPF/DKIM/DMARC and Mailgun tenant/domain provisioning, privilege-scoped cloud workload secrets and egress policy, customer-facing change-password CSRF/Origin UI and cookie invalidation behavior, MFA/step-up/recovery policy, financial action authorization, and owner G2/G3/G4 approvals. Existing account-change notice recipients should be reviewed for privacy when an address is compromised. **Keep all stacked PRs Draft and unmerged.**
