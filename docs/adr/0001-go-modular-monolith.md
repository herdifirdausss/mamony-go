# ADR-0001 — Modular monolith and service boundaries

**Status:** Proposed

**Date:** 2026-11-21

## Context

Mamony Go needs clear data ownership and predictable imports. The default HTTP application can host capabilities in one process. Workers, maintenance jobs, and migration commands already have separate entry points. A capability may later need its own deployment or database.

## Decision

Keep one repository and one root `go.mod`. Build the default HTTP application from `cmd/mamony`. Use separate `cmd/<program>` entry points for workers, maintenance, and migrations that operators invoke independently.

Place each capability's facade at `internal/modules/<capability>` and keep its implementation under its nested `internal/` directory. Each use case owns the interfaces it consumes. Wire cross-capability calls in `internal/app` through the owning capability's facade. Keep business rules in the capability that owns them.

Use separate public and private HTTP listeners. The capability owns its handler and route metadata. The application registers each capability route on its assigned listener. Both listeners can expose liveness.

Follow [ADR-0002](0002-ledger-and-data-ownership.md) for local transactions and the initial PostgreSQL topology. Keep one schema per capability, scoped runtime and migration roles, and separate migration histories. A capability owns the records it must update atomically.

## When to separate a boundary

| Boundary | Evidence for a separate decision |
|---|---|
| Process and deployment | A measured workload needs independent scaling or release cadence, or a specific failure, network, or credential isolation requirement calls for a separate process. Define the expected improvement and how to verify it. |
| Network contract | Separate processes need a call or event contract. Define service authentication, caller authorization, version compatibility, deadlines, retry semantics, and how callers resolve uncertain outcomes. |
| Database | A capability needs a stronger connection, resource, or recovery boundary. Separate databases in one PostgreSQL cluster still share cluster resources, cluster-level administration, and physical point-in-time recovery. Use separate clusters when independent physical recovery or infrastructure is required, and verify the isolation the design claims. |
| Go module | An independently managed dependency graph, a different toolchain requirement, or a versioned Go library justifies another `go.mod`. Multiple deployables can use the root module. If standalone module builds are required, verify them with `GOWORK=off`. |

Propose a separate ADR for an extraction. Describe the baseline, the expected benefit, and the effect on package dependencies, data ownership, authorization, and recovery. The first independently deployed service can stay in this repository and Go module. Give it dedicated wiring, runtime credentials, telemetry, rollout, and tests for the failure modes it introduces. The [change lifecycle](../development-workflow.md#choose-a-change-path) applies to a change in trust boundaries or an irreversible migration.

For a database split, inventory the owner's data, grants, migrations, and recovery dependencies. Include replay and outbox records when they exist. Rehearse snapshot or backfill, any required catch-up, and parity checks on disposable data. Prevent writes at the source before opening the target for writes. Plan recovery for both sides of the cutover. After the target accepts writes, reconcile or transfer those effects before restoring write authority to the source.

## Consequences

The Go import rules and architecture checks give early feedback on package boundaries. A process split adds deployment, communication, and recovery work. A database split introduces a separate cutover and restore plan.

See [package and route rules](../architecture.md), [Go conventions](../conventions.md), and the [PostgreSQL contract](../contracts/postgresql.md).

## Research sources

- Go documents multiple commands in one module, private implementation packages, and the workspace model for separate modules. [Go module layout](https://go.dev/doc/modules/layout) · [Go internal packages](https://go.dev/doc/go1.4#internalpackages) · [Go workspace reference](https://go.dev/ref/mod#workspaces)
- Shopify describes component entry points, dependency checks, and selective extraction. Uber describes domain interfaces and dependency rules in a distributed system. [Shopify Engineering](https://shopify.engineering/shopify-monolith) · [Uber Engineering](https://www.uber.com/us/en/blog/microservice-architecture/)
- Grafana Mimir uses one Go codebase for monolithic and per-component deployments. Martin Fowler describes the cost of starting with distributed services and the value of discovering boundaries first. [Grafana Mimir deployment modes](https://grafana.com/docs/mimir/latest/references/architecture/deployment-modes/) · [Monolith First](https://martinfowler.com/bliki/MonolithFirst.html)
- PostgreSQL documents schema access and cluster-wide physical recovery. Sam Newman's book covers database decomposition, data synchronization, and transactional integrity. [PostgreSQL schemas](https://www.postgresql.org/docs/18/ddl-schemas.html) · [PostgreSQL PITR](https://www.postgresql.org/docs/18/continuous-archiving.html) · [Monolith to Microservices, Chapter 4 preview](https://www.oreilly.com/library/view/monolith-to-microservices/9781492047834/ch04.html)
- AWS describes incremental extraction, data ownership per service, and transactional outbox delivery. [Strangler fig](https://docs.aws.amazon.com/prescriptive-guidance/latest/modernization-decomposing-monoliths/strangler-fig.html) · [Database per service](https://docs.aws.amazon.com/prescriptive-guidance/latest/modernization-data-persistence/database-per-service.html) · [Transactional outbox](https://docs.aws.amazon.com/prescriptive-guidance/latest/cloud-design-patterns/transactional-outbox.html)
