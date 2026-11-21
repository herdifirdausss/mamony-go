# Testing strategy and evidence

The aim is to prove **observable behavior, invariants and failure handling**, not to maximize a single coverage percentage or the number of AI-generated tests. Every accepted requirement maps to at least one meaningful check and a negative/failure case when applicable. See the [change lifecycle](development-workflow.md) and [security matrix](engineering.md#security-verification).

## Test taxonomy and current state

| Test | Purpose and boundary | Trigger / execution | Status in HTTP bootstrap |
|---|---|---|---|
| Unit | Pure domain rules, usecase orchestration, invalid inputs and error classification with fakes for external ports | `make unit`; files named `*_test.go` | **Implemented** for existing bootstrap/checker packages; financial rules absent |
| HTTP/component | Handler routing, method/resource exposure, request/response mapping, deadlines and private/public listener separation using `httptest` or bound ephemeral listeners | `make unit`; focused Go tests | **Implemented** only for health endpoints |
| Integration — database | Actual SQL constraints, runtime roles and tenant RLS; future migrations, transaction/retry, outbox, balance-floor and contention | `make integration-local` starts a disposable PostgreSQL 18.7 container; `make integration` verifies exact tagged test cases and requires separate admin/runtime test URLs | **Implemented for synthetic role/RLS primitives only**; production migrations, Go DB pool, ledger and restore remain NOT RUN |
| Contract | Public OpenAPI/schema compatibility, event versions, external provider signature fixtures and stub behavior | Focused tests when published contracts/adapters exist | **NOT RUN** for payment/provider APIs |
| Scenario / E2E | Customer/operator flows crossing capability boundaries with synthetic providers, including recovery, denial, retry, duplicate/out-of-order messages | Dedicated scenario tests after target flows exist | **NOT RUN** for financial flows |
| Concurrency / fault | Race, lost response, concurrent idempotency key, rollback, outbox replay, restart/restore | `make race` for shared memory; real DB multi-connection fault tests for durable state | Race detector runs; **financial fault tests NOT RUN** |
| Property / fuzz | Arithmetic conservation, allowed state transitions, parsers/decoders on hostile inputs | `go test -fuzz=Fuzz... -fuzztime=... <package>` when targets exist; committed corpus runs during unit tests | **NOT RUN** for absent financial/parser targets |
| Performance / recovery | Bounded latency, resource budget, load, backups and safe restoration | Dedicated reproducible benchmarks and restore drills with declared fixtures | **NOT RUN** |

**Never claim `make race` proves a SQL concurrency invariant.** Never report `make unit` as a database integration PASS. The `integration` Make target now enforces actual `TestPostgres*` results from Go's JSON events, refusing missing URLs, an unsafe target, unavailable `psql`, missing required tests and any test failure. A file and `TEST_DATABASE_URL` alone never prove tests executed. The synthetic RLS fixture tests **database mechanics**, not production authorization, migration integrity, or ledger semantics.

## Go test conventions

- Test names express behavior, e.g. `TestJournalRejectsDifferentPayloadForSameKey`. Use table-driven cases and descriptive `t.Run` names for variants. Distinguish valid and rejected transitions.
- Use **black-box packages** (`package foo_test`) when testing public contracts; white-box tests are acceptable for precise private invariants. Do not create exports only for tests.
- Inject time, IDs and I/O effects when deterministic ordering matters. Avoid sleeps, global environment mutations under `t.Parallel`, execution-order assumptions and fragile magic numbers.
- For each authorization operation, test missing identity, wrong tenant, wrong owner, missing permission, suspended subject, stale session and success as applicable.
- For money, test zero/negative/overflow, mixed currency, imbalance, duplicate/reused key with changed payload, simultaneous postings, rollback, commit-then-lost-response, replay and restore.
- Integration tests must exercise **actual** runtime and migration DB roles, not only a database owner's credentials. Use multiple distinct DB connections and deterministic barriers to make races reproducible. Isolate schemas/test databases and clean them safely; never connect to live financial databases.
- Test both observable responses and durable side effects: journal/entry/outbox counts, balance, retry identity, audit obligation and provider dispatch count. A success response alone is not enough.
- No testing-only bypass of authentication, authorization, RLS, or signature validation on the same production path unless the bypass is a test-only adapter that cannot ship.

## CI and local commands

| Command | Guarantee and limitation |
|---|---|
| `make check` | Formatting, vet, unit, architecture, docs/skills and build on current tree. |
| `make test-repeat` | `go test -shuffle=on -count=3 ./...` catches some order dependence but not all flakiness. |
| `make race` | Shared-memory race detector; only exercised code paths are checked. |
| `make coverage` | Generates `test-reports/coverage.out` and per-function coverage; diagnostic only, no arbitrary percentage gate. |
| `make integration` | Runs real tagged PostgreSQL **fixture** checks against strictly guarded runtime/admin test URLs; fails on missing or empty tests. Requires `psql`. |
| `make integration-local` | Runs those tests against an isolated ephemeral Docker container on a random loopback port and removes it after the test. |
| `make lint`, `make security`, `make archlint` | Static/vulnerability/dependency gates; not a substitute for behavioral proof. |
| `make test-report` | Test evidence for base Go suite via gotestsum; does not cover absent database tests. |

Run targeted tests while implementing, then required CI gates. For critical changes, independent review must inspect **both code and tests**: an AI can weaken a test to make an incorrect behavior pass. CI must not assume a changed test is trustworthy merely because it is green.

## PostgreSQL integration stages

The first fixture-only gate is implemented by `tools/postgrestest` and the `PostgreSQL integration / postgresql` CI job. It tests synthetic owner/runtime privileges and `FORCE ROW LEVEL SECURITY`, including tenant-context expiry on one PostgreSQL connection. Both the Go gate and the tagged suite reject unsafe test URLs. See the [foundation proposal](proposals/postgresql-foundation.md) for scope and limitations.

**Implemented and executed on disposable PostgreSQL:** platform migration v1 with applied-file SHA-256 fingerprints, altered/missing-history rejection, synthetic upgrade, rollback on failing SQL and dirty-state detection (PR #17). The `TestPostgresRuntimePool*` cases verify a real, bounded Go `pgxpool` under synthetic tenant RLS, including physical session reuse, rollback and cancellation; these are implemented as part of the runtime-pool PR and must show actual CI PASS before a completion claim.

**Still NOT RUN:** full production schema upgrade/restore and secret provisioning, tenant authorization by Identity/usecase, a real module repository, HTTP database readiness, ledger locks/idempotency/reversal, outbox, backup restore and deterministic concurrent financial SQL execution. A green RLS/pgxpool test is **not** a financial or user-authorization guarantee.

Next, after DEC-01 is accepted:
1. Choose the supported PostgreSQL version, Go driver and migration tool, with explicit immutable migration evidence and scoped owner/runtime credentials.
2. Test production migrations from an empty database and a supported prior version. Verify actual role ownership and grants, including the tenant table's `FORCE ROW LEVEL SECURITY`.
3. Add direct connection-pool tenant reuse tests, not only `psql` sequential connection tests. Exercise deterministic contention and rollback against the first actual ledger schema.
4. Add failed-startup, ambiguous commit, crash/restart and fresh restore tests before claiming financial or recovery gates.
5. Keep the integration CI job non-optional, and fail it on missing database, failed migrations, zero required tests or any invariant failure. A test-file presence check is insufficient.

Dependency and release choices remain DEC-01 in the [decision register](open-decisions.md).

## Test evidence for AI changes

For every PR record the AC ID, test function / scenario, the adversarial case or invariant, exact command, resulting commit/fixture and observed outcome. Use `PASS`/`FAIL`/`NOT RUN`; only use `N/A-disabled` for a verifiably absent feature. For critical changes, request review of changed tests and inspect unexpected fixture or assertion deletions.

When an agent introduces a bug that slipped through, add a **minimal failing regression first**, then fix the behavior, and if the error is structural add a deterministic architecture/static check. This forms a feedback loop; more prose without enforcement is not a guardrail.
