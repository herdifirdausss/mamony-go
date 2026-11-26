# PostgreSQL contract

**Implementation status:** DEC-01 is accepted and the platform migration runner (PR #17) passed disposable PostgreSQL CI. The HTTP application still has no database connection or payment/ledger runtime. A bounded runtime [pgxpool primitive](../../internal/platform/postgres/pool.go) is being verified with the existing synthetic RLS fixture; it is **not** tenant authorization, an owned business repository, or a production deployment. Restore and ledger behavior remain NOT RUN.

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

**Candidate only.** [ADR-0042](../adr/0042-context-aware-migration-executor.md) proposes an amendment to DEC-01; current DEC-01 and [current operations runbook](../runbooks/platform-migrations.md) remain effective. Owner accepted detailed design candidate C1–C6 for documentation review only. No change to the active engine, migration behavior or permissions is authorized here.

The candidate must satisfy the following before separate engine cutover approval:

| Local proposed requirement | Acceptance boundary |
|---|---|
| M105-R1 | Fixed per-capability target and dedicated schema owner, immutable reviewed forward-only manifest; no untrusted SQL/target input or app pool. |
| M105-R2 | Context-aware admission and work; bounded connection, statement, lock, transaction and cleanup budgets. Stop starting versions once cancellation observed; no claim cancellation proves rollback. |
| M105-R3 | Separate termination cause, requested version, attempted version, last verified prefix and durable outcome. Success only on exact requested target with settled cleanup. |
| M105-R4 | One explicit transaction per version includes reviewed SQL body, existing fingerprint trailer and clean version record; prior versions remain committed. |
| M105-R5 | Refuse legacy dirty, missing/contradictory history, unknown state or identity. Fresh admission requires approved provisioning provenance, expected target/incarnation, schema object inventory and effective owner/grants. No automatic force, repair, down or replay. |
| M105-R6 | Dedicated session owns fixed advisory lock through verification and writes; bounded cleanup; independent settled observation; exclusive durable pre-mutation admission/intent and restart-safe operational quarantine. |
| M105-R7 | Preserve historical bytes, digests, timestamps, metadata shape and ownership; actual old/new equivalence and supported upgrade evidence. |
| M105-R8 | Actual disposable fault tests, independent observation and regression; no weakening existing legacy characterization or production acceptance inference. |

Outcome vocabulary: REJECTED_BEFORE_EXECUTION (no mutation proved), NO_CHANGE_VERIFIED (already exact target), APPLIED_VERIFIED (exact target reached), STOPPED_AT_VERIFIED_PREFIX (settled verified prefix short of requested target, possibly empty or including a newly committed version), RECOVERY_REQUIRED (legacy dirty/inconsistent), OUTCOME_UNKNOWN (unsettled execution or unverified result). No-change or applied success additionally requires settled resource cleanup. Cancellation/timeout/transport error alone is not evidence of rollback. Acknowledged commit is not reversed by later cancellation; a postflight failure may still mean durable change. Keep separate cause, attempted-version effect (not started, confirmed rolled back, confirmed committed or unknown), last verified durable prefix and cleanup/verification fields. The verified empty prefix is not a stored version 0. Preserve known committed effects even if failed postflight or unsettled cleanup makes the overall outcome UNKNOWN; no success or further mutations while unresolved.

Before any mutation, including metadata bootstrap, an exclusive trusted admission gate must durably record unresolved execution intent bound to the verified target/incarnation. The gate must reject new runs if unresolved intent exists or admission state is unavailable/untrusted, including after process death before outcome reporting. Only explicitly authorized reconciliation of settled execution and verified state may resolve unknown intent; successful runs require durable resolution after verified outcome and cleanup. Durable quarantine/restart enforcement mechanism remains a design blocker; a process-local error or record written only after failure is insufficient. No new metadata registry or live orchestrator is authorized.

All candidate requirements and [AC105-01–11](../proposals/native-pgx-migration-executor.md#actual-fault-test-acceptance) are **NOT RUN** as native implementation evidence. This proposal is not FND-03/08/09 completion, production recovery, or permission for K2b/K3 or runtime integration.
