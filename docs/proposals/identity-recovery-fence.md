# Identity recovery fence — proposed requirements and design boundary

**Owner:** Identity (recovery semantics); Platform #14 (recovery infrastructure); coordinate with #78 (v36 capability).  
**Status:** PROPOSED / Critical / G2–G3 NOT ACCEPTED; implementation BLOCKED.  
**Decision:** [Owner-approved scope split](https://github.com/herdifirdausss/mamony-go/issues/14#issuecomment-7152224953) authorizes preparation of this proposal, not its adoption, implementation, merge or activation.  
**Evidence date:** 2026-11-25. Main inspected `f46459615189ea0657c630f589219b082aee9b71`.

## G1 — Research and problem

Restoring a consistent database does not establish current legitimate bindings or invalidate outstanding temporary authority. PR #95 is a separate Platform-owned synthetic reconciliation lab, not a release mechanism. Its exact prior candidate `afd2779f6f0088ebedb58094b007a8f4de1543fb` had five successful CI workflows; [actual PostgreSQL evidence](https://github.com/herdifirdausss/mamony-go/actions/runs/45541683026/job/138961456817) includes twelve RH-03 and six RH-01/RH-02 markers. Existing current-session controls were issued before recovery: they show preserved-state semantics, not fresh post-fence authorization.

| Source | Observation | Limitation |
|---|---|---|
| [Identity contract](../contracts/identity.md), IAM-08/10/11 | Separate durable binding and temporary authority; local epoch and proof lifecycle controls | Current database trust is not independent recovery currentness |
| [Platform contract](../contracts/platform.md), FND-08/12 | Recovery checkpoints and durable event obligations | No independent completeness demonstrated |
| [Recovery runbook](../runbooks/recovery.md) | Recovery ownership and evidence boundary | No approval to release a target |
| [Approved RH-03 scope](https://github.com/herdifirdausss/mamony-go/issues/14#issuecomment-7149877826) and [scope split](https://github.com/herdifirdausss/mamony-go/issues/14#issuecomment-7152224953) | Known-boundary synthetic reconciliation, HOLD on uncertainty | Full RH-03 remains PARTIAL |
| [Unmerged #81](https://github.com/herdifirdausss/mamony-go/pull/81), inspected head `c40b74c0ea4dc7a6ec19aeaa52f90be238b62bd6` | v36 and IRF-04 proposal are related design inputs | Not canonical main behavior or an adoption decision; do not import unmerged code |

The owner split does not reduce IRF-04/RH-03 requirements. An unknown history must remain held even when a fresh proof can be established.

## G2 — Proposed PRD

Actors: Identity capability owner, Platform recovery operator, independent technical reviewer and repository owner. Intended outcome: determine whether a reconciled synthetic candidate can be reviewed without reviving obsolete authority or manufacturing a successful user action. No permission to release the candidate follows.

Non-goals: production recovery, live credentials/data/funds, public routes, real providers, migration or runtime changes, production registry, automated release, activation of a held target, importing or merging #81, or changing governance. RH-04/RH-05 registry and release are separate Platform decisions.

Proposed requirements (traceability labels below are proposal-local, not new canonical contract AC IDs):

| Requirement | Existing trace | Acceptance outcome | Status |
|---|---|---|---|
| Fence prerequisites | RH-03 AC-1/3, IAM-11, FND-12 | Unknown binding or evidence completeness keeps affected scope HOLD; unknown scope keeps Identity HOLD | NOT RUN |
| Outstanding authority | RH-03 AC-2, IAM-08/10 | Every in-scope temporary authority from before recovery is denied by the actual owning capability; no assertion of success from a storage/network denial alone | NOT RUN |
| Durable identity | RH-03 AC-1 | Current authorized bindings and original audit/notice identities preserved; no fabricated consumption or enrollment/possession history | NOT RUN |
| Fresh operation | RH-03 AC-2 | Fresh authorization succeeds only on a separately approved, reconciled disposable fixture; no old authority or held target is released | NOT RUN |
| Failure closure | RH-03 AC-3 | Missing, stale, ambiguous or contradictory evidence and incomplete invalidation keep HOLD; no automatic retry/release or new success event | NOT RUN |
| Notice/delivery | FND-12, IAM-10 | Original durable notice obligations remain identifiable; obsolete proof delivery stays suppressed; unknown provider outcomes remain unknown | NOT RUN |

### Required authority coverage

| Family | Desired policy outcome | Current evidence boundary |
|---|---|---|
| Sessions and password-purpose grants | Pre-recovery authority invalid; fresh authorized operation separately reviewed | #95 proves selected already-invalid state preservation only |
| Email verification and password recovery proof/assurance | Outstanding temporary proof not treated as current; historical consumption preserved | Source integration and selected recovered observations only |
| Recovery-code sets and independent factor binding | Reconcile durable authorized generation/revocation; no resurrection of removed codes or revoked factors | #95 lifecycle/relationship evidence only; independent history gap |
| Factor challenges and enrollment approvals | Pre-recovery temporary approvals not accepted as new action | Source tests only |
| Passkey registration/admission/activation proofs | Pre-recovery temporary proofs not accepted; pending state not treated as possession or login | v34 selected state; v36 depends on #81 |
| Proof-delivery leases and mandatory notices | No blind resend or obsolete proof delivery; preserve original notice identity | Provider delivery/custody unverified |

The distinction between durable recovery-code bindings and temporary proofs must be decided explicitly by Identity; no automatic revocation of legitimate durable credentials is implied by this proposal.

## G3 — Design boundary and decisions

Compare alternatives: (A) hold plus reconciliation alone, already exercised, insufficient for release-bound authority invalidation; (B) Identity-owned bounded invalidation policy with explicit fresh-authorization review and Platform-owned evidence boundary, recommended for further design; (C) production-grade independent continuity and release authority, outside this proposal and deferred to RH-04/05 and separate governance.

Identity owns identity semantics, audit/notice obligations and local transaction boundaries; Platform owns backup provenance and isolation. Neither a restored database nor a local epoch is an independent trust root. Keep production release outside this design. The original held recovery target is never activated to demonstrate success. No new APIs, SQL, role grants, schema, code, recovery commands, or implementation-ready operational procedure are proposed here.

Failure cases for design review: incomplete evidence, unknown scope, stale binding, partial invalidation, interrupted recovery, ambiguous outcomes, duplicate requests, concurrent normal activity, and unknown delivery status. The policy must fail closed, preserve original history, distinguish invalidation from consumption, and prevent unapproved release. Independent custody, actual ACL parity, and availability objectives remain unresolved. No claim of resilience to a compromised authority administrator.

Decisions requiring separate owner and technical review before implementation:
- D1: precise durable-versus-temporary classification per capability and disposition of ambiguous history.
- D2: authority and evidence preconditions for a separately reconciled synthetic execution fixture; fresh authorization eligibility.
- D3: atomicity/idempotency, interrupted invalidation, retention and notification obligations; no invented successful actions.
- D4: sequencing with #81 and boundaries with RH-04/05; no registry or release design adoption here.
- D5: explicit containment and G3 acceptance for any later synthetic implementation; production activation remains prohibited.

## G4 — Proposed slices (not authorized)

| Slice | Owner | Evidence objective | Gate |
|---|---|---|---|
| Authority-policy inventory and decision review | Identity | Reviewed classification and ownership; complete coverage matrix | G2/G3 pending |
| Bounded synthetic fence evaluation | Identity | Actual negative and independent fresh positive controls, unchanged original obligations | Explicit G3/experiment approval required |
| v36 reconciliation | Identity / #78 | Capability-specific durable/temporary state and notice evidence | #81 adoption dependency |
| Recovery/release integration | Platform / #14 | Independent registry, attempt scope, reviewed release boundary | Separate RH-04/05 |

## G5–G7 — Verification and outcome

All new proposal acceptance tests NOT RUN. PR #95 CI is historical evidence for its exact candidate, not verification of this proposal. No implementation changes or adoption of contract IDs in this PR. Review of the policy and separate owner authorization required before any bounded implementation. Independent technical acceptance not recorded; production readiness NOT ACCEPTED; G6 NOT AUTHORIZED. No merge or release authorization.
