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

## Consolidated technical-design package (2026-11-26)

[Owner D-CAP-01–04 acceptance and design-only authorization](https://github.com/herdifirdausss/mamony-go/pull/109#issuecomment-7194673324). Prior candidate 732e2b0f6f34da007783d9b844f3f0a8d29913ab passed five workflows and seven required contexts. During review, synchronization to e0ea68497885ea3bc44fcc2343a9c3a34dc3068b was observed at d7c89d949684a18326ecadcef33db7612651f1ed; proposal blob unchanged, one-file diff. No synchronization performed by this review and no merge approval inferred. Main delta since prior base comprises five documentation files concerning proposed native migration executor/ADR-0042. Current DEC-01 and golang-migrate remain effective; no native-executor adoption assumed.

### Capability and compatibility

Select a distinct logical Identity-owned capability within the modular monolith, not a service, ordinary logout, credential reset, or public recovery flow. Platform provenance is contextual evidence, not authorization. Subject is the unit of scoped evaluation; whole-Identity HOLD remains in effect pending separate release governance. Preserve existing normal authentication assurance, purpose, idle/absolute expiry, suspension, revocation, concurrent-session policy and sensitive-action boundaries. Evaluation never confers release, financial permissions or new recovery-proof authority. Prefer owning-boundary consistency for any future authorized security change, rather than distributed service or registry. This is a guarantee, not an implementation mechanism; no original held-target access or mutation proposed.

| Area | Compatibility and dependency decision |
|---|---|
| Session and issuance | Direct semantic boundary; preserve existing normal lifecycle and assurance |
| Authentication grants and dependent operations | Coverage review required; no inherited authorization or implicit grant-family acceptance; affected behavior requires separate scope |
| Durable bindings | Preserve legitimate credential/contact/factor relationships; independently justified currentness prerequisite |
| Security events and notices | Preserve original obligations and identities; no external delivery integration |
| Platform | Provenance, scope, isolation and surviving evidence only |
| v36, other temporary families, financial approvals | Explicit exclusions from this capability; unresolved coverage remains HOLD; #81 observed unmerged at 9836f0bcb94bd60ecb481ea3959c3bdfaf2515f7 |
| Registry/release | Separate RH-04/05; no new dependency |

Latest-main tree confirms existing Identity session, authentication/grant, password-change and recovery components. Source-level inventory is not exhaustive call graph, permission audit or deployment attestation. Trace IAM-03/08/10/11/12, FND-08/12, OPS-02, EVT-03 and prior source integration evidence. No new dependency selected.

### Consistency and audit

Prefer owning-transaction consistency for accepted security-state changes and applicable durable audit/notice obligations under OPS-02/FND-12. Unknown, interrupted, partial or contradictory outcomes remain unresolved; explicit policy denial is distinct from unavailability. Eventual design must address concurrent/duplicate outcomes without duplicated logical obligations, fabricated completion or partial authorization; local atomicity does not establish independent completeness. Subject-scoped review does not prescribe transaction granularity.

Recommend attributable mandatory administrative audit for an accepted security change, distinct from user login/logout/password change/proof consumption. Assess user-notification applicability before adopting any new event or notice rule. Preserve original event IDs and obligations irrespective of delivery/provider acknowledgement. Minimize and redact evidence; review custody/access/disposal. Preserve current retention by default; no reconstruction of purged records or new durations. No event identifier, trigger, schema, storage contract, role or technical mutation mechanism adopted.

### Impact assessment

| Area | Assessment | Prerequisite |
|---|---|---|
| Identity behavior | Actual distinct policy gap, not another test gap | Separate scope for actual behavior changes |
| Existing session and issuance | Potential direct impact | Complete dependency and permission review |
| Persistence and migration | May be needed for durable obligations and disposition; necessity not established | Separate persistence/migration decision under effective DEC-01 |
| Audit and notices | Potential new administrative event/obligation policy | Resolve applicability, retention and custody |
| Runtime and deployment | No new service, route, worker, provider or registry proposed | Exact candidate containment review before any experiment |
| Performance/availability | No measured workload or accepted contention/recovery budget | Establish applicable bounds and evidence; no invented target |
| Trust | Profile A arbitrary-authority-DB-write and lost independent history risk | Establish trusted evidence assumptions and residual risks; independent acceptance separate |

### RSC-01–07 acceptance trace

| Requirement | Boundary | Evidence / status |
|---|---|---|
| RSC-01 | Distinct owning-capability recovery policy, not ordinary expiry or outage | Actual behavior / NOT RUN |
| RSC-02 | Legitimate bindings and original obligations preserved, no false history | Scoped preservation evidence / NOT RUN |
| RSC-03 | Unknown, incomplete or interrupted outcome never counts as authorization or completion | Negative and consistency evidence / NOT RUN |
| RSC-04 | Independently justified fresh control under applicable assurance, never release or financial authority | Separate positive-control review / BLOCKED |
| RSC-05 | Explicit session/issuance coverage and exclusions; no partial release | Coverage / PARTIAL source review |
| RSC-06 | Correct durable administrative audit and applicable obligations; no duplicate logical history | Owning consistency evidence / NOT RUN |
| RSC-07 | Exact bounded synthetic containment and unchanged held target; no live access or transferable artifacts | Exact candidate evidence / NOT RUN |

No new AC names, canonical contract adoption or runnable test plan. Existing regression CI and E1 are context, not behavioral acceptance.

### Disposition and remaining blockers

Recommendations are consolidated: distinct Identity capability, normal lifecycle compatibility, owning consistency guarantee, separate administrative audit, existing retention by default, no registry/release. No more generic policy reviews or toy models recommended. Actual unresolved technical decisions: (1) authoritative currentness and affected-scope evidence including issuance dependencies; (2) feasible integration and durable consistency under an accepted trust boundary; (3) event/notice applicability and evidence custody/retention; (4) exact change impact, permission/containment and independent review. Requirements direction accepted; implementation-ready G3/G4 and stable contract details not adopted. Do not infer that a test-only change can close the gap or that implementation is impossible.

Recommend deferring actual E2 until these prerequisites can be established through separately authorized technical design and qualified review. No operational recovery procedure, credential replay workflow, mutation algorithm, schema, role grants, interfaces or registry design proposed. No implementation authorization requested at this checkpoint. E1 deferral does not transfer. E2 BLOCKED, RF-01–07 NOT RUN, RF-08 BLOCKED, RH-03 PARTIAL, G6 NOT AUTHORIZED, production NOT ACCEPTED. No coding, migration, registry, runtime, canonical contract or governance changes, merge or activation.

## Targeted post-merge dependency reassessment — PR #81

Owner authorizes documentation-only reassessment, not implementation or merge. Current main 76d417d35571b28b7aa748577861fc3a1f9e6dcb is the #81 merge commit with parents e0ea68497885ea3bc44fcc2343a9c3a34dc3068b and owner-approved 74ac898117ff75d51e17bb42076728bd81160484. Complete 428-entry repository tree comparison matches the approved candidate exactly. #81 bounded synthetic merge is accepted; independent technical acceptance DEFERRED for #81 only, production NOT ACCEPTED. No dedicated main-commit workflow runs were returned at inspection; do not report new post-merge execution. [Exact pre-merge PostgreSQL job](https://github.com/herdifirdausss/mamony-go/actions/runs/45838246945/job/139972730758) actually records PASS for passkey possession, finisher-role coverage, Identity migration and local epoch invalidation. Recovery review markers F05/F06 remain PARTIAL; external controller and release authority NOT RUN. These observations establish the bounded merged dependency, not a recovery-session capability or independent currentness.

### Changed decisions only

- Version mapping RESOLVED: [atomic activation proposal](identity-passkey-atomic-activation.md) explicitly maps capability v36 to SQL 000035 (fingerprint version 35); capability v35 was assertion verification without new SQL. No migration 000036 is expected for v36. Historical “unmerged candidate” text in the owning contract is stale; separate documentation reconciliation recommended, not edited here.
- RF-08: prior blocker “#81 unmerged” is removed. Status is NOT RUN for the required integrated recovery-specific acceptance, rather than PASS or blocked on merge. Remaining prerequisites: accepted scope and recovery-specific policy, provenance/coverage, actual behavioral evidence, containment and review. Existing bounded possession/role/migration evidence is not equivalent to recovery-wide invalidation or fresh post-recovery authority.
- Authority coverage: passkey possession is now part of the merged source inventory, not an unmerged dependency. It remains a separate authority family outside the selected session-first capability implementation scope. Normal session/issuance and dependent authority coverage is still incomplete; local epoch and recovery review observations cannot establish independent currentness. No inference of live deployment, complete authority coverage or release.
- Consistency and audit: merged passkey evidence adds context for local owning-transaction obligations; does not decide the recovery-session integration, administrative event/notice applicability, custody/retention or cross-family completeness. Existing OPS-02/FND-12 guarantees remain unchanged.

| RSC | Delta after merge |
|---|---|
| RSC-01 | No recovery-session-specific behavior established; NOT RUN |
| RSC-02 | Additional passkey-local obligation evidence is context only; NOT RUN for proposed capability |
| RSC-03 | Bounded recovery refusal evidence available; no E2 consistency acceptance, NOT RUN |
| RSC-04 | Merge prerequisite removed for passkey-related dependency only; fresh control still BLOCKED on evidence and containment |
| RSC-05 | Inventory updated to include merged possession; PARTIAL source coverage, no all-family acceptance |
| RSC-06 | Local passkey transaction/audit evidence not administrative recovery-disposition evidence; NOT RUN |
| RSC-07 | #81 owner-attested bounded synthetic scope does not authorize or establish containment of future capability; NOT RUN |

Four earlier blocker groups remain; only the merge prerequisite and version mapping changed. No additional policy refinement or symbolic experiment recommended. Further work requires specific accepted design/evidence and independent review decisions, not merely the availability of #81. RF-01–07 NOT RUN, RF-08 NOT RUN (previous merge blocker removed), RH-03 PARTIAL. E2 and implementation-ready G3/G4 remain blocked, G6 NOT AUTHORIZED for #109, production NOT ACCEPTED. Independent technical acceptance unrecorded for #109; #81 and E1 deferrals do not transfer. No new tests, coding, migrations, registry, runtime, canonical contract or governance changes, merge or activation.

## Decision-ready prerequisite review — 2026-11-26

The #81 merge is verified at main `76d417d35571b28b7aa748577861fc3a1f9e6dcb`. Subsequent direct push evidence is now available: [engineering baseline](https://github.com/herdifirdausss/mamony-go/actions/runs/45839562201) and [PostgreSQL integration](https://github.com/herdifirdausss/mamony-go/actions/runs/45839562241) succeeded on that exact main commit. This supersedes only the earlier main-CI discovery limitation. The proposed recovery-session behavior has not been executed.

| Existing blocker | Decision recommendation | Required exit evidence |
|---|---|---|
| Authoritative currentness and affected scope | Retain Profile A for the bounded candidate; do not adopt a stronger trust model by implication. Defer the experiment if currentness or scope cannot be independently justified. | Platform and Identity reviewers identify an accepted source of evidence, its scope, custody and limitations; owner confirms the evidence assumptions. Local restored state alone is insufficient under IAM-11/FND-12. |
| Integration and durable consistency | Retain the Identity-owned modular-monolith boundary and normal lifecycle compatibility; reject a new service or registry as an unapproved dependency. | Qualified review establishes actual affected component coverage, compatibility and consistency feasibility against IAM-03/08/10/12 and FND-12; design approval precedes implementation. |
| Audit, notices and retention | Adopt for review the policy direction of distinct attributable administrative audit for an actual security-state change. Preserve original obligations; do not treat recovery observation as a new user action or evidence of delivery. Keep existing retention by default and no live sender. | Owner/security reviewer accepts notification applicability for the actual proposed disposition and confirms custody, access, retention and durable-obligation requirements. No event or storage contract is adopted here. |
| Exact impact, containment and review | Use a separately scoped Critical change only after the first three decisions are supported; no automatic transfer of #81 or E1 review deferrals. | Qualified reviewer signs off the proposed scope and applicable risks, owner authorizes the exact bounded implementation and containment, and the relevant independent review gate is recorded. |

**Decision requested:** accept these constraint and audit-policy recommendations as documentation-only design direction, or defer the capability. Acceptance of this recommendation does not establish any missing evidence or adopt canonical contract amendments. The authoritative evidence source and concrete integration feasibility remain blocking; a further generic policy review cannot resolve them. Do not mark RSC-01–07 or RF-01–08 PASS from this review. RSC-04 remains BLOCKED, RSC-05 PARTIAL source review, RH-03 PARTIAL, E2 BLOCKED, G6 NOT AUTHORIZED, production readiness NOT ACCEPTED. No implementation, migration, registry, runtime, governance, merge or activation authorized.
