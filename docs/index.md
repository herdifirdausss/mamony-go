# Documentation

Start with [current state and active work](current-state.md) to see what runs, what has merged, and which gates remain open. Use GitHub issues and PRs for live status, approvals, and execution evidence.

| What you need | Where to start |
|---|---|
| Triage an idea or bug, or plan a new feature | [Choose a change path](development-workflow.md#choose-a-change-path). For Critical work, use the [proposal template](templates/feature-proposal.md) for research, PRD, technical design, and the implementation plan. |
| Understand the product and capability ownership | [Product scope](product.md) |
| Find the delivery order or a requirement and its acceptance checks | [Roadmap](roadmap.md), then the owning capability contract linked there |
| Look up a Ledger stage or its prerequisites | [Ledger implementation sequence](roadmap.md#ledger-implementation-sequence) |
| Change Go packages, listeners, or cross-module interfaces | [Architecture](architecture.md) and [architecture decisions](adr/README.md) |
| Check an active decision or prerequisite before implementing it | [Decision lookup by capability](open-decisions.md#decision-lookup-by-capability). Follow the linked section or ADR for status and scope. |
| Follow Go conventions and capability package patterns | [Go conventions](conventions.md) |
| Plan unit, integration, concurrency and recovery tests | [Testing strategy](testing.md) and [test-plan checklist](templates/test-plan.md) |
| Run checks and record PR evidence or approvals | [Engineering practices](engineering.md#verification) and the [PR template](../.github/pull_request_template.md) |
| Review security authority, threat scope and tests | [Security boundaries](architecture.md#security-authority-and-trust-boundaries), [security verification](engineering.md#security-verification) and [open security decisions](open-decisions.md#security-architecture-decisions) |

Use the [PostgreSQL contract](contracts/postgresql.md) for schema grants and tenant isolation. Use the [public HTTP decision](adr/0003-public-http-contract.md) for routes and amount formats. The [recovery runbook](runbooks/recovery.md) covers restore and stalled workflows.

For an AI handoff, start with the issue and use the [context pack](../.agents/skills/mamony-context-pack/SKILL.md) to link the base commit, accepted requirements, applicable decisions, checks, and blockers.

For tool changes, see [Tooling](tooling.md). [Development history](history.md) records the earlier research and verification.
