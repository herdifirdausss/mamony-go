# Feature proposal template

**Use only for Critical changes** under the [change lifecycle](../development-workflow.md#choose-a-change-path). Make a working copy at `docs/proposals/<short-topic>.md` if the design needs an enduring review record. Remove prompts that are genuinely inapplicable; explain exclusions that affect money, access, privacy or recovery. Do not fill unknowns with AI assumptions.

- **Title / owning capability:**
- **Issue / decision owner / technical reviewer:**
- **Status:** PROPOSED | ACCEPTED | SUPERSEDED
- **Risk path and rationale:** Critical
- **Affected contracts and requirement IDs:**
- **Last evidence review date:**
- **Related ADR or open decision:**

## G1 — Research and problem validation

**Current behavior and problem:** Who encounters it, how is the behavior observed, and what happens if nothing changes?

**Evidence ledger:**

| Claim or option | Source / exact section | Version / checked date | Observation | Confidence and limitation |
|---|---|---|---|---|
| Pending | Pending | Pending | Unverified | UNKNOWN |

Record alternatives, relevant counter-evidence, assumptions and unresolved questions. Include current repository behavior and a minimal reproduction or failing scenario when applicable. Do not claim another codebase's implementation is proof for this repository.

## G2 — PRD / accepted requirements

**Actors and intended outcome:** What will users or system operators be able to do, and what measurable problem does this solve?

**Scope and non-goals:** Include the initial pilot and explicit disabled features.

**Functional and non-functional requirements:** Include access restrictions, privacy, performance, availability, auditability, compatibility and operational constraints where relevant.

**Acceptance criteria and traceability:**

| AC ID in owning contract | Given / When / Then or precise invariant | Test or measurable evidence | Decision status |
|---|---|---|---|
| Pending | Pending | NOT RUN | PROPOSED |

For accepted changes, update the **owning capability contract** with the final AC and stable ID. This proposal explains decisions; it is not a parallel source of truth. Record the product owner's explicit acceptance reference (issue comment or PR review).

## G3 — Technical design and security review

**Alternatives considered:** Include a smaller design, what it fails to meet, operational cost and decision.

**Ownership and trust boundaries:** Show which module owns data and transactions; who is authorized to mutate them; attacker capabilities and trusted roots.

**Flow and contracts:** Add a sequence/DFD only when helpful. Specify interfaces, schema/migrations, public APIs, events, calls to providers, authentication and authorization points.

**Consistency and failure model:** Include races, replay/idempotency, duplicate or out-of-order messages, timeouts, ambiguous commits, rollback, restore and privilege boundaries as applicable.

**Security and privacy:** Threat scenarios, control enforcement locations, known bypass paths, sensitive data, retention and residual risks. State what is not protected under the assumed attacker scope.

**Compatibility, observability and rollout:** Migration/rollback, release ordering, alerts, feature flags, safe disable behavior and repair path.

**Decision status:** Proposed | Owner-approved draft scope / independent review pending | Accepted | Blocked. Record owner approval and independent technical acceptance references separately and create/update an ADR only for a lasting architecture decision.

## G4 — Implementation slices

| PR / slice | Owner module | AC IDs | Tests and negative cases | Dependencies | Status |
|---|---|---|---|---|---|
| Pending | Pending | Pending | NOT RUN | Pending | PLANNED |

Avoid mixing broad refactoring, speculative infrastructure and new product behavior in one PR. Block coding on unresolved money/security/irreversible-state decisions affecting the slice. An owner-approved, documented synthetic slice may be implemented in a Draft PR under the workflow exception; independent technical acceptance is required before merge and separate approval before production activation.

For AC-to-test mapping, use the [test-plan checklist](test-plan.md) inside this proposal or the PR. A default `go test` PASS is not evidence for real PostgreSQL integration tests.

## G5–G7 — Verification and outcome

| Requirement / threat | Test / command | Commit and evidence | Result |
|---|---|---|---|
| Pending | Pending | Pending | NOT RUN |

Include independent review record for Critical work, unresolved risks, compatibility and rollout observations. Distinguish local synthetic tests from live deployment evidence. Update this record when the implementation diverges from the accepted design, and link the authoritative contract and ADR revisions.
