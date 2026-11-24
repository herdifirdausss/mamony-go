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
