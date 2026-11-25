# Identity recovery fence — proposed requirements and design boundary

**Owner:** Identity (recovery semantics); Platform #14 (recovery infrastructure); coordinate with #78 (v36 capability).  
**Status:** Critical / G2 requirements and P1–P4 policy baseline owner-accepted; exact E1 experiment package PROPOSED; implementation BLOCKED.  
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

## Final G3 policy review — proposed decision package (2026-11-25)

[Owner C1–C4 direction acceptance](https://github.com/herdifirdausss/mamony-go/pull/102#issuecomment-7152424857) authorizes this review, inventory, retention requirements and bounded experiment proposal only. Prior exact documentation HEAD 6cedb8a23aacf9c0ff3bfd4993c1e062ffa50ea6 passed all five workflows. Main inspected f46459615189ea0657c630f589219b082aee9b71. #81 remains unmerged (observed head 8c6bb4ac8143f529824c61fcc23307560238c5c9); no v36 import. This package is a proposed policy baseline, not accepted implementation design.

### P1 — Coverage inventory (source-level, not catalog/ACL attestation)

The main tree contains Identity migration versions 1–34. Review of table creation definitions in the relevant migrations identified 28 table names grouped below. Twenty-two are selected in #95's snapshot; six additional names are not in that selected inventory. This is a source-level inventory of identified tables, not exhaustive function/role/dependency or deployed-catalog parity. Classifications are proposed; an omitted or unknown authority family blocks any completeness claim.

| Group | Main creation sources (Identity migration version) | Identified table names | Required recovery review | Evidence |
|---|---|---|---|---|
| Durable binding | 1, 24, 28, 33 | subjects, email_aliases, password_verifiers, password_recovery_offline_codes, recovery_code_factor_keys, passkey_credentials | Authorized generation, binding and disposition; staged passkey not activated | #95 selected state; complete history NOT RUN |
| Temporary proof / session | 2, 10, 11, 19, 21, 25, 28, 30, 32, 34 | email_verification_challenges, opaque_sessions, password_login_grants, password_recovery_challenges, password_recovery_assurance_grants, password_recovery_code_enrollment_approvals, recovery_code_factor_challenges, passkey_registration_intents, passkey_registration_admissions, passkey_activation_challenges | Outstanding pre-recovery authority invalid before any separately authorized release; preserve historical outcomes | #95 selected observations and preserved-state controls; full fence NOT RUN |
| Durable security obligation | 1, 16, 21, 25, 33 | security_outbox, password_change_notices, password_recovery_notices, password_recovery_code_notices, passkey_registration_notices | Original event identity, disposition and notice obligation; not proof of delivery | #95 selected evidence; full obligation continuity NOT RUN |
| Schema evidence | 1 | migration_fingerprints | Exact compatible version and provenance; no upgrade or migration in proposal | #95 selected evidence; deployed parity NOT RUN |
| Operational state outside #95 selection | 3, 5, 19 | email_proof_deliveries, anonymous_admission_buckets, password_recovery_deliveries | Review obsolete proof work, delivery uncertainty and admission continuity without relaxing existing policy | NOT RUN |
| Provider evidence outside #95 selection | 17, 23, 27 | security_notice_mailgun_receipts, password_recovery_notice_mailgun_receipts, recovery_code_notice_mailgun_receipts | Distinguish provider evidence from user receipt; no automatic resend or live provider action | NOT RUN |

The six outside #95 selection are not asserted defects in its owner-approved bounded scope. They remain in the broader Identity recovery review. Admission counters are operational protection, not permission or durable authenticator binding; do not infer that recovery permits resetting budgets. Table-level inventory is insufficient to establish all functions, active runtime roles, external state, retention dependencies or sensitive-action approval coverage. Financial authorization remains under IAM-12/#18 and its owning transaction, not granted by Identity recovery. V36 possession and related notice are explicitly blocked by #81. For later implementation review, require confirmed enabled/disabled family list, owning capability, dependency and retention map; unknown classifications block coverage.

### P2 — Retention and evidence requirements

Observed existing policy (source and contract, not new recovery retention): v6 retention maintenance clears expired email proof envelopes after expiry +24h and anonymous accounting after window start +48h; v10/v11 creation migrations include session/grant cleanup after expiry +24h. Existing notice policy v16/v22/v26 has seven-day recipient-bearing notice cleanup, with provider receipt retention described in Identity contract for v17/v23/v27. These are existing local maintenance boundaries, not permission to change retention, suspend privacy obligations, purge physical backups/WAL, or reconstruct records that legitimately expired. The current source does not establish a complete recovery history retention schedule.

| Evidence class | Proposed requirement | Decision still needed |
|---|---|---|
| Credential/proof material | Never include raw passwords, bearer values, recovery codes, private keys, encrypted payloads or recipient contents in review output; no transferable artifact | Existing policy and privacy review |
| Historical security events and disposition | Preserve legitimate original event identity and distinguish invalidation from successful user action | Audit taxonomy, access and retention duration |
| Notice and provider evidence | Preserve original logical obligation and classify unknown outcomes; do not assume a missing purged row proves loss or delivery | Reconciliation with approved retention and surviving evidence |
| Admission and temporary operational state | Respect existing protective controls and retention; unknown state stays unresolved | Continuity and resource policy review |
| Synthetic evidence artifacts | Aggregate/redacted evidence only; disposable fixture material removed with fixture; record cleanup limits | CI artifact lifetime and authorized access before experiment |
| Backup/WAL and external history | Separate Platform retention/custody policy; logical deletion not proof of physical deletion | Independent history availability and legal/privacy review |

No new numeric retention period, legal hold, retention bypass, deletion policy, event schema or storage authority is approved here. If currentness cannot be established with legitimately retained evidence, HOLD remains the outcome; do not extend retention ad hoc or fabricate history. Owner and independent privacy/security review must settle a minimum necessary evidence window, access/custody, disposal and lawful exception policy before production. These are production blockers, not waived by a synthetic review.

### P3 — Proposed bounded experiment (design gate only)

Goal: evaluate whether a separately approved disposable synthetic fixture can support meaningful evidence of the selected recovery-policy requirements under the common HOLD invariant, without any release or production behavior. This document authorizes no execution or implementation. Preferred first slice is inventory/coverage validation and reviewed evidence classification, before any behavioral implementation. Do not expand a passing subset into all-family acceptance.

| Proposed stage | Evidence objective | Exit / stop rule |
|---|---|---|
| E0 — Inventory review | Confirm exact main revision, enabled authority families, source-level ownership, retention and evidence assumptions | Unknown coverage or incompatible policy stays BLOCKED |
| E1 — Bounded policy evaluation | Verify that the proposed review classifications distinguish complete, missing, contradictory, unknown and partial evidence; preserve HOLD | No actual authorization behavior claim from classification alone |
| E2 — Separate behavioral evidence review | After separate technical and experiment approval, assess actual owning-capability denial of pre-recovery temporary authority and fresh independently scoped positive controls in disposable fixtures; preserve original obligations | Stop on unexpected outcome or need for new runtime/migration/role/registry semantics; do not treat E1 as E2 |
| E3 — Review consolidation | Record exact candidate, actual results, limitations, containment and coverage | RF-01–07 remain NOT RUN until their own actual evidence; RF-08 remains BLOCKED; no release |

Allowed future scope, subject to separate exact approval: existing synthetic test infrastructure, integration-only test and redacted review artifacts; no production code, routes, role changes, live credentials/providers, deployment, migration, registry, release or unmerged #81 import. Exact allowed paths, fixture boundaries, resource/retention limits, independent review conditions and test ownership must be agreed before any coding. The original held recovery target remains untouched and unavailable; no transferable credential or operational recovery procedure. Use only controlled synthetic data. Failure or inability to safely evaluate a family is recorded as NOT RUN/BLOCKED, not converted to PASS. No change to #95 or #81.

### P4 — G3 gate recommendation

Recommend owner review of the policy baseline P1–P4: accept the source-level inventory as a starting point (not parity), retention principles without new durations, staged experiment *proposal only*, and explicit blockers. Require a further bounded scope decision before E1 implementation, and a separate reviewed technical design before E2. No automatic authorization from accepting this document, direction approval or CI. Independent technical acceptance unrecorded, production NOT ACCEPTED, G6 NOT AUTHORIZED. Full RH-03 PARTIAL; no change to canonical contracts/governance, no merge or activation.

## Exact bounded E1 approval package — proposed, not authorized

[Owner policy baseline P1–P4 acceptance](https://github.com/herdifirdausss/mamony-go/pull/102#issuecomment-7152543073) authorizes preparation of this package only. Prior documentation HEAD cf34cee053c132dc03865d7c15ff8a79472f4e7c passed 5/5 workflows. Inspected main f46459615189ea0657c630f589219b082aee9b71. #81 remains unmerged; no dependency import.

### Goal
Evaluate a small, pure test-only model of evidence completeness and review classification for E1. Establish only consistency of proposed policy classifications using invented labels, not actual recovery, credential invalidation, authentication, or release. E1 is deliberately narrower than RF-01–07; a model PASS cannot close any full RF acceptance requirement.

### Scope
One separate Draft experiment from the then-confirmed main after explicit owner acceptance of this exact package. Use in-memory symbolic fixtures and Go standard-library tests only. No actual Identity state, database, credentials, proof material, providers, deployment, network, subprocesses, files containing fixture data, or mutable external state. All classifications remain HOLD with respect to access/release. A complete synthetic review record may be classified as ready for review, never eligible to activate. Do not create production policy interfaces or reusable recovery tooling.

### AC IDs (proposal-local E1 references, not contract amendments)

| ID | Synthetic evidence case | Expected review classification | Required status |
|---|---|---|---|
| E1-01 | All expected categories present, consistent, known, complete symbolic scope | REVIEW_COMPLETE; HOLD | NOT RUN |
| E1-02 | No evidence | INCOMPLETE; HOLD | NOT RUN |
| E1-03 | Required category absent | INCOMPLETE; HOLD | NOT RUN |
| E1-04 | Required category present but outcome unknown | UNKNOWN; HOLD | NOT RUN |
| E1-05 | Contradictory observations | CONTRADICTORY; HOLD | NOT RUN |
| E1-06 | Evidence references an older symbolic revision | STALE; HOLD | NOT RUN |
| E1-07 | Evidence references a different symbolic scope | MISMATCH; HOLD | NOT RUN |
| E1-08 | One reviewed group and one unresolved group | INCOMPLETE; HOLD | NOT RUN |
| E1-09 | Identical duplicate observation | Same review outcome; no additional logical evidence | NOT RUN |
| E1-10 | Conflicting duplicate observation | CONTRADICTORY; HOLD | NOT RUN |
| E1-11 | Unrecognized evidence category | UNKNOWN; HOLD | NOT RUN |
| E1-12 | Equivalent observations reordered and evaluated repeatedly | Same classification and unchanged input; HOLD | NOT RUN |

The classification vocabulary is proposed for a toy model; it is not a registry, state machine, production evidence validator, or authority. Use one isolated defect per negative case; do not claim a precedence policy for simultaneously conflicting failures. Every case independently asserts HOLD and zero release/authorization behavior. Explicit expected outcomes must be authored independently of model output; do not weaken assertions to make tests pass. No actual durable effects are expected or simulated as committed user actions.

### Sources (path and heading)
- docs/proposals/identity-recovery-fence.md: D1–D5, C1–C4, P1–P4, RF-01–08 and this E1 package, reviewed in #102.
- docs/contracts/identity.md: IAM-08/10/11; docs/contracts/platform.md: FND-08/12.
- docs/runbooks/recovery.md: Recover compromised identity or approval state.
- docs/development-workflow.md: Critical lifecycle and bounded solo synthetic mode.
- docs/testing.md: testing strategy; Makefile: check, race, test-repeat.
- .agents/skills/mamony-context-pack/SKILL.md and mamony-pr-evidence/SKILL.md: context pack and exact evidence requirements.
- #14 scope split; #95 historical bounded evidence only; #78/#81 dependency unchanged.

### Invariants
HOLD is not an account status or permission; no decision authorizes release. Unknown scope, missing evidence or contradiction cannot be silently accepted. No actual authority or event mutation, no fabricated consumption, no source/held-target access, no credential material, no broad coverage claim, no changed canonical contract. Pure deterministic evaluation of symbolic cases, no clock/environment dependencies. Do not turn model output into a real evidence validator.

### Allowed packages / exact file allowlist (future experiment, not created here)
- New: internal/modules/identity/recovery_fence_policy_model_test.go (only test-local types, model and table-driven tests, standard library; package identity; no new non-test source).
- New: docs/proposals/identity-recovery-fence-e1-evidence.md (scope, exact-head evidence and limitations only).
Two files maximum. The paths are proposed, absent at inspected base; recheck before creating the branch. No edits to existing source, existing tests, workflows, Makefile, migrations, contracts, governance, dependencies, configuration, #95 or #81. Existing checks run unchanged. No additional artifacts, services, permissions or dependencies. Separate proposal #102 remains docs-only; E1 changes must never be added to it.

### Interfaces/contracts
No application-facing interface, API, schema, roles, config, DB call, production package dependency, release integration or new canonical AC. Test-local inputs are invented category/scope/revision labels and abstract observations only. No credential fields or connection parameters. Results are review classifications, not authorization decisions. No runtime wiring.

### Checks (planned, NOT RUN)
After separate approval, verify exact approved base and allowlist; targeted test: go test -count=1 -run '^TestIdentityRecoveryFencePolicyModel$' ./internal/modules/identity; targeted repeat: go test -shuffle=on -count=3 -run '^TestIdentityRecoveryFencePolicyModel$' ./internal/modules/identity. Existing repository gates: make check, make race, make test-repeat; all five exact-head workflows / seven required job contexts under existing ruleset. Actual case evidence must be inspected; routine integration CI is regression evidence only, not E1 database evidence. No new CI gate, no direct external call or special fixture. Record PASS/FAIL/NOT RUN honestly, including any checks not independently executed. Resource boundary: 12 small in-memory cases, no dedicated infrastructure, no new retained fixture artifacts; stop if evaluation needs any external state or change beyond allowlist. Normal existing CI resource and retention settings unchanged.

### Unresolved decisions / final approval gate
E1 implementation BLOCKED until explicit owner approval of this package at the exact reviewed proposal head and base, including confirmation of bounded synthetic environment and authorized solo experiment scope if independent technical review is deferred. Recheck main and proposal revisions before writing; changed scope/base or new security semantics requires review. A failure, unexpected behavior, missing coverage, dependency, or need to change another file is a stop condition, not permission to expand scope. E2 actual capability behavior requires its own design and approval. RF-01–07 NOT RUN, RF-08 BLOCKED on #81; independent technical acceptance unrecorded for this proposal. No merge, migration, registry, role changes, live credentials, production activation or held-target release authorization.

Suggested owner decision after reviewing exact candidate: authorize ONLY E1 test-local symbolic model and evidence document within the two-file allowlist, on a separate Draft PR; confirm bounded synthetic environment and explicitly defer independent technical acceptance for this experiment under adopted solo policy if applicable. No E2, production, migration, registry, governance, #81 changes, or merge authorization. Without explicit acceptance, no files may be created for the experiment.

## E2 research proposal — G1 only (2026-11-25)

**Status:** research PROPOSED; implementation BLOCKED. [Owner synchronized E1 acceptance](https://github.com/herdifirdausss/mamony-go/pull/106#issuecomment-7152841045) authorizes final E1 review and this research proposal only. [Final E1 review](https://github.com/herdifirdausss/mamony-go/pull/106#issuecomment-7152847805) is COMPLETE for the enumerated symbolic model at exact HEAD 9010c5cf51748d04af2cf7fed468a0904d504111, base f6b9a2d1f46413e7c87db0cc09d13acc4261c438, with five workflows and seven required contexts SUCCESS. E1 remains Draft, no merge authorized. The E1 model is not a dependency or production policy validator. No E2 scope or technical design approved.

### Research question and baseline

What defensive evidence would demonstrate that a recovery policy actually preserves authorized durable bindings and denies outstanding pre-recovery temporary authority, while the original recovery target remains held? Distinguish evidence classification (E1), preserved-state behavior (#95), and recovery-specific enforcement (E2). A successful negative test due only to disconnection, timeout or role unavailability does not prove owning-capability denial. Fresh authorization is a separate conditional control, never a release decision.

| Evidence source checked 2026-11-25 | Supported observation | Limitation |
|---|---|---|
| Main f6b9a2d1f46413e7c87db0cc09d13acc4261c438: IAM-03/08/10/11/12, FND-08/12, recovery runbook and testing strategy | Existing contracts demand session lifecycle, restricted recovery, trusted current bindings, durable obligations and actual authorization evidence | No accepted recovery-specific fence implementation established |
| [#95 bounded lab](https://github.com/herdifirdausss/mamony-go/pull/95) | Selected recovered state and application semantics already investigated | Not global pre-recovery invalidation, external completeness or release |
| [#106 final E1 review](https://github.com/herdifirdausss/mamony-go/pull/106#issuecomment-7152847805) | Twelve fixed symbolic cases; HOLD by construction | No actual authority, durable mutation, database concurrency or evidence authenticity claim |
| [NIST SP 800-63B-4](https://pages.nist.gov/800-63-4/sp800-63b.html), sections 4 and 5 | Authenticator lifecycle, account recovery and session management have distinct requirements | Does not specify Mamony database restoration/fencing or establish compliance |
| [PostgreSQL 18 PITR](https://www.postgresql.org/docs/18/continuous-archiving.html) and [transaction isolation](https://www.postgresql.org/docs/18/transaction-iso.html) | Recovery point and local transaction consistency are database properties | Neither establishes authorized currentness or independent release eligibility |
| #81, observed unmerged head 98adecb0517a7e361d4ac1d3dd2e26c3703205aa | Related v36 possession dependency | Not adopted; do not import code or assume its status on main |

### Alternatives and recommended research direction

| Alternative | Evidence value | Limitation / disposition |
|---|---|---|
| Continue symbolic E1 extensions | Classification only | Diminishing returns; do not count as E2 |
| Reuse existing source and #95 evidence | Regression and state-preservation baseline | Insufficient to show recovery-specific behavior |
| Review a single Identity capability first, under the existing whole-Identity HOLD policy | Bounded semantic and evidence feasibility review | Only recommended as research; actual behavioral experiment requires a separate accepted design and containment approval |
| All-family fence or new recovery infrastructure immediately | Broader eventual coverage | Unresolved cross-family, retention, consistency and custody decisions; reject as current scope |

Recommend researching a single session-lifecycle capability first, with IAM-08 as trace, to establish whether a meaningful bounded defensive evaluation is feasible under existing approved behavior. Do not infer that existing local epoch behavior alone satisfies recovery-wide invalidation. No executable protocol, credential replay workflow, database operations, recovery command, new interfaces, implementation paths, or experiment allowlist is proposed at this research stage.

### Candidate evaluation questions (not approved acceptance criteria)

| Research dimension | Question / evidence needed | Status |
|---|---|---|
| Authority semantics | What distinguishes policy denial, infrastructure failure and unknown result under the owning capability? | OPEN |
| Durable state | Which authorized bindings and original logical obligations must remain unchanged? | OPEN |
| Fresh control | What independent prerequisites are required for a separate synthetic positive control, without inherited authority or target release? | OPEN |
| Containment | Can evaluation be isolated, disposable, non-transferable and independent of live credentials/providers/routes and original held target? | OPEN |
| Coverage | Which family-specific dependencies and operational/retention limits prevent a single-capability result from being generalized? | OPEN |
| Failure | How will unexpected/partial outcomes and uncertain evidence be recorded without automatic retry, fabricated success or weakened assertions? | OPEN |

### Next gates and stop conditions

E2 G1 research review precedes G2 scope acceptance, G3 technical/threat review, exact bounded experiment and explicit owner authorization. No implementation or execution now. If feasibility requires changed runtime semantics, new migration/roles, registry, live access, release, transferable artifacts or unmerged v36 dependency, stop and request separate governance review; do not use a synthetic claim to waive the issue. Independent technical acceptance for E2 is not recorded and the E1 deferral does not transfer. Existing P1–P4 and C1–C4 requirements remain; all RF-01–07 NOT RUN, RF-08 BLOCKED, full RH-03 PARTIAL. RH-04/05 registry and release remain separate. No G6 authorization, production acceptance or activation. This is a documentation-only continuation of #102; no changes to #95/#81/#106 code or canonical contracts.
