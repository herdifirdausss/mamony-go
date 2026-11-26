# Identity recovery-session capability

- **Issue / owner:** #108; Identity; repository owner; independent technical reviewer not recorded.
- **Status:** PROPOSED; implementation BLOCKED.
- **Risk path:** Critical — distinct recovery authorization semantics, durable obligations and trust boundaries.
- **Decision:** [Owner ARC-01–05 / Option A](https://github.com/herdifirdausss/mamony-go/pull/102#issuecomment-7164103403) authorizes this documentation-only proposal, not adoption or implementation.
- **Affected contracts:** IAM-03/08/10/11/12, FND-08/12, OPS-02, EVT-03. No canonical contract amendments.
- **Evidence review:** 2026-11-25, main `f6b9a2d1f46413e7c87db0cc09d13acc4261c438`. Related #20/#14/#78/#95/#102/#106/#81. No new ADR adopted.

## G1 — Problem and evidence

Existing normal session lifecycle semantics and selected recovered-state evidence do not establish the distinct recovery-session policy. An internally consistent restore is not proof of authorized currentness. The required outcome is bounded, meaningful Identity policy evidence under whole-Identity HOLD, not release.

| Evidence | Observation | Limit |
|---|---|---|
| [Identity contract](../contracts/identity.md), IAM-03/08/10/11/12; ADR-0006/0009/0010/0011/0012 | Existing normal lifecycle and authentication authority | No established recovery-specific policy |
| [Platform contract](../contracts/platform.md), FND-08/12; [recovery runbook](../runbooks/recovery.md) | Trusted surviving evidence and isolated recovery prerequisites | No proof of independent completeness |
| [Reconciliation and events](../contracts/reconciliation-and-events.md), OPS-02/EVT-03 | Owning-transaction durable obligations and preservation of original event identity | No recovery-specific audit taxonomy or retention accepted |
| [Existing PostgreSQL evidence](https://github.com/herdifirdausss/mamony-go/actions/runs/45546872289/job/138978496689) | Actual normal lifecycle source integration cases PASS | Not recovered application policy verification |
| [#95](https://github.com/herdifirdausss/mamony-go/pull/95), [#106 final E1 review](https://github.com/herdifirdausss/mamony-go/pull/106#issuecomment-7152847805) | Bounded selected preservation and 12 symbolic cases respectively | Neither proves actual recovery-wide invalidation |
| [#102 architecture checkpoint](https://github.com/herdifirdausss/mamony-go/pull/102#issuecomment-7164029814) | ARC-01–05 architecture direction; approved separately by owner | No implementation-ready design |
| [NIST SP 800-63B-4](https://pages.nist.gov/800-63-4/sp800-63b.html) and [PostgreSQL 18 PITR](https://www.postgresql.org/docs/18/continuous-archiving.html) | Prior #102 research: lifecycle/assurance requirements and recovery-point semantics differ | No compliance or independent currentness claim |

Alternatives: ordinary lifecycle regression alone cannot meet the new requirement; additional symbolic tests are not actual capability evidence; a distinct logical Identity responsibility is the accepted direction, not a separate service; registry/release infrastructure is excluded. This proposal is the separate capability intake, not a duplicated #102 research log.

## G2 — Proposed requirements and scope

Actors: Identity capability owner, Platform recovery evidence owner, authorized independent reviewer and repository owner. Intended outcome: evaluate recovery-session policy without treating a recovery observation as permission to release. Initial target is bounded synthetic review only; no experiment authorized.

Scope is the session family and its necessary dependency review. Authentication grants and other temporary authorities must be accounted for as dependencies, not implicitly accepted. Financial authorization, v36, external delivery, live state, RH-04/05 registry and release remain separate. No changes to routes, workers, dependencies, schema, migrations, runtime or access policy under this proposal.

| Proposed AC reference | Falsifiable policy-level requirement | Evidence required (future approval) | Status |
|---|---|---|---|
| RSC-01 | Recovery-session disposition is distinct from normal lifecycle; all in-scope outstanding pre-recovery session authority is ineligible under accepted policy | Actual owning-capability behavioral evidence, not ordinary expiry or outage | PROPOSED / NOT RUN |
| RSC-02 | Legitimate durable binding and original logical security obligations are preserved; no fabricated user action | Independently reviewed scoped preservation evidence | PROPOSED / NOT RUN |
| RSC-03 | Incomplete, contradictory, interrupted or unknown outcome does not confer authorization | Observed negative evidence and explicit failure classification | PROPOSED / NOT RUN |
| RSC-04 | Fresh authorization control requires independently justified prerequisites and does not confer release or financial permission | Separately approved positive-control evidence | BLOCKED |
| RSC-05 | Partial family coverage cannot authorize partial release; session result is not all-family acceptance | Reviewed coverage and dependency inventory | PROPOSED / NOT RUN |
| RSC-06 | Administrative disposition has distinct audit meaning and applicable durable obligations; no duplicate/fabricated history | Actual owning-transaction evidence subject to accepted design | PROPOSED / NOT RUN |
| RSC-07 | Original held target unchanged; bounded disposable synthetic containment; no live access or transferable artifacts | Exact candidate containment review | PROPOSED / NOT RUN |

These are proposal-local references, not canonical AC IDs. Owner G2 acceptance and eventual stable owning-contract promotion require separate reviewed changes. No implementation test plan or execution authorization.

## G3 — Technical scope and dependency decisions

| Boundary | Decision / constraint |
|---|---|
| Placement | Distinct logical capability inside Identity; no standalone service |
| Platform | Provenance/isolation evidence only; no authority to grant access |
| Existing session and issuance | Preserve ordinary lifecycle, current assurance and purpose boundaries |
| Durable bindings | Independently justified currentness prerequisite; Profile A residual risk remains |
| Authentication grants and dependent sensitive actions | Dependency review required before complete session boundary claims; separate acceptance |
| Audit and notice | Original obligations preserved; security-sensitive changes require owning-transaction consistency under OPS-02/FND-12 |
| Retention | Existing policy preserved; no new numeric duration, custody, deletion or backup policy |
| Other authority families / v36 | No generalization; RF-08 blocked by #81 |
| Release | Whole-Identity HOLD; no registry, release eligibility or partial release |

Consistency direction: subject-scoped review granularity is not a transaction design. Overall completeness is distinct from local atomicity. Ambiguous or incomplete evidence remains unresolved. Do not fabricate success, infer independent history, or substitute ordinary lifecycle activity for recovery semantics. Administrative disposition is not successful user login/logout/password change or proof consumption. No new event taxonomy, notice trigger or retention override adopted. Independent currentness and protected evidence custody remain prerequisites.

Threat review: stale consistent state, incomplete authority coverage, unavailable/unknown outcome, interrupted or partial review, loss of original obligations, conflation of fresh control with release, and privileged authority compromise require explicit defensive evidence. Profile A cannot establish arbitrary authority-DB-write resistance. Technical feasibility of a distinct behavior under current constraints is unproven; no operational procedure, mutation algorithm, API, SQL, role, schema, registry, or deployment design is proposed. No new runtime, migration, or role changes are allowed by this decision.

### Implementation prerequisites — unresolved decisions

| ID | Decision required | Owner | Status |
|---|---|---|---|
| D-CAP-01 | Accept proposed scope and stable contract requirements through ordinary G2 review | Owner / Identity | OPEN |
| D-CAP-02 | Resolve distinct policy behavior, dependent authority semantics and feasible capability boundary | Identity technical review | OPEN |
| D-CAP-03 | Resolve consistency, interruption and durable-outcome semantics with obligations | Identity / Platform | OPEN |
| D-CAP-04 | Resolve administrative audit, applicable notices, retention and custody | Identity / security review | OPEN |
| D-CAP-05 | Establish reviewed containment, independent-review requirements and exact authorized implementation scope | Owner / reviewer | OPEN |

If a feasible design needs runtime, migration, access-policy, dependency or registry changes, stop and obtain a separate scope decision. No inferred permission. G3 technical acceptance not recorded; E1 deferral does not transfer.

## G4 — Sequencing (not implementation plan)

| Slice | Owner | Trace | Prerequisite | Status |
|---|---|---|---|---|
| Requirements and dependency review | Identity | RSC-01–07 | D-CAP-01/02 | PROPOSED |
| Design, threat and audit review | Identity / Platform | RSC-02/03/05/06 | D-CAP-02/03/04 | BLOCKED |
| Exact bounded capability proposal | Identity | RSC-01–07 | Accepted G2/G3; D-CAP-05 | BLOCKED |
| Future E2 behavioral verification | Identity | Approved capability and separate evidence scope | Accepted capability, exact experiment approval | BLOCKED |

No code paths, test execution procedures or allowed implementation files adopted. An exact G4 plan requires accepted technical design and separate owner authorization; do not turn this sequence into an implementation order without those decisions.

## G5–G7 — Verification and outcome

All RSC behavioral checks NOT RUN (RSC-04 blocked). Prior CI verifies existing behavior/documentation, not this capability. RF-01–07 NOT RUN; RF-08 BLOCKED; RH-03 PARTIAL. E1 bounded review COMPLETE, Draft #106 not authorized for merge. Independent technical acceptance for this Critical proposal unrecorded; G6 NOT AUTHORIZED; production NOT ACCEPTED. No E2 experiment, coding, migrations, runtime, registry, contract, governance, #95/#81/#106 changes, merge or activation. This document does not promote any active security contract.

## Consolidated G2/G3 review — D-CAP-01–04 (2026-11-26)

[Owner RSC-01–07 direction acceptance](https://github.com/herdifirdausss/mamony-go/issues/108#issuecomment-7180999826) authorizes consolidated documentation review only. Prior exact HEAD 24cd55b7740d82752d3a071aca43655be5440d90 passed five CI workflows. Main inspected f6b9a2d1f46413e7c87db0cc09d13acc4261c438. The approved architecture is a logical Identity capability; no new execution authority, contract amendment or runtime implementation is adopted.

### D-CAP-01 — Product boundary and acceptance trace

Recommend a session-family-first capability with a narrowly bounded claim, not recovery-wide release. The unit of review is a subject within the identified recovery scope; completeness of the entire affected Identity scope remains a separate requirement. A session family is not complete merely because an existing session is denied: issuance dependencies and session-dependent operations require a coverage decision. Unresolved dependent authority must be identified explicitly and precludes a whole-family or release claim. The target outcome is denial of in-scope pre-recovery session authority under accepted policy, legitimate durable binding preservation, and a separately justified fresh control. Unknown affected scope retains whole-Identity HOLD. No new capability authorizes financial or privileged actions.

RSC-01–07 requirements direction is owner-accepted; the table above remains the proposed acceptance detail. Map RSC-01/04/05 to IAM-03/08/10/11/12, RSC-02/06 to FND-12 and OPS-02/EVT-03, RSC-03/07 to FND-08/12 and the recovery runbook. Stable owning-contract AC IDs and effective amendments remain unadopted. Recommend one eventual reviewed contract/design package rather than another cycle of renaming proposal-local requirements.

### D-CAP-02 — Dependency and feasibility decision

| Boundary | Architectural decision | Evidence or prerequisite |
|---|---|---|
| Session eligibility | Identity-owned distinct recovery policy, preserve ordinary lifecycle contract | Actual capability behavior not established |
| Issuance and dependent authority | Explicit coverage review; no inherited authority or permission inflation | IAM-08/10/12; not all-family acceptance |
| Legitimate durable bindings | Independently justified currentness required | IAM-11; selected #95 evidence insufficient for independent completeness |
| Platform provenance and isolation | Required contextual evidence, not Identity access authority | FND-08; recovery runbook |
| Other temporary families, notices/delivery, v36 | Keep as explicit dependencies or exclusions; do not imply complete recovery | RF-03/06/08; #81 still unmerged |
| Registry and release authority | Separate RH-04/05; no new implementation dependency authorized | Whole-Identity HOLD retained |

Compare existing lifecycle reuse (insufficient without proven semantic equivalence), distinct Identity policy (preferred; actual capability and integration boundary unresolved), and separate trust infrastructure (outside scope). No evidence establishes that current approved semantics alone suffice. The feasibility result is **conditional**, not a guarantee of implementation or a proof of impossibility. Source review is not full runtime call graph, role/ACL inventory or deployed attestation. Recommend the owner explicitly decide whether to consider a future capability change, with separately reviewed technical scope, rather than assume the current test-only restriction can close the gap.

### D-CAP-03 — Consistency and failure decision

Recommend preserving security state and applicable durable obligations within the owning consistency boundary under OPS-02/FND-12. Subject-scoped review is a granularity for evidence collection, not a prescription for transaction granularity. Overall review completeness cannot be inferred from local atomicity. An explicit expected denial is only evidence of the policy actually evaluated. Infrastructure unavailability, incomplete evidence, contradictory observations and uncertain durable outcomes remain unresolved; no automatic retry, fabricated completion or partial release. The future accepted design must address duplicate/concurrent/interrupted behavior and the relation between session and issuance dependencies without weakening existing normal lifecycle guarantees. No transaction mechanism, operational procedure, schema, new interfaces, roles or release mechanism is adopted. Actual durability and concurrency feasibility remain open.

### D-CAP-04 — Audit, notice and retention decision

Recommend separate administrative recovery-disposition meaning, distinct from user authentication, logout, password change or proof consumption. Preserve original event identity, legitimate bindings and existing notice obligations; associated audit and applicable notices must meet owning-transaction consistency under OPS-02. Distinguish logical obligation, dispatch and observed receipt. Existing retention is unchanged; no reconstruction of purged historical records or extension of retention without policy review. Require minimal redacted, attributable evidence and custody/access/disposal review. New event taxonomy, notification applicability, retention periods and persistence implementation remain explicitly undecided. Independent surviving evidence remains necessary for currentness; Profile A privileged-authority compromise is not contained by local audit or checksums.

### Decision register and exit gate

| Decision | Review result | Remaining blocker |
|---|---|---|
| D-CAP-01 | Requirements direction accepted; bounded scope and trace proposed | Stable contract detail and effective adoption |
| D-CAP-02 | Identity-owned distinct policy recommended | Actual capability boundary and dependency feasibility |
| D-CAP-03 | Consistency and failure invariants recommended | Technical durability/concurrency design |
| D-CAP-04 | Audit/retention policy direction recommended | Event/notice applicability, custody and technical retention design |
| D-CAP-05 | Not authorized for implementation | Accepted design, exact scope, containment and independent review decision |

Consolidated review is prepared, not implementation-ready G3 acceptance. Next owner decision should consider either a separate bounded technical-design scope for an actual capability change, explicitly identifying any proposed runtime/migration/role implications for separate approval, or deferral. No further symbolic policy experiment or generic requirements-renaming round is recommended. Do not create code or execute tests to close these blockers under current authority. Actual E2 remains BLOCKED; all new RSC evidence NOT RUN (fresh control blocked), RF-01–07 NOT RUN, RF-08 BLOCKED (#81 observed unmerged head 6c12b5cee007acbc98109fa67f3b216ae16abd1d), full RH-03 PARTIAL. No E1 independent review deferral transfer, G6 authorization or production acceptance. No changes to runtime, migrations, registry, canonical contracts, governance, #95/#81/#102/#106 code or release.
