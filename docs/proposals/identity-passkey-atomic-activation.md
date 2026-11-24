# Identity passkey atomic activation — Critical change proposal

**Issue:** [#78](https://github.com/herdifirdausss/mamony-go/issues/78)  
**Owner/technical reviewer:** repository owner / designated independent reviewer (**acceptance not yet recorded**)  
**Capability:** Identity / WebAuthn, following merged #75–#77  
**Status:** **PROPOSED** — G0/G1 completed, G2/G3 owner and technical decisions pending  
**Risk:** Critical: new credential trust state + SQL runtime authority + forward-only migration  
**Evidence reviewed:** 2026-11-23  
**Related:** [ADR-0038](../adr/0038-webauthn-atomic-provisional-registration.md), [ADR-0039](../adr/0039-passkey-activation-challenge-evidence.md), [ADR-0040](../adr/0040-passkey-activation-signed-assertion-verifier.md), [proposed ADR-0041](../adr/0041-passkey-atomic-activation.md); `IAM-08`, `IAM-10`, `IAM-11`, `IAM-13`

## G0 — Problem and desired outcome

The user has completed a valid WebAuthn registration staging and a fresh, signed assertion against the original credential. V35 `VerifyOnly` validates the signature but does not persist an activation decision: while the nonce is current, the *same signature can verify repeatedly*. There is no binding of verified assertion evidence to the current durable session/credential/counter/nonce, no one-winner transaction, and no separate activation audit or notification.

**Outcome:** authorize at most one non-login **possession verification** of the same staged credential, with one committed nonce consumption, state/counter/backup-state update, identifier-only audit and mandatory recipient-verified notification obligation. Preserve all public/private routes as health-only.

## G1 — Evidence and alternatives

| Observation | Evidence | Checked | Limitation |
|---|---|---|---|
| V33 records exactly `PENDING_ASSERTION`, unique credential ID and COSE key, consumed registration intent/admission and queued registration notice | `db/migrations/identity/000033_passkey-registration-atomic-stage.up.sql` | 2026-11-23 | `fmt:none` alone does not authenticate later possession |
| V34 reader issues 3m challenge capped 10m from stage; restricts original session/subject/epoch/contact/RP and returns COSE material, nonce, counter and flags | `db/migrations/identity/000034_passkey-activation-challenge.up.sql` | 2026-11-23 | Reads can repeat and do not consume |
| V35 verifier uses pinned `go-webauthn v0.18.2` and returns signed facts plus original read evidence via private `verifyForActivation` | `usecase/passkey_activation_assertion.go`, `repository/webauthn/activation_assertion.go`, ADR-0040 | 2026-11-23 | A successful `ValidateLogin` is NOT a DB authorization commit |
| Counter nonprogress and BE/BS semantics must be explicitly respected; zero counters don't establish clone detection | [W3C WebAuthn L3 Recommendation](https://www.w3.org/TR/2026/REC-webauthn-3-20260825/); [go-webauthn v0.18.2](https://pkg.go.dev/github.com/go-webauthn/webauthn@v0.18.2/webauthn) | 2026-11-23 | Device provenance, synchronized keys and recovery policy remain separate |
| Role privilege restrictions cannot withstand a compromised owning DB root or activated SQL verifier runtime | `docs/architecture.md` Profile A; `docs/development-workflow.md` | 2026-11-23 | Not Profile B |

**Options:**
- **A — Recommended:** new `POSSESSION_VERIFIED` state but *no* login/recovery entitlement. Pro: establishes durable one-time possession evidence without an implicit privilege upgrade; lower deployment risk. Con: requires later explicit promotion and authorization design.
- **B — Immediate `ACTIVE` passkey:** shorter subsequent login path, but grants ambiguous privilege to any future consumer of `ACTIVE` and may silently elevate password-backed enrollment. **Not recommended** without independent policy/review.
- **C — No state transition, just mark nonce consumed:** stops replay but does not provide a named durable evidence state or explicit maintenance/monitorability. Insufficient for the stated outcome.

## G2 — Proposed requirements and acceptance (not accepted contract)

The source of truth for accepted requirements remains `docs/contracts/identity.md`. After approval, promote stable AC IDs there. These are **proposed IDs**.

| Proposed AC | Requirement | Evidence (all NOT RUN) |
|---|---|---|
| PKA-01 | The exact response validated once by the pinned WebAuthn library and the original v34 evidence is passed into a dedicated trusted finish; it cannot bind another credential/subject/RP/nonce. | Go unit + real DB |
| PKA-02 | One v34 nonce can commit **once**, including under at least 16 concurrent finishes; all replays denied. | PostgreSQL real concurrency/barriers |
| PKA-03 | Recheck live original session, current ACTIVE/verified subject, epoch/contact, RP/policy, immutable COSE key, exact v33 stage/provenance/consumed original registration intent and admission, and nonce/credential age after lock acquisition. | PostgreSQL negative matrix |
| PKA-04 | UP+UV required, BE immutable, BS valid; counter must advance if either is nonzero; 0/0 permitted but not called clone proof; SQL validates expected *previous* counter to prevent TOCTOU. | WebAuthn signature tests + DB race |
| PKA-05 | Credential state, counter/flags, challenge consumed_at, identifier-only audit and mandatory notice obligation commit as one transaction; failure of any INSERT/constraint/event rolls all back. | PostgreSQL injected collision/rollback |
| PKA-06 | Only new activation-finisher SQL login role can EXECUTE finish; original `session_user` fence, no table DML, no stage/issuance/session/recovery grants; other roles cannot execute. | PostgreSQL privilege matrix |
| PKA-07 | Strict 404 for public/private passkey routes; no WebAuthn login/session mint, no independent recovery factor or live provider. | Listener inventory + static/negative tests |
| PKA-08 | Unexpected DB/COMMIT/cancellation always sanitizes to unavailable and is never auto-retried; no bearer, challenge, raw SQL/COSE in logs or error response. | Unit/integration |

**Non-goals:** WebAuthn Begin browser options, UI, public registration/activation/login, device enrollment/revocation ceremony, AAL2 or independently bound factor, hardware attestation, notification worker, key-management and production rollout. No authz permission for financial actions.

## G3 — Proposed technical design

### Trust boundary and authority

- Go `PasskeyActivationFinisher` has a **concrete** `*PasskeyActivationAssertionVerifier` (not an injectable fake allow decision in production composition) and calls its internal `verifyForActivation(ctx,bearer,challengeID,raw)`, obtaining **one exact evidence snapshot** paired with the signed assertion facts.
- The verifier verifies exact original v34 nonce, RP origin and hash, credential ID/userHandle, COSE signature, UP/UV, sign count, BE/BS via `go-webauthn v0.18.2`. Public untrusted bytes never become a direct SQL role/subject choice.
- The coordinator derives the opaque session digest from the *server-validated* bearer, creates a random identifier-only audit event, and calls **one** `PasskeyActivationFinisherStore.Finish(ctx, request)` through the new isolated `mamony_identity_passkey_activation_finisher_runtime` DB credential. No untrusted HTTP path can call it.
- PostgreSQL `SECURITY DEFINER` `finish_passkey_activation` is deliberately a **trusted Go assertion oracle**: it cannot cryptographically verify WebAuthn within SQL. A compromised finisher DB credential or Identity DB owner can forge metadata under Profile A; it must **not** be shared with public ingress or the v34 issuer/reader/registration/other roles.

### Proposed schema and forward migration

**Identity capability v36 requires SQL migration `000035_passkey-atomic-activation.up.sql` (fingerprint version 35)**; v35 assertion verification added no SQL migration. Never rewrite v33/v34 migrations.

1. Change `passkey_credentials.state` CHECK from `PENDING_ASSERTION` to `PENDING_ASSERTION | POSSESSION_VERIFIED`. Add nullable `possession_verified_at` and `possession_event_id` (unique FK to security event), with a consistency CHECK ensuring both non-null exactly when possession verified. Existing rows remain pending; no backfill or silent activation.
2. Extend `security_outbox` event type allowlist with `PASSKEY_POSSESSION_VERIFIED`; never change generic event permissions.
3. Add `passkey_activation_notices` with verified recipient snapshot, event/subject/timestamp and strict `PENDING` only. Trigger only when `event_type=PASSKEY_POSSESSION_VERIFIED` and original `session_user=...finisher_runtime`. Failure aborts activation. No sender is created.
4. Add only schema USAGE + EXECUTE on `finish_passkey_activation` for separately provisioned finisher role. Revoke PUBLIC and deny other runtime roles. No direct SELECT/UPDATE on credential/session/challenge/outbox/notice tables.

### Proposed commit sequence (one SQL transaction)

```mermaid
sequenceDiagram
    participant Caller as Trusted internal caller
    participant Crypto as go-webauthn v0.18.2
    participant DB as Identity finisher SQL
    Caller->>Crypto: Original nonce + signed assertion
    Crypto-->>Caller: Exact evidence + verified facts
    Caller->>DB: finish(session digest, original evidence, verified facts, event)
    DB->>DB: Lock subject+alias -> session -> credential -> challenge
    DB->>DB: Recheck epoch, RP, COSE, state, nonce, clock, counter, stage proofs
    DB->>DB: Consume exact nonce
    DB->>DB: Transition POSSESSION_VERIFIED, update counter/BS
    DB->>DB: Insert audit; trigger enqueue mandatory notice
    DB-->>Caller: true only on certain commit, else denied/unavailable
```

**Commit checks:** Resolve subject *only* from session digest, never caller-supplied subject; subject ACTIVE, alias verified/contact_version=1; exact original session digest, current epoch, nonrevoked, 30m idle/12h absolute; pending credential provenance `PASSWORD_REAUTH_ONLY`, epoch, contact, RP/origin and **byte-exact original COSE key**; current v34 challenge ID + 32-byte nonce, purpose, RP generation, unconsumed/unexpired, same credential/session; original v30 intent+v32 admission already consumed and stage event + queued notice present. Sample DB clock **after** row locks. Check previous counter from the cryptographically verified snapshot still equals the locked DB row, plus signed observed counter semantics and backup flags. Requiring matching original COSE prevents accepting a response verified for a key that changed before commit.

**Lock ordering:** subject+alias → session → credential → challenge (v34 issuer uses credential before challenge; v34 reader uses SHARE locks and can block without inverting). Inspect actual query plans/wait graphs in disposable PostgreSQL; a deadlock is a FAIL, not a retry guarantee. Any side-effect failure aborts all writes. A competing v34 challenge replacement or account freeze cannot sneak in after validation.

**Lost acknowledgment:** if the DB committed and the connection dropped, Go reports sanitized `UNAVAILABLE`, not a fabricated success/rollback; no automatic replay. A separate future operator-safe inspection may confirm durable state without exposing session/challenge.

### Security / failure matrix

- **Attacker with only client JSON, replayed assertion or old v34 nonce:** signature/nonce/credential checks and one-use consumption reject.
- **Attacker with another live session belonging to same subject:** original same-session digest binding rejects.
- **Concurrent session freeze, alias update, credential rebinding, challenge overwrite, stale counter:** DB row locks and fresh state check reject or force ordered decision.
- **Malicious or compromised finisher DB credential:** can forge signature-derived claim within permitted function — **residual Profile A risk**, not a proof of independent assurance. Production credential separation/attested provenance requires new review.
- **Event/notice insert failure:** entire activation rolls back; may retry only as a distinct intentional user action while nonce is still current, never automatically on unknown COMMIT.
- **Expiry during wait:** DB clock rechecked after locks rejects.
- **Database backup restore:** stale/rolled-back credential activation or nonce replay remains a recovery/restore policy blocker; not solved by this slice.

### Compatibility, rollout and operations

- No API or public/private router change. All main listeners remain health-only and any passkey route returns 404.
- Controlled forward migration plus security role provisioned **only** in disposable test harness at first. No production secrets, deployment, sender, monitoring or role binding.
- Application rollback: disable finisher invocation/SQL credential, preserve forward migration and event records; never drop/rewind applied SQL in production.
- A security-notice queue row means *enqueued*, not delivered. Separate operator release gate for worker/retry/monitor/retention.
- Future passkey login and independent recovery factors must explicitly evaluate provenance, state, assurance policy and lifecycle, not simply `state=POSSESSION_VERIFIED`.

### Unresolved explicit decisions

- **Decision D1:** adopt `POSSESSION_VERIFIED` vs `ACTIVE` and what (if anything) may consume it as a login factor. Recommendation: non-login.
- **Decision D2:** approve distinct audit+notice table vs extending registration queue. Recommendation: separate activation event/queue.
- **Decision D3:** accept Profile A finisher role as a privileged verifier oracle for an **unmounted** candidate, or require stronger workload separation before any activation semantics. Recommendation: internal-only with explicit residual risk.
- **Decision D4:** approved activation timeout/abandoned staged credential retention, notification delivery and incident/recovery policy remain **release blocked**; v34 TTL reused for candidate.

**G2 owner acceptance:** NOT RECORDED. **G3 independent technical acceptance:** NOT RECORDED. This proposal is NOT authorization to code or merge SQL behavior.

## G4 — Implementation slice (planned, gated)

1. After D1–D3 accepted, update `docs/contracts/identity.md` with stable agreed ACs.
2. New forward-only SQL `000035`, migration fingerprint + migration canary, synthetic role bootstrap, workflow DSNs and testgate required test.
3. Go finisher usecase + restricted repository adapter, unit negatives and real PostgreSQL 16-way replay, freeze/counter/role, late event collision and rollback integration. Use existing genuine signed ES256 fixture; mocked cryptography alone is not sufficient.
4. ADR-0041 accepted revision, runbook/contract updates, PR risk and CI record. Keep Draft until exact final SHA checks and independent reviewer signoff.

## G5–G7 — Current verification

| Control / evidence | Result |
|---|---|
| Existing v35 signed verifier source and existing v33/v34 DB boundaries inspected | REVIEWED |
| W3C WebAuthn Level 3 and pinned library contract checked | RESEARCHED |
| Candidate SQL/Go, ES256 race/replay, SQL role tests, migration fingerprint | **IMPLEMENTED IN UNMERGED DRAFT #81; selected synthetic CI checks PASS, exact current SHA in Issue #78** |
| Security review, owner G2/technical G3 approval | **PENDING** |
| Production/browser rollout and independent security-notice delivery | **NOT IMPLEMENTED** |

## G3 supplement — precise TOCTOU, SQL lock and lineage review (2026-11-23)

This section is a **design review, not a signoff**. It was checked against the **actual** v33 and v34 migration code and v35 Go verifier after #77 merged. Primary outside reference: [PostgreSQL 18 explicit locking](https://www.postgresql.org/docs/18/explicit-locking.html), [PostgreSQL 18 SELECT locking clauses](https://www.postgresql.org/docs/18/sql-select.html), and [W3C WebAuthn Level 3 Recommendation, 2026-08-25](https://www.w3.org/TR/2026/REC-webauthn-3-20260825/).

### Snapshot binding is mandatory

V35 `verifyForActivation` deliberately returns the **original read-only evidence and signed assertion facts in one call**. These must be passed unchanged to the future finisher in one opaque request struct. In particular, the database must **compare the COSE public key bytes, credential ID, expected stored counter and BE/BS from the originally signed evidence** to the locked current rows, not query a potentially changed credential and silently treat it as what the signature verified.

The future SQL function is a privileged *claim consumer*, not a cryptographic verifier; a compromised finisher runtime credential can lie about a signed result. The trusted Go constructor must prohibit injecting an arbitrary insecure assertion validator into public composition; the role must remain inaccessible from HTTP and excluded from the regular identity runtime DSN.

A verified assertion proves possession of the **private key for the staged public key** in this ceremony. It does not prove legitimate account owner, manufacturer provenance, device uniqueness, independent enrollment assurance, successful notice delivery, login authority or Profile B resistance.

### Proposed lock plan, consistent with existing v33/v34

The future function must read and lock in **separate ordered statements** rather than trusting a planner-dependent multi-relation JOIN to imply lock order:

1. Lookup subject ID by 32-byte session digest **without** accepting a caller-supplied subject.
2. Lock active `subjects` + currently verified `email_aliases` (`FOR UPDATE OF s,a`), revalidate subject and contact epoch/version.
3. Lock the **same** `opaque_sessions` row (`FOR UPDATE OF ss`), revalidate subject, epoch, revoked status, and 30-minute idle plus absolute expiry.
4. Lock the exact `passkey_credentials` row (`FOR UPDATE OF c`) before mutating state; recheck `PENDING_ASSERTION`, credential subject/epoch/contact, `PASSWORD_REAUTH_ONLY`, RP/origin, **byte-equal COSE key**, signed ID, stored counter and expected BE/BS flags.
5. Lock the exact `passkey_activation_challenges` row (`FOR UPDATE OF a`), recheck **challenge ID AND random nonce**, credential selector, original session digest, subject/epoch/contact, policy generation, fixed purpose, unconsumed status, 3-minute expiration and 10-minute stage cap.
6. Revalidate the credential's **original** v30 registration intent and v32 consumed admission, including immutable intent ID/session digest, the required stage audit and registration notice, and ensure a newer registration intent did not replace the one used for staging. Gate on the subject lock used by the older writers; tests must confirm this ordering with actual PostgreSQL sessions.
7. Sample **`clock_timestamp()` after acquiring the conflicting locks**, then reject any session/challenge/stage expiry that elapsed during a wait. Never rely on the time measured during v35's earlier read.
8. Under the same transaction, insert event (whose provenance-checked trigger enqueues activation notice), conditionally transition the credential and stored signed counter/backup state, set immutable possession metadata, and conditionally consume the exact challenge. Require one affected row for every conditional UPDATE; any failure raises/returns an error causing **rollback**, not a swallowed partial commit.

V34 reader already locks subject+alias and session before reading `c,a` with a joint `FOR SHARE`; v34 issuer locks subject+alias and session before `c` and challenge. A new finisher must be compatible with these paths; the added real-DB race fixture must run **reader vs issuer vs finisher** and fail if any unexpected `40P01` deadlock occurs. Do not assume planner JOIN order proves the invariant.

**Schema auditability:** the currently proposed possession timestamp and unique `possession_event_id` preserve the activation event, but v34 overwrites a consumed challenge on subsequent issuance. Consider retaining a non-secret **`possession_challenge_id`** or event-to-challenge link in the credential to preserve proof lineage after that overwrite. This is a design decision (no challenge nonce/bearer persisted in audit), not an approved additional column.

### No entitlement confusion

The proposed `POSSESSION_VERIFIED` state must not be treated by existing or future login queries as `ACTIVE` or as the separate independently enrolled recovery code authority. The resulting storage transition is **verification of possession only**, not authentication of a new browser session. Explicitly inventory any credential queries introduced later, rather than equating `state != PENDING_ASSERTION` with authorization.

### Operational choices still unaccepted

- One-use challenge retention/audit linkage and abandoned-stage cleanup.
- Responsibility for secure role provisioning and a separately protected verifier workload; Profile A remains the scoped guarantee.
- Activation notification worker, retry/dead-letter, recipient changes, bounded PII retention, incident runbook and human receipt policy.
- Independently reviewed public Browser Begin/Finish contract, exact RP/Origin, HMAC CSRF, direct/forwarded TLS, session cookie and abuse budget.

## G4 supplement — executable test matrix for the future accepted slice

These are **PROPOSED acceptance cases, not a blanket PASS**. Draft #81 implements a subset with disposable PostgreSQL and genuine v35 ES256 signing. The coverage ledger below distinguishes executed test behavior from missing tests; reviewer acceptance is still pending.

| ID | Failure/adversarial setup | Expected authoritative DB outcome |
|---|---|---|
| PKA-T01 | Signed ES256 assertion on matching staged credential + live original session/nonce | One `POSSESSION_VERIFIED` row, consumed challenge, one activation event and one pending activation notice |
| PKA-T02 | Same signed assertion submitted again, even with signCount 0/0 | DENY, no second event/notice, no counter change |
| PKA-T03 | **16 concurrent preverified ES256 SQL finish calls** using separately bounded pools (maximum 8 connections per pool, per repository guard), synchronized before SQL admission | Exactly one commit; 15 non-success outcomes (denied, or sanitized unavailable for actual infrastructure failure); no unexpected deadlock or extra event |
| PKA-T04 | Valid but different live session of same subject | DENY; no cross-session adoption |
| PKA-T05 | A different subject (even if it knows the challenge ID/credential ID) | DENY; subject derives only from live session |
| PKA-T06 | Session revoked, epoch rotated or subject frozen after signature verification but before commit | DENY under locks; no mutation |
| PKA-T07 | Verified alias changed or contact version stale after signature verification | DENY; no obligation to stale recipient |
| PKA-T08 | v34 challenge replaced after expiry, same ID with different nonce or different ID | DENY old signature/evidence; exact ID **and** nonce checked |
| PKA-T09 | Acquire a subject lock, hold finisher waiting across expiry, then release | DENY using **post-lock** DB clock |
| PKA-T10 | Credential staged more than ten minutes earlier | DENY even if a stale challenge row was manually left unexpired in test fixture |
| PKA-T11 | Credential state already changed; stage provenance wrong/missing | DENY, no second state transition |
| PKA-T12 | Original v30 intent overwritten by a fresh registration | DENY; no stale admission fallback |
| PKA-T13 | Original v32 admission not consumed, absent, mismatched session or wrong provenance | DENY |
| PKA-T14 | Stage security event or registration notice missing/wrong subject | DENY; no late assertion privilege bypass |
| PKA-T15 | Current RP/origin, policy generation or COSE key differs from original signed v35 evidence | DENY; byte-level binding |
| PKA-T16 | Current stored signCount changed after v35 read | DENY (snapshot mismatch), even if signed counter would advance relative to the new value |
| PKA-T17 | Counter transitions 0→0, 0→1 and 1→2 | ALLOW in **separate newly staged credential fixtures**, each with its own one-use challenge; 0/0 explicitly has no clone detection |
| PKA-T18 | Counter transitions 2→2 or 2→1 (fresh credential fixtures) | DENY; no persisted count regression |
| PKA-T19 | Signed BE differs from staged BE, or BS=1 with BE=0 | DENY; a legal signed BS transition with stable BE is allowed |
| PKA-T20 | Duplicate `evt_` identifier at late event insert | Entire transaction rolls back credential, nonce, counter and notice |
| PKA-T21 | Test-only DB owner injects failure at activation-notice INSERT/trigger | Entire transaction rolls back; after removing fault a new intentional attempt may succeed **only if still unexpired** |
| PKA-T22 | Conflict: v34 reader vs issuer vs finisher under multiple independent sessions | No unexpected deadlock (`40P01`) and all successful state consistent |
| PKA-T23 | Finisher runtime attempts direct `SELECT/UPDATE`, `SET ROLE`, grant staging, session issue, recovery approval or migration | DENIED by SQL privileges |
| PKA-T24 | Any previous issuer/reader/staging/authenticator/ordinary runtime invokes new finisher function | DENIED by privilege and original `session_user` fence |
| PKA-T25 | Commit acknowledgment deliberately dropped after definite SQL commit | Go returns sanitized **UNAVAILABLE**, no automatic retry, durable DB state exactly once |
| PKA-T26 | Malformed proof, truncated nonce/credential, canceled context, oversized assertion | Fail closed without persistent writes or raw SQL/secret disclosure |
| PKA-T27 | Default public/private listeners hit hypothetical passkey registration/activation/login endpoints | 404; no runtime privilege or user session granted |
| PKA-T28 | Future migration installs over empty and upgraded v34 schema; migration SQL altered after apply | Exactly 35 committed fingerprints; original 1–34 immutable; history tampering denied by existing migrator preflight |

### Required evidence before calling implementation PASS

- Run the existing **unchanged** real-PG and Chromium test gates and the **new** named v36 integration test; add it to `tools/testgate/main.go` and verify missing/skipped tests fail the gate.
- Update `tools/postgrestest/testdata/identity-bootstrap.sql`, isolated DSN role variables in **both** PostgreSQL and browser workflows, `tools/postgrestest/run-local.sh`, test role-matrix and fingerprint count **only after** accepted G2/G3 design.
- `make migrations-check`, `make check`, `make test-repeat`, targeted Go tests and disposable PostgreSQL concurrency tests are independently logged with **exact HEAD SHA**; local execution **NOT RUN** for this docs-only review.
- Migration file `000035` must include canonical `-- mamony:integrity:v1` trailer calculated over bytes preceding the marker; never edit applied v33/v34 SQL or manually change their fingerprints.
- Separate human/technical review must inspect the new test assertions themselves to ensure a green test isn't obtained by relaxing the expected denial behavior.

**Candidate evidence and remaining gaps (unaccepted):** The new real-PostgreSQL test `TestIdentityZPasskeyAtomicPossession` exercises signed v35 ES256 evidence, exact original same-session nonce, negative RP/COSE/counter claims, forced notice-trigger failure and event-collision rollback, replay, a 16-preverified-claim SQL one-winner race, and expiration after subject-row lock contention. `TestPostgresIdentityZPasskeyActivationFinisherRole` covers restricted EXECUTE and cross-role denial. Existing browser, migration, and package gates run separately. **These tests do not exhaust PKA-T01–T28.** Additional draft real-DB negative fixtures now exercise a distinct valid subject/session, direct subject epoch rotation, canonical-email mutation (including the real epoch trigger), superseded v30 intent, unconsumed v32 admission, overwritten v34 nonce, revoked original session, stale staged credential, and challenge expiry after a subject-lock wait. The 16 preverified SQL finisher race also starts v34 issuer and reader in the same contention window. **New synthetic PostgreSQL + real signed ES256 fixtures cover:** distinct staged credentials for count 0→0, 0→1, 1→2, negative count 2→2 and 2→1; stable-BE signed BS transitions false→true and true→false; negative BE mismatch and BS-without-BE; durable counter/BS, one-use nonce, event+notice, and replay assertions. **Still missing:** deterministic forced lock-order permutations among all three principals, epoch/contact changes while a finish is blocked, simulated lost acknowledgment after a *definite* database COMMIT, production security notice delivery, and database restore/rollback behavior. A successful synthetic race is not a full proof of every timing interleaving. Count these added tests as PASS only after exact-head PostgreSQL CI succeeds. A synthetic Go fake-unit error cannot substitute for a definite-commit/lost-ack PostgreSQL test. Do not promote proposed PKA IDs into accepted Identity contract until G2/G3 decisions are recorded. Design acceptance remains pending in [#78](https://github.com/herdifirdausss/mamony-go/issues/78).
