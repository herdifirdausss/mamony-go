## Outcome

- What changed and why
- Change path (Routine / Feature / Critical) and owning capability
- Issue, affected acceptance criteria, and relevant design links, as applicable

Link new milestone or experiment IDs to their definitions. If this PR changes a milestone's status, list the affected status pages.

## Evidence

| AC or check | Command, scenario, or evidence link | Result |
|---|---|---|
| Repository checks | `make check` | NOT RUN |
| Affected ACs and failure cases | Targeted tests | NOT RUN |
| Applicable CI | Run for this commit | NOT RUN |

Identify the commit and fixture for the evidence. Use `PASS`, `FAIL`, `NOT RUN`, or justified `N/A-disabled` under the [testing strategy](../docs/testing.md#test-evidence-for-ai-changes). Map each affected accepted AC to a test or a recorded `NOT RUN`. Include applicable denial, replay, concurrency, migration, crash, and recovery cases. For database behavior, link real PostgreSQL checks with the relevant roles and durable effects.

Record review of changed tests and any removed assertions, fixtures, or checks. Explain skipped cases. Preserve required checks and their assertions. Use the [test-plan checklist](../docs/templates/test-plan.md) to review coverage.

## Security, compatibility, and rollout (when applicable)

- Trust boundary, enforcement owner, caller and attacker capability, and authority that remains trusted
- API, event, and schema compatibility, including migration, rollback, and recovery
- Residual risks, blockers, and the applicable negative test evidence

See the [security verification matrix](../docs/engineering.md#security-verification). Use `NOT RUN` for a specified control that has no implemented check. Use `N/A-disabled` only with evidence that the affected capability and routes are absent or disabled.

## Critical review and authorization (when applicable)

Follow the [change lifecycle](../docs/development-workflow.md) for the applicable gate.

- Link pre-implementation product and technical design acceptance, accepted ACs, and any explicitly accepted scope departure. For a bounded synthetic Draft, link the owner authorization, allowed scope, and containment evidence.
- Link review of the exact candidate code and tests, independent technical acceptance status, and unresolved risk or release blockers.
- For a proposed solo synthetic merge, record separate owner authorization with the full head and reviewed target/base SHAs, accepted scope, and evidence link. Recheck the candidate and target before merge. The owner merges under the [solo synthetic rules](../docs/development-workflow.md#g6--synthetic-only-merge). Record independent technical acceptance as DEFERRED and production readiness as NOT ACCEPTED.
- For a production-capable Critical release, record independent human technical acceptance and the separate activation decision. If the expedited security hotfix path applies, record skipped steps and the plan to complete the evidence review.
