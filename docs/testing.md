# Testing strategy and evidence

The aim is to prove **observable behavior, invariants and failure handling**, not to maximize a single coverage percentage or the number of AI-generated tests. Every accepted requirement maps to at least one meaningful check and a negative/failure case when applicable. See the [change lifecycle](development-workflow.md) and [security matrix](engineering.md#security-verification).

## Test taxonomy and current state

| Test | Purpose and boundary | Trigger / execution | Status in HTTP bootstrap |
|---|---|---|---|
| Unit | Pure domain rules, usecase orchestration, invalid inputs and error classification with fakes for external ports | `make unit`; files named `*_test.go` | **Implemented** for existing bootstrap/checker packages; financial rules absent |
| HTTP/component | Handler routing, method/resource exposure, request/response mapping, deadlines and private/public listener separation using `httptest` or bound ephemeral listeners | `make unit`; focused Go tests | **Implemented** only for health endpoints |
| Integration — database | Actual SQL, migrations, constraints, runtime roles, tenant RLS, transaction/retry, outbox, balance-floor and race semantics | `make integration` with real PostgreSQL fixture and `TEST_DATABASE_URL`; `//go:build integration` and `*_integration_test.go` | **NOT RUN**: database code, fixtures and integration cases not yet available |
| Contract | Public OpenAPI/schema compatibility, event versions, external provider signature fixtures and stub behavior | Focused tests when published contracts/adapters exist | **NOT RUN** for payment/provider APIs |
| Scenario / E2E | Customer/operator flows crossing capability boundaries with synthetic providers, including recovery, denial, retry, duplicate/out-of-order messages | Dedicated scenario tests after target flows exist | **NOT RUN** for financial flows |
| Concurrency / fault | Race, lost response, concurrent idempotency key, rollback, outbox replay, restart/restore | `make race` for shared memory; real DB multi-connection fault tests for durable state | Race detector runs; **financial fault tests NOT RUN** |
| Property / fuzz | Arithmetic conservation, allowed state transitions, parsers/decoders on hostile inputs | `go test -fuzz=Fuzz... -fuzztime=... <package>` when targets exist; committed corpus runs during unit tests | **NOT RUN** for absent financial/parser targets |
| Performance / recovery | Bounded latency, resource budget, load, backups and safe restoration | Dedicated reproducible benchmarks and restore drills with declared fixtures | **NOT RUN** |

**Never claim `make race` proves a SQL concurrency invariant.** Never report `make unit` as a database integration PASS. The `integration` Make target intentionally fails with `NOT RUN` (exit 2) until a real suite and `TEST_DATABASE_URL` exist. Do not add an empty dummy integration test merely to make it green.

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
| `make integration` | Requires actual tagged integration files **and** explicit `TEST_DATABASE_URL`; never a pretend PASS. |
| `make lint`, `make security`, `make archlint` | Static/vulnerability/dependency gates; not a substitute for behavioral proof. |
| `make test-report` | Test evidence for base Go suite via gotestsum; does not cover absent database tests. |

Run targeted tests while implementing, then required CI gates. For critical changes, independent review must inspect **both code and tests**: an AI can weaken a test to make an incorrect behavior pass. CI must not assume a changed test is trustworthy merely because it is green.

## First real PostgreSQL integration gate

When the first database schema/migration lands:

1. Introduce a disposable PostgreSQL service matching the pinned version. Decide whether to use CI service containers or a reviewed Testcontainers adapter, but avoid introducing both without need.
2. Add `*_integration_test.go` with `//go:build integration` and explicit setup/cleanup. Use `TEST_DATABASE_URL` only for isolated disposable test infrastructure. Fail closed if absent.
3. Run migrations from empty database and a prior supported version. Verify ownership, `FORCE ROW LEVEL SECURITY`, runtime grants and unauthorized tenant access using **actual roles**.
4. Add deterministic concurrent transactions, duplicate keys, forced abort, lost response and process restart tests for the first ledger slice.
5. Then and **only then** enable a required CI integration job. Confirm it fails if PostgreSQL is unavailable, migrations fail, no tests run, or an invariant is violated; do not mark it optional via `continue-on-error`.

Dependency selection and pinning belong to DEC-01 in the [decision register](open-decisions.md). An off-the-shelf test container library is an option, not a prerequisite for Go unit tests.

## Test evidence for AI changes

For every PR record the AC ID, test function / scenario, the adversarial case or invariant, exact command, resulting commit/fixture and observed outcome. Use `PASS`/`FAIL`/`NOT RUN`; only use `N/A-disabled` for a verifiably absent feature. For critical changes, request review of changed tests and inspect unexpected fixture or assertion deletions.

When an agent introduces a bug that slipped through, add a **minimal failing regression first**, then fix the behavior, and if the error is structural add a deterministic architecture/static check. This forms a feedback loop; more prose without enforcement is not a guardrail.
