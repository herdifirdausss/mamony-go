# PostgreSQL contract

**Effective design:** [DEC-01](../open-decisions.md#foundation-and-access) uses `golang-migrate/v4` with the pgx v5 driver and committed migration fingerprints. The default HTTP process serves health endpoints. Its readiness check covers the HTTP process. See the [migration runbook](../runbooks/platform-migrations.md) for the separate administrative command.

## Implementation and evidence scope

| Scope | Merged disposable evidence | Remaining acceptance |
|---|---|---|
| Migration runner | [PR #17](https://github.com/herdifirdausss/mamony-go/pull/17) covers platform apply, upgrade within the fixture, runtime-role denial, fingerprint checks, and dirty-state refusal. | Deployment grants, secrets, and supported upgrade and recovery evidence for the deployed target. |
| Runtime pool and tenant isolation | The bounded [runtime pool](../../internal/platform/postgres/pool.go) and [PostgreSQL fixtures](../../tools/postgrestest/postgres_integration_test.go) exercise actual role and RLS checks, pooled connection reuse, rollback, and cancellation. | Tenant and resource authorization through the owning capability, deployment grant parity, and database-aware readiness. |
| Platform and Identity restore | [RH-01 and RH-02](../proposals/platform-native-pitr-lab.md) cover the bounded physical restore lab. [RH-03](../current-state.md#evidence-at-the-inspected-main-commit) covers selected Identity state reconciliation. | RH-03 remains PARTIAL. Integrated RF-01–08 evidence is NOT RUN, and E2 remains BLOCKED. Full authority coverage and production recovery objectives remain open. |
| Ledger storage and restore | [K2a](../proposals/ledger-k2a-isolated-persistence.md) covers the initial schema, scoped grants, and restore of a sealed fixture into a separate disposable database. | Full account lifecycle, authorized posting, outbox, and whole-Ledger recovery under the [Ledger acceptance checks](ledger.md#ledger-gate-evidence). |

See the [testing strategy](../testing.md#postgresql-integration-stages) for the integration suite and the [recovery runbook](../runbooks/recovery.md#restore-a-database) for full restore acceptance. Review the [proposed native migration contract](#proposed-native-migration-contract-not-effective) when assessing a future change to DEC-01.

## Schemas and roles

Use one schema per capability. Give each schema an owner role and a limited runtime role. Restrict migration privileges to the migration role or owner. The runtime role cannot own tables, create objects in the schema, act as a superuser, or bypass row-level security.

Each capability owns its business schema and local transaction. Default integration uses authorized facades or events, not shared transaction handles, private cross-schema reads/writes, foreign keys or routine business DML. Narrow technical relay/verification grants or an exceptional cross-owner atomic boundary require a separately accepted design/ADR, named caller, exact operations and grants, threat model and actual catalog verification. Platform provides infrastructure and migration tooling, not ownership of capability financial records. Refer to the [Ledger acceptance checks](ledger.md) for posting and recovery.

## Tenant isolation

Classify each table in its migration. Use this classification when assigning grants and writing tests.

| Table class | Access rules |
|---|---|
| Platform-owned infrastructure and canonical reference records | Limit access with schema grants and authorized interfaces. Platform does not own Ledger financial records. |
| Capability-owned, non-tenant-local records, including core Ledger financial records | Limit access to the owning capability's scoped roles and authorized facade. Merchant access requires resource authorization; a book is not a tenant. |
| Tenant-owned, such as merchant account mappings | Require `tenant_id`. Enable and force RLS. Check the transaction tenant for reads and writes. Include the tenant in tenant-local unique keys. |

Authorize the caller's tenant before setting it in the database. Set the tenant locally at the start of each tenant transaction. Use the same transaction for all tenant queries. Reject missing or invalid tenant context.

Use a separate named role for a job that must cross tenants. Grant only the operations the job needs. Review and document any RLS bypass before creating the role. Request handlers must never receive its credentials or connection pool.

## Privileged paths and tampering boundary

- The application runtime must not connect as schema owner, superuser, a role with `BYPASSRLS`, or the migration role. Table owners and privileged roles can bypass ordinary RLS unless restrictions are applied; `FORCE ROW LEVEL SECURITY` is required for tenant-owned tables but is not a replacement for privilege review.
- Define grants, default privileges, view ownership, `SECURITY DEFINER` functions, allowed schemas and `search_path` for every executable DB entry point. No tenant-facing path may obtain an unrestricted cross-tenant connection through an admin helper, view, or pooled job client.
- Apply transaction-local tenant context only from an already authenticated and authorized caller. Rollback, pool reuse, missing context, and conflicting client-supplied tenant IDs must not reveal or mutate another tenant's rows.
- RLS and scoped queries enforce tenant boundaries only while the selected database and role trust assumptions hold. An attacker with arbitrary writes to the same **authoritative identity database** can alter credential and ownership state. Hashes, checksums, or signed rows without a separately trusted current head cannot provide a stronger anti-rollback claim. See the [authority model](../architecture.md#security-authority-and-trust-boundaries).

## Implementation checks

- Choose the PostgreSQL version, driver, and migration runner before adding production migrations. Document the supported upgrade path.
- Run migrations on an empty database and from each supported prior schema. Record the expected locks and recovery steps.
- Check ownership and privileges against the database catalog. Connect as the actual runtime role for integration tests.
- On tenant tables, verify that tenant A cannot read, insert, change, or delete tenant B's data. Test missing tenant context and reuse of the same pooled connection by two tenants.
- On platform and capability-owned tables, check schema grants, permitted owner operations and denied routine foreign-schema access. Test tenant-facing Ledger access through the authorized calling capability; verify any separately accepted technical grant or exception against actual catalogs.
- Restore a backup onto a fresh database. Verify the ledger, outbox, roles, and grants. Record the observed recovery time.

Keep `make check` independent of PostgreSQL. Record database test results in the [PR evidence](../../.github/pull_request_template.md).


## Proposed native migration contract (not effective)

**Candidate only.** [ADR-0042](../adr/0042-context-aware-migration-executor.md) proposes an amendment to DEC-01; current DEC-01 and [current operations runbook](../runbooks/platform-migrations.md) remain effective. Owner accepted C1–C6 (native executor), DA-A (durable admission), IP-D1–D4 (identity and provisioning) and EB-D1–D4 (execution budgets and evidence) as detailed design candidates for documentation review only. No change to the active engine, migration behavior or permissions is authorized here.

The candidate must satisfy the following before separate engine cutover approval:

| Local proposed requirement | Acceptance boundary |
|---|---|
| M105-R1 | Fixed per-capability target and dedicated schema owner, immutable reviewed forward-only manifest; no untrusted SQL/target input or app pool. |
| M105-R2 | Context-aware admission and work; bounded connection, statement, lock, transaction and cleanup budgets. Stop starting versions once cancellation observed; no claim cancellation proves rollback. |
| M105-R3 | Separate termination cause, requested version, attempted version, last verified prefix and durable outcome. Success only on exact requested target with settled cleanup and durable terminal resolution. |
| M105-R4 | One explicit transaction per version includes reviewed SQL body, existing fingerprint trailer and clean version record; prior versions remain committed. |
| M105-R5 | Refuse legacy dirty, missing/contradictory history, unknown state or identity. Fresh admission requires approved provisioning provenance, expected target/incarnation, schema object inventory and effective owner/grants. No automatic force, repair, down or replay. |
| M105-R6 | Dedicated session owns fixed advisory lock through verification and writes; bounded cleanup; independent settled observation; single-host global administrative gate, exclusive durable pre-mutation admission/intent and restart-safe global quarantine across all managed slots. |
| M105-R7 | Preserve historical bytes, digests, timestamps, metadata shape and ownership; actual old/new equivalence and supported upgrade evidence. |
| M105-R8 | Actual disposable fault tests, independent observation and regression; no weakening existing legacy characterization or production acceptance inference. |

Outcome vocabulary: REJECTED_BEFORE_EXECUTION (no mutation proved), NO_CHANGE_VERIFIED (already exact target), APPLIED_VERIFIED (exact target reached), STOPPED_AT_VERIFIED_PREFIX (settled verified prefix short of requested target, possibly empty or including a newly committed version), RECOVERY_REQUIRED (legacy dirty/inconsistent), OUTCOME_UNKNOWN (unsettled execution or unverified result). No-change or applied success additionally requires settled resource cleanup and durable terminal resolution. Cancellation/timeout/transport error alone is not evidence of rollback. Acknowledged commit is not reversed by later cancellation; a postflight failure may still mean durable change. Keep separate cause, attempted-version effect (not started, confirmed rolled back, confirmed committed or unknown), last verified durable prefix and cleanup/verification fields. The verified empty prefix is not a stored version 0. Preserve known committed effects even if failed postflight or unsettled cleanup makes the overall outcome UNKNOWN; no success or further mutations while unresolved.

Before any mutation, including metadata bootstrap, an exclusive trusted admission gate must durably record unresolved execution intent bound to the verified target/incarnation. The owner selected Option A (global quarantine) for the bounded synthetic design. Unresolved intent or an unresolved restore fence anywhere in the managed scope blocks new migration admission for all slots, including unrelated slots and H2 no-op requests. Unavailable or untrusted global admission authority also blocks all admission. The active authorized attempt may complete normally while authority and budget remain valid; uncertainty stops new mutation. Only separately authorized reconciliation of the blocking state may proceed under the global gate, not unrelated migration or restore work. Resume new admission only after every blocking state is durably resolved, continuity and authority are verified, and a fresh admission is authorized. This policy does not stop application runtime or change the effective engine. Only explicitly authorized reconciliation of settled execution and verified state may resolve unknown intent; successful runs require durable resolution after verified outcome and cleanup. The bounded candidate uses one preprovisioned trusted local journal outside target DB recovery, one global administrative gate, and stable logical slots across externally approved incarnations (DA-A/IP-D1–D4). Refuse missing/uncertain journal continuity, unknown identity, or sole authority failure; no auto-init, TTL, retry or repair. Positive fresh-state admission requires independent provisioning history and approved actual catalog/role inventory, not missing metadata. Restore must durably fence all affected slots before mutation, preserve unresolved attempt evidence, verify old execution settlement and explicitly rebind the same slots; new incarnation is not clearance. Exact storage durability, credential enforcement and fixture oracle remain implementation blockers. If single-host or control continuity cannot be established, stop and reevaluate DA-C. Initial read-only identity and prefix observations under the global gate must be recorded with intent and revalidated under the target lock before execution; refuse mismatches. H2 clean adoption/no-op requires the same durable admission, verification and terminal resolution, without target database mutation. No new metadata registry or live orchestrator is authorized.

The EB-D1–D4 candidate uses one shared monotonic 180s work budget or earlier caller deadline, phase ceilings clipped to remaining budget and combined additional 10s cleanup/observation/terminal publication target in an independent cleanup context, not permission for new migration work; no new version after observed expiry or insufficient reserve. These are proposed limits, not hard OS/storage or backend exit guarantees. Uncertain I/O or cleanup stays quarantined; success requires durable terminal resolution. H1–H7 (historical compatibility scenarios) bound synthetic historical compatibility; no arbitrary prior-version or real Platform/Ledger next-release claim. Require separate additive mandatory isolated actual fault tests without weakening existing checks or changing workflow/timeouts without approval. See [consolidated design and supplemental requirement traceability](../proposals/native-pgx-migration-executor.md#consolidated-admission-identity-and-execution-design). DA-R1–8, IP-R1–7 and EB-R1–6 refine M105-R1–8; DA-AC01–10, IP-AC01–10 and EB-AC01–08 supplement AC105-01–11. Exact G4 scope, actual storage/fault/CI feasibility (including both ambiguous commit effects) and separate implementation/execution acceptance remain open.

All candidate requirements and [AC105-01–11](../proposals/native-pgx-migration-executor.md#actual-fault-test-acceptance) are **NOT RUN** as native implementation evidence. This proposal is not FND-03/08/09 completion, production recovery, or permission for K2b/K3 or runtime integration.
