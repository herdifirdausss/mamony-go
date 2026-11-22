# ADR 0007 — First-party browser session HTTP contract (Draft PR #42)

Status: **Proposed contract only. No HTTP handlers, cookie issuance, mounted routes, OpenAPI publication or production credentials are implemented by this ADR.**

## Context

ADR-0004 establishes a read-only Argon2id authenticator, ADR-0005 a durable pre-KDF login admission gate, and ADR-0006 a transactionally rechecked, epoch-bound 256-bit opaque session store. The existing registration and email-proof HTTP candidate handlers require **direct TLS with an exact reviewed HTTPS Host and Origin**, and the default application routers remain health-only. Introducing cookie authentication without an explicit browser boundary would invite session fixation, cross-site request forgery (CSRF), credential exposure, login CSRF, cache leakage and confused-deputy use of a subject as authorization.

The v10 PostgreSQL session authority is **not** an authorization authority for tenants, financial resources or actions. The session role is highly sensitive: possession of its database credential permits direct invocation of a function that can mint a session for any known eligible subject ID. This threat remains out of scope of an HTTP-only design and must be addressed by deployment isolation and/or a stronger credential-bound issuance proof before production.

## Candidate same-origin topology

- First-party browser UI and identity HTTP API live on **one exact canonical HTTPS origin**, for example `https://app.example.test` in **synthetic tests only**. Do not permit wildcard hosts or trust a user-supplied `Host`, `Forwarded`, `X-Forwarded-Proto` or `X-Forwarded-Host` as a trusted TLS or session boundary.
- Proposed direct TLS edge semantics match existing candidate handlers: `req.TLS != nil`, exact `Host`, a single strict `Origin` for state-changing requests, and no redirects that downgrade TLS. A real reverse-proxy/TLS-termination deployment needs a **separately approved ingress trust design**; adding proxy support ad hoc is not allowed.
- Browser identity transport is a **host-only cookie**, not `Authorization`, query strings, URL fragments, localStorage, sessionStorage or JSON bearer. Reject alternate bearer placements and duplicate/ambiguous session cookies, including same-name cookies with conflicting values. Do not accept a client-asserted tenant, subject, role or session epoch.
- Use `Cache-Control: no-store`, `Pragma: no-cache` where applicable, `Referrer-Policy: no-referrer`, `X-Content-Type-Options: nosniff` and reviewed CSP on browser responses; no bearer or password may enter logs, analytics, error bodies, traces, redirect URLs or third-party content.

## Cookie construction (candidate, NOT emitted)

A successful login may issue exactly one cookie:

`Set-Cookie: __Host-mamony_sid=<43-char base64url opaque token>; Path=/; Secure; HttpOnly; SameSite=Strict; Max-Age=43200`

- **No `Domain` attribute**. The `__Host-` prefix enforces Secure + host scope + `Path=/` in supporting browsers; the backend must also enforce these choices and compare the exact cookie name. Do not use a public-suffix or a parent-domain cookie shared with other apps.
- `HttpOnly` blocks ordinary JavaScript cookie access but does not prevent XSS-driven requests; a first-party CSP and output encoding remain necessary.
- `SameSite=Strict` is a conservative **candidate**; assess third-party sign-in/federation and legitimate cross-site navigation before choosing Lax. Never rely on SameSite as the only CSRF defense.
- `Max-Age=43200` mirrors the current **12-hour DB absolute expiry**, but **database time and server-side revocation are authoritative**. No implicit sliding expiry or silent refresh. Clear using the **same name and Path** with `Max-Age=0; Secure; HttpOnly; SameSite=Strict` and no Domain.
- Cookie value must be the raw random 256-bit bearer **only in a Set-Cookie header after confirmed session transaction commit**; no token may appear in a response body. An ambiguous commit must never set a cookie or return a session. Every login must issue a new bearer (no fixation/reuse).

## Candidate endpoints (NOT implemented or routed)

