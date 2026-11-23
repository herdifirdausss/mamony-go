# ADR 0016 — Signed Mailgun notice webhook, provider correlation, and failure-health counts

Status: **PROPOSED / Draft PR #53**, stacked on Draft #52. No inbound HTTP route is mounted and no live webhook is configured, no production Mailgun message is sent. This is a reviewed *candidate*, not production activation.

## Problem and evidence

Draft #52 treated HTTP 200 from Mailgun's Messages API as **provider accepted** and discarded the returned `id`. The `PASSWORD_CHANGED` event and v16 notice queue then had no trustworthy mapping to later `delivered`, `failed`, `complained` or `unsubscribed` callbacks. Without that mapping a notification system cannot distinguish submission from receipt at the recipient's mail server or alert operators on confirmed bounces.

Mailgun Send webhook documents: Event JSON uses `signature.{timestamp,token,signature}` and `event-data.message.headers.message-id`; HMAC-SHA256 is computed from **timestamp + token**, using the distinct **HTTP webhook signing key**, not the Domain Sending API key. Mailgun event ID is unique within a day. `failed` needs `severity=temporary|permanent`; `delivered` means *recipient SMTP server* accepted the message, not human inbox/read confirmation. Provider retry schedules can extend to eight hours, though documentation excludes delivery notifications from some retry guarantees.

Authoritative references (policy must be rechecked against deployed Mailgun account):
- https://documentation.mailgun.com/docs/mailgun/user-manual/webhooks/securing-webhooks
- https://documentation.mailgun.com/docs/mailgun/user-manual/webhooks/webhook-payloads
- https://documentation.mailgun.com/docs/mailgun/user-manual/webhooks/webhook-retries
- https://documentation.mailgun.com/docs/mailgun/user-manual/events/event-structure

## Design and enforced trust boundaries

1. Sender returns a **normalized Mailgun message ID**, stripping only a matched angle bracket pair. The notice worker refuses to treat a malformed/missing response ID as accepted. v17 `finalize_password_change_notice_v2` requires the ID's SHA-256 digest for `ACCEPTED`; the previous three-argument finalize loses worker EXECUTE privilege. The raw provider ID, recipient and provider body never enter receipt storage.
2. **v17 `security_notice_mailgun_receipts`** stores only 32-byte SHA-256 digests of the signature token (unique replay fence), Mailgun event ID + domain + UTC day (unique event dedup), normalized message ID (correlation), a narrow normalized state, event timestamp and insertion timestamp. The raw signed request, Mailgun API key, signature, provider message, recipient email and headers are never persisted in the receipt table. Seven-day bounded erasure uses the existing separate maintenance SQL role.
3. The unmounted `MailgunSecurityNoticeWebhook` rejects non-POST/non-JSON, body >16 KiB, invalid/expired/forged HMAC (constant-time comparison), oversized/malformed token/signature, wrong configured sending domain, malformed message IDs, unknown failure severities and event timestamps outside the bounded window. Only an in-process trusted component can construct it with the distinct webhook signing key, supporting at most two configured keys during rotation. All failures are redacted. 406 means do not retry invalid webhooks; 503 on database storage failure requests retry; 200 idempotently acknowledges successful receipt or an irrelevant signed event.
4. **Subtle limitation:** Mailgun's HMAC authenticates `timestamp+token`, not the whole JSON event body. Do not claim body-level cryptographic integrity from this signature alone. Enforce the TLS ingress boundary, sending-domain equality, normalized message identity hash, isolated SQL role, token/event replay uniqueness and operational network controls. The receiver credential can still forge outcomes if compromised; restrict its secret and privilege separately from the sending key. Do not accept unsigned raw Mailgun event objects from other code paths.
5. **Race safety:** a Mailgun webhook can arrive before the HTTP send response has been finalized into PostgreSQL. The handler persists its **unmatched digest-only receipt** first; later successful finalize selects the highest-priority signed receipt under the same transaction and updates `provider_state`. If finalize happens first, the incoming receipt updates the matching notice atomically. Later lower-severity events cannot downgrade a complaint/bounce; event precedence is UNSUBSCRIBED > COMPLAINED > PERMANENT_FAILURE > DELIVERED > TEMPORARY_FAILURE > SUBMITTED. This is a conservative, not an email inbox metric.
6. **Unmatched caveat:** when Mailgun accepts a send but the HTTP response is lost, the worker cannot know that send's message ID. A webhook for that send is held as an unmatched digest and reported by the operator aggregate but **cannot safely be assigned to a particular notice**. The worker may resend and Mailgun Messages API does not provide generic exactly-once semantics. Opaque provider correlation variables would be a separate security/privacy design and are deliberately not introduced here.
7. **Count-only observability:** `security_notice_health()` is a SECURITY DEFINER aggregate granted ONLY to `mamony_identity_maintenance_runtime`, counting PENDING due, DEAD, submitted-unconfirmed, delivered-to-server, provider failures/complaints, unmatched provider events. `ReadSecurityNoticeHealth` is an explicit, one-shot operator facade with a separate maintenance DB pool; it does not schedule monitoring, page an on-call, expose a dashboard, return emails, or mount a public HTTP path.

## Mandatory evidence

- Real disposable PostgreSQL `TestIdentityMailgunMockWebhookReconciliation` exercises receipt-before-HTTP-ack, duplicate concurrent delivery, provider message ID hashing, late bounce/delivery precedence, complaint escalation, invalid signature refusal, unmatched event monitoring, and seven-day receipt purge keeping security audit.
- `TestPostgresIdentityZMailgunWebhookRole` proves separate webhook role cannot SELECT/UPDATE notices, security receipts, aliases, verifiers, sessions, audit or call monitor/maintenance functions; all other runtime roles cannot record webhook receipts, and even the notice worker cannot use the old uncorrelated 3-arg finalize.
- Mock HTTP unit tests cover HMAC signing verification, timestamp freshness/rotation, invalid body/domain/message-ID/authorization, and redacted database failures; previous real-Argon2id change, Mailgun local sender E2E, race/lint/vuln and real Chromium tests remain mandatory.

## Release blockers

**No public webhook endpoint is mounted**, so Mailgun cannot deliver callbacks to this project yet. Production ingress design needs an HTTPS reverse-proxy route, rate limits, request-body controls, secrets from KMS/secret manager, real signing key rotation, network exposure and verification, multitenant/subaccount domain rules and a trusted executable with the webhook-only DB credential; no signing key or real domain is checked into code.

No operational scheduler, dashboard, alert threshold, on-call escalation, bounce suppression policy, reconciliation of missed webhooks via Mailgun Events API, guaranteed delivery of delivery notifications, user-visible password-change page, MFA, recovery, or financial authorization is provided. Old notices previously accepted before v17 **lack message IDs and cannot be backfilled reliably**. Passing CI means only disposable DB + local mock proof; not inbox delivery or deployment approval.

**KEEP DRAFT / UNMERGED; no production Mailgun webhooks, sending or public Identity activation.**
