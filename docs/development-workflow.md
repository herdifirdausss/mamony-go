# Change lifecycle

Use this workflow for every proposed change, including changes initiated by an AI agent. The [product scope](product.md), [capability contracts](roadmap.md), [architecture decisions](adr/README.md), and [open decisions](open-decisions.md) remain the sources of truth. This workflow governs how to create, validate, and change them; it does not replace them.

The goal is **traceable decisions and testable requirements**, not a fixed number of documents. Size the effort to the expected harm and uncertainty. No document, agent review, or successful scanner proves a financial or security control works.

## Choose a change path

| Path | Typical change | Required preparation | Design record and verification |
|---|---|---|---|
| Routine | Typo, isolated refactor, well-understood bug without new behavior or trust boundary | Issue or clear PR description; reproduction and preserved behavior for a bug | Targeted regression plus applicable repository checks. No standalone PRD or proposal. |
| Feature | New behavior inside an established capability; additive API or job with known security boundary | Issue, evidence where uncertain, product outcome, explicit scope and acceptance criteria added to the owning contract | Short technical plan in issue or PR; affected diagrams/ADR only if the boundary changes; targeted positive and negative tests. |
| Critical | New money/identity/authorization semantics, trust boundary, public compatibility, irreversible migration, sensitive recovery, new external provider, or a high-uncertainty cross-capability feature | Issue, research evidence, accepted PRD and technical design using the [proposal template](templates/feature-proposal.md), and accepted contract requirements **before implementation** (see the bounded solo synthetic mode below for the timing of independent technical acceptance) | Threat/failure review, explicit approval of unresolved decisions, tests for negative/race/crash/restore cases, review of migration and rollback. New ADR for consequential long-lived architecture decisions. |

When risk is **unknown**, start with Critical until discovery establishes a smaller scope. A one-line change can be Critical if it changes authorization or monetary behavior. A large documentation-only update is not automatically Critical. A security hotfix may proceed through an expedited issue/PR, but must document the skipped pre-implementation steps and complete the evidence review afterward.

For Critical work, store one proposal under `docs/proposals/<short-topic>.md` only when needed. The proposal records exploration, an accepted product decision and implementation reasoning. It is **not** a second set of active product contracts: promote accepted, stable requirements and acceptance checks into the owning `docs/contracts/<capability>.md`. Use an ADR only for enduring decisions that cross capabilities or change system guarantees. Do not create stage-numbered research dumps or duplicated PRDs for routine work.

## Lifecycle and quality gates

| Gate | Activity | Evidence required to continue |
|---|---|---|
| G0 — Intake | Record actor, problem, desired outcome, affected capability, risks, unknowns and path. Start with the [feature issue form](../.github/ISSUE_TEMPLATE/feature.yml), or a concise issue for a bug. | Problem and outcome are distinct from a proposed solution. Scope, owner and path are visible. |
| G1 — Research | Inspect current code, contracts, failure history and alternatives. Verify applicable claims against primary sources; record source URL/version/date, observations, uncertainties and why each candidate is or is not suitable. | Evidence separates **fact**, **proposal**, and **unknown**. Do not turn an unverified benchmark or another company's implementation into a requirement. |
| G2 — Requirements / PRD | Define users/actors, jobs, success and failure outcomes, non-goals, limits, compatibility, operational and security requirements; assign stable AC IDs in the owning contract. | Product decision is accepted by repository owner or authorized decision maker; every in-scope behavior has a falsifiable acceptance criterion. For Feature changes, a concise issue/contract update is enough. |
| G3 — Technical design and threat review | Explore options and smallest viable solution. Record boundary/authority, modules, sequence/data flow, API/schema/events, consistency, idempotency, timeouts, migrations, recovery, abuse cases and trade-offs according to risk. | Design satisfies accepted ACs, addresses significant threats, records residual risk, and resolves release-blocking decisions. For Critical changes, obtain explicit design acceptance **before** code, except for the explicitly owner-authorized bounded synthetic experiment defined below. |
| G4 — Implementation plan | Slice work into reviewable PRs, map each AC to owner, code location and tests, list dependencies and release order. | No implementation slice silently changes accepted scope; unverified assumptions have owners or blocking status. |
| G5 — Implement and verify | Follow the [Go conventions](conventions.md) and [testing strategy](testing.md); write tests and implementation, run applicable checks, adversarial cases and actual infrastructure integration where needed. Update affected contract, diagram and runbook in the same PR. | AC-to-test evidence, exact commands/commit, `PASS`/`FAIL`/`NOT RUN` or justified `N/A-disabled` recorded. No acceptance test is replaced by a scanner. |
| G6 — Review and merge | Review changes against the accepted design, diff and threat model; assess deployment/rollback, data compatibility and residual risks. | Required CI and repository rules pass; ordinary Critical work requires independent human technical acceptance of the final design and exact candidate. A separately authorized solo synthetic merge may defer independent acceptance only under the conditions below. No unresolved blocker for the intended scope remains. Merge changes only through the approved repository policy. |
| G7 — Operate and learn | Verify actual post-merge state. Before any production-capable Critical release or activation, obtain independent human technical acceptance and separate explicit production activation approval; apply rollout/monitoring, test recovery where relevant, and capture newly discovered faults. | Publish outcome and evidence; update contracts/ADR or create follow-up issues when reality differs. Never claim production safety for synthetic-only checks. |

