# ADR 0017 — Read-only, thresholded security-notice health probe

Status: **PROPOSED / stacked Draft PR #54**, based on PR #53. No deployment, scheduler, paging integration, public endpoint, live Mailgun webhook or real send is activated.

## Context

The v17 signed Mailgun callback/notice queue provides `security_notice_health()` as a count-only SQL function, but an operator had to use the **maintenance runtime** to query it. That DB role also legitimately has data-erasure capabilities. Continuous monitoring should never require a principal able to purge retained data or interact with sender/receiver roles. The system also lacked an explicit machine-readable threshold/exit-code contract.

## Decision

1. **Identity v18 immutable migration** grants only `USAGE` on the Identity schema and `EXECUTE` on `security_notice_health()` to independently provisioned SQL LOGIN `mamony_identity_notice_monitor_runtime`. It does not grant SELECT/INSERT/UPDATE/DELETE on any tables, other SECURITY DEFINER functions, role membership or maintenance purge. The existing maintenance role keeps its earlier behavior for backwards compatibility and manual cleanup; this is **not** the recommended monitoring identity.
2. Add `ReadSecurityNoticeHealthAsMonitor(ctx, dsn)` facade that pins PostgreSQL `RuntimeRole` to `mamony_identity_notice_monitor_runtime`, opens one connection, obtains only count fields and redacts database errors. Existing `ReadSecurityNoticeHealth` retains maintenance semantics.
3. New one-shot binary `cmd/mamony-identity-notice-health` requires exactly `--check-security-notices`, CI=false, a guarded **absolute private regular non-symlink** DSN file with strict `postgres[s]://mamony_identity_notice_monitor_runtime:<secret>@host:port/db?sslmode=verify-full` format, and **five explicitly provided nonnegative integer thresholds**:
   - `MAMONY_NOTICE_MAX_PENDING_DUE`
   - `MAMONY_NOTICE_MAX_DEAD`
   - `MAMONY_NOTICE_MAX_SUBMITTED_UNKNOWN`
   - `MAMONY_NOTICE_MAX_PROVIDER_FAILURES`
   - `MAMONY_NOTICE_MAX_UNMATCHED_EVENTS`
4. Outputs exactly **one JSON line** with `status` (`OK`, `ALERT`, `ERROR`) and count-only metrics. Exit code **0** when all observed counts are at or below configured thresholds, **2** on at least one threshold breach, **1** on invalid configuration/credential/DB failure. No email/recipient, provider message, password, DSN, internal SQL error, webhook signature, correlation token or user ID is emitted. The `delivered` metric remains informational and means recipient SMTP server acceptance only, not inbox delivery.
5. This is **not** an always-on monitor: it runs once when an independently approved scheduler invokes it. No network service, alert sender, email sending, webhook endpoint, cron schedule, on-call roster, K8s resource, or production secret is configured. Explicit thresholds are mandatory to avoid silent arbitrary alert policy. Operator owns meaningful thresholds, schedule, escalation and false-positive policies.

## Test evidence

- `TestNoticeHealthProbeThresholdClassification` proves each threshold, JSON counts and exit code; `TestNoticeHealthProbeFailClosed` covers missing args, CI, thresholds, elevated maintenance role, test/prohibited TLS DSNs, world-readable files and symlinks; `TestNoticeHealthProbeRedactsStorageErrors` proves sensitive errors cannot leak.
- Disposable SQL `TestPostgresIdentityZNoticeMonitorRole` tests exact limited role grants, protected data/table/maintenance/send/receive denials and lack of elevated role attributes. Existing runtime roles cannot call the count-only function.
- `TestIdentityMailgunNoticeMonitorRole` connects using only the v18 credential, exercises the public Go facade and confirms maintenance-role confusion/retention erasure denial.
- Exact SHA five-workflow GitHub Actions SUCCESS is required and not a claim of production readiness.

## Risks and open items

Provider events can be missed; a zero `provider_failures` count does **not** prove successful inbox delivery, working webhook ingress, or availability. The SQL aggregate is a snapshot, not a time-series SLA. `PENDING` due count may briefly be nonzero during healthy polling, and unmatched events might include valid delayed callbacks; the owner must determine safe thresholds based on workloads. A DB connectivity failure produces ERROR (exit 1), not zero-valued OK. Exposing even aggregate counts can reveal operational activity, so monitor credentials/outputs require bounded IAM, log retention and operator controls.

**Keep Draft and unmerged; do not activate production webhook/sender/account endpoints or automate paging without explicit release review.**
