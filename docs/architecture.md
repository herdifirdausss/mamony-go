# Architecture — proposed Go implementation

The original Mamony [ADR-0004](https://github.com/herdifirdausss/mamony/blob/main/docs/adr/0004-modular-monolith-first.md) and [ADR-0027](https://github.com/herdifirdausss/mamony/blob/main/docs/adr/0027-first-runtime-and-http-layer.md) establish strict logical capabilities, adapter composition and one initial runtime. This Go translation is **Proposed**, not a modification of the source ADRs.

- Single Go module and single binary, with separate public/private HTTP listeners.
- `internal/modules/<capability>` owns facade; `internal/modules/<capability>/internal` hides its domain/usecase/repository/handler implementations.
- `internal/app` wires capabilities via adapters; modules never import other modules.
- `internal/platform` contains domain-neutral primitives.
- One PostgreSQL database, one schema per capability, with scoped DB privileges: accepted original [ADR-0010](https://github.com/herdifirdausss/mamony/blob/main/docs/adr/0010-one-database-schema-per-capability.md).
- Tenant isolation via transaction-local context and runtime roles: original [ADR-0021](https://github.com/herdifirdausss/mamony/blob/main/docs/adr/0021-row-level-security-for-tenant-isolation.md). No SQL/RLS implementation yet.

The import checker is a structural guard, not a proof of security or accounting correctness. PostgreSQL integration, schema grants, cross-tenant and concurrent replay tests are prerequisites for Gate A.
