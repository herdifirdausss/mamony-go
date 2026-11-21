# PostgreSQL contract

**Implementation status:** Pending. The bootstrap runs without a database.

## Schemas and roles

Use one schema per capability. Give each schema an owner role and a limited runtime role. Restrict migration privileges to the migration role or owner. The runtime role cannot own tables, create objects in the schema, act as a superuser, or bypass row-level security.

Grant access to another capability's database interface only for a named contract. Record the caller, operations, and grant. Refer to the [Ledger acceptance checks](ledger.md) for posting and recovery.

## Tenant isolation

Classify each table in its migration. Use this classification when assigning grants and writing tests.

| Table class | Access rules |
|---|---|
| Platform-owned, including core ledger tables | Limit access with schema grants and capability authorization. Merchant access uses the owning capability's authorized API. |
| Tenant-owned, such as merchant account mappings | Require `tenant_id`. Enable and force RLS. Check the transaction tenant for reads and writes. Include the tenant in tenant-local unique keys. |

Authorize the caller's tenant before setting it in the database. Set the tenant locally at the start of each tenant transaction. Use the same transaction for all tenant queries. Reject missing or invalid tenant context.

Use a separate named role for a job that must cross tenants. Grant only the operations the job needs. Review and document any RLS bypass before creating the role. Request handlers must never receive its credentials or connection pool.

## Implementation checks

- Choose the PostgreSQL version, driver, and migration runner before adding production migrations. Document the supported upgrade path.
- Run migrations on an empty database and from each supported prior schema. Record the expected locks and recovery steps.
- Check ownership and privileges against the database catalog. Connect as the actual runtime role for integration tests.
- On tenant tables, verify that tenant A cannot read, insert, change, or delete tenant B's data. Test missing tenant context and reuse of the same pooled connection by two tenants.
- On platform tables, check schema grants and the permitted service interface. Test tenant-facing ledger access through the calling capability.
- Restore a backup onto a fresh database. Verify the ledger, outbox, roles, and grants. Record the observed recovery time.

Keep `make check` independent of PostgreSQL. Record database test results in the [PR evidence](../../.github/pull_request_template.md).
