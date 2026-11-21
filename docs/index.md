# Documentation

| What you need | Where to start |
|---|---|
| Understand the product and capability ownership | [Product scope](product.md) |
| Pick work or find its requirements and acceptance checks | [Roadmap and contracts](roadmap.md) |
| Change Go packages, listeners, or cross-module interfaces | [Architecture](architecture.md) and [architecture decisions](adr/README.md) |
| Check a design choice before implementing it | [Open decisions](open-decisions.md) |
| Run checks and prepare review evidence | [Engineering practices](engineering.md) |

Use the [PostgreSQL contract](contracts/postgresql.md) for schema grants and tenant isolation. Use the [public HTTP decision](adr/0003-public-http-contract.md) for routes and amount formats. The [recovery runbook](runbooks/recovery.md) covers restore and stalled workflows.

For tool changes, see [Tooling](tooling.md). The [agent evaluation plan](agent-evaluation.md) is for workflow experiments. [Development history](history.md) records the earlier research and verification.
