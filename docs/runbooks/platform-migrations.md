# Platform migrations: operations and recovery

**Effective decision:** [DEC-01](../open-decisions.md#foundation-and-access), accepted 2026-11-21. Use the [PostgreSQL evidence scope](../contracts/postgresql.md#implementation-and-evidence-scope) and [testing strategy](../testing.md#postgresql-integration-stages) to select the applicable checks. [ADR-0042](../adr/0042-context-aware-migration-executor.md) records the proposed native executor amendment.

## Scope

`cmd/mamony-migrate` applies **forward-only** SQL under `db/migrations/platform/` with `golang-migrate/v4` and the pgx v5 driver. It is a **separate administrative command**, never invoked by `cmd/mamony` or an HTTP request. The first migration creates only `mamony_platform.migration_fingerprints`; it does not create a tenant table, ledger, outbox or authentication API.

The migration command requires one controlled `MAMONY_MIGRATION_DSN`, `current_user = mamony_platform_migrator`, a non-superuser migration role that **owns the pre-provisioned `mamony_platform` schema**, and a dedicated schema-version table. Do not use the schema owner's credential for routine application traffic. No test passwords are valid production credentials.

## Pre-provisioning (administrative infrastructure, not application startup)

An administrator or secret manager must provision the database and login roles **outside the application**:

1. Create the role `mamony_platform_migrator` with login; deny `SUPERUSER`, `CREATEDB`, `CREATEROLE`, `REPLICATION`, `BYPASSRLS`. Supply a real rotated secret out-of-band. No `PUBLIC` owner membership.
2. Create `mamony_platform` **owned by `mamony_platform_migrator`** and revoke unexpected `PUBLIC` schema grants. Grant schema `USAGE` only to the limited `mamony_platform_runtime` role as appropriate. No broad DML grants, no schema `CREATE` for runtime.
3. Ensure the migrator can connect but cannot create other databases, modify unrelated schemas, or bypass RLS. Validate using actual runtime and migration credentials; don't rely on SQL grants alone.
4. Provision `mamony_platform_runtime` separately, with no schema ownership or migration-table reads. App/HTTP processes receive **only** runtime credentials. Keep the privileged administrative/bootstrap account away from request-serving pods, jobs and logs.
5. Pin the production PostgreSQL 18.7 image **digest**, TLS policy, patch cadence, database backup policy and migration secret source before a real deployment. The disposable CI fixture uses tag `postgres:18.7` and synthetic credentials and is **not** a production bootstrap script.

The disposable local/CI role provisioning fixture is `tools/postgrestest/testdata/platform-bootstrap.sql`. **Never run that test SQL against a real database:** it drops the `mamony_platform` schema by design.

## Running and verifying

```sh
make migrations-check           # checks SQL/trailer structure; no database
make integration-local          # Docker + psql: disposable PostgreSQL, role tests, migrations
# Only in an approved environment with separately provisioned migrator role:
MAMONY_MIGRATION_DSN='postgres://...' make migrate-platform
```

The migration command applies `up` only; no `down`, `drop`, or `force` operation is exposed. It requires `MAMONY_MIGRATIONS_DIR` only for a reviewed custom migration directory, such as an isolated test fixture.

### Fingerprint and atomicity rule

Every `000001_name.up.sql` file ends with a canonical `-- mamony:integrity:v1` trailer that inserts the SHA-256 of **all bytes before the trailer marker** into `mamony_platform.migration_fingerprints`. The migration runner validates that trailer before connecting. The SQL body and fingerprint insert run in **one server-side PostgreSQL statement batch**, without explicit transaction control and with `MultiStatementEnabled=false`. PostgreSQL commits or rolls back that implicit SQL transaction as a whole; **golang-migrate's separate version/dirty metadata updates are not part of that transaction**.

The command verifies `schema_migrations` and all previously committed fingerprints *before* and *after* `migrate.Up()`, while holding its own session advisory lock. Missing or conflicting hashes, unknown applied versions and dirty metadata stop execution. A changed SQL file with an updated local digest still conflicts with database history. This does not prove immutable records against a compromised database superuser. Do not edit previously applied SQL or manually update the fingerprint table.

Before adding a new migration, use the [migration file validation tests](../../internal/platform/migration/files_test.go) as the canonical trailer format. Add a new numbered file; **never update an applied version**. Review every SQL statement for explicit transaction-control commands, external effects, table locks, grants and schema ownership. Changes to this mechanism require an independent negative test.

## Failure and recovery

| Fault | Expected behavior |
|---|---|
| Invalid or missing migration credentials | Command refuses to start; no SQL applied |
| Wrong database role, elevated role attributes, or unexpected schema owner | Command refuses to start. The required schema owner is the dedicated migrator. |
| Edited file or trailer | Preflight refuses before SQL migration |
| SQL fails after one DDL statement | PostgreSQL rolls back that batch, `golang-migrate` retains a dirty version; no automatic retry |
| Process crashes after body/digest commit but before version becomes clean | `schema_migrations` may remain dirty; next call refuses. An operator inspects the committed schema/digest before any explicit recovery |
| Digest row missing or changed while version is clean | Next call refuses; **do not** self-heal, rehash old SQL or force version |
| Two migration commands race | The second command cannot enter fingerprint verification while the first holds the session lock |
| Restore a stale database | Use the [database restore procedure](recovery.md#restore-a-database). Verify schema version, fingerprints, role grants, and domain data before deciding whether the target may resume service. |

**Manual incident handling:** stop new application writes, capture DB backup and current schema catalog, compare applied SQL fingerprints and schema versions with the audited release, determine whether the DDL committed, and document a forward-fix or explicit version repair separately. `golang-migrate force` is intentionally not wired into `make` because it bypasses dirty-state checks. A repair requires explicit operator approval, an audited SQL change and re-run of integration/restore verification. Do not claim the separate version table is atomic with the migration SQL.

## Verification inventory

- **Migration-file validation:** regular Go unit tests, `make migrations-check`.
- **Real migration integration:** disposable PostgreSQL `TestPostgresPlatformMigration` covers fresh apply, replay, runtime privilege denial, altered/re-signed SQL rejection, missing history, successful upgrade, rollback on failed SQL and sticky dirty state.
- **Related disposable checks:** the [PostgreSQL evidence scope](../contracts/postgresql.md#implementation-and-evidence-scope) links runtime pool and RLS, Platform and Identity restore, and Ledger K2a storage and restore.
- **Release evidence pending:** production secret provisioning, deployment image and grants, capability-level tenant authorization, long-running migration performance, and full recovery for the deployed scopes.

Record `PASS`, `FAIL` or `NOT RUN` separately. A green SQL fixture CI job is **not** evidence that live traffic can safely perform financial transactions.
