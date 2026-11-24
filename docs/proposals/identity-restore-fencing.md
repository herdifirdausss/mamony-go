# Identity recovery fencing — Profile A

**Status:** PROPOSED / NOT ACCEPTED. Critical design; no runtime, schema, deployment or release change.
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

All IRF requirements are proposed, NOT RUN. Existing PKA-R01 is separate evidence of a limitation. Stable accepted IDs belong in the owning contract after G2 approval.

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
