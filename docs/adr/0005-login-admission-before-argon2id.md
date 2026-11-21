# ADR 0005 — Pre-Argon2id login admission (Draft PR #40)

Status: **Candidate, internal only; no public login or session route.**

## Context

ADR-0004 added a password comparison primitive with a read-only authenticator role and Argon2id v1 (64 MiB, 3 iterations, p=1). For an unknown but syntactically valid email, the service deliberately executes one dummy Argon2id verification to limit an account-existence timing signal. Without **prior** request admission, arbitrary unknown emails can exhaust the bounded KDF slots and deny service to legitimate users. A local in-memory mutex or client-provided retry token is not sufficient across replicas.

The Identity module already owns a durable `mamony_identity.anonymous_admission_buckets` table with HMAC-SHA256 digest keys, single-DB-clock fixed windows, transaction-atomic multi-scope increments, restricted `mamony_identity_runtime` SQL grants, and a 48-hour maintenance purge. REGISTER/RESEND/CONFIRM must retain their original quota and authority; LOGIN must not borrow those budgets.

## Decision

### Account-independent rate admission

`LoginAdmission.Admit(ctx, email, peer)` needs a **validly shaped** canonical email address and a trusted direct client network address from the existing `ClientIPResolver` (not a caller-supplied forwarded header). Admission never checks whether an email is registered or verified, and does not receive a password, subject ID, session or bearer.

A distinct HMAC namespace `mamony:identity:login-admission:v1` derives three key types: one *global constant*, an exact IPv4 address or IPv6 /64 prefix, and the canonical email. Even when the HMAC secret happens to be shared in a test, LOGIN keys cannot be mistaken for REGISTER/RESEND/CONFIRM keys. The **secret must be provisioned securely**, rotated with an explicit dual-key/window strategy, and must not appear in SQL, logs or HTTP. The fixed digest and action/scope columns are the only persisted identifiers.

Five budgets, all mandatory and evaluated in this exact row-lock order under **one PostgreSQL transaction and one database clock sample**:

| Scope | Fixed window | Candidate maximum |
| --- | --- | ---: |
| `GLOBAL_MINUTE` | 60 seconds | 80 |
| `CLIENT_MINUTE` | 60 seconds | 8 |
| `CLIENT_HOUR` | 3,600 seconds | 80 |
| `EMAIL_HOUR` | 3,600 seconds | 5 |
| `EMAIL_DAY` | 86,400 seconds | 20 |

All are conservative **unapproved** candidate limits; production sizing must consider expected TPS, NAT, mobile networks, IPv6 /64 sharing, KDF concurrency/memory, distributed edge rate limits and accessibility/retry UX. For example, 80 globally admitted per minute is **not** a reasonable scale assumption for a busy public financial app. If any budget is exhausted, the transaction rolls back all partial increments and the internal result is denied. If the store, clock, Begin/Commit, timeout or remote network identity is unavailable, the request **fails closed**, without attempting a credential lookup or KDF. A commit response may be ambiguous; do not automatically re-spend within this call.

**Important limitation:** On a denied attempt, the transaction rolls back its earlier global/client increments. This preserves prior v5 semantics but means very high volumes of **already throttled** emails still reach the database without increasing global counts. A separately enforced, account-agnostic **edge/WAF load-shedding gate** must protect the database itself before public use. Fixed UTC-aligned windows can allow double bursts at boundaries; the global bucket is also a potential single-row contention hotspot. A dedicated partitioned/time-sliced or distributed limiter needs load evidence before production.

### Composed internal boundary

`PasswordLoginFlow.Authenticate(ctx, email, password, trustedPeer)` requires two **non-nil** ports: `PasswordLoginAdmission` and `PasswordLoginAuthenticator`. It always calls admission **first**, and only calls the existing `PasswordAuthentication` when admission committed a positive result. No KDF/credential read is attempted on denied, malformed or unavailable admission. For syntactically valid unknown aliases, the admitted password-authenticator still executes the dummy v1 Argon2id operation from ADR-0004.

The composed result can distinguish internal `ErrPasswordLoginThrottled`, generic `ErrPasswordAuthenticationDenied`, and sanitized `ErrPasswordLoginUnavailable`; **none are approved public wire responses**. Internal denial does not disclose account existence. A future HTTP endpoint must explicitly review uniform error/timing behavior (including 429), privacy-preserving observability and session creation before exposure.

### Persistence and privileges

Forward-only, SHA-256-fingerprinted Identity migration **v9** extends the **existing** bucket table's `action` CHECK to include `LOGIN` and the `scope` CHECK to include `GLOBAL_MINUTE`. It creates no new table, user, grant or data rewrite and leaves the existing hits CHECK upper bound (80) unchanged. The prior login-admission `mamony_identity_runtime` can only perform the previously granted column-scoped INSERT and `UPDATE(hits)` operations. It cannot read or edit password verifiers, account statuses or proof material. The authentication reader retains its **separate read-only** `mamony_identity_authenticator_runtime`. There is no LOGIN route registration, password session, cookie or authorization grant.

## Security evidence and limits

Required Go tests assert HMAC namespace isolation, normalized email and IPv6 /64 accounting, unknown-alias independence, fixed five-bucket order, SQL error redaction, cancellation/invalid-peer fail-closed, and **zero** credential/KDF calls unless the admission port approves. PostgreSQL integration runs independent admission instances with separate pools against the disposable database; only five concurrent requests for the same nonexistent email can win, five allowed login checks can reach the real Argon2id adapter, the sixth is rejected pre-KDF, and global/per-subnet bounds persist as 32-byte digests. Existing registration/resend budgets remain independent. The exact migration fingerprint and role isolation remain CI gates; the real Chromium verification flow continues unchanged.

Release blockers: secure secret provisioning/key rotation, a trusted edge client-IP/TLS model, load-tested global+per-account budget tuning, a perimeter limiter and denial-service defenses, account stuffing telemetry without PII, response-time/UX behavior, possible MFA/step-up, fully designed opaque sessions with **atomic** ACTIVE/verified recheck, expiry, logout and revocation, plus formal operational approval. Do not mount login or merge to main solely from green CI.
