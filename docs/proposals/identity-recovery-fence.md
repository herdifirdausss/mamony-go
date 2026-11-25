# Identity recovery fence — proposed requirements and design boundary

**Owner:** Identity (recovery semantics); Platform #14 (recovery infrastructure); coordinate with #78 (v36 capability).  
**Status:** Critical / G2 requirements direction owner-accepted; detailed G2/G3 design PROPOSED; implementation BLOCKED.  
**Decision:** [Owner-approved scope split](https://github.com/herdifirdausss/mamony-go/issues/14#issuecomment-7152224953) authorizes preparation of this proposal, not its adoption, implementation, merge or activation.  
**Requirements-direction decision:** [Owner approval D1–D5](https://github.com/herdifirdausss/mamony-go/pull/102#issuecomment-7152312494). Design review only; no implementation, migration, registry, merge or activation.  
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

## G2/G3 refinement — owner-approved direction, detailed design pending (2026-11-25)

### Evidence and alternatives

Primary references checked 2026-11-25: [NIST SP 800-63B-4, authenticator event management and session management](https://pages.nist.gov/800-63-4/sp800-63b.html) separates authenticator binding/invalidation, account recovery and session assurance; it does not prescribe this database recovery architecture. [PostgreSQL 18 PITR](https://www.postgresql.org/docs/18/continuous-archiving.html) describes WAL continuity/timeline recovery and limits regarding configuration files; it is not proof of authorized latest Identity history or production access parity. The local IAM-03/08/10/11 and FND-08/12 contracts and recovery runbook govern Mamony; no claim of NIST compliance. PR #81 remains unmerged (observed head 8c6bb4ac8143f529824c61fcc23307560238c5c9); its design is contextual only.

| Alternative | Security property | Cost / limitation | Recommendation |
|---|---|---|---|
| Restore then keep authority unavailable | Contains uncertainty | No fresh operation or fence verification | Mandatory default when evidence is incomplete |
| One local generation rule alone | Potentially simplifies some local authority checks | Does not establish independently surviving currentness or coverage of all proof families | Insufficient as sole recovery policy |
| Identity-owned family-specific disposition policy under common HOLD invariant | Separates durable binding reconciliation from temporary-authority disposition; explicit coverage and negative controls | Larger review burden; cross-family consistency and retention must be designed | Preferred design direction for bounded evaluation, not implementation approval |
| External recovery authority plus production release | Addresses independent attempt/currentness and release governance under defined trust assumptions | New critical trust boundary, operational cost and failure modes | Separate RH-04/05 decision; not designed here |

### D1 — Classification and invariants

| Domain | Classification | Required disposition | Evidence limitation |
|---|---|---|---|
| Subject status, verified contacts, password verifier | Durable binding | Reconcile authorized state; unknown stays HOLD | Selected #95 observations only |
| Saved code set, factor key, staged passkey credential | Durable binding with lifecycle and provenance | Preserve authorized current generation and historical obligations; never infer eligibility from presence | Rotated-out rows may be absent; staged is not verified possession |
| Session and password-purpose grant | Temporary authority | Pre-recovery outstanding authority invalid for future release; preserve existing historical disposition | #95 preserved-state controls, not recovery fence |
| Verification/recovery challenge and assurance | Temporary authority | Require disposition across all enabled families; preserve original outcomes | Source tests and recovered observations only |
| Factor challenge, enrollment approval, registration intent/admission, activation challenge | Temporary authority | No implied fresh authorization from restored state | Actual recovery fence NOT RUN; v36 separate |
| Security audit and notice | Durable obligation | Preserve original logical event identity; distinguish obligation, provider acceptance and receipt | No provider guarantee |
| Delivery lease / proof-delivery work | Operational temporary authority | Uncertain/obsolete delivery remains suppressed pending reviewed policy | No live delivery or resend |

Do not equate recovery of data with recovery of permission. No repair of uncertain durable bindings by fresh proof alone. No fabricated consumption, success event or possession history. A saved recovery code is not interchangeable with an ephemeral challenge. Precise disposition, retention and audit semantics remain design-review requirements; this document does not prescribe database mutations or grant changes.

### D2 — Conceptual state and trust boundaries

The following are review labels, not a runtime state machine, API, registry, permission, or executable recovery procedure.

| Review label | Necessary evidence | Failure disposition |
|---|---|---|
| HOLD | Original candidate unavailable to application/production | Default; cannot infer release |
| RECONCILIATION_REVIEW | Scoped source provenance, evidence completeness, compatible schema/access assumptions, authorized current bindings | HOLD on missing/contradictory evidence |
| FENCE_EVIDENCE_REVIEW | Separately approved disposable fixture; independently reviewed family coverage, expected denials and original obligations | HOLD on unknown or partial outcomes |
| FRESH_CONTROL_REVIEW | Actual approved fresh authorization control on separate synthetic fixture under existing policy; no inherited authority | HOLD on unexpected result |
| BOUNDED_REVIEW_COMPLETE | Exact candidate, evidence and limitations ready for owner/technical review | Still HOLD for all real targets; no release action |

Platform supplies provenance and isolation evidence; Identity owns interpretation of legitimate bindings and authorization outcomes. Neither party can infer permission to release from the other's evidence alone. Scope is whole Identity if affected scope cannot be established. A synthetic fixture can model a complete controlled boundary, but is not independent production custody. Evidence categories must identify exact attempt/candidate scope, relevant source boundary, policy/schema revision, family coverage, outcome and limitations; no registry schema or interface is adopted here.

### D3 — Threat and failure review

| Threat / failure | Required defensive outcome | Remaining verification |
|---|---|---|
| Stale or incomplete restored history | HOLD; no claim that a consistent restore is current | Independent completeness separate |
| Uncertain credential ownership or revocation | HOLD affected scope; do not infer authorized re-binding | Durable policy review |
| Outstanding pre-recovery temporary authority | All enabled families included in reviewed invalidation coverage; actual owner-capability denials required | Recovery-wide fence NOT RUN |
| Partial/interrupted or ambiguous invalidation | No partial release, automatic success or blind retry; preserve evidence of uncertainty | Atomicity and outcome model review |
| Concurrent normal activity or duplicate evaluation | No inconsistent eligibility or duplicated logical obligations | Concurrency/idempotency review and tests |
| Incorrect or forged success evidence | Reject incomplete/contradictory scope or evidence; no invented consumption/audit | Independent custody not established |
| Notice/provider uncertainty | Preserve event identity, suppress obsolete proof delivery, do not infer user receipt | Delivery-specific design and external outcomes NOT RUN |
| Privileged authority compromise | Profile A does not guarantee resistance; hold and reassess independent trust | Explicit residual risk |
| Cross-capability authority confusion | Recovery status does not confer login, money, or privileged-action authority | Existing IAM-10 and #18 boundaries remain |

Atomicity principle: Identity must own the consistency boundary for each approved disposition and its audit obligations. An incomplete multi-family operation is never equivalent to completion. The choice of transaction granularity, durable outcome model, retention and retry policy is unresolved and requires further independent review; no database or runtime design adopted. Availability is subordinate to known authorized state; no production downtime or RPO/RTO target promised.

### D4/D5 — Proposed acceptance evidence and dependencies

| Proposal-local test label | Required result | Owner / dependency | Current status |
|---|---|---|---|
| RF-01 | Missing, stale, unknown or contradictory provenance and scope remain HOLD | Platform evidence + Identity interpretation | NOT RUN |
| RF-02 | Legitimate selected bindings and original obligations preserved; uncertain bindings held | Identity; #95 observations inform but do not close | NOT RUN |
| RF-03 | Actual negative outcomes for every enabled pre-recovery temporary authority family; no new successful user action | Identity, explicit per-family policy required | NOT RUN |
| RF-04 | Separately authorized fresh control on disposable reconciled fixture; original held target untouched | Identity, approved containment | NOT RUN |
| RF-05 | Interrupted/ambiguous and concurrent/duplicate evaluation never authorizes partial release or duplicates logical obligations | Identity, consistency design pending | NOT RUN |
| RF-06 | Original event identity and notice obligations preserved; obsolete delivery not treated as current; unknown provider outcome remains unknown | Identity, delivery policy pending | NOT RUN |
| RF-07 | No new production path, credentials, registry or activation; exact fixture scope and actual isolation evidenced | Platform + Identity, separate experiment approval | NOT RUN |
| RF-08 | Capability-specific v36 possession and notice reconciliation | #78 / #81 adoption prerequisite | BLOCKED |

Labels are local review references, not canonical IAM/FND additions. RF-01–08 complement existing RH-03 ACs; none is waived or marked PASS by #95 or proposal CI. Detailed implementation context remains blocked. Proposed later work order: review policy classification, evidence prerequisites, consistency/audit semantics and containment; obtain explicit approved technical design and exact bounded experiment scope; only then plan implementation and checks. No new migration, role, API, registry, runtime or delivery implementation authorized. No changes to #95, #81, canonical contracts or governance. Independent technical acceptance remains unrecorded for this proposal, production NOT ACCEPTED, G6 NOT AUTHORIZED.

## G3 refinement — consistency, audit and bounded experiment review

Status: proposed for review, not implementation authorization. Owner direction D1–D5 remains accepted; detailed decisions below are not accepted. Primary references verified 2026-11-25: [PostgreSQL 18 transactions](https://www.postgresql.org/docs/18/tutorial-transactions.html), [transaction isolation](https://www.postgresql.org/docs/18/transaction-iso.html), [NIST SP 800-63B-4](https://pages.nist.gov/800-63-4/sp800-63b.html). Local atomicity and isolation cannot establish independent recovery currentness or a cross-transaction release decision. NIST authenticator lifecycle guidance informs classification but does not specify this recovery mechanism.

### C1 — Consistency scope alternatives

| Candidate | Advantage | Trade-off |
|---|---|---|
| Whole-Identity atomic operation | Single local consistency boundary | Large transaction and availability cost; not independently authenticated history |
| Subject-scoped atomic evaluation | More bounded work and review | Partial subject coverage cannot authorize global or per-subject release |
| Authority-family evaluation | Capability-specific reasoning | Cross-family dependencies and incomplete outcomes need separate review |

Recommend evaluating subject-scoped consistency as a bounded unit of analysis under the whole-Identity HOLD invariant, not adopting a transaction or mutation algorithm. No per-subject release, no distributed transaction assumption, no exposure of partially reviewed state. Scope uncertainty means global HOLD. Transaction granularity and cross-subject obligations remain unresolved until a reviewed dependency and resource analysis establishes feasibility. No specific locking, persistence, schema, role or procedure is accepted.

### C2 — Outcome and audit semantics

| Evidence classification | Meaning | Required review outcome |
|---|---|---|
| Original committed user/security action | Existing legitimate durable event | Preserve original identity and historical disposition |
| Recovery invalidation | Administrative safety disposition, not a successful user action | Distinguish from consumption, possession, enrollment, authentication or delivery success |
| Reconciled unchanged | Observed equality under stated scope and provenance | No fabricated event or claim of independent completeness |
| Not established | Missing, contradictory or ambiguous evidence | HOLD; do not infer successful completion |
| Review complete | Bounded evidence available for separate approval | Not authorization to activate or release |

Preserve logical event identities and notice obligations; no deletion, mutation, duplication or regeneration policy adopted by this document. New administrative audit taxonomy, retention and custody require separate reviewed decisions. Minimize evidence: category, outcome, scoped reference, revision, limitations and reviewer attribution; no credentials, raw proofs, private keys or recipient contents in review output. Distinguish provider acknowledgement, durable obligation and user receipt. Unknown external outcomes remain unknown; no blind resend.

### C3 — Failure and completion semantics

| Scenario | Required result |
|---|---|
| One scope reviewed, another unresolved | Overall HOLD; no partial release |
| Interrupted evaluation or ambiguous result | Unknown until reviewed evidence establishes outcome; no blind retry or success claim |
| Duplicate evidence or evaluation | No duplicated logical obligation or conflicting completion claim |
| Evidence or policy revision changes | Previous review does not authorize changed candidate |
| Missing capability coverage | BLOCKED, not silently treated as disabled or PASS |
| Unexpected authority or audit behavior | Stop bounded evaluation, retain sanitized finding, keep original target held |

Completion is an evidence classification, never permission. No recovery registry, release state machine or automated decision is introduced. The original held target must not be used for application execution.

### C4 — Proposed bounded experiment acceptance boundary

Only a future explicitly approved disposable synthetic evaluation could investigate RF-01–07. This proposal authorizes none of it. Suggested review scope: approved synthetic fixtures and existing owning-capability behavior; actual expected denial and independently scoped positive controls, original durable obligations, unknown/incomplete evidence and isolation. No transferable credentials, external services, real user data, production resources, new runtime, migration, registry, or access-policy changes. Use existing test containment where appropriate only after review of exact fixture scope. RF-08 is BLOCKED pending separate v36 capability adoption. No implication that v34 or coordinator-only evidence satisfies v36.

Before any implementation: owner accepts precise consistency/disposition semantics, event/audit meaning and retention bounds, exact allowed files and fixture scope, coverage inventory, positive/negative checks, and stop conditions; independent technical acceptance or explicitly authorized bounded solo experiment per repository governance. A proposal CI pass is not behavioral evidence. If required behavior depends on a new runtime/migration or unresolved cross-family semantics, stop and request separate design review rather than weaken scope or tests.

| Decision | Recommended direction | Status |
|---|---|---|
| C1 | Subject-scoped evaluation with whole-Identity HOLD, no release | PROPOSED |
| C2 | Preserve original obligations; distinguish administrative invalidation from consumption; minimized evidence | PROPOSED |
| C3 | Explicit unknown outcome and completeness; no partial release | PROPOSED |
| C4 | Separate exact synthetic scope/implementation authorization; no v36 import | PROPOSED |

All RF-01–07 NOT RUN; RF-08 BLOCKED. No accepted implementation procedure. Independent technical acceptance unrecorded; G6 NOT AUTHORIZED; production NOT ACCEPTED.
