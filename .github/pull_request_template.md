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

A PR link or checkbox is **not** evidence of product approval, independent review or successful tests. For Critical changes, record explicit owner and technical acceptance **before coding**; if a security hotfix used the expedited path, explain why and how the skipped evidence will be completed. See [change lifecycle](../docs/development-workflow.md).

## Evidence

| Check | Command or test | Result |
|---|---|---|
| Repository checks | `make check` | NOT RUN |
| Applicable contract and failure cases | Targeted tests | NOT RUN |
| Relevant CI jobs | Link the run for this commit | NOT RUN |

Use `PASS`, `FAIL`, or `NOT RUN` for each check. See [engineering practices](../docs/engineering.md#verification) for commands. Record PostgreSQL contention, migration, or recovery evidence for affected changes.

## Security impact

- Trust boundary, caller capability and owning resource:
- Threat/attacker foothold; which authority remains trusted:
- Money, identity, secrets, tenant, provider, audit or recovery impact:
- Negative/replay/concurrency tests that exercise the boundary:
- Residual risk and unresolved decisions (or state `not applicable`):

Use `N/A-disabled` only when the affected feature and routes are genuinely absent or disabled. Record `NOT RUN` for any security control that is specified but not implemented. See [security verification](../docs/engineering.md#security-verification).

## Compatibility and rollout

State API, event, or schema compatibility. Describe migration, recovery, and remaining risks where relevant.