| Route | Request and credential boundary | Candidate response |
| --- | --- | --- |
| `POST /v1/identity/login` | 2048-byte bounded strict JSON `{"email":"...","password":"..."}`; no cookie or Authorization input; exact TLS Host, Origin, peer and Fetch Metadata; duplicate/unknown JSON keys rejected. Call **SessionLogin** (concrete durable admission → Argon2id → SQL recheck) only. | **204 with Set-Cookie** after confirmed commit, no body; 400 malformed, generic **401** known/unknown/bad/unverified/frozen credential, **429** admission exhausted, **503** unavailable/ambiguous; no account identifiers or role reason. |
| `GET /v1/identity/session` | Accept **only** one valid `__Host-mamony_sid` cookie and exact trusted HTTPS origin/Host; call database session Resolve. GET is read-only and does not rotate/extend the session. | Candidate 200 with a minimal, non-sensitive authenticated boolean; 401 invalid/expired/revoked; 503 DB failure, all `no-store`. Never return raw bearer, verifier, roles, permission or contact data. |
| `POST /v1/identity/logout` | Require exact TLS Host, same-origin Origin, strict cookie selection, **validated CSRF token bound to the current session**, and revocation via v10 `Revoke`. No logout via GET. | 204 with cookie deletion on definitively completed revocation; invalid or already revoked well-formed token may be normalized for UX without revealing account existence; DB/commit uncertainty must **not** be represented as guaranteed logout. |
| `GET /v1/identity/csrf` | Proposed same-origin `no-store` endpoint after valid cookie Resolve, to supply a CSRF token (never the session token). Details below must be implemented and tested before any state-changing cookie-authenticated route. | Minimal CSRF token returned only to the first-party origin; 401/503 on invalid/unavailable session. |

Status, body and timing details are **candidates** for acceptance tests, not promises about deployed behavior. Uniform generic credential failure status/body does not imply constant-time SQL, proxy or KDF timing. The existing login gate's 80-global/minute/5-alias/hour candidate quotas remain unapproved for production and denied attempts still hit PostgreSQL: perimeter rate limiting and credential-stuffing monitoring are required.

## CSRF and login CSRF

- Every cookie-authenticated state-changing route (including logout and **financial mutations**) must validate **a strong CSRF mechanism bound to the server-side session**, in addition to exact `Origin` and restrictive Fetch Metadata. Treat absent/ambiguous Origin as invalid for browser POST; never accept an Origin inferred from Referer or a forwarded header.
- Candidate design for JavaScript-based same-origin UI: use `GET /v1/identity/csrf` to return a separate unguessable or server-HMAC-authenticated token; the client sends it in `X-Mamony-CSRF` on state changes. Compare in constant time, bind to the **current session digest or server-side session record**, enforce short validation window and rotation behavior. Do **not** reuse the session bearer, pass the CSRF token in URLs, or accept an unsigned attacker-chosen double-submit cookie. Production secret distribution, key rotation, session binding and anti-replay test cases are **open**, and no CSRF token generator is implemented here.
- Login CSRF can cause a victim to become logged in to an attacker's account; the login POST must require a strict same-origin Origin/Fetch Metadata check and should obtain an approved **pre-login CSRF strategy** if browser topology permits cross-origin or old clients without trusted Origin. Do not confuse login absence of an existing session cookie with CSRF safety.
- SameSite, CORS, JSON content type or a custom header **alone** is insufficient. Explicitly forbid credentialed cross-origin CORS until an independently reviewed trust model exists.

## Safe revocation and concurrent account changes

The PostgreSQL issue function locks the subject and alias, then conditionally writes a digest and mandatory security event. **New integration coverage in PR #42** observes actual PostgreSQL lock waits in both orders: (a) frozen status commits before the waiting issuer, which must deny and write no session/event; (b) issuer commits before a waiting freeze, which bumps the epoch and makes the issued digest invalid. A subsequent thaw cannot resurrect it. A rollback after nominal issuance must undo **both** the digest and audit event. These tests are not an authorization guarantee for any later financial action; resource/tenant/amount-level authorization must recheck state at its own transaction boundary.

Logout POST must be idempotent at the database layer. A disconnected client or ambiguous commit after revocation may observe 503 even if revocation completed; the frontend should not claim success and should present a safe retry/recovery path. Clearing a local cookie alone does **not** invalidate a copied session on an attacker device.

## Release blockers and acceptance checklist

Before mounting any new endpoint: approve session-role direct-mint risk and deployment isolation; cookie+CSRF implementation and browser tests; edge TLS/trusted peer and reverse-proxy contract; exact public response/error/timing contract and anti-enumeration tests; 12h/idle/refresh/step-up/MFA policy and reauthentication; session theft and reuse monitoring, logout handling, account recovery/password/contact change epoch bump; cross-device session limits; real traffic/NAT/KDF capacity tests and WAF; privacy-preserving audit; and action/resource/tenant authorization separate from session Resolve. Require all five CI workflows on an exact commit, including the deterministic lock-interleaving integration test. No API permission may derive solely from possession of a resolved subject ID.

Standards consulted as design context, not a claim of certification: NIST SP 800-63B-4 session guidance (2025), OWASP CSRF Prevention Cheat Sheet, and MDN HTTP Set-Cookie prefix/attribute behavior. The contract and project-owned code/tests in this repository remain the actual source of requirements.

**Do not publish this candidate in released OpenAPI or enable it in the default public/private router.**
