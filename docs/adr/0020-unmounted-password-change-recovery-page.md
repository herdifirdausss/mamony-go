# ADR 0020 — One-shot, unmounted browser password-change experience

Status: **PROPOSED / stacked Draft PR #57**, based on #56. No public or private application route, live Identity service, Mailgun sender, notification scheduler, migration or external account-recovery endpoint is activated.

## Problem

ADR-0018 introduced an internal browser password-change mutation and ADR-0019 proved it against real Chromium and PostgreSQL. The API cannot tell the client whether a connection loss or an unacknowledged SQL commit did in fact change the credential. A client that automatically retries a password change could send the old password repeatedly, consume durable pre-KDF budgets, or mislead the account owner about the session state.

Account recovery, MFA and operational support are not yet approved or wired. The initial UI must therefore be intentionally small, truthful about uncertainty, and explicit that there is no automatic credential retry, redirect, or new session issuance.

## Decision

1. Introduce only a manually constructed **unmounted** `PasswordChangePage` candidate for `GET /account/change-password`. It takes the *existing concrete* `BrowserSessionHTTP` boundary and reuses exactly the same direct-HTTPS, canonical host/Origin, no-forwarded-identity and Fetch Metadata read policy. No second session/CSRF authority or app router registration is permitted.
2. Serve embedded `password_change_page.js` and `.css` from a static Go response. Inline script and style bytes are individually pinned in CSP by SHA-256; default-src, external scripts/fonts/images, workers, frames, form POST fallback and all other network destinations are denied. The only allowed `connect-src` is `'self'`; no CDN, tracking, local/session storage, credential URL, service worker, cookie read or credential logging is introduced. Private navigation uses no-store, no-referrer, nosniff and deny-framing response headers.
3. The initial GET is a display-only operation. The client must verify the existing browser session via `GET /v1/identity/session` and then obtain its session-bound CSRF capability via `GET /v1/identity/csrf` before revealing the password fields. No existing session yields generic signed-out instructions without exposing a mutation form. GET never changes the account.
4. Explicit human submission requires an email alias, current password, replacement, and matching confirmation. The confirmation stays solely in the DOM and is never submitted to the server. The `POST /v1/identity/change-password` body uses the existing strict three-field JSON contract; the CSRF value is sent in the approved header. Browser session cookies are handled only through `credentials: "same-origin"`. Redirects are rejected and no values are stored persistently.
5. **One attempt per page instance.** On actual submission, the UI immediately hides the form, clears all password inputs, invalidates its in-memory CSRF and locks submission before the network operation. A definite HTTP 204 renders success and advises a fresh login through an independently trusted account entry point. A generic 401 renders a non-enumerating denial. Any other response, including 503, browser timeout, or response loss *after commit*, renders the **same explicit uncertain-outcome** guidance: the password might already have changed, do not re-submit it, and use the established trusted sign-in/recovery process. No automatic resend, token refresh, session restoration, fresh login, password validation oracle, or dynamic redirect is allowed.
6. No unimplemented login/recovery URL is linked from the page. Recovery channel, support ownership, accessibility/localization copy and product rollout remain **owner decisions**; the page does not imply those paths are available through Mamony Go yet.

## Test evidence

- Go handler tests prove URL/method/host/direct-TLS/Origin/forwarded-header rejection without rendering executable bytes, immutable embedded CSP hashes, no external resources and no public route.
- Real Chromium with disposable PostgreSQL tests signed-out UI, client validation for mismatched confirmation before any POST, exactly one POST on certain success, cleared browser inputs, all-session epoch invalidation, explicit denial lock, and a dropped-after-commit HTTP response resulting in an **uncertain locked page** with no automatic retry.
- Existing authenticated-session, SQL-role, audit/notice, integration, race, lint and toolchain tests must remain green; all five workflows require SUCCESS on the **exact PR head SHA**.

## Nonclaims and remaining risk

The candidate only proves a test-only HTTPS mux and the local browser shim. It does **not** prove a deployed reverse proxy, native CORS behavior, production CSRF key rotation, live frontend authentication, account recovery, MFA/step-up, a production breached-password list, user consent/locale/accessibility review, production identity role isolation, delivery SLA or financial operation authorization. Browser memory is not guaranteed to be immediately erased by JavaScript string clearing; HTTPS transport, role separation, CSP and a no-storage policy are defense-in-depth, not memory zeroization.

**Keep Draft and unmerged. Do not enable the route or claim that the recovery flow is production-ready.**
