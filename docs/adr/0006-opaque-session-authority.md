# ADR 0006 — Opaque session authority with epoch-bound eligibility (Draft PR #41)

Status: **Candidate, internal only; all public/private routers remain health-only**.

## Context

ADR-0004 provides real, bounded Argon2id password comparison; ADR-0005 adds distributed pre-KDF LOGIN admission, returning an **internal subject candidate**, not a session or financial authorization grant. Between password verification and token issuance, another transaction can suspend or freeze the account. A naive `INSERT session(subject_id)` after authentication would mint a credential for an account that has already become ineligible. In addition, sessions issued before an account is frozen **must not resurrect** when an administrator later restores the account to ACTIVE.

The existing Identity database owner already uses immutable, fingerprinted migrations, separate restricted registration/proof/authenticator roles and an identifier-only security outbox. We need a minimal separately authorized session authority without granting the registration or verifier role the ability to read or write session records.

## Decision

### Secrets, expiry, and internal API

Use **opaque, random 256-bit session bearers**, encoded as canonical 43-character unpadded base64url. Go creates a fresh value using `crypto/rand` and sends **only** `SHA-256("mamony:identity:session-token:v1\\x00" || 32 raw bearer bytes)` to PostgreSQL. This is appropriate for a uniformly random 256-bit secret, **not** a password hash. Never persist raw bearer strings, include them in SQL/logs/traces, place them in a URL, or expose them via automatic JSON marshaling or `fmt` representations. The one explicit `ForTrustedTransport()` extraction method is for a **future reviewed TLS/secure-cookie boundary**, NOT enabled in this PR.

Sessions have a fixed **12-hour absolute lifetime from the PostgreSQL clock**; there is no sliding renewal, refresh token, remember-me, session family, or idle timeout in this candidate. The `opaque_sessions` table contains only a token digest, subject ID, account epoch, issue time, expiry, and optional revocation time. A 32-byte digest is indexed by primary key. `Resolve` returns an internal subject ID only if the digest matches an unexpired, unrevoked session whose subject is currently **ACTIVE** with a verified contact, and whose **session_epoch** still equals the epoch captured when issuing. An unknown, stale, expired, revoked, frozen or invalid token is always an invalid session; SQL faults remain a separately sanitized unavailable result.

### Atomic recheck and status epoch

A new `subjects.session_epoch` (positive bigint, initially 1) is bumped by a database `BEFORE UPDATE OF status` trigger **in the same transaction** whenever status changes. The issue function takes `FOR UPDATE OF s,a` on the subject and current contact row before rechecking ACTIVE, verified contact and contact version, then inserts the digest **and** identifier-only `SESSION_CREATED` security event within the transaction. A concurrent freeze cannot commit between this recheck and issuance commit. If the freeze wins, the issuer returns denied without committing a credential. If the issuance wins and a later freeze occurs, the epoch bump invalidates the session at resolution. A later thaw **does not** resurrect an old session. This avoids holding row locks during Argon2id: PR #40 admission and KDF happen first, followed by a short atomic DB recheck.

`SessionLogin` is an **internal composition** that calls the approved PR #40 password login flow (which itself requires durable admission before KDF) and then the session issuer; it never bypasses that gate. `OpaqueSessionAuthority.Issue` also exists for trusted internal composition/tests, and must **never be exposed directly to a public endpoint**. Any transport, login handler or future session consumer needs a separate security review.

The session role cannot independently prove a password: database-side issue functions trust the **highly sensitive server-side composition** to call them only after successful authentication. If a malicious actor obtains direct `mamony_identity_session_runtime` DB access, the EXECUTE privilege itself allows minting a session for any currently ACTIVE/verified subject ID the actor knows. That role therefore requires network isolation, narrowly scoped credentials, an audited invocation boundary, and production security review; this PR does not claim otherwise.

### Minimal, separate SQL authority

Identity **migration v10** (forward-only with SHA-256 integrity trailer) adds the session table, epoch trigger and a small set of fully qualified, `SECURITY DEFINER` PostgreSQL functions using only `pg_catalog,pg_temp` search_path:

- `issue_opaque_session(subject,digest,event)` — short row-locked eligibility recheck, DB-clock 12h expiry, atomic token digest + `SESSION_CREATED` event
- `resolve_opaque_session(digest)` — DB-clock expiry, status, verified-contact, epoch and revocation checks under row-share locks
- `revoke_opaque_session(digest,event)` — idempotent single-token revocation with exactly one `SESSION_REVOKED` event when a live/unrevoked digest is first marked
- `purge_expired_opaque_sessions()` — bounded 100-row deletion only after expiry **plus 24 hours**, with `SKIP LOCKED`; the existing maintenance facade invokes this along with its current encrypted-email envelope and admission-bucket purge within one transaction

A separately provisioned `mamony_identity_session_runtime` receives only schema USAGE and EXECUTE on the first three functions. No SELECT/UPDATE/INSERT/DELETE on any Identity table is granted. The bounded purge function is **maintenance-role only**. The functions are explicitly revoked from PUBLIC, no SQL strings are constructed from user values, and every parameter is bound. Existing verifier, registration, authenticator and delivery role grants are unchanged; neither may execute session issuance.

The Go repository adapter uses only the three reviewed functions, not direct table SQL. Errors from Begin/Commit, including **ambiguous commit**, are sanitized. The usecase MUST discard the in-memory secret on any unresolved issue outcome rather than retrying or returning a token that may not be durably committed. Denied/unknown outcomes never contain a subject ID or secret. Revoking an already revoked or unknown syntactically valid bearer is idempotent, with no duplicate security event.

### Lifecycle and limits

Tests use real disposable PostgreSQL, real Argon2id, PR #40 admission and atomic email confirmation, not a mocked active-user flag. They verify no session for an unverified or frozen account; successful session issue after password match and verified contact; digest-only persistence; 12h DB TTL; freeze/thaw non-resurrection; idempotent revoke with one outbox event; unknown token rejection; 100-row bounded maintenance interface; and privileges denied for all other runtime roles. Unit tests enforce redaction, entropy, correct hash domain, rejection on invalid secrets, nil roles, truncated CSPRNG and commit ambiguity, and the required session-login call order.

This **does not** create public login, cookie attributes, CSRF protection, tenant authorization, MFA, refresh rotation or logout HTTP routes. Account recovery/password reset/email change is **not** implemented; those future flows MUST revoke sessions or bump `session_epoch` in their own authoritative transaction. A database status change already bumps the epoch, but contact/credential changes without a status change require a later ADR. Resolve determines eligibility at one DB serialization point: a session could become ineligible immediately after resolve, so future authorization of a financial operation MUST recheck the relevant status/tenant/resource policy at the time of that action. Secure production credential provisioning, transport, network isolation, rate quotas, IP ingress, telemetry, KDF sizing, cookie flags, CSRF and continuous enforcement are separate launch gates.

**Keep the stacked PR Draft; do not merge or mount any route solely because CI is green.**
