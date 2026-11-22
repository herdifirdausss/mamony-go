# ADR 0011 — Atomic five-session cap per Identity subject (Draft PR #48)

Status: **PROPOSED Profile A candidate; policy owner/security review NOT complete**.
Stack: #48 → #47 → #46 → #45 → #44 → #43 → #42 → #41 → #40.
All Identity browser routes remain UNMOUNTED (default public/private listeners return 404). No financial authorization or production workload boundary is activated.

## Problem and policy

v10–v12 sessions have 256-bit opaque bearer digests, status/epoch/revocation validation, 12-hour fixed expiry and DB-clock 30-minute inactivity expiry. They do **not** bound how many distinct, currently usable sessions a verified account may have. An attacker holding a password or a compromised session device can keep opening extra logins, increasing the number of still-usable bearer tokens.

**Candidate policy:** for one currently ACTIVE/verified Identity subject and current `session_epoch`, count only unrevoked sessions whose 12-hour absolute and 30-minute DB-clock idle cutoffs are both unmet. At 5 currently usable sessions, a successful *new* password login **atomically revokes the oldest live session** (ordered by `issued_at`, then `token_digest`) and creates one replacement. Login with fewer than 5 slots does not revoke. Account/session status is always authoritative in PostgreSQL, never a browser-side counter. The 5-session limit is **not user approved** or claimed to satisfy NIST AAL requirements.

OWASP Session Management Cheat Sheet discusses concurrent-session management and user-accessible session revocation. See https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html. PostgreSQL row-lock semantics: https://www.postgresql.org/docs/16/explicit-locking.html.

## SQL v13 contract

A **forward-only, integrity-fingerprinted** v13 Identity migration creates a subject/issuance ordering index and replaces the existing restricted `issue_opaque_session_with_grant`, `resolve_opaque_session` and `revoke_opaque_session` functions using `CREATE OR REPLACE` and the same signatures/owners/EXECUTE-only privileges. Historical v10, v11 and v12 files are never edited.

1. Session issuer accepts only v11 grant digest, token digest and an identifier-only `SESSION_CREATED` event ID. It derives the subject **from the locked, single-use grant**, then locks verified account/status before any session decisions. No caller-controlled `subject` argument and no bypass of the Argon2id/admission composition in the trusted Go flow. A 30s grant is checked with the DB clock *after* locks, then consumed exactly once.
2. One subject lock serializes *distinct* grants on the **same account**. After that lock and grant consume, count only sessions currently unrevoked, same epoch, not past 12h and not idle for 30 minutes. The count and eviction execute under the **same transaction** as the new session INSERT and `SESSION_CREATED` event. No distributed lock, in-process map or extra privileged role is introduced.
3. If live count is 5, lock and revoke the **oldest live** token digest without leaking the digest or bearer outside PostgreSQL; insert one `SESSION_REVOKED` security-outbox record with a fresh DB-generated identifier. `gen_random_uuid()` is part of PostgreSQL core on supported PG versions; the 16-byte UUID's base64url encoding uses `uuid_send`, `encode`, `rtrim`, and `translate` (22 chars after `evt_`). The outbox event is **not** a bearer or proof secret. If the event INSERT/eviction/session INSERT fails, the entire transaction must roll back; no partial eviction is allowed.
4. If a historical account already has **more than five live sessions** before this migration, **deny further session issuance**; do not silently bulk-evict arbitrary devices or perform an unbounded row update in a login transaction. Existing sessions may remain until revoked, idle-expired, absolutely expired, or an explicit, separately reviewed operational cleanup. This is a **bounded new-issue cap**, not an immediate retroactive global cleanup guarantee. Investigate pre-v13 active counts before any rollout.
5. Idempotence and ambiguity: a consumed/replayed/expired grant still cannot mint; a denied/ambiguous issuance does not return its bearer. The UI must not automatically retry with the same grant. If two distinct successful logins race, the second sees the first committed session under the subject lock and makes its own oldest-session decision, never exceeding the cap.

## Lock-order change: prerequisite to avoiding deadlock

The earlier v12 resolver combined `FOR UPDATE OF ss, s, a` in one join. An issuer taking the **subject lock before the oldest session lock**, concurrent with a resolver locking the **session before subject**, can form a lock cycle. Therefore v13 updates all relevant session operations to acquire locks in an explicit, consistent order: **identify subject without privilege escalation → lock subject/contact eligibility → lock particular session row**. Logout locks subject before the session row and preserves idempotent revoke and outbox semantics. These are `SECURITY DEFINER` functions; application SQL roles remain EXECUTE-only, with no direct SELECT/UPDATE on session tables.

The resolver samples `clock_timestamp()` **after** both locks and rechecks current epoch/status/verified contact, idle and absolute expiry, revocation, and clock rollback. It refreshes `last_seen_at` only for still-valid sessions; it never extends `expires_at`. The logout/revoke function still marks the digest revoked exactly once and never releases a secret. Status freeze takes the same subject lock, so a freeze committed first invalidates the session; when a pre-freeze read wins, a subsequent protected financial action still requires its own fresh action-level authorization.

## Mandatory proof and operational risks

`TestIdentityZMaxActiveSessions` must use **real disposable PostgreSQL**, a grant-role fixture (which is an asserted login, not independent password proof), and verify:
- First five grants produce five usable sessions and five `SESSION_CREATED` records, no revocation.
- Sixth grant leaves **exactly five live sessions**, oldest is immediately invalid, newest resolves, and exactly one `SESSION_REVOKED` event has a valid bounded event ID.
- A session past idle expiry does **not** count against the cap; a new login fills the free slot without a forced revocation.
- An aborted transaction rolls back grant consumption, forced oldest-session revocation, new token and outbox inserts together.
- Two distinct grants blocked behind a deterministic subject lock both succeed sequentially without ever admitting a sixth live session, with corresponding audit events.
- v13 migration fingerprint, privilege matrix, all prior v11 grant/freeze and v12 idle cases, both browser E2E tests, and all five CI workflows pass on the exact final SHA.

**Operational blockers**: subject-row hot spot and per-request write locks already introduced by v12 can cause denial of service under sustained session resolution; cap/eviction amplifies contention. Benchmark expected concurrency and configure lock/deadlock timeout and upstream pre-KDF/WAF capacity. Real production role/network isolation from the grant assertion oracle remains unproven. Also outstanding: password/contact change epoch invalidation, MFA/recovery/step-up, remote session listing/revocation UX, key rotation, accessible 401/expired/evicted UI, alerting, financial action tenant/resource permission checks and G2/G3 owner acceptance. No Profile B independent issuer assurance is implied.

Keep PR **Draft**. No merge to main, public route mounting, or production-readiness claim from green CI.
