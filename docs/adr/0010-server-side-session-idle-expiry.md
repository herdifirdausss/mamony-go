# ADR 0010 — Server-side idle expiry for opaque browser sessions (Draft PR #47)

Status: **PROPOSED candidate; NOT approved for production**.
Stack: #47 → #46 → #45 → #44 → #43 → #42 → #41 → #40.
Scope: Profile A, unmounted first-party browser session handler. No public, admin, machine or financial operation is activated.

## Problem

v10/v11 `resolve_opaque_session` enforces 12-hour absolute expiry, revocation, active/verified contact and the current subject epoch, but **no inactivity limit**. A browser cookie still present 6 hours after its last request can authenticate if not otherwise invalidated. Setting only client-side Max-Age does not address this: PostgreSQL must enforce both clocks.

OWASP Session Management Cheat Sheet recommends server-side idle and absolute session timeouts. NIST SP 800-63B-4 distinguishes timeout and assurance-level policies; **these synthetic password-only sessions are not claimed to meet AAL2**. The **30-minute** idle limit here is an initial conservative, testable Profile A candidate, *not an approved production value*. References: https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html and https://pages.nist.gov/800-63-4/sp800-63b/session/.

## Decision candidate

- Add `opaque_sessions.last_seen_at timestamptz NOT NULL` in immutable, SHA-256 fingerprinted **migration v12**; backfill existing rows with `issued_at`. The migration therefore **fails closed** for sessions issued more than 30 minutes ago, rather than silently treating migration time as fresh user activity. On an actual deployment this creates an explicit existing-session invalidation UX change; the stack has no public session deployment.
- `resolve_opaque_session` is replaced forward-only, preserving its signature and existing owner/restricted EXECUTE grants. No v10/v11 SQL history is edited; do not add SELECT/UPDATE to the session runtime role.
- For each syntactically valid, digest-indexed resolve, lock the session row, subject row and verified current alias row with `FOR UPDATE`, rechecking ACTIVE, contact v1, epoch, revocation. After acquiring all locks, **sample PostgreSQL `clock_timestamp()`**. Reject if `now >= expires_at`, `now >= last_seen_at + 30min`, or `now < last_seen_at` (DB clock regression). The strict boundary means `30:00` elapsed is expired. Under those same locks, update `last_seen_at=now` only on success, then return only the subject.
- A successful lookup does **not** extend `expires_at`, which remains the original 12-hour DB-clock absolute limit. The browser cookie's `Max-Age=43200` is an upper bound only; clients receive 401 for idle expiry before the cookie disappears. A `GET /csrf`, `GET /session` or a valid pre-logout Resolve counts as activity; only actual requests, not a UI heart-beat promise, can cause the DB update.
- A failed/expired/revoked/frozen/unknown lookup cannot update last activity. Database errors, including ambiguous activity UPDATE commit, remain sanitized unavailability (503) and never an authenticated result. The activity update is not a new `SESSION_CREATED` event.
- The session SQL role is now capable of updating a row through the existing **restricted** SECURITY DEFINER resolution function. It still lacks direct table mutation privilege; it cannot mint a grant or bypass the expiry predicates.
- `FOR UPDATE` serializes simultaneous resolves of the *same* cookie; account freezes can wait behind in-flight resolves. This is a correctness-first candidate and **increases read-path DB writes/locks**. Do not expose it publicly before representative traffic, contention, lock-timeout and denial-of-service load tests; any future optimization must preserve server-clock and freeze ordering.

## Why this, not alternatives?

- Cookie-only expiry is attacker-controllable and cannot invalidate a still-valid server-side bearer early.
- Unsynchronized in-process maps do not work across processes, crash/restart or horizontally scaled instances.
- Requiring client-supplied last activity is forgeable.
- A coarse, uncommitted last-seen cache is susceptible to stale access windows; caching or sampling requires a separately proven lock/cache-invalidation policy and an explicit maximum staleness budget.
- No refresh token or new authentication protocol is added. An idle timeout does **not** substitute for password-change epoch revocation, MFA, step-up, reauthentication or financial authorization.

## Required adversarial evidence

Mandatory real PostgreSQL test `TestIdentityZSessionIdleExpiry` plus prior lock-interleaving, grant and Chromium tests:

1. Real, newly issued v11 grant-bound session resolves and commits DB activity.
2. An active session last seen 29 minutes ago is valid and its activity is refreshed, while `expires_at` is byte-for-byte unchanged.
3. A session idle 31 minutes is denied on multiple attempts; failed lookup must not refresh activity or resurrect it.
4. A session past absolute expiry is denied even if activity is recent.
5. Frozen subject and subsequent thaw remain invalid due to the session epoch.
6. Privilege matrix denies direct `UPDATE opaque_sessions.last_seen_at` by the session runtime and by unrelated roles; only the restricted resolver changes it.
7. Real Chromium cookie/CSRF/logout tests remain unchanged, with no new route registration.
8. Five mandatory GitHub CI workflows pass on the *exact final SHA*, including migration integrity and the machine-readable testgate's new required test name.

## Deferred release gates

Formal approval of idle/absolute durations for browser/operator clients, public UX and key management, reauthentication policy, true production workload/network isolation of grant and session role credentials, multi-instance measurement, active-session disclosure, alerting, safe takeover/incident response, password/contact change epoch invalidation, MFA and financial tenant/action/resource checks. No statement here claims security against compromise of the whole Profile A runtime or migrator.

**Keep stacked PRs Draft. No merge to main, production secrets, public route activation, or production readiness claim.**
