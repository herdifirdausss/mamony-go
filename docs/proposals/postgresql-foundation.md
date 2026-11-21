# PostgreSQL foundation: research and first verifiable slice

**Status:** Test-only RLS/role harness verified and merged in [PR #15](https://github.com/herdifirdausss/mamony-go/pull/15); production DEC-01 **PROPOSED / NOT ACCEPTED**. Owner/technical acceptance is required before implementing production database choices.  
**Initiating issue:** [Issue #14](https://github.com/herdifirdausss/mamony-go/issues/14) (GitHub issue; the text below is self-contained).  
**Research checked:** 2026-11-21.  
**Risk path:** Critical.  
**Active contracts:** [PostgreSQL](../contracts/postgresql.md), [platform FND-03/FND-09](../contracts/platform.md), [ledger](../contracts/ledger.md); no supersession.

## G0 — Problem and non-goals

The HTTP-only runtime now has a **separate, executed PostgreSQL test fixture**, but no application database connection, production schema or migrations. A green Go unit suite or a synthetic RLS fixture cannot prove runtime authorization, real schema privileges, or financial transaction correctness.

The first slice must prove database test plumbing before implementing production schema or ledger posting. It does **not** establish production migration policies, connection pooling, tenant authentication, an authorization API, a ledger, recovery, or an approved DEC-01. Those remain blocked/pending.

## G1 — Research, alternatives and confidence

| Decision | Options assessed | Preliminary finding; open risk |
|---|---|---|
| Database engine | PostgreSQL 18.7 versus 19.0 (released 2026-11-19) | **18.7** is a supported maintenance series and available as the official `postgres:18.7` image; use as the **test fixture only** while production pin and immutable image digest remain open |
| Disposable DB lifecycle | GitHub Actions PostgreSQL service and Docker-run local versus Testcontainers | Service container plus a one-command local Docker wrapper uses no Go dependencies and yields the same SQL checks. A Testcontainers implementation would add a Go package and lifecycle complexity; reassess when test cases need multi-service orchestration |
| SQL access for test suite | `psql` CLI versus Go `pgx` connection pooling | The **first role/RLS primitive tests** can invoke `psql` with standard-library Go tests. This intentionally avoids selecting the runtime driver prematurely; it does not prove Go transaction/pool behavior |
| Production Go driver | `pgx/v5` direct vs `pgx` `database/sql` adapter | No choice implemented. Current `pgx/v5` requires Go 1.25+, while the repository `go.mod` still advertises 1.23 (CI's toolchain is newer). Resolve the minimum supported Go release and benchmark/ergonomic trade-offs before DEC-01 acceptance |
| Migration runner | `pressly/goose`, `golang-migrate/migrate`, or a minimal immutable SQL executor | No production migration runner chosen. Applied-file immutability is a requirement; do not assume the runner's version tracking automatically detects modifications. Require a checksum regression check whichever runner is chosen |
| Role model | schema owner/migration credentials separate from limited runtime; cross-tenant job role only if justified | Fixture uses a disposable bootstrap admin, one owner and one runtime login, `FORCE RLS`. Neither role is wired to the application. Production role catalog, schema grants and secret distribution remain pending |

Primary-source verification references (not runtime requirements): [PostgreSQL support](https://www.postgresql.org/support/versioning/), [PostgreSQL 18 RLS](https://www.postgresql.org/docs/18/ddl-rowsecurity.html), [transaction-local SET](https://www.postgresql.org/docs/18/sql-set.html), [GitHub PostgreSQL service](https://docs.github.com/en/actions/tutorials/use-containerized-services/create-postgresql-service-containers), [pgx README](https://github.com/jackc/pgx), [Goose](https://github.com/pressly/goose), [golang-migrate](https://github.com/golang-migrate/migrate).

## G2 — Scope and falsifiable acceptance for this slice

- **Proof-A:** a disposable `mamony_test_*` database starts in CI and locally; the test runner rejects off-loopback, non-test databases and role/URI overrides **before** applying fixture SQL.
- **Proof-B:** running `make integration` without preconditions, with missing `psql`, with invalid DSNs, with empty suites, or with failed required cases returns nonzero; no fake PASS.
- **Proof-C:** connect using **actual test runtime credentials**; verify non-superuser, no createdb/createrole/bypassrls, no schema CREATE, and no membership in the owner role.
- **Proof-D:** one synthetic RLS table enforces missing tenant deny, cross-tenant read/update/delete invisibility, cross-tenant insert/update refusal and permitted same-tenant insert.
- **Proof-E:** the same PostgreSQL connection has tenant A/B context inside a transaction and no usable tenant context after commit/rollback; the table owner cannot bypass RLS when `FORCE` is enabled.

**Not proven:** production DB migrations and checksums (FND-03); real Go connection-pool reuse; actual tenant authentication; production schema grants; ledger transactions/idempotency; restore/reconciliation. Record these as **NOT RUN**, not PASS.

For the future production foundation, retain Issue #14's PGF-01–PGF-07 candidate criteria and promote accepted stable requirements into the owning contract after DEC-01.

## G3 — Technical design, security and trade-offs

- `tools/testgate` performs a **strict loopback/test-name/role/URL-parameter preflight**, then reads `go test -json -tags=integration -run '^TestPostgres'` events. It requires specific top-level test cases to PASS; package-level PASS or zero tests never qualify. The tagged test suite repeats the safety preflight so manual invocation is also constrained. The `psql` subprocess additionally strips inherited libpq `PG*` environment variables, including `PGHOSTADDR` and `PGSERVICE`, that could redirect a loopback URL.
- `tools/postgrestest/testdata/bootstrap.sql` provisions **disposable synthetic** owner/runtime roles and a single small RLS table. It re-creates only the `mamony_test_fixture` schema. No production schema or migration record is created.
- `tools/postgrestest/postgres_integration_test.go` invokes `psql` under actual admin/runtime credentials and expects SQLSTATE 42501 on forbidden writes. Its tests are black-box SQL behavior checks, not application runtime tests.
- The CI job starts official PostgreSQL `18.7` as a service. The local runner starts and cleans up a single loopback-only container with a random host port. Passwords in both are **synthetic test fixtures**, not deployable credentials.
- `make check` remains independent of PostgreSQL; `make integration` is a separate CI job for every PR and main push. GitHub ruleset enforcement of merge blocking was deferred by the sole repository owner; successful workflow execution is not proof it is required to merge.
- Host/DB and URI safety checks reduce accidental real-DB writes. They are **not a robust sandbox against a malicious local database endpoint impersonating a disposable one**; use only self-provisioned test containers.
- A tag such as `postgres:18.7` selects a fixed PostgreSQL maintenance release but not an immutable image digest. Capture digest when DEC-01/runtime supply-chain pinning is approved.

## G4–G7 — Plan and verification

| Slice | Owner | Result | Evidence |
|---|---|---|---|
| [PR #15](https://github.com/herdifirdausss/mamony-go/pull/15): tagged test harness, fixture, CI, safe local run | Test tooling | **VERIFIED / merged** | PostgreSQL service, four `TestPostgres*` cases, required pass-event detection, CI jobs all PASS at `958aa037`; not production proof |
| Next: decide DEC-01 driver/migration version/roles and Go minimum | Owner + technical reviewer | **Pending approval** | Decision record and supported upgrades |
| Next: production migrations, checksum evidence and upgrade fixtures | Platform | **NOT RUN / DEC-01 approval required** | Fresh/upgrade DB, edited-applied-migration rejection and runtime grants |
| Later: ledger locks, atomicity, replay, failure injection | Ledger | **NOT RUN** | Multiple connections under deterministic contention; restore drill |

Do not promote this proposal's examples into accepted production contracts without explicitly reviewing and resolving DEC-01.

## DEC-01 — Proposed runtime/database implementation decision (G1–G3)

**Status: PROPOSED, not accepted.** Checked against the Mamony Go repository and the linked primary sources on 2026-11-21. This section records one coherent candidate that can be accepted or amended by the owner, rather than introducing unchosen dependencies to production code. It does not supersede [DEC-01](../open-decisions.md) or the [PostgreSQL contract](../contracts/postgresql.md).

### Decision matrix: minimum viable financial database foundation

| Dimension | Preferred candidate | Alternatives and reason for not choosing them yet |
|---|---|---|
| PostgreSQL release | **PostgreSQL 18.7**, initially the same maintenance version as the already executed synthetic test harness; pin explicit image digest when promoting to production runtime | PostgreSQL 19.0 became available on 2026-11-19, only two days before this review. Fresh major-version rollout adds upgrade risk without a specific accepted feature requirement; evaluate 19 with an independent compatibility matrix later |
| Go version | Align `go.mod` and README with the already pinned **Go 1.27.4 build toolchain**, using `go 1.27.0` as the prospective language-version directive; maintain a single initially supported toolchain | Keeping `go 1.23.0` while installing libraries requiring Go 1.25+ is misleading. Keeping a wide lower-version promise would require an additional compatibility test matrix that currently does not exist |
| Application driver | **`github.com/jackc/pgx/v5` v5.13.0 native `pgxpool`** for PostgreSQL adapters; no `database/sql` abstraction unless required by a later adapter | The `database/sql` adapter is valid but adds an extra abstraction when Mamony Go has just one database type. pgx v5.13.0 (2026-11-11) documents a minimum Go version of 1.25; check actual transitive requirements, `go mod tidy`, vulnerability scan, startup and pool tests before acceptance |
| SQL migration runner | **`golang-migrate/migrate/v4` v4.20.1**, SQL-only, executed out-of-process with dedicated migration credentials; no automatic migration on web application startup | **Goose** is a credible SQL alternative with a version table, but does not eliminate the need for an applied-file checksum check. **Atlas Community** has versioned migrations and `atlas.sum`, but its Community feature matrix excludes advanced migration lint/drift features and inspection support for RLS objects; the full feature set introduces different distribution/licensing/operational dependencies. A custom migration engine is unjustified at this stage |
| Migration integrity | An **additional version + SHA-256 integrity record** for every applied migration and a repo/CI manifest gate; reject deleted, modified or duplicate applied files. A migration-runner version/dirty table alone is **not** a content-integrity proof | Hashing only the current directory allows a modified historical SQL file to be re-hashed and blessed. Persist applied fingerprints on the database side and test refusal against actual applied state; design atomic recording/recovery before implementation |
| Credential ownership | One-time bootstrap admin only for creating roles/database and restricted schema owners; per-capability migration/owner identity and per-capability runtime identity, no superuser/owner credentials in application | One all-powerful application runtime account would permit cross-capability SQL access and contradict the existing ownership contract. A cross-tenant job role is **not** created until DEC-05 has an approved use case |
| Connectivity | App composition root constructs bounded connection pools and injects a narrow adapter into each capability; a transaction and tenant context are controlled by the owning usecase/repository | No global `db` accessor, blanket schema grants, repository import from domain, or direct SQL in handlers; follow [architecture](../architecture.md) and [Go conventions](../conventions.md) |

These are **recommended pins for the next acceptance decision**, not dependencies installed by this proposal and not assertions of passed compatibility. Verify exact versions, source signatures, package licensing and security posture at the implementation PR.

### Why not mistake migration directory hashes for durable applied-state proof?

The contract requires **applied migration files not to be silently edited**. The verification mechanism must distinguish:

1. **Repository integrity:** the current SQL files match committed expected SHA-256 digests. If an agent edits an old file without updating the manifest, CI fails.
2. **Applied-state integrity:** the database's immutable migration history contains the version and digest of SQL that was actually applied. If an agent changes a file *and* recalculates the repo manifest, the test must still detect conflict with the stored applied fingerprint.
3. **Atomicity and recovery:** ideally the schema modification and its digest are committed in one PostgreSQL transaction, or the runner must demonstrate a fail-closed recoverable dirty state when a crash splits those operations. Do not invent atomicity when the chosen runner's APIs do not provide it.
4. **Database drift:** equal file digests do not show that a privileged operator has not modified the live schema outside migrations. Real privilege tests and fresh-schema comparison remain separate evidence. This plan does **not** claim to defeat a database superuser who can rewrite both schema and audit history.

**Implementation condition:** before writing the custom digest adapter, verify whether the selected runner can expose per-file applied-transaction hooks. If it cannot guarantee the required consistency without substantial custom orchestration, reconsider the runner choice rather than writing a second migration system by accident.

### Exact role and runtime trust model

```text
Admin/bootstrap credential (provisioning only; not supplied to app)
   └─ create database, schemas and login roles
Schema owner / migration credential for capability X
   └─ CREATE/ALTER inside schema X, run approved SQL migrations
Runtime credential for capability X
   └─ DML on named X-owned tables; no schema CREATE, no owner membership,
      no SUPERUSER, CREATEDB, CREATEROLE or BYPASSRLS
Application composition root
   └─ only runtime connections → X repository adapter → X usecase/domain
```

- Scope grants and default privileges per schema; revoke unexpected `PUBLIC` schema/database permissions. Verify `pg_roles`, `pg_class`, `pg_namespace`, `pg_default_acl`, grants and actual denial behavior under **runtime credentials**.
- Tenant-owned tables must have `tenant_id`, `ENABLE ROW LEVEL SECURITY` and `FORCE ROW LEVEL SECURITY`. Use transaction-local tenant context only after **authenticated, authorized tenant resolution**, then perform all tenant queries in that same transaction. Test `COMMIT`/`ROLLBACK`, connection reuse, missing context and cross-tenant DML.
- **Important authority limitation:** `current_setting('app.tenant_id')` is a session/transaction setting, not proof of caller identity. A compromised runtime SQL channel able to run `SET LOCAL app.tenant_id` could impersonate another tenant. RLS is defense in depth *under trusted application control and SQL-injection resistance*, not an independent identity authority. Do not grant clients arbitrary SQL.
- No `SECURITY DEFINER` function is proposed for the first schema. If one becomes necessary, specify and test fixed trusted `search_path`, restricted `EXECUTE` grants and its authority effect.
- Runtime and migration DSNs are separate. An HTTP process **must never** read admin/owner secrets or run schema migrations at startup; CI may provision disposable roles with synthetic credentials. Application startup fails if required runtime roles are missing or privileged.

### Implementation slices after G2/G3 acceptance

| Slice | Minimum code and tests | Required observed outcome | Status |
|---|---|---|---|
| DEC-01 acceptance | Record approved version/tool and exact role/credential model; update `go.mod` minimum in an independent diff | Go/tool/driver compatibility proof, checked primary-source versions, explicit owner acceptance | **PROPOSED** |
| 1. Migration foundation | `db/migrations/<schema>/` with SQL-only versioned migrations; separate bootstrap/migration entrypoint and applied-file hash gate | Empty DB, supported upgrade, edited historical SQL rejection, interruption/dirty-state recovery, least-privilege catalog queries | **NOT RUN** |
| 2. Runtime data adapter | `internal/platform` connection primitive plus an owning module repository wired by `internal/app` | Scoped runtime role, bounded `pgxpool`, no admin credentials, invalid config refusal, cancellation, explicit retryability classification | **NOT RUN** |
| 3. Tenant adapter verification | One real owned table and owning repository/usecase; no new payments behavior | Real `pgxpool` connection reuse, cross-tenant negative reads/writes, missing identity denial, transaction-local context cleanup | **NOT RUN** |
| 4. Ledger implementation | Only after DEC-02 and accepted ledger semantics | Durable journal posting, atomic outbox, idempotency, amount/floor races, process restart and restore evidence | **NOT RUN** |

A candidate G2 acceptance record must state the *minimum supported Go version*, exact migration runner version, whether the digest record can be atomic with DDL, named runtime/migration grants, local/CI startup path and negative tests for every new privilege. If one answer is unknown, keep the relevant implementation slice blocked.

### Evidence provenance and limitations

- PostgreSQL 18.7 (2026-11-12) and 19.0 (2026-11-19) release/version support: [PostgreSQL versioning policy](https://www.postgresql.org/support/versioning/).
- Native pool and driver support: [pgx v5 documentation](https://github.com/jackc/pgx) and [pgx changelog](https://github.com/jackc/pgx/blob/master/CHANGELOG.md).
- SQL migration behavior/dirty-state limitations: [golang-migrate documentation](https://github.com/golang-migrate/migrate), [Goose documentation](https://github.com/pressly/goose).
- Atlas migration checksums and licensing/feature limits: [directory integrity](https://atlasgo.io/concepts/migration-directory-integrity), [Community edition matrix](https://atlasgo.io/community-edition).
- Row security, ownership and safe privileged functions: [PostgreSQL 18 RLS](https://www.postgresql.org/docs/18/ddl-rowsecurity.html) and [function execution context](https://www.postgresql.org/docs/18/sql-createfunction.html).
- **Executed in Mamony Go:** PR #15 validated only the synthetic PostgreSQL role/RLS fixture. It **did not** execute or verify any production choice above.

**Decision requested from repository owner:** approve the proposed PostgreSQL 18 / Go 1.27 / pgx native / golang-migrate + applied-hash approach **as a coherent bundle**, or identify the part requiring modification. Until then, no production migration/driver dependency is approved.
