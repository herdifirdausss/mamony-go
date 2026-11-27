# Testing strategy and evidence

The aim is to prove **observable behavior, invariants and failure handling**, not to maximize a single coverage percentage or the number of AI-generated tests. Every accepted requirement maps to at least one meaningful check and a negative/failure case when applicable. See the [change lifecycle](development-workflow.md) and [security matrix](engineering.md#security-verification).

## Test taxonomy and current state

The default HTTP process serves health endpoints. Internal component and lab suites exercise additional paths with synthetic fixtures. See [current state](current-state.md) for exact baseline evidence and release blockers.

| Test | Where to verify | Boundary |
|---|---|---|
| Unit and component | `make unit` for Go unit and handler tests | Covers health listeners, internal Identity flows, and isolated Ledger domain rules. Handler fixtures can exercise unmounted routes. |
| PostgreSQL integration | `make integration-local` or the required PostgreSQL CI job | Covers disposable role/RLS, runtime pool, migration, selected Identity, and Ledger K2a storage and role cases. End-to-end resource authorization and deployed grants remain open. |
| Contract and E2E | [Identity browser E2E](runbooks/identity-browser-e2e.md) and owning contract checks | Browser and PostgreSQL fixtures cover selected Identity paths. End-to-end financial flows and public payment/provider contracts remain pending. |
| Concurrency and fault | `make race`, followed by applicable real database tests | The race detector covers executed shared-memory paths. Use separate multi-connection fixtures to establish SQL contention, rollback, and durable outcomes. Ledger posting, replay and outbox fault evidence remain pending. |
| Property and fuzz | `go test -fuzz=Fuzz... -fuzztime=... <package>` for the applicable target | Record the invariant or parser, target, and run. Committed corpus inputs run during ordinary unit tests. |
| Performance and recovery | [Login performance runbook](runbooks/identity-login-performance.md), [PostgreSQL stages](#postgresql-integration-stages), and [recovery procedures](runbooks/recovery.md) | The login harness, PITR lab, and isolated Ledger K2a restore have synthetic evidence. Deployed capacity, full Ledger recovery, and production recovery objectives remain open. |

`make integration` requires guarded disposable database URLs and verifies the named required tests from Go's JSON events. It fails on an unsafe target, missing fixtures or checks, unavailable `psql`, or any test failure. Record the actual fixture and command with the result.

## Go test conventions

- Test names express behavior, e.g. `TestJournalRejectsDifferentPayloadForSameKey`. Use table-driven cases and descriptive `t.Run` names for variants. Distinguish valid and rejected transitions.
- Use **black-box packages** (`package foo_test`) when testing public contracts; white-box tests are acceptable for precise private invariants. Do not create exports only for tests.
- Put integration tests that wire several layers in `internal/modules/<capability>/internal/integration`. Put browser tests in its `browser` subpackage. Keep adapter-level tests beside the adapter and shared suite fixtures in `integration/internal/fixture`.
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
| [Identity login performance](runbooks/identity-login-performance.md) | Measures the candidate login flow against a new local PostgreSQL cluster with a fixed CPU affinity and Go memory target. |
| `make lint`, `make security`, `make archlint` | Static/vulnerability/dependency gates; not a substitute for behavioral proof. |
| `make test-report` | Test evidence for base Go suite via gotestsum; does not cover absent database tests. |

Run targeted tests while implementing, then required CI gates. For Critical changes, review must inspect **both code and tests**: an AI can weaken a test to make an incorrect behavior pass. Independent review remains the ordinary gate; the adopted solo synthetic mode permits only the explicitly owner-authorized synthetic merge defined in the lifecycle, with recorded review and deferred independent acceptance, not production readiness. CI must not assume a changed test is trustworthy merely because it is green.

## PostgreSQL integration stages

The `PostgreSQL integration / postgresql` CI job uses disposable fixtures. Use the record for each scope to verify which cases actually ran.

| Scope | Disposable evidence | Remaining evidence |
|---|---|---|
| Platform roles, runtime pool, and migrations | Actual role and `FORCE ROW LEVEL SECURITY` checks, physical session reuse, rollback, cancellation, and migration fingerprint and dirty-state checks. See the [Foundation proposal](proposals/postgresql-foundation.md). | Deployment grants and secrets, tenant authorization through the owning capability, and database-aware readiness. [Effective DEC-01](open-decisions.md#foundation-and-access) still uses golang-migrate. Actual fault and supported upgrade evidence for the proposed native executor is pending. |
| Identity and Platform recovery | Selected Identity persistence/role tests, the PITR lab, and bounded RH-03 recovered-state reconciliation. See the [recovery evidence](current-state.md#evidence-at-the-inspected-main-commit). | RH-03 remains **PARTIAL**. Integrated RF-01–08 evidence is **NOT RUN**, and E2 remains **BLOCKED**. Full authority coverage, accepted recovery objectives, and deployed access and delivery remain open. |
| Ledger isolated persistence | K2a covers the initial schema, fixed USD definition, scoped grants, and disposable storage/restore. See the [K2a scope and evidence](proposals/ledger-k2a-isolated-persistence.md). | Full account lifecycle, atomic authorized posting, idempotency under contention, reversal, outbox, and whole-Ledger restore acceptance. |

Before claiming a database or recovery gate, verify the applicable upgrade paths, actual roles, durable effects under failure, and restore results against the owning contract. The required integration job must fail on a missing fixture, failed migration, missing named case, or invariant failure.

## Test evidence for AI changes

For every PR record the AC ID, test function / scenario, the adversarial case or invariant, exact command, resulting commit/fixture and observed outcome. Use `PASS`/`FAIL`/`NOT RUN`; only use `N/A-disabled` for a verifiably absent feature. For critical changes, request review of changed tests and inspect unexpected fixture or assertion deletions.

When an agent introduces a bug that slipped through, add a **minimal failing regression first**, then fix the behavior, and if the error is structural add a deterministic architecture/static check. This forms a feedback loop; more prose without enforcement is not a guardrail.
