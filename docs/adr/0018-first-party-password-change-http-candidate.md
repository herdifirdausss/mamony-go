# ADR 0018 — Unmounted first-party password-change HTTP candidate

Status: **PROPOSED / stacked Draft PR #55**, based on Draft PR #54. No public/private application router registration, live credential provisioning or production rollout.

## Context

The v15 Identity operation already demands an active opaque session, distributed pre-Argon2id admission, a successful current-password comparison for the *same* subject/epoch, mandatory new-password policy, a single-use purpose-bound grant and atomic verifier/epoch/security-outbox mutation. v16–v18 provide notice queue, sender/webhook reconciliation and read-only health observation. None of this previously defined a browser-facing mutation boundary.

Adding a route without direct-TLS origin checks and a session-bound CSRF token would expose account takeover via cross-site browser actions. Adding another independently configured CSRF key would risk inconsistent browser-session validation. Allowing an injected arbitrary password-change callback could silently bypass the mandatory internal reauthentication chain.

## Candidate decision

1. Implement a manually constructed `BrowserPasswordChangeHTTP` bound to the **existing** concrete `BrowserSessionHTTP` instance and **concrete** `AuthenticatedPasswordChange` usecase. Reuse its exact canonical HTTPS origin, host, no-forwarded-header direct-TLS policy, trusted peer resolver, session cookie parser and 15-minute session-bound CSRF verifier. There is no new independent CSRF key or public route.
2. Candidate operation is **POST /v1/identity/change-password**, with JSON fields `email` (current canonical alias), `currentPassword`, `nextPassword`; no subject ID, role, session bearer, grant or proof field. Reject duplicate/unknown fields, incorrect JSON types, trailing documents, noncanonical alias, and any request over 4096 bytes. Each password field is capped at 512 bytes.
3. Require an existing `__Host-mamony_sid` cookie and exactly one `X-Mamony-CSRF` header. The request must have the same exact Origin, Host, TLS and Fetch Metadata constraints as the existing session POST operations. Validate CSRF before invoking admission/KDF and do not accept bearer Authorization or proxy-controlled identity headers.
4. Delegate authority exclusively to `AuthenticatedPasswordChange.Change` with the cookie bearer and trusted direct peer. The usecase enforces original session ownership, real old-password proof, current epoch, policy/blocklist, a new salt, grant purpose and SQL atomicity. The handler itself cannot assign a target subject or mint a new session.
5. A **certain commit** returns 204 with an expired secure/HttpOnly/SameSite=Strict host-only cookie. This is not automatic login: the old session is invalidated by the SQL epoch increment and the user must authenticate again with the new password. A denied change returns generic 401 without revealing which credential/policy factor failed; malformed requests are 400/413/415, origin/CSRF failures 403, and database/untrusted peer/commit ambiguity 503. No cookie deletion or confirmed-success claim on ambiguous commit, because the password might already have changed.
6. All responses use existing no-store security headers and sanitized problem JSON. No password, cookie secret, event ID, subject ID, database detail, correlation material or URL appears in response bodies or audit logs.

## Tests and release gate

- Unit `TestBrowserPasswordChangeCandidateSuccessInvalidatesOldCookie` verifies the full internal reauthentication chain is invoked and the expired cookie is sent only on certain success.
- `TestBrowserPasswordChangeCandidateRejectsUnsafeInputsBeforeKDF` checks wrong method/host/origin/TLS, proxy spoofing, cross-site requests, duplicate cookie/CSRF/JSON keys, oversized JSON and untrusted peer before identity work.
- `TestBrowserPasswordChangeCandidateDenialAndAmbiguousCommit` covers wrong current password, account mismatch, consumed budget, blocked next password and a sanitized uncertain-commit response.
- `TestBrowserPasswordChangeCandidateCSRFBoundToSession` prohibits reuse of a valid CSRF token across two different sessions.
- Existing PostgreSQL atomicity/outbox and actual Chromium session tests must remain passing; those tests **do not yet exercise this new route in a real browser**. Full real PostgreSQL + browser password-change E2E is required before any future route activation.
- Run all five mandatory GitHub Actions on the final exact head SHA. Keep every stacked PR Draft/unmerged.

## Open security and UX decisions

The canonical email in the body is not an authorization selector: the current session and SQL-bound password proof must resolve to the **same account**. This form currently requires the user to know that canonical alias; a future UX may remove the form field only after a safe server-side session-to-alias read contract is designed.

A successful change logs out all sessions without issuing a replacement. Response loss or a database-ambiguous commit may leave a stale cookie in the browser and requires safe re-login guidance; do **not** auto-retry a password change. Accessible password-policy feedback, product release throttling/WAF, MFA/step-up, recovery flows, device UX, real provider/suppression operations, managed CSRF keys, direct-TLS ingress/network identity, workforce credentials, and protected finance authorizations remain unresolved.

**This is transport-boundary test evidence, not approval for public account-management endpoints, production authentication or real notifications.**
