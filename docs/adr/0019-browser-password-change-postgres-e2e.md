# ADR 0019 — Real Chromium and PostgreSQL password-change evidence

Status: **PROPOSED / stacked Draft PR #56**, based on #55. This adds tests, not live routing, a new SQL authority, a production Mailgun sender or an operational enablement.

## Why this test is necessary

ADR-0018 defines a manually constructed, non-routed browser password-change handler. The earlier handler-only `httptest` suite proved request shape, CSRF binding and usecase invocation with fakes, while v15 SQL integration separately proved atomic password rotation and session-epoch invalidation. Neither had shown that actual Chromium cookies, the actual HTTP handler, the authentic Argon2id implementation and role-separated PostgreSQL all work **together**.

A particularly dangerous blind spot is HTTP response loss *after* the password rotation commits: the browser may keep the original cookie even though the authority has already invalidated it. The frontend must never assume a network exception means the credential was not changed.

## Decision and test boundary

- Add mandatory `TestIdentityZBrowserPasswordChangeE2E` with build tags `integration,browser_e2e`. It constructs **only** an ephemeral `httptest.NewTLSServer` mux for the existing candidate login/session/CSRF and password-change handlers. No route is added to either application listener or `main`. Both default listeners must still return 404.
- Use real PostgreSQL v18 fixtures, independently provisioned synthetic runtime roles for admission/authentication/grants/session/password-change, actual Argon2id and immutable SQL epoch/audit/notice behavior. Privileged `mamony_test_admin` is confined to provisioning and verifying **disposable** test data.
- Chromium makes real `fetch` requests using browser-managed `__Host-mamony_sid` cookies. Its JS cannot see `Set-Cookie` or the HttpOnly bearer. The pinned Playwright network shim forwards only an allowlisted `app.example.test` path to the loopback TLS fixture. It does not prove a deployed reverse proxy or native CORS architecture. No external Internet or Mailgun is contacted.
- Scenario A: two independent browser contexts log into **the same subject**. Missing or tampered session-bound CSRF, duplicate/oversized JSON, a different subject's valid password, and cross-origin POSTs must be rejected. A successful change removes the initiator's cookie, invalidates the second browser's still-retained session via the database epoch, denies login with the old password, and admits fresh login with the new password.
- Scenario B: forward a valid change from Chromium until the Go handler has returned 204 **after DB commit**, then deliberately abort the test-only browser response. Chromium reports an uncertain network error and retains its stale cookie; server-side resolution must still return 401. After explicitly clearing the stale cookie in the test harness, the old password cannot log in but the new password can. **No automatic retry** is performed.
- The SQL fixture verifies exactly one `session_epoch` advancement, one `PASSWORD_CHANGED` security outbox event and one event-bound pending security notice for **each** subject. Password-change notices are quarantined after testing so no live provider can send them.
- The bridge uses source-bound addresses **127.0.0.2 and 127.0.0.3** to keep distinct synthetic subjects under the existing 8/client/minute pre-KDF quota without changing production rate rules. All forwarding remains inside local loopback.

## Mandatory gate

The existing `Identity real browser E2E` workflow runs this test alongside `TestIdentityZBrowserVerificationE2E` and `TestIdentityZBrowserSessionE2E`. A missing or failing password-change case must fail that workflow. Require all five workflow conclusions SUCCESS at the **exact final PR head SHA**; do not disable vulnerability/lint/architecture/SQL/browser checks.

## Remaining production blockers

This test verifies a **test-only TLS mux**, synthetic browser contexts and disposable role credentials. It is not proof of deployed ingress/proxy trust, secret rotation, KMS, live Mailgun and bounce escalation, a reviewed frontend reset/login UX, MFA and recovery, a production breached-password policy, finance action authorization or end-user readiness. The test shim deliberately bypasses production routing and does not establish independently verified CORS behavior.

**Keep Draft, do not merge or deploy. No production listener, webhook or mail worker may be activated from green test evidence alone.**
