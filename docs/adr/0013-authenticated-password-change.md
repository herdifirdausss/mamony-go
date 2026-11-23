# ADR 0013 — Authenticated password change with separate SQL capabilities

Status: **PROPOSED Profile A candidate; no public mutation route, no release approval**.
Stack: Draft #50 → #49 → #48 → #47 → #46 → #45 → #44 → #43 → #42 → #41 → #40.

## Threat and authority

A password change must not be authorized by a valid cookie alone. A stolen session should not silently become an account-takeover credential, and a login grant must not double as a password-change grant. There are three independently checked things: **a currently valid session**, **a freshly verified existing password with pre-Argon2id admission**, and **a new password meeting the reviewable policy**. None alone authorizes mutation.

OWASP Authentication Cheat Sheet recommends verifying the current password on a password-change request, and NIST SP 800-63B-4 requires checking prospective passwords against common/compromised-password blocklists. References: https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html and https://pages.nist.gov/800-63-4/sp800-63b.html.

## Internal Go composition

`AuthenticatedPasswordChange.Change(ctx,sessionSecret,canonicalEmail,oldPassword,newPassword,peer)`:
1. Requires a usable server-side session via `OpaqueSessionAuthority.Resolve`. This is still internal and refreshes idle timestamp, **not** a cookie-route contract.
2. Requires the existing concrete `PasswordLoginFlow.AuthenticateWithEpoch` to spend distributed pre-KDF admission and run actual Argon2id. It matches the verified subject **exactly** to the resolved session; the client cannot submit a subject ID. The subject epoch comes from the **same SQL snapshot** as its original verifier.
3. Validates and NFC-normalizes the new password with the mandatory `domain.PasswordPolicy` blocklist; rejects an unchanged normalized secret; hashes with bounded 64 MiB Argon2id + new 128-bit salt.
4. Stages a separate `PASSWORD_CHANGE` grant using the current-password-verified subject and pre-KDF epoch. It is a 256-bit random opaque secret persisted as a domain-separated digest, one-use and **30 seconds**.
5. Passes **session digest + change-grant digest + allowlisted new verifier + fresh event ID** to the dedicated password-change repository. If denied, rolled back or commit-ambiguous, returns sanitized failure and no new session.
6. Upon commit, the earlier session is invalidated by v14 epoch trigger. The client must authenticate anew; no token is silently extended or issued in the change response.

This is single-factor **old-password reauthentication** in addition to an authenticated session, not completed MFA/step-up, phishing resistance or a full account recovery flow. No HTTP handler or public route is mounted. The new-password blocklist requires an **actual reviewed production implementation**, not the synthetic test stub.

## SQL v15 capabilities and transaction

- Dedicated `mamony_identity_password_change_runtime` SQL credential has **EXECUTE only** on `rotate_password_verifier`, no table SELECT/UPDATE/INSERT, no grant staging, no session issuance and no migration capability.
- Existing grant-stager role gets only the new purpose-bound `stage_password_change_grant`. `password_login_grants.purpose` adds `PASSWORD_CHANGE`; the previous session issuer still rejects any purpose other than `PASSWORD_LOGIN_SESSION`.
- `rotate_password_verifier` locks the change grant first, derives its subject and epoch, requires a matching existing session token, then locks subject/current verified contact, the session row, and password verifier in consistent order. Rechecks account ACTIVE, epoch, revoked status, 12-hour absolute expiry and 30-minute idle expiry **after locks** using DB clock. The change grant is consumed once, and the Argon2id verifier changed atomically. The v14 trigger increments `session_epoch`, revoking old login and change grants and sessions, and the same transaction writes an identifier-only `PASSWORD_CHANGED` security outbox record. Outbox insert failure rolls back the whole change, including epoch bump and grant consumption.
- No password, raw bearer, grant secret, email, verifier hash or salt enters the security outbox. No actual notification mail is sent.
- Commit ambiguity cannot be distinguished from success in the caller; it reports unavailable and must not retry automatically. Old sessions are *not* exempted.

### Compromise boundaries

A compromised grant-writer credential remains an authentication assertion oracle. A change-role credential *alone* cannot rotate anyone; it needs a usable session digest and a fresh `PASSWORD_CHANGE` grant. If **multiple** sensitive credentials are compromised, Profile A trusted Go code can still be bypassed: SQL never independently verifies Argon2id. Public edge, transport/TLS, CSRF, secure origin/cookie, rate limiting and production workload secret isolation are **not** validated by this PR. No Profile B guarantees.

## Required tests and release blockers

- Unit tests: invalid bearer denies before KDF; mismatched current-password subject denies; wrong old password/limited admission denies before mutation; same or blocked new secret denies; successful composition calls exactly one rotation; storage commit ambiguity never returns success.
- Mandatory **real PostgreSQL** integration: actual admission + Argon2id + staged change grant + restricted SQL function; a change-purpose proof cannot mint a session; a normal login proof cannot rotate a password; incorrect old password leaves the verifier untouched; duplicate event triggers atomic rollback; one successful change logs one event and advances epoch; old bearer and password fail; new password logs in with a fresh session.
- SQL role matrix and clean fingerprinted v15 migration; all v11 grant, v12 expiry, v13 cap, v14 change epoch, Chromium cookie/CSRF/logout and 404 route contract tests remain mandatory. All **five workflows must pass on exact final HEAD**.

**Outstanding:** independent MFA/step-up policy; password history/reuse prevention (other than unchanged old); correct deployed common/breached/contextual password blocklist; raw HTTP endpoints and CSRF/on-change cookie removal; security email notifications; device-session UX; lock contention/load and deadlock handling; operational secret isolation; recovery with verified ownership; and tenant/resource financial action authorization. All stacked PRs remain Draft; **do not merge or activate**.
