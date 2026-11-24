## Outcome

- Issue and requirement IDs
- Changed behavior and owning capability
- Applicable decision, trade-offs, or open choices

## Change lifecycle and traceability

- Path: Routine / Feature / Critical (explain uncertainty or escalation)
- Intake issue and approved product decision (for Critical; otherwise issue/contract):
- Research/PRD/technical design proposal (when required) and any accepted ADR:
- Requirement IDs from the owning contract and relevant implementation changes:
- AC-to-test mapping; identify negative, concurrency, migration and recovery cases:
- Any departure from accepted scope and its explicit acceptance:
- If solo synthetic mode applies: implementation approval, containment evidence, independent acceptance status, production blockers, separate exact-head owner merge authorization (otherwise not applicable):

A PR link or checkbox is **not** evidence of product approval, independent review or successful tests. For Critical changes, record explicit owner and technical acceptance **before coding**, or exact owner approval and containment/ACs for a bounded synthetic experiment under the adopted lifecycle. For any proposed solo synthetic merge, record containment evidence, actual review of code and tests, exact SHA owner merge authorization, deferred independent acceptance, and outstanding production blockers. The owner performs the merge. Otherwise pending independent technical acceptance is a Critical merge blocker; production-capable Critical release requires independent acceptance and separate activation approval. If a security hotfix used the expedited path, explain why and how the skipped evidence will be completed. See [change lifecycle](../docs/development-workflow.md).

## Evidence

| Check | Command or test | Result |
|---|---|---|
| Repository checks | `make check` | NOT RUN |
| Applicable contract and failure cases | Targeted tests | NOT RUN |
| Relevant CI jobs | Link the run for this commit | NOT RUN |

Use `PASS`, `FAIL`, or `NOT RUN` for each check. See [engineering practices](../docs/engineering.md#verification) for commands. Record PostgreSQL contention, migration, or recovery evidence for affected changes.

## Test integrity and evidence

- [ ] Every affected accepted AC maps to an executed test or an explicitly documented NOT RUN.
- [ ] Positive/negative and failure cases cover the risk; changed tests and assertions were reviewed for weakened behavior.
- [ ] Database changes use real PostgreSQL, runtime/migration roles, concurrency/rollback and migration tests, or are blocked as NOT RUN.
- [ ] Money/access/recovery changes address replay, unauthorized action, unknown outcome and crash/restart where applicable.
- [ ] No skipped/disabled test or CI gate was introduced to manufacture a green result.

See [testing strategy](../docs/testing.md) and the [test-plan checklist](../docs/templates/test-plan.md). A `make unit` PASS is not a PostgreSQL integration PASS.

## Security impact

- Trust boundary, caller capability and owning resource:
- Threat/attacker foothold; which authority remains trusted:
- Money, identity, secrets, tenant, provider, audit or recovery impact:
- Negative/replay/concurrency tests that exercise the boundary:
- Residual risk and unresolved decisions (or state `not applicable`):

Use `N/A-disabled` only when the affected feature and routes are genuinely absent or disabled. Record `NOT RUN` for any security control that is specified but not implemented. See [security verification](../docs/engineering.md#security-verification).

## Compatibility and rollout

State API, event, or schema compatibility. Describe migration, recovery, and remaining risks where relevant.
