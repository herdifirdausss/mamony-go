## Outcome

- Issue and requirement IDs
- Changed behavior and owning capability
- Applicable decision, trade-offs, or open choices

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
