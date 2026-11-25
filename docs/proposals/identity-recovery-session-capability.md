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
