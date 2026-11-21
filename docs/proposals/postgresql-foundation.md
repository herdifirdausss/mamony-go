# PostgreSQL foundation: research and first verifiable slice

**Status:** Proposed — requires repository owner/technical acceptance for DEC-01 before production database choices.  
**Initiating issue:** [Issue #14](https://github.com/herdifirdausss/mamony-go/issues/14) (GitHub issue; the text below is self-contained).  
**Research checked:** 2026-11-21.  
**Risk path:** Critical.  
**Active contracts:** [PostgreSQL](../contracts/postgresql.md), [platform FND-03/FND-09](../contracts/platform.md), [ledger](../contracts/ledger.md); no supersession.

## G0 — Problem and non-goals

The HTTP-only bootstrap has static Go checks but **no executed PostgreSQL security tests**. A green Go unit suite cannot prove runtime database privilege separation, RLS or tenant context reset.

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
| This PR: tagged test harness, fixture, CI, safe local run | Test tooling | Proposed implementation | Three named tests, required pass-event detection, unit tests for DSN and zero-test rejection; CI job |
| Next: decide DEC-01 driver/migration version/roles and Go minimum | Owner + technical reviewer | **Pending approval** | Decision record and supported upgrades |
| Next: real migrations, checksum table and upgrade fixtures | Platform | **NOT RUN** | Fresh/upgrade DB, altered migration rejection and runtime grants |
| Later: ledger locks, atomicity, replay, failure injection | Ledger | **NOT RUN** | Multiple connections under deterministic contention; restore drill |

Do not promote this proposal's examples into accepted production contracts without explicitly reviewing and resolving DEC-01.
