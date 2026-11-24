# Identity recovery fencing — Profile A

**Status:** OWNER DIRECTION ACCEPTED / G3 TECHNICAL ACCEPTANCE PENDING. Critical design; no runtime, schema, deployment or release change. [Owner decision record](https://github.com/herdifirdausss/mamony-go/issues/78#issuecomment-7130151646).
**Tracking:** [Identity #78](https://github.com/herdifirdausss/mamony-go/issues/78) D4; [Platform #14](https://github.com/herdifirdausss/mamony-go/issues/14) PGF-06; [Identity #20](https://github.com/herdifirdausss/mamony-go/issues/20).
**Owner:** Platform/recovery operator for containment; Identity owner for credential and proof reconciliation. Independent technical reviewer not yet recorded.
**Evidence reviewed:** 2026-11-24. Related: [ADR-0041](../adr/0041-passkey-atomic-activation.md), [recovery runbook](../runbooks/recovery.md), [platform contract](../contracts/platform.md) FND-08/09/12, [Identity contract](../contracts/identity.md) IAM-08/10/11, DEC-12/14/17.
 
## G1 — Evidence and problem

The v36 candidate is not deployed. Exact candidate SHA `0549bc9f3d918af68deb90740811c2a716ad5330` has five successful workflows; [PostgreSQL integration](https://github.com/herdifirdausss/mamony-go/actions/runs/45382063847) includes PKA-R01 negative control, crash/restart and post-commit restore. The negative control demonstrates loss of security continuity on stale restore. It is NOT a passed recovery protection. CI evidence applies to that commit only.

| Source | Observation | Limitation |
|---|---|---|
| [PostgreSQL 18 role attributes](https://www.postgresql.org/docs/18/role-attributes.html) and [host authentication](https://www.postgresql.org/docs/18/auth-pg-hba-conf.html) | Initial connection authorization is configurable | Blocking new logins is not proof existing sessions are gone |
| [PostgreSQL 18 shutdown](https://www.postgresql.org/docs/18/server-shutdown.html) and [administration functions](https://www.postgresql.org/docs/18/functions-admin.html) | Existing connections can be drained or terminated under authorized control | Unknown transaction outcomes still require reconciliation |
| [PostgreSQL 18 PITR](https://www.postgresql.org/docs/18/continuous-archiving.html) | WAL restore can reach a selected recovery point; external configuration is separately managed | Recovery point and database consistency alone do not prove complete security history |
| Existing recovery runbook and FND-12 | Require surviving evidence and review before resume | No deployed Identity recovery gate or independent history source |

Primary documentation checked 2026-11-24. CIAM design input R-OP03/V32 and D15/AD-19 calls for reconciliation or invalidation and explicit release. This proposal restates the relevant requirements without depending on external project repository files.

## G2 — Proposed scope and policy

**Goal:** prevent an operational restore from silently reinstating old authority. Keep the entire Identity capability offline initially, including internal callers, until a clean recovery has been demonstrated. This is a planned-restore/declared-incident model with trusted Platform and Identity administrators, not arbitrary malicious authority-DB writer containment.

Non-goals: always-available identity, automatic failover, distributed consensus/current-head service, customer-facing recovery, passkey login, new financial authority or real provider delivery.

| Proposed ID | Requirement | Evidence |
|---|---|---|
| IRF-01 | Restore targets start isolated and inaccessible to all Identity runtime principals; missing external authorization means HOLD | Real disposable target; cold start and missing-state negative cases |
| IRF-02 | Hold acknowledgment requires no reachable old authority and no active runtime connection or unresolved in-flight decision; fresh and existing pools/workers cannot use the held authority | Real SQL runtime-role/connection fixture and bounded drain test |
| IRF-03 | Restore eligibility, independently surviving evidence, backup provenance, schema and role parity must be verified; incomplete history stays held | Positive complete evidence; missing, mismatched or stale evidence negative tests |
| IRF-04 | Reconcile revoked sessions/credentials, freezes, alias/factor changes, consumed proofs and mandatory notice obligations. Invalidate outstanding pre-recovery grants before release; uncertain bindings remain quarantined | PKA-R02/R03/R04 plus positive fresh authorization |
| IRF-05 | Release is explicitly approved for the exact recovery target and evidence revision; previous release cannot authorize another restore. Old runtime credentials cannot connect to the new authority | Repeated restore, old process and stale release tests |
| IRF-06 | Record privacy-safe containment/recovery decisions, unknown outcomes, elapsed time and lost data; failure to write required release evidence prevents reopening | Fault/cancellation tests and recovery drill |

IRF-01–06 are owner-approved lab verification targets; all remain NOT RUN. Existing PKA-R01 is separate evidence of a limitation. The owner has not selected the evidence-store implementation, recovery targets or production topology. Independent technical acceptance remains pending.

## G3 — Recommended first deployment boundary

Prefer an operational, whole-Identity hold using existing infrastructure/DB administration rather than a new per-request service. Use a distinct isolated restore target; provision runtime access only after review. Keep existing SQL role names compatible with definer policies but do not restore usable runtime credentials from backup. Platform controls connection reachability and secret issuance outside the restored Identity database. Never point application pools at the candidate target merely because health checks pass.

An external recovery control record is required for each recovery attempt. It records a unique recovery ID, target identity, selected backup provenance/recovery point, expected release artifact and schema, fence evidence, reconciliation evidence reference, decision, reviewer and audit time. No bearer tokens, password verifiers, nonce bytes or provider secrets. Records live in a separately administered operations evidence store excluded from Identity restore. Backend, access policy, durable write semantics, incident custody and retention must be chosen and reviewed before implementation; a GitHub comment or a local environment flag is not a runtime release authority.

### State and enforcement

| State | Allowed work | Transition gate |
|---|---|---|
| HOLD | Authorized containment and evidence preservation only | Confirm access barriers and settle/fence outstanding operations |
| ISOLATED_RESTORE | Recovery-admin restore and restricted inspection | Verify target, migration fingerprints, roles and trusted recovery point |
| RECONCILING | Approved recovery operations only | Complete surviving-history reconciliation, invalidate outstanding grants, verify denial behavior |
| RELEASE_REVIEW | Restricted inspection; no runtime traffic | Independent review of exact target, evidence and release record |
| RELEASED | Only approved new runtime access | Fresh credentials/configuration; controlled restart and checks |

Every failed or ambiguous transition remains held; no timeout or retry can promote a state. Treat containment request and acknowledged containment as different events. An already committed decision cannot be canceled by a later hold: capture/reconcile it. An unavailable external control plane cannot authorize release.

### Boundary details

- Stop new runtime admission and workers, isolate old and candidate endpoints, drain or terminate existing runtime connections through trusted administration, verify absence of reachable old pools and record unresolved transactions before acknowledging hold. Include every enabled role and alternate path, not just HTTP or the passkey finisher. Health-only routes do not prove this boundary.
- Restore a distinct target under recovery-admin control. Verify original schema ownership/grants and restricted roles separately from the existing data-only dump test. No production credentials in CI. No reuse of old runtime secrets after cutover.
- Reconcile from independently surviving trustworthy history. A consistent stale database or a new local epoch alone is insufficient. Without completeness evidence hold the whole affected scope; if scope is unknown, hold all Identity. Fresh proof cannot by itself authorize restoration of a binding whose legitimacy is uncertain.
- Plan invalidation across sessions, authentication grants, verification/recovery challenges, passkey staging/activation proofs, recovery-factor approvals and relevant delivery leases. Inventory each enabled capability and its owning transaction before selecting an implementation. Preserve durable security events and reconcile missing notification obligations using original event identity; do not infer user receipt.
- Verify the actual fence, denial of old authority, role parity and positive fresh operation on the isolated target. Only a separately approved release of that exact recovery attempt enables the new runtime endpoint/credentials. Repeat restore requires a new hold/review. On failure, keep both old and new endpoints closed; never fallback to the old database automatically.

### Alternatives and limits

1. Local hold flag or local generation alone: rejected, restored together with state.
2. Operational isolation and reviewed cutover: recommended for first Profile A lab; simpler, deliberately accepts downtime and manual reconciliation.
3. Independent online current-head or monotonic authority: defer to separately approved Profile B design; may be justified if continuous availability or malicious writer containment is required.

This first approach does not continuously detect a silent in-place rollback while an authority is live, resist a compromised Platform/Identity administrator, ensure availability if evidence is missing, or provide global execution fencing. RTO/RPO are unmeasured. Recovery hold is not a new account status or a guarantee that notification delivery can be reconstructed.

## G4 — Planned increments, gated

| Slice | Owner | Evidence required | Status |
|---|---|---|---|
| Isolated recovery target and role/connection fence drill | Platform #14 | IRF-01/02; old pools and new connections denied; no production access | NOT RUN |
| Evidence/reconciliation and old-proof invalidation design | Identity #78/#20 | IRF-03/04; all enabled grant families inventoried; missing history holds | NOT RUN |
| Explicit release and repeated-restore drill | Platform + Identity | IRF-05/06, PKA-R02–R05; independent review | NOT RUN |

The existing runbook stays the authoritative procedure until an accepted, verified refinement is made. No SQL migration or role grant change is proposed for this documentation-only slice. Before any implementation, record G2 owner decisions on outage scope, recovery evidence ownership/completeness, handling unrecoverable bindings, operator approval and acceptable recovery targets; record independent G3 technical acceptance of the actual access boundary and test plan. Keep PR #79–#81 draft/unmerged. No new login entitlement.

## G3 supplement — first disposable PostgreSQL fencing drill

Owner direction accepted 2026-11-24 (see decision record above). The initial drill is deliberately narrower than the full IRF requirements. It is an infrastructure mechanism test, not release orchestration, identity reconciliation, or proof of rollback resistance. No code implementation is approved by independent G3 review yet.

### Test boundary and preconditions

- A fresh disposable PostgreSQL 18.7 instance with a separate lifecycle from existing integration fixtures; no production endpoint, credential, backup or customer data. Derive a synthetic runtime-role inventory from the actual fixture/bootstrap and compare it with live catalog membership; refuse an unexpected target or role.
- Use a separate trusted test administrator and runtime test principals. The administrator and external test oracle are outside the runtime privileges. Start with a known valid connection and a harmless committed synthetic operation, to distinguish real fencing from an already broken login.
- Use fresh connections and pre-opened runtime sessions; include an idle connection and a deliberately uncommitted synthetic write. Assert actual database identity and principal for positive checks. Bound waits; classify environmental/transport failure separately from policy denial.
- Restore target must start with runtime login disabled, no reusable source secrets, and explicit access policy. Database role identifiers required by grants/definers are not the same as usable credentials. Verify effective privileges, including indirect membership and alternate connection paths. Do not simply rely on absence of HTTP routes.

### Proposed test matrix

| Drill case | Observable expectation | Coverage limit |
|---|---|---|
| IRF-F01 baseline | A known-good synthetic runtime connects and performs its allowed harmless operation; unrelated admin remains distinct | Establishes a valid positive baseline |
| IRF-F02 held target | Every inventoried runtime principal is denied a new connection while the restore target remains available to the test administrator | Partial IRF-01; no external recovery record yet |
| IRF-F03 source containment | After the barrier is acknowledged, new runtime connections and pre-opened sessions cannot perform operations on the disposable old authority; administrator verifies no remaining affected connections | Partial IRF-02; not deployed network/secret custody |
| IRF-F04 transaction boundary | Known committed synthetic marker persists; known uncommitted write is absent after containment; any unknown result is explicitly held for reconciliation rather than classified by socket closure | Partial IRF-02; not a general in-flight outcome resolver |
| IRF-F05 failure handling | Incomplete inventory, unexpected target, timeout or unverifiable fence cannot produce an acknowledged hold/release | Partial IRF-01/02/06 |
| IRF-F06 repeat check | A newly initialized isolated candidate remains inaccessible without a new release decision | Partial IRF-05; no actual release authority exercised |

**Mechanism review:** PostgreSQL LOGIN/NOLOGIN and database CONNECT controls apply to connection admission; they cannot substitute for verifying existing sessions have ended. A bounded shutdown/termination operation and subsequent independent observation are separate checks. See [role attributes](https://www.postgresql.org/docs/18/role-attributes.html), [privileges](https://www.postgresql.org/docs/18/ddl-priv.html), [shutdown](https://www.postgresql.org/docs/18/server-shutdown.html), and [activity monitoring](https://www.postgresql.org/docs/18/monitoring-stats.html) (checked 2026-11-24).

### Evidence and approval gates

Each case remains NOT RUN. No local PostgreSQL server/client or Docker was available in the review environment. A later approved test-only implementation should use disposable CI infrastructure, fail on missing/skipped required cases, report exact-head evidence and avoid changing any application runtime grant or migration. Passing IRF-F01–F06 cannot be reported as full IRF-01/02/05/06 acceptance, PKA-R02 PASS, reconciliation, production release, or Profile B currentness.

Independent technical review must confirm actual source/target access fencing, principal inventory, barrier acknowledgment conditions, bounded failure behavior, and test isolation before code. Runtime rollout additionally needs a selected surviving evidence backend, explicit release authority, reconciliation policy, and tested old-credential denial. No change to the unrelated v36 D1–D3 decision status.

## G3 review supplement — concrete fixture scope and hold acknowledgment

**Review baseline:** `d22008f35d148d958349816e82dfd0dd61bc1c67`, 2026-11-24. This is a source inspection and a proposed test refinement, not independent G3 acceptance.

### Principal inventory (static source inspection)

The synthetic Identity bootstrap declares 32 LOGIN principals: 31 runtime principals (7 passkey, 13 recovery, 11 other Identity) and one Identity migrator. All 32 names occur in each of the two CI workflow definitions, local runner and integration testgate. This is a reference-presence comparison, not a live privilege or completeness test. Sources: [bootstrap](../../tools/postgrestest/testdata/identity-bootstrap.sql), [PostgreSQL CI](../../.github/workflows/postgres-integration.yml), [browser CI](../../.github/workflows/identity-real-browser-e2e.yml), [local runner](../../tools/postgrestest/run-local.sh), [testgate](../../tools/testgate/main.go).

- Derive the expected role inventory from the reviewed fixture rather than a manually copied list of only passkey roles. Independently inspect the actual catalog for extra login principals, role membership and effective access, including platform/shared principals, default PUBLIC privileges and alternate paths. Unknown/unaccounted authority fails the drill. The count is an observed baseline, not a permanent hardcoded product requirement.
- Distinguish 31 runtime roles from the privileged migrator; the migrator is not an ordinary worker, must not be a standing access exception, and may be admitted only as an explicitly scoped recovery-admin operation. Keep test administrator distinct. Do not alter the shared integration bootstrap or its database, since it contains destructive reset setup intended only for the existing synthetic test lifecycle.
- For each source runtime principal verify a positive connection to the intended synthetic database with the actual role before fencing. Verify the corresponding denial after fencing and independently confirm the administrator can still inspect the isolated target. Missing/incorrect credentials, a dead server, a wrong endpoint or transport errors cannot establish an authorization denial. Use redacted status/counts, not DSNs.

### Hold acknowledgment — gaps found and reviewer dispositions

| Finding | Required evidence before acknowledging the corresponding lab barrier |
|---|---|
| Connection admission differs from existing sessions | Independently observe both fresh connection refusal and loss of existing usable connections. A successful signal submission is not termination evidence. Bound observation and treat a timeout as unverified. |
| Active connection count is not a complete transaction inventory | The first slice proposes no two-phase commit. Verify `max_prepared_transactions=0` and no outstanding prepared transaction in the disposable authority. If unexpected prepared work exists, hold and investigate; do not automatically resolve it. Ordinary in-flight or ambiguous COMMIT decisions still need separate evidence. |
| Database-only fencing does not settle work already outside SQL | The Identity contract includes bounded lease-based delivery workers. Stop and account for in-flight external requests separately; lost response is an unknown provider outcome, not delivery failure or permission to resend. Use synthetic mock provider only in later approved worker tests. |
| Shared database / role membership can widen impact | First test only dedicated disposable instances. Catalog and connection evidence must be compared to the exact intended database and authorized administrative scope. No changes to unrelated platform/ledger connections. |
| External release record is not yet selected | Fencing mechanism evidence alone cannot authorize release. No automatic reopening on successful restore or green CI. |

Primary references checked 2026-11-24: [PostgreSQL 18 privileges](https://www.postgresql.org/docs/18/ddl-priv.html), [administration functions](https://www.postgresql.org/docs/18/functions-admin.html) (termination with zero timeout confirms signal submission only), [prepared transaction semantics](https://www.postgresql.org/docs/18/sql-prepare-transaction.html), [prepared-transaction catalog](https://www.postgresql.org/docs/18/view-pg-prepared-xacts.html). Existing [Identity contract](../contracts/identity.md) documents lease-based delivery and ambiguous provider outcomes. No claim that these conditions are already enforced in the repository runtime.

### Independent reviewer handoff

Review the exact proposal and fixture baseline. Before G4, a designated reviewer should record acceptance or requested changes for: (1) dedicated target and principal inventory, (2) effective admission and existing-connection barriers, (3) known/unknown transaction handling including prepared work, (4) excluded worker/provider effects and separate test obligations, (5) bounded failure and cleanup without touching shared or live infrastructure. The initial drill is not approval of the external release mechanism or reconciliation policy. Owner direction is recorded; no independent review or new lab test execution was observed. All IRF-F01–F06 remain NOT RUN.

## Owner-approved draft implementation — 2026-11-24

[Workflow option B approval](https://github.com/herdifirdausss/mamony-go/issues/78#issuecomment-7130263898) supersedes the earlier requirement for independent G3 acceptance before synthetic coding only. Independent acceptance remains mandatory before merge. The candidate `tools/postgrestest/check-identity-recovery-fence.sh` and PostgreSQL workflow step now exist on Draft #81. The drill creates three separate network-isolated PostgreSQL 18.7 containers, derives role names from the actual bootstrap, uses new synthetic credentials, and restores only a harmless committed marker table. It does not restore Identity tables or application ACLs and never modifies the shared CI service database. It exercises positive logins, held target admission, idle and in-flight backend termination, committed/uncommitted marker consistency, wrong-target and incomplete-inventory refusal, and repeated held target. IRF-F05 timeout and IRF-F06 actual release authority remain NOT RUN; their implemented checks are partial. All executed results require exact-head CI evidence; no full IRF-01–06 or PKA-R02 acceptance is implied. External worker/provider, reconciliation, actual role parity, evidence store, recovery generation, production release and independent review remain outstanding.


## G5 evidence and G3 reconciliation inventory — 2026-11-24

Exact SHA `0598b238a454b99fe1f164b2a13678bd4dbab1f9` completed five of five workflows. [PostgreSQL integration 45387092315](https://github.com/herdifirdausss/mamony-go/actions/runs/45387092315) logged IRF-F01–04 PASS and the new bounded observation refusal while existing connections remained. Both held restore targets passed effective CONNECT denial. F05/F06 remain partial (external controller and release authority absent). The synthetic marker is not restored Identity authority.

### Classification of restored security state

Reviewed all 35 Identity forward migrations on the above SHA. Their CREATE TABLE declarations enumerate 29 ordinary tables. The new read-only [catalog inventory](../../tools/postgrestest/identity-recovery-inventory.sql) classifies each by its primary recovery responsibility; the new integration workflow requires exact equality with actual migrated table names, reporting missing, unexpected or duplicate classifications. This is a table inventory guard, not a proof of complete field/permission/sequence coverage. New CI check is NOT RUN until exact-head evidence is recorded.

| Class | Count | Proposed disposition while held |
|---|---:|---|
| schema_integrity | 1 | Verify migration history and independently reviewed artifact/role parity |
| binding_reconcile | 6 | Reconcile authoritative account, credential/contact, freeze, factor and offline-code state against surviving evidence; uncertain bindings remain held |
| temporary_authority | 10 | Deny pre-recovery sessions, grants, challenges and admissions through a reviewed invalidation policy; preserve necessary consumption/provenance evidence |
| proof_delivery | 2 | Reconcile outstanding proof delivery and prevent obsolete proof delivery; no blind resend |
| notice_obligation | 5 | Preserve mandatory event linkage, reconcile missing obligations and uncertain delivery |
| provider_receipt | 3 | Reconcile with surviving provider evidence; do not manufacture delivery state |
| security_event | 1 | Preserve evidence, identify missing event history, reconcile stable identifiers |
| abuse_budget | 1 | Review safe restart of rate budgets; never silently relax abuse restrictions |
| Total | 29 | All are classified; none modified by the inventory |

A class is not a delete/update directive. Several tables also contain historical provenance required by downstream checks. Revocation or invalidation must not erase evidence or accidentally reinstate consumed recovery codes, replaced credentials or old contact bindings. A fresh login or local epoch increase is not independent evidence that the restored password/contact/freeze state is current. The next bounded design must inventory per-table fields, referencing functions and lifecycle constraints, specify a surviving history source, and define which old proofs are invalidated versus retained as non-authoritative evidence. If history is missing, keep scope held. No external recovery control store or production policy has been chosen; no SQL mutations, runtime changes or release implementation in this slice. IRF-03/04 and PKA-R02–R05 remain unverified; independent review still required before merge.


## G5 inventory correction and G3 temporary-authority field review — 2026-11-24

**Actual CI finding:** SHA `576c4ad949c444b4b7aedbb6691667c85c7a8c06` had four successful workflows and [PostgreSQL integration 45388202024](https://github.com/herdifirdausss/mamony-go/actions/runs/45388202024) failed the new inventory check with `29|30|0|1|0|-|schema_migrations`. The preceding integration suite passed; subsequent durability/fencing steps were skipped, not failed or passed on that SHA. The migration driver creates its own metadata table outside the 35 SQL migration bodies. It is security-relevant: the migration runbook distinguishes version/dirty metadata from the SQL-body/fingerprint transaction. Corrected inventory: **30 tables**, with **2 schema_integrity** and all other class counts unchanged. Both metadata tables must be checked against the independently reviewed artifact and expected schema version, including dirty state; do not force-clean metadata or edit fingerprints to make restore pass. The guard remains strict and does not exclude the unexpected table. Corrected CI evidence pending.

### Temporary authority: source-level field and provenance map

The following review covers ten temporary-authority tables from migrations 000002, 000010, 000011, 000019, 000021, 000025, 000028, 000030, 000032 and 000034. These are observed field families from the creation definitions (and relevant later session/epoch changes), not an exhaustive current-column catalog or an approved repair algorithm.

| Table | Authority / lifecycle evidence | Proposed recovery disposition |
|---|---|---|
| email_verification_challenges | subject, contact version, expiry, consumed/revoked timestamps | Invalidate outstanding proof; preserve historical consume/revoke evidence and delivery dependency |
| opaque_sessions | subject epoch, expiry, revocation | Deny every pre-recovery session; retain security history |
| password_login_grants | subject epoch, purpose, expiry, consume timestamp | Deny outstanding grants without representing invalidation as successful consumption |
| password_recovery_challenges | subject epoch, contact version, expiry, consumed/revoked timestamps | Invalidate outstanding recovery proofs; reconcile delivery obligations |
| password_recovery_assurance_grants | subject epoch, approval/expiry/consume timestamps | Deny outstanding grants; review later provenance fields before designing repair |
| password_recovery_code_enrollment_approvals | subject epoch, approval/expiry/consume timestamps | Deny old approvals without altering durable factor history |
| recovery_code_factor_challenges | subject epoch, expiry, failure count, consume timestamp | Deny outstanding challenges; preserve evidence of use and abuse |
| passkey_registration_intents | subject epoch, contact version, session binding, intent and consumption | Deny outstanding registration; preserve provenance referenced by staged credential evidence |
| passkey_registration_admissions | intent/session binding, epoch/contact, provenance, expiry, consumption | Deny outstanding admission; do not erase historical verified registration provenance |
| passkey_activation_challenges | credential/session binding, epoch/contact, policy generation, expiry, consumption | Deny outstanding activation; preserve links to verified possession records |

Existing epoch checks in 000010/000011/000014 establish a local session/grant fence under the trusted current database. This does not establish anti-rollback currentness or invalidate every proof family (email verification has contact-version binding, not a subject epoch in its creation definition). A recovery design must explicitly address each family; never mark an unused proof as consumed solely to simulate a successful user action. The first proposed lab negative test must verify old sessions and grants are denied while the restored authority stays held; fresh acceptance can be tested only against a separately approved reconciled synthetic fixture. The broader requirements still need current-column and function dependency inventory, retention/FK review, surviving evidence and explicit release policy. No data repair, epoch update, credential mutation, new runtime authority or release code was added in this iteration. Independent review remains required before merge.


## G5 corrected inventory evidence and G3 binding/provenance review — 2026-11-24

[PostgreSQL integration 45388769073](https://github.com/herdifirdausss/mamony-go/actions/runs/45388769073), exact SHA `3828a60f298486342ad7031f45dbfc32c8a8730a`, explicitly logged 30 classified tables, no missing/unexpected/duplicate names, and IRF-F01–04 PASS, bounded observation refusal and F05/F06 PARTIAL. This supersedes the failed 29-table inventory evidence, not the unverified recovery requirements.

### Durable binding fields (source-level review; proposed dispositions)

| Table | Reviewed field families | Restore concern and disposition |
|---|---|---|
| subjects | subject ID, status, session epoch, creation time | Reconcile current restriction and identity ownership; never thaw by default. |
| email_aliases | canonical alias, subject, contact version, verified timestamp | Verify current binding and proof status independently; stale verified flag is not current proof. |
| password_verifiers | subject, KDF version/parameters, salt/digest, creation time | Reconcile latest authorized verifier; do not log or export verifier bytes in recovery evidence. |
| password_recovery_offline_codes | subject, enrolled epoch/contact, consume timestamp | Restore can resurrect consumed or rotated codes; preserve/reconcile history, do not infer current membership from backup. |
| recovery_code_factor_keys | subject/public key, bound epoch/contact, revocation timestamp | Reconcile authorized binding and latest revocation; uncertainty stays held. |
| passkey_credentials | subject, intent, RP/origin, registration provenance, state, counter/backup state, possession event and challenge | Preserve provenance; reconcile latest authoritative credential state. POSSESSION_VERIFIED does not authorize login/recovery. |

Sources: migrations 000001, 000002, 000010, 000014, 000024, 000028, 000033 and 000035. Reviewed creation definitions and selected later changes, not an exhaustive field-level current schema review. Actual reconciliation policy requires owner decision on missing/incomplete evidence and surviving authority; no mutation is authorized here.

### Audit, proof delivery and notice dependency review

The five notice tables reference both the security event and subject. Both proof-delivery tables reference their respective challenge. Passkey credentials reference original registration and possession events; activation challenges reference the credential. Deleting these parents or reconstructing events with a fabricated successful action is not a valid recovery procedure. Migration 000035's possession notice trigger checks original runtime authority and trusted contact. A recovery operation may not silently bypass that policy. Provider receipt ledgers store digests and delivery-state evidence; the creation definitions in 000017/000023/000027 do not establish a direct foreign key to notice rows. Application-level matching, provider uncertainty, retention and suppression require separate review; a provider acceptance is not user receipt.

Added [targeted read-only provenance guard](../../tools/postgrestest/identity-recovery-provenance.sql) for 17 selected single-column FK links. It requires a validated FK with NO ACTION or RESTRICT delete semantics. It is intentionally not a complete dependency inventory: it does not prove application-level semantic consistency, triggers, privileges, matching of notice destination/event type, immutable audit evidence, or anti-rollback. New CI step is NOT RUN pending exact-head verification. Full reconciliation and pre-recovery grant invalidation remain unimplemented. No production runtime, migration, role, route or release change.


## G5 provenance evidence and local epoch mechanism candidate — 2026-11-24

Exact SHA `c149e6a0d4b9f2d7b3d4122c5e2916c29098b449` completed five of five workflows. [PostgreSQL integration 45389256127](https://github.com/herdifirdausss/mamony-go/actions/runs/45389256127) logged 30-table inventory PASS, 17 selected validated non-cascading foreign keys PASS, IRF-F01–04 PASS, bounded observation denial, and F05/F06 PARTIAL. No full recovery control acceptance follows.

Added `TestIdentityZRecoveryEpochInvalidationMechanism` (5cc9ac0) to the required integration gate (b2b1f69). This uses a new synthetic subject in the existing guarded disposable integration database, not a separate restored authority. An administrator-only subject-scoped epoch increment models a local invalidation mechanism; it is not a recovery command. The test exercises real restricted login-grant and session SQL functions, rollback, committed denial of existing sessions and an outstanding grant, denial of stale-epoch issuance, unchanged consumption and session/audit counts, and a fresh synthetic grant positive control. Fresh success assumes current bindings solely for this test; no real password verification or release is implied. New test NOT RUN pending exact-head CI. Password-change purpose and other proof families, actual stale restore, independent surviving history, monotonic generation, reconciliation and external release remain outside this check. A restored local epoch can still roll back; never treat this test as proof of recovery currentness. No production runtime or migration change.


## G5 local grant evidence and contact/recovery proof candidates — 2026-11-24

Exact SHA `0265a0dd328a4f03c772ad9c981436b7586ad577` completed 5/5 workflows. [PostgreSQL integration 45390894453](https://github.com/herdifirdausss/mamony-go/actions/runs/45390894453) explicitly logged IRF-EPOCH and IRF-CHANGE mechanism PASS: stale login/session and change-purpose grants denied, fresh change positive control rolled back, verifier/audit/notice unchanged. This is local current-database behavior, not a restore or anti-rollback test.

New synthetic candidates in existing required integration tests:
- Email verification (5bf9c23): negative control expects the contact-bound proof to remain usable after an epoch-only update; the separate revocation fixture expects a still-live old proof to be denied as an authoritative invalid proof, with original issuance and unconsumed history intact and fresh proof accepted after the synthetic cooldown. This models explicit local revocation, not recovery orchestration. No provider delivery test.
- Password recovery (ca10581): original proof and assurance positive control rolled back; explicitly advanced local epoch and test-only administrator-adjusted assurance isolate denial by the stale challenge epoch even with a live current-epoch assurance. Credential, audit, notice and consumption state must remain unchanged. The fixture assurance is not a legitimate proofing flow.
- CI evidence markers are required in addition to existing parent testgate checks (cb929f4). All three new cases NOT RUN pending exact-head verification; negative-control success exposes a limitation and is not recovery-security PASS.

Source: migration 000002 and ProofStore.Consume bind email verification to contact/status and challenge lifecycle, not session_epoch. Migration 000019 and replacement redeem_password_recovery in 000024 bind password recovery to the current subject epoch and challenge-specific assurance. No production mutations, migration, route, release or surviving evidence authority introduced. Pending worker/provider, proof-family coverage, actual stale restore, reconciliation and independent review remain blocking for release.


## G5 contact proof evidence and recovery-code/factor candidates — 2026-11-24

Exact SHA `62c1bfd4d8526b406592f9064b4ac70a2115defc` completed 5/5 workflows. [PostgreSQL integration 45391602066](https://github.com/herdifirdausss/mamony-go/actions/runs/45391602066) explicitly logged the email epoch-only negative control (old proof still accepted), email revocation mechanism PASS, old password recovery challenge with current assurance denial PASS, and local epoch/change-grant PASS. Negative-control success is evidence of a limitation, not recovery acceptance.

Reviewed migrations 000024/000025/000028/000029. Saved-code rotation atomically deletes the previous set and inserts the new set; it does not retain old code digests as tombstones. Thus a stale backup can contain a formerly rotated-out set, and local epoch comparison is not an independent history check. The recovery factor challenge and enrollment approval also depend on epoch and current bound/unrevoked key; these can roll back together. Do not automatically rebind factor keys or re-enable saved codes on restore. The existing rotation path marks prior assurance consumed as a terminal transition; this does not authorize fabricating successful consumption in recovery.

New bounded mechanism tests:
- 2a8a77c: extend existing recovery-code enrollment/rotation test after its positive and concurrent rotation controls. Retain five old-epoch saved-code rows; advance the synthetic subject epoch; mint fresh session and rotation grant via restricted SQL roles. Both old rotation grant and old code with fresh grant must be denied; rows, grant state, event and notice counts must remain intact. This checks rotation, not the offline-code password-recovery approval path.
- 259a4f0: extend existing signed-factor test after genuine signature approval and revocation. Keep key revoked and approval on old epoch; create fresh session and rotation grant. Enrollment must be denied and revocation, original consumed factor challenge, outstanding approval, grant, and event evidence retained. This checks combined stale/revoked denial, not independent currentness or factor re-provisioning.
- e51d597: require both evidence markers in CI in addition to existing mandatory parent testgate tests. New cases NOT RUN pending exact-head CI.

No new grant authority, production mutation, route, migration, release or credential reconciliation implemented. Future coverage: offline-code assurance approval, isolated stale enrollment approval versus current factor, passkey proof family, actual stale restore and surviving trusted history. Independent review before merge remains mandatory.


## G5 saved-code/factor evidence and isolated assurance/approval candidates — 2026-11-24

Exact SHA `859f89ba39656c2b2500f09277ddf38d18594184` completed 5/5 workflows. [PostgreSQL integration 45392368342](https://github.com/herdifirdausss/mamony-go/actions/runs/45392368342) explicitly logged IRF-OFFLINE and IRF-FACTOR mechanism PASS; prior email/recovery/epoch/change evidence also passed within its bounded scope. No independent restore-currentness proof follows.

Added two previously unisolated cases in existing mandatory parent integration tests:
- 61b8e03: a live current recovery challenge with an unused prior-epoch offline code must deny assurance without consuming the code, creating a grant or emitting an approval event. A separate current-epoch synthetic code succeeds against the same challenge. Pre-enrollment is a privileged fixture, not a supported recovery enrollment flow.
- 0bcaa38: a separate synthetic subject has an outstanding prior-epoch enrollment approval, matching current non-revoked factor key, fresh session and current rotation grant. Denial must retain approval/grant state and create no code/event/notice. Current factor binding is deliberately simulated by the disposable administrator, not a proposed restore rebind policy. Earlier revoked-factor fixture remains revoked. Genuine signature/enrollment positive control is covered earlier in the same parent test.
- eb58533: both exact evidence markers required by CI. New tests NOT RUN pending exact-head verification.

These cases close specific local epoch isolation gaps, not the external recovery authority gap. Avoid more duplicate local cases unless they reveal an uncovered binding. Next examine passkey registration/admission/activation, then prioritize integrated stale-restore negative/positive controls against independently surviving evidence and a separately approved hold/release policy. No production runtime, migration, route, data mutation or release change. Independent review remains required before merge.


## G5 local assurance evidence and actual stale-target hold candidate — 2026-11-24

Exact SHA `4f3a85634b475efb4fcbd0c78dade4d11659ab6a` completed 5/5 workflows. [PostgreSQL integration 45393335216](https://github.com/herdifirdausss/mamony-go/actions/runs/45393335216) explicitly logged IRF-ASSURANCE and IRF-APPROVAL mechanism PASS. Existing registration intent/admission, activation challenge and atomic possession integration tests already exercise exact-session binding, epoch denial, expiry/replay, and signed evidence. Do not duplicate these tests simply to rename them as recovery tests. PKA-R01 continues to expose stale restore replay.

New candidate afc9fa6 links the actual stale Identity restore to the hold mechanism: restore ownership and ACLs into the existing distinct disposable PostgreSQL target with all recreated Mamony roles NOLOGIN (except admin); verify the source event/notice exists in the surviving source while the same event ID is absent on target; verify catalog and active-session hold; attempt three actual runtime connections using synthetic credentials and require SQLSTATE 28000, not generic transport failure. Recheck unchanged stale challenge/credential/event state. Only then deliberately enable three test roles to run the existing exposure negative control. This is an intentional lab-only bypass, NOT a release workflow. CI requires both markers (5fde195). New hold case NOT RUN pending exact-head verification. The source is a narrow test oracle, not a complete independently administered history store. Loopback target does not demonstrate deployed network isolation, real credential rotation, source containment, all role login attempts or a persistent external hold decision. No full IRF/PKA-R02 acceptance, no production migration/runtime/route changes.

### Next integrated slice: evidence-bound release review (proposed, not implemented)

Use the owner-accepted Profile A whole-Identity outage boundary. Separate the negative-control target from a held reconciliation candidate in subsequent drills. A test-only external record should bind attempt ID, target cluster identity, backup digest, expected schema/role evidence and the surviving event checkpoint. Missing, mismatched or incomplete evidence must remain HOLD; no inference from successful PostgreSQL restore or a locally increased epoch. First validate rejection of missing/mismatched evidence and stale release across a repeated restore. Only after a separately approved reconciliation policy may a candidate be eligible for review; independently verify pre-recovery session/proof denial and mandatory original event/notice reconciliation. Do not fabricate possession history or re-enable stale credentials to make a test pass. Production store, custody, completeness, authorization and availability remain undecided; independent review required before merge.
