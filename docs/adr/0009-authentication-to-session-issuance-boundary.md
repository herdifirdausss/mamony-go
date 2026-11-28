# ADR 0009 — Authentication-to-session issuance trust boundary (Proposed; Draft PR #45)

Status: **PROPOSED design.** [PR #46](https://github.com/herdifirdausss/mamony-go/pull/46) merged the synthetic v11 grant-bound candidate. Owner and security acceptance and production workload proof remain outstanding.
Parent stack: Draft #45 → #44 → #43 → #42 → #41 → #40. Profile **A**, synthetic first-party browser only.
No new public routes, migration, DB roles or trusted production workload boundaries are introduced by this ADR.

## The concrete bypass (reproduced)

This section records the v10 finding. The v11 candidate removed the subject-only issuer, and the existing integration test now checks denial. See the [merged synthetic scope](../contracts/identity.md#password-login-and-browser-sessions) for the current implementation path.

Identity v10 has `mamony_identity.issue_opaque_session(subject,digest,event)`, a `SECURITY DEFINER` function. It checks that the subject is ACTIVE, has a verified contact, and has a current session epoch; it does **not** check that the caller verified a password. Its dedicated `mamony_identity_session_runtime` SQL role has direct EXECUTE. An attacker who obtains **only that DB role credential** and learns an eligible `sub_...` ID can choose their own 256-bit bearer, submit its SHA-256 digest, and receive a valid session. The subsequent cookie's HttpOnly, CSRF, pre-KDF admission and Argon2id in Go cannot defend this direct SQL path.

`TestIdentityZDirectSessionMintThreatFixture` proves this in disposable PostgreSQL using a synthetic ACTIVE+verified account with **no password-verifier row**. The test intentionally **passes while the weakness exists** and must be replaced by a deny test once the SQL authority is redesigned. This is not a pen test, nor a production-credentials exposure.

Relevant existing code: `db/migrations/identity/000010_opaque-sessions.up.sql`, `internal/modules/identity/internal/repository/postgres/opaque_session.go`, `internal/modules/identity/internal/usecase/opaque_session.go`; ADR-0006 through ADR-0008.

## Scope and threat boundaries

| Attacker capability | Current v10 | Proposed narrow improvement | Explicit limit |
|---|---|---|---|
| Only session-runtime DB credential, known ACTIVE/verified subject | Can mint any chosen bearer without password | **Deny** subject-only mint; session-role use requires unpredictable proof from separate restricted authority | A stolen **unconsumed** proof can still be spent within its scope |
| Only grant-writer DB credential | Not applicable today | Cannot call session-mint/revoke/resolve or read session/credential tables | If it can create arbitrary subject proofs, compromising it still enables impersonation via a collaborating issuer or proof theft |
| Full Identity login runtime/RCE with both credentials | Can mint | **Not guaranteed** to resist: attacker can ask for/issue false proofs and sessions | Needs a separately enforced verifier/issuer boundary for a stronger claim |
| Identity migrator, DB owner/superuser, deployment root | Can defeat local DB authority | **Not contained** in Profile A | Independent authority design belongs to Profile B |
| Ordinary unauthenticated browser, BFF only | No direct SQL access assumed | Existing pre-KDF admission, Origin, CSRF and cookie controls stay necessary | A malicious internal credential must be modeled separately |

A new database role by itself is *not* proof that the Go runtime can only sign/record valid password comparisons. The database cannot directly verify an Argon2id password using the current SQL authority. A cryptographic MAC or KMS signing oracle also provides no protection against an attacker who can invoke it for arbitrary subjects.

## Options and recommendation

| Option | Defends leaked session DB credential? | Complexity | Caveat |
|---|---|---|---|
| A. Network/secret isolation only | Limits reachability but **no** once stolen | Small | Single credential is still an unrestricted mint capability |
| B. Short-lived, single-use, epoch-bound grant **issued by a separate restricted grant role after successful Go authentication** | **Yes, conditional**: SQL session role alone cannot invent an unpredictable grant | Moderate (v11 SQL, role, usecase, migrations and race tests) | Attacker with grant-writer credential can fabricate grants unless grant creation itself has another trust/control boundary |
| C. Independent, policy-restricted authentication issuer with non-exportable proof keys and independent commit authority | Potentially, depending on issuer and verifier proof rules | High | If signer accepts arbitrary subject/payload requests, nothing is gained; inappropriate to imply Profile B by adding a service/KMS name |

**Proposed next implementation: B as a narrowly scoped Profile A containment control**, conditional on documenting and enforcing whose workload may possess each role. Do not describe B as independent assurance against compromised Identity runtime. If the target is to survive full login-runtime compromise, revisit C with an independent authority and adversarial oracle tests **before** claiming that security property. Option A remains a deployment hardening requirement even with B.

## Proposed v11 contract (not yet implemented)

1. **No legacy entrypoint:** forward-only, integrity-fingerprinted migration v11 revokes and removes the old subject-only `issue_opaque_session(text,bytea,text)`. Do not edit fingerprinted v10 in place. Also inventory `PUBLIC`, inherited role, function overloads, migrator and owner privileges. Test effective privileges with `has_function_privilege`, not only GRANT text.
2. **Separate grant authority:** a new narrowly scoped `mamony_identity_auth_grant_runtime` role may call only a typed `stage_password_auth_grant(subject,grant_digest)` function. It must have no session ISSUE/RESOLVE/REVOKE, no direct table writes, no KDF verifier SELECT, no role membership allowing escalation. Creation of a grant means **the trusted Go authenticator asserts it completed the required password policy**, not that PostgreSQL independently proved the user's password.
3. **Cryptographically unpredictable secret:** after durable pre-KDF admission and successful real Argon2id, Go generates 32 random bytes from `crypto/rand`. The database stores only a domain-separated SHA-256 digest, with exact purpose `PASSWORD_LOGIN_SESSION`, subject, current session epoch, DB-issued timestamp, short DB-clock expiration (candidate 30 seconds), and consumed state. The raw secret never enters SQL, logs, URLs or cookies.
4. **Atomic consumption:** a replacement `issue_opaque_session_with_grant(grant_digest,session_digest,event_id)` callable by the **session** role finds an unexpired, unconsumed grant for the exact purpose; row-locks it and the current subject/contact, checks current epoch/ACTIVE/verified contact, then **in one DB transaction** consumes the grant, inserts the 12-hour session digest and inserts `SESSION_CREATED` evidence. Subject is resolved from the trusted grant, not a second caller-supplied `subject_id`. No alternative unguarded INSERT/function privileges.
5. **Race/replay:** parallel consumes of the same grant yield at most one session; a freeze/epoch change before consumption denies issuance. If issuance commits and subsequent freeze occurs, Resolve rejects the earlier epoch. Missing/expired/wrong-purpose/replayed/modified grants fail closed, and no session/audit record appears on denial.
6. **Uncertain commit:** if grant stage or session issue returns an ambiguous result, never invent success, return a bearer, silently retry a different command or repeat the grant. Use a fresh verified authentication on a new explicit attempt. No reuse of the old grant after timeout is claimed.
7. **Maintenance and availability:** stage/consume limits and bounded retention of expired/consumed grant digests must not allow unbounded rows, lock amplification or a global account-existence oracle. Rate limits remain mandatory before Argon2id. No secret or grant should appear in application/problem+json, GitHub traces or audit fields. Review cleanup races against in-flight SQL before applying a TTL.
8. **No permission inflation:** a valid grant creates a session only, never tenant membership, reauthentication assurance beyond password, MFA completion, recovery, administrative permission or financial transaction approval.

The logical protocol should be evaluated against the Stage 3 reference's **I04 (proof consumed at most once)** and **I08 (login is not business authorization)** without assuming this Profile A implementation already possesses Profile B current-head anchors or independent security roots.

## Mandatory G3/G4 verification before public availability

The v11 implementation PR must add real-PostgreSQL evidence for:

- **Negative v10 capability:** direct old `issue_opaque_session` denied even for a known eligible subject; test `PUBLIC`, app registration, authenticator, delivery, grant and session roles against all new functions and tables.
- **Positive chain:** real password check + pre-KDF admission → purpose/subject/epoch-bound grant → one session, identical cookie/CSRF/logout semantics in actual Chromium.
- **Negative proof:** wrong/unknown/unverified/frozen password never produces a grant or session; a SQL session role alone cannot create, read or guess a grant; no attacker-chosen subject parameter on new issuer.
- **Concurrency:** two consumers of one grant, grant-stage vs freeze, consume vs freeze, rollback at every SQL boundary and lost COMMIT reply; no duplicate session/audit event.
- **Lifecycle:** freeze/thaw non-resurrection, replay after consume, expired proof under DB clock, forced session revocation and safe bounded cleanup.
- **Workload reality:** prove the grant writer credential and session writer credential are isolated as declared in the deployed runtime and network policies. If they remain in the same compromised process, record the uncontained RCE scenario without calling it independent verification.
- **Regression:** all five mandatory workflows PASS on exact final SHA. Existing public/private listeners return 404 for login/session/CSRF/logout. Do **not** merge the stack based on these tests alone.

## Decision gates / no implicit acceptance

**G2 — architecture:** approve the explicit threat scope and choose A, B, or C; decide whether protecting a single leaked DB role, rather than compromised Identity runtime, is an acceptable interim Profile A objective.

**G3 — protocol and permissions:** review the 30-second/one-use grant lifecycle, status/epoch locks, consumption ordering, role and DB migration permissions. Set production secret/network boundaries and rollback/incident procedure before deployment. Never rely on a second role that the same untrusted request process can freely call while claiming to survive that process's RCE.

**G4 — proof:** only after adversarial tests prove the old direct-mint capability is removed and all required integration/browser tests pass may the issue be considered *mitigated for the declared attacker scope*. Application routes remain disabled until other published release blockers are closed.

## Reference notes

- PostgreSQL 18 `CREATE FUNCTION` and `GRANT/REVOKE`: a `SECURITY DEFINER` function runs with owner privileges. Carefully revoke default `PUBLIC` EXECUTE in the creation transaction and use a trusted `search_path`. See https://www.postgresql.org/docs/18/sql-createfunction.html and https://www.postgresql.org/docs/18/sql-revoke.html.
- OWASP Session Management Cheat Sheet: unpredictable server-generated session identifiers, server-side lifecycle/invalidations, and not treating an identifier alone as proof that the authentication process was completed. https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html

These references inform the controls, but **neither guarantees** that a custom SQL proof grant is safe or that credentials sharing one runtime establish an independent trust domain.

## Implementation candidate checkpoint — stacked Draft PR #46

Migration v11 and its Go code have been introduced **as an unapproved Draft candidate**: the old subject-only SQL issuer is removed; a new stage-only DB role writes 30-second password-login grant digests; a different session DB role consumes an issued grant exactly once in the same transaction as the opaque session digest and outbox event. The subject is read from the stored grant rather than request input. These candidate controls are assessed by privilege-matrix, replay, expiry, freeze, rollback, concurrency and real Chromium tests.

The implementation remains inside a single Profile A Identity authority in the synthetic tests. A login-process RCE or grant-role compromise could fabricate successful-authentication assertions. These tests therefore do not justify a Profile B, independently verified authenticator, or full runtime-compromise security claim. Routes stay unmounted; approval and operational role isolation are still open.
