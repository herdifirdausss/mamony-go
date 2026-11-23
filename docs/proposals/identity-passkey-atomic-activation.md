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
| New SQL/Go integration test, race/role tests, migration fingerprint | **NOT RUN — not implemented** |
| Security review, owner G2/technical G3 approval | **PENDING** |
| Production/browser rollout and independent security-notice delivery | **NOT IMPLEMENTED** |