These are **human/agent review gates**, not automatically enforced by `make check`. GitHub configuration must separately require PRs, review, and status checks. The current repository's effective branch-protection rules must be checked by an administrator; a CI workflow existing in the tree does not itself make it merge-blocking.

## Bounded solo synthetic development mode

This is a limited development policy for the current solo-maintainer synthetic simulator, not a production security model. Ordinary Critical G3/G6 acceptance remains the default outside this mode. The historical [owner decision on Issue #78](https://github.com/herdifirdausss/mamony-go/issues/78#issuecomment-7130263898) concerns bounded Draft experiments, not blanket merge approval. The mode takes effect only after the repository owner explicitly adopts this policy through a separately reviewed PR under the policy in force at that time.

### G3–G5 — Synthetic implementation

- Complete G0–G3 preparation and G4 plan. Record issue, accepted requirements and exact allowed scope, threat model, synthetic containment, negative tests, unresolved decisions and owner approval reference. Owner acceptance of the bounded experiment permits Draft implementation before independent review; unresolved semantics affecting the experiment remain blocking.
- Keep implementation and execution in Draft using disposable infrastructure and synthetic identities, funds, providers and credentials. No production data, secrets, access changes, live migrations, live routes or release. No test weakening or security-policy changes under this implementation exception.
- Record exact candidate and actual verification evidence. A green test, owner acceptance, or AI review is not independent technical acceptance.

### G6 — Synthetic-only merge

A synthetic merge is a distinct owner decision, not implied by approval to implement. It may defer independent human review only when **all** of the following hold:

- The repository is in explicitly adopted solo synthetic mode; no production deployment or external real-user/fund access is enabled by this merge. Establish this with evidence for the actual candidate and target environment: inspect runtime composition/entrypoints, route and worker activation, deployment/CI triggers, credentials and access boundaries as applicable. A disabled flag or a claim in the PR alone is not proof. Test negative activation/authorization paths when relevant. Unknown containment blocks merge.
- Scope/ACs and technical design are accepted by the owner; actual candidate code **and tests** have a recorded review with limitations; all applicable checks pass on the exact candidate and protected target requirements are met. No unresolved blocker affecting the synthetic scope, omitted mandatory tests, unexplained assertion removal, or security-policy/CI weakening. Production-only gaps are separately recorded as release blockers, not marked PASS.
- The owner explicitly authorizes the exact head SHA for synthetic-only merge after reviewing the evidence and residual risks. Material changes or a changed target invalidate that decision until rechecked. The owner performs the merge through the protected PR; no unattended agent merge or automatic transition from Draft. Mark independent technical acceptance DEFERRED (not APPROVED) and production readiness NOT ACCEPTED.

Test-only fixtures and unmounted synthetic runtime code are distinct cases; runtime code requires positive evidence it cannot activate real access or money movement in the current environment. A synthetic label does not waive any accepted contract requirement. This mode is not retroactive approval of existing PRs.

### Governance changes

This exception does not authorize changes to its own rules, security policy, branch protection, CI gates, test integrity, or access permissions. Such changes require a dedicated PR, comparison against the policy currently in force, explicit human owner review and adoption of the exact candidate, and any independent review required by that existing policy. Do not use a proposed rule to approve itself. No automatic policy adoption, review impersonation or bypass of protected branch rules.

### G7 — Production acceptance and exit conditions

Independent human technical review of the exact production-capable Critical candidate, closure of applicable security/recovery blockers, and separate owner activation approval are mandatory before any production use. Synthetic merge is not release readiness.

Suspend the solo exception and reassess governance before introducing real users, funds, production credentials or data, live providers, automated deployment/activation or unattended agent merging, material changes to trust boundaries, or when an independent maintainer becomes available. Review previously deferred Critical candidates and enable appropriate independent review enforcement before production-capable release. If the environment or reviewer status is uncertain, use the ordinary gate. Record the owner decision and evidence in [Issue #13](https://github.com/herdifirdausss/mamony-go/issues/13). GitHub zero required approvals and passing CI are not independent review or evidence of enforcement of this policy.

## Ownership and AI behavior

- The requester/repository owner accepts product scope and risk trade-offs. The designated independent technical reviewer accepts high-impact architecture decisions outside the bounded solo synthetic mode and before production readiness. The owner accepts the bounded synthetic experiment and separately decides any eligible synthetic merge. Agents may research, draft, implement and review, but an agent's second-pass review is **not independent human approval**.
- Prefer verified project sources over assumptions. If a critical product, money, security or migration decision remains unresolved, stop that slice at its gate; do not invent a default. Record it in [open decisions](open-decisions.md) with the evidence required.
- When research conflicts with existing contracts, explicitly compare both and propose an edit. **Never silently override** an accepted contract. Mark superseded decisions and preserve rationale; do not keep two active conflicting specifications.
- For every feature, give the implementation agent a small context pack: objective, relevant accepted AC IDs, code paths, interfaces, invariants, tests and blockers. The [context pack skill](../.agents/skills/mamony-context-pack/SKILL.md) and [PR evidence skill](../.agents/skills/mamony-pr-evidence/SKILL.md) provide the task-specific checks.
- Use **planned**, **proposed**, **accepted**, **implemented** and **verified** precisely. A written requirement is not implemented; passing CI is not proof of all security guarantees.

## Research and design evidence

Use official specifications, source code, maintainer documentation and authoritative primary reports first. Record a check date for changing facts such as dependency versions, algorithms, provider APIs and vulnerabilities. Include evidence **for alternatives**, not only the preferred option. When sources disagree, state the conflict and confidence level. External sources can inform decisions but must not become mandatory links for understanding the internal contract: state the locally accepted conclusion and its checks.

A PRD answers **why and what**; a technical design answers **how and why this design**; an ADR records **one lasting architectural choice**. Keep those roles separate even when the sections live in one proposal file. The [proposal template](templates/feature-proposal.md) captures them in one reviewable artifact when the Critical path applies. Use the [test-plan checklist](templates/test-plan.md) when translating ACs into runnable tests.

## Traceability example

A hypothetical transfer-approval change could be tracked as:

- Issue: user needs approval tied to the actual beneficiary and amount, not only a login.
- PRD: approval must fail after recipient, amount or currency changes; a consumed approval cannot execute twice.
- Contract: `PAY-05` and `IAM-12` already define relevant checks; extend them only if the product outcome requires it.
- Technical design: specify the owner, immutable intent, approved fields, consumption consistency boundary and replay behavior.
- Implementation PR: map each acceptance case to a test and actual commit. Concurrent consumption and crash/retry require real database evidence when that path exists.
- Verification: `PASS` only for executed tests; `NOT RUN` for absent runtime paths. Security claims inherit the declared [authority limitations](architecture.md#security-authority-and-trust-boundaries).

See [engineering practices](engineering.md#design-and-review) for Go implementation and [security verification](engineering.md#security-verification) for high-risk test cases.
