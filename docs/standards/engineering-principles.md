# Mamony Go — Engineering Principles & Design Handbook

**Status:** Draft guideline, 21 November 2026. **Relation:** Companion to Mamony Go Production Workflow v5, §12–§17. The handbook is available at `docs/standards/engineering-principles.md` for review and adoption by the owner. Section numbering follows the full workflow for easy cross-references.

## 12. Software engineering principles and how to apply them

**Status:** Proposed engineering policy for Mamony Go, pending an Accepted ADR. Scope covers all changes. Source: summary and interpretation for Mamony of [Effective Go](https://go.dev/doc/effective_go), [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments), [Organizing Go code](https://go.dev/blog/organizing-go-code), [Martin Fowler — YAGNI](https://martinfowler.com/bliki/Yagni.html), as well as [Refactoring.Guru](https://refactoring.guru/). This is not an official rule from the same source for all languages; some are specifically proposed policies Mamony.

### 12.1 Priorities when principles conflict

1. **Correctness and domain invariants.** Preserve the approved balance semantics, double-entry rules, idempotency, tenant isolation, rounding, and retry behavior.
2. Do not override access checks, audits, transactions, validations or rollbacks for the sake of a shorter code.
3. Clarity and simplicity. Choose the simplest design that meets (1) and (2); one easy path to follow is better than a new framework with no proven need.
4. Changeability. Keep low coupling and high cohesion at the actual boundary; refactor when change needs to be known.
5. Performance and scalability. Optimization based on budget, profiling, capacity, and proven load, not guesswork.

The principle is a heuristic of decision making, not a keyword that can automatically prove quality. Linters can examine some of the symptoms, but they cannot infer SRP, YAGNI, or semantically correct design patterns.

### 12.2 Core principles with Mamony examples

| Principle | Practical meanings | Applied examples | Misunderstanding | How to review |
|---|---|---|---|---|
| **KISS** (Keep It Simple) | The minimum possible solution is to stay on the contract and fail path. | Starting with `net/http`, SQL clear, explicit constructor, and one binary. | The fewest lines don't mean the simplest; removing DB constraints can change correctness. | Explain why each layer/dep exists; scenario testing is difficult. |
| **YAGNI** (You Aren't Gonna Need It) | Don't build speculative ability just because it might be needed. | No adding Kafka, plugin engine, multi-region, or 5 abstraction providers before there's a trigger. | It does not apply to approved requirements or security controls that are actually needed at this time. | Connect features/abstractions to AC/ADR or real data. |
| **DRY** (Don't Repeat Yourself) | An authoritative domain of knowledge, avoiding biased logic. | The definition of currency exponent and permission comes from a single, clear source. | Two lines SQL similar to not automatic knowledge duplication; shared helper cross-capability can create poor coupling. | Look for rule changes to be edited in many places. |
| **SoC** (Separation of Concerns) | HTTP parsing, business rules, and persistence each have distinct responsibilities. | `handler` parse DTO, `usecase` orchestration, `repository` SQL. | Don't make one package per function if the boundary is not independent. | Look at the change path and import graph. |
| **High cohesion / low coupling** | Parts that change for the same reason are placed together, cross-border dependency is reduced. | Posting/reversal/invariant fixed C01; C03 uses a port, not a query to the schema C01. | Small packages are not always cohesive; many interfaces can add conceptual coupling. | Review dependency direction and change history. |
| **Encapsulation / information hiding** | Changing details are hidden behind a stable surface. | SQL/pgx is not leaking from the façade ledger or payment contract. | Don't hide the domain rules until they can be tested. | Check the exported API and use of external type. |
| **Explicitness over magic** | Important decisions and dependencies can be traced from code and configuration. | Constructor injection; route/permission inventory; transaction stated in use case. | Manual configuration duplication without synchronization is also bad. | Search the entry point → action → DB and check out wiring results. |
| **Least astonishment** | Functional/API behavior according to the expected name, type and contract. | `Post` is not secretly sending callback providers; `GetBalance` is not a mutated operation. | Don't assume the meaning of `amount` without currency, unit, or contract. | Contract tests and API names/descriptions. |
| **Fail fast on invalid setup / fail closed on authorization** | An invalid configuration stops the startup; empty scope/permissions are rejected. | Startup rejects the wrong secret/config; empty tenant context does not read. | Don't be too quick to thwart all requests because of the error of an isolated external provider. | Invalid-config, RLS, permission tests. |
| **Defensive boundaries, trusted internals** | Full validation at the trust boundary, then use domain types that keep invariant. | Parse the nominal number in the handler; `Money`/`Currency` valid in the domain; constraint fixed in the DB. | Repeated validation for any internal function can blur the design. | Input boundary and invariant tests. |
| **Composition over inheritance** | Combine functions, structures, and small interfaces rather than subtype hierarchies. | Middleware `http.Handler`, adapter provider, injected clock/port. | Imitate OOP inheritance with embedded layered structures without need. | Review API Go idiomatik. |
| **Make invalid states hard to represent** | Model transitions, types and constraints to limit unauthorized circumstances. | Status of payment validated; journal balancing in DB; amount not `float64`. | Struct doesn't guarantee all invariants without constructors, DB constraints, and tests. | Property test, invalid transition, DB check. |
| **Single source of truth** | Every requirement, scheme, and contract has an owner and status. | PRD/ADR for policy; OpenAPI for wire; migrations for schema. | Don't multiply the final rules in skill, prompt, README and easily different code. | Traceability, drift check. |
| **Principle of least privilege** | Minimum access to roles/runtime/tools and least privilege at all times. | C03 runtime DB roles can't write ledgers; CI PR has no secret prod. | Internal services are not automatically trusted; authn is not authz. | Grants tests and permission inventory. |
| **Boy Scout Rule** | When changing an area, leave it clearer without expanding the wild scope. | Rename the helper ambiguously, add bug-related reproductive tests. | Do not allow a large refactor to be unrelated to the PR settlement. | Review scope diff and fixed behavior. |
| **Rule of Three** (heuristic) | Consider abstraction after there are several independent cases and proven semantic similarities. | After several adapter providers actually have the same workflow, evaluate shared contracts. | Two cases can suffice when the constraint is clear. A single case can justify abstraction to enable testing or enforce a boundary. | Abstract justification and simple alternatives. |
| **Avoid premature optimization** | Set a target first, measure, then optimize. | Tuning index of `EXPLAIN (ANALYZE, BUFFERS)` in the representative fixture. | Do not delay safety constraints or timeout which are basic requirements. | Benchmark/profiling, baseline, regression. |
| **Keep changes reversible** | Changes to features and schemes are cut into a test phase and can be recovered. | Expandmigrate switch contract; flag for new routing. | Reversal ledgers are a business operation in their own right, not an immutable journal editing. | Migration upgrade, rollout, restore evidence. |
| **Observability as design** | Plan for runtime proof when designing important behaviors. | request correlation, ledger mismatch count, outbox age, status provider unknown. | Excessive financial/PII/secret logs violate the principle of data minimisation. | Check log schema, safe fields, smoke alerts. |

### 12.3 Applying SOLID in idiomatic Go

[SOLID](https://refactoring.guru/) comes from the tradition of object-oriented design. For Mamony Go, translate into packet, function, struct, interface, contract, and dependency direction, rather than forcing inheritance or creating an interface across all layers.

| Letter | Reviewing questions | Recommended Go implementation | Examples of violations |
|---|---|---|---|
| **S — Single Responsibility** | Did this part change for some unrelated reason? | Handler transport, use case business, repo persistence; one capability retains its invariant. | `ConfirmPayment` at the same time parse HTTP, write SQL, specify FX, format JSON, send email. |
| **O — Open/Closed** | Is there any real variation and should it be added without changing the core policy? | Small port + adapter, if the variance is proven (e.g. simulator provider with two implementations). | One big switch in the domain that imports all SDK providers; but a small switch explicitly for 2 status remains permissible. |
| **L — Liskov Substitution** | If port implementations are changed, will the result and error contracts remain valid? | Contract tests across adapters: timeout, status unknown, idempotency, cancellation, retry eligibility. | Fake providers are always successful while production adapters can provide unknowns; use cases assume all errors are safe for retry. |
| **I — Interface Segregation** | Is the consumer forced to know a method he doesn't use? | Small interface on the user side, or concrete function parameters when no interface is needed. | `AllFinancialServices` with dozens of methods imported by each module. |
| **D — Dependency Inversion** | Does business policy depend on SQL or HTTP details? | Use case depends on the contract with domain type, root/app module injects adapter. | `internal/modules/payment/internal/usecase` importing `pgxpool`, HTTP framework, or SDK vendor. |

Example of a minimum port from the consumer side (illustration, not final contract C01/C03):

```go
// The package usecase on the capability payment defines its narrow requirements.
type LedgerPoster interface {
    Post(ctx context.Context, cmd PostCommand) (PostResult, error)
}

type Service struct {
    ledger LedgerPoster
}

func New(ledger LedgerPoster) *Service {
    return &Service{ledger: ledger}
}
```

`PostCommand`/`PostResult` should use the approved type on an app-accessible contract package; This example illustrates the direction of the dependency. An adapter in the composition root implements `LedgerPoster` and translates between the payment and ledger contracts. An interface doesn't need to be created when the consumer only needs a local concrete function and there are no distinct boundaries. Go: [Code Review Comments — Interfaces](https://go.dev/wiki/CodeReviewComments#interfaces), [Package names](https://go.dev/blog/package-names).

### 12.4 Resolving conflicts between principles

| If there is a conflict | Preliminary ruling |
|---|---|
| YAGNI vs security/ledger safeguards | Requirements of integrity, audit, permission, and recovery that have been approved win. YAGNI just rejected the speculative feature. |
| DRY vs module independence | Keep small duplicates that haven't shared invariants; don't connect C01 and C03 via global `utils`. |
| OCP vs KISS | Use clear `switch` for two variations when simpler; extract strategy when the variation and behavior are actually divergent. |
| SRP vs too many files/packages | Use separate files in the same package first; new packages after the owner/test limit is clear. |
| Performance vs correctness | Prove invariant first; optimize only if you maintain semantics, atomicity, and replay. |
| Testability vs dependency injection kompleks | Select the smallest seam (consumer interface, function param, injected clock) without an IoC framework. |

**The decision of AI:** Before adding layer/interface/abstraction/pattern/dep, mention (a) the measurable problem, (b) simpler alternatives, (c) acceptance criteria that benefit, and (d) additional risks. If there's no evidence, don't add to it.

## 13. Refactoring, code smells, and change discipline

**Basis:** [Refactoring.Guru — Refactoring](https://refactoring.guru/refactoring), [Code Smells](https://refactoring.guru/refactoring/smells), [How to refactor](https://refactoring.guru/refactoring/how-to), [Catalog](https://refactoring.guru/refactoring/catalog), [Martin Fowler — Opportunistic Refactoring](https://martinfowler.com/bliki/OpportunisticRefactoring.html). These sources display indications and techniques; not commands to perform each refactor automatically.

### 13.1 Definitions and Mamony guardrails

- Refactoring changes internal structure without intentionally changing observed behavior. Bug fix and feature change are different activities; separate commit or diff if possible to make review easy.
- Protected behavior identification: HTTP status/body, authz, SQL side effects, event schema, monetary semantics, time/retry/idempotency behavior, migration compatibility. For a ledger, even seemingly insignificant internal behaviors can affect financial results.
- Write characterization/regression tests on areas that are still minimal to test; run tests before and after. Keep meaningful snapshots of results; don't use large unvalidated snapshots as the only evidence.
- Make a small change, run `go test` in the affected package, and then check broader risks. Follow up with `gofmt`, `go vet`, lint, and integration tests according to the change class.
- Document behavior changes as decisions or features with their own acceptance criteria.

### 13.2 Code smells, refactorings, and practical limits

| Smell or signal | Candidate engineering | Example Mamony | Don't automate when.. |
|---|---|---|---|
| Long function / deep nesting | Extract function; guard clauses; breakdown of business phase | `Confirm` menyatu validasi, state transition, persistence, response mapping | Break one transaction into a function that hides the boundary commit/rollback. |
| Large package / multiple reasons to change | Move function; extract cohesive package | HTTP DTO and SQL queries are mixed in `payment` | There's a lot of files, but it's still one cohesive business model. |
| Primitive obsession | Introduce domain type; validate constructor | `int64` nominal without currency, `string` any for state or ID | Just an alias with no invariants and instead blurred wire formats. |
| Duplicate *knowledge* | Extract shared rule in owner | A formula for FX rounding is used in quotes, settlements and test oracles | The same code has a different lifecycle/domain owner. |
| Long parameter list | Parameters of small objects, if cleared | `Post(ctx, account, amount, currency, reference, ...)` | Hiding all the dependencies in a map/generic bag. |
| Feature envy / domain logic in the handler | Move logic to the domain/usecase | Handler calculates debits and credits himself | Pure transport mapping is the handler's job. |
| Shotgun surgery | Set the owner to a single policy | One change of permission and need to edit eight unrelated files | Many contracts do have to be explicitly changed because of compatibility. |
| Speculative generality / unused abstraction | Inline/delete dead code, deprecate aman | Generic plugin provider that doesn't have a second provider | There's a real security requirement or extension point. |
| Excessive indirection / middleman | Inline layer or narrow down API | Handler → manager → facade → service → interactor → repo without different functions | The layer is the actual boundary of authorization/transaction or contract. |
| Mixed error handling | Introduce classification at one boundary | SQLSTATE, not-found, retryable and provider-unknown equated to HTTP 500 | Specific mapping per boundary is needed. |
| Hidden mutable global state | Inject dependency or create explicit ownership | `var pool *pgxpool.Pool`, mutable settings, clock global | Static value is immutable and completely without lifecycle. |
| Concurrency without owner | Simplify to synchronous, add cancellation/owner | The routine of the handler processes settlement after a request is completed without durable warranty | There's a job runner/queue/outbox with a clear shutdown and retry. |

### 13.3 Safe refactoring sequence

```text
1. Identify: name the smell, impact, boundary and risk.
2. Characterize: record the behavior to preserve, the baseline tests, and the contract sources.
3. Plan: select the smallest transformation plus rollback.
4. Change: one structural change; avoid changing the domain rules.
5. Verify: run targeted go test checks and any affected integration, contract, or concurrency tests.
6. Review: diff behavior, import graph, DB side effects and relevant performance.
7. Record: record the preserved behavior and evidence in the PR. Update documentation when paths change.
```

**Examples of decisions:** If `confirm.go` has 250 lines, the number 250 is not evidence of foul play. Check if there are different responsibilities, cognitive load, test requirements, and whether extraction hides the transaction. On the contrary, a 40-line file that enters `float64` into the ledger is a serious problem even if it's short.

### 13.4 Refactoring across database and API changes

- Change the name or move a Go function in one package: usually class S if the behavior is fixed and the external contract remains unchanged; upgrade class if it touches permission or transaction path.
- Move business logic from the handler to the usecase: test HTTP contracts and domain errors; make sure you don't change the semantics of the response.
- Move the repository or SQL: testing SQLSTATE, isolation, row lock, `Rows.Close`, `rows.Err`, transaction ownership, RLS, connection reuse, and runtime grants.
- Changing public API/events/DB column: not refactoring purely if the consumer can observe the change. Use a compatibility contract and a rollout/migration plan.
- Refactor critical ledger: run invariants, replay, concurrency, rollback, verifier and outbox tests before claiming behavior-preserving.

## 14. Design patterns for idiomatic Go

**Sources:** [Refactoring.Guru — design patterns](https://refactoring.guru/design-patterns), [Go examples](https://refactoring.guru/design-patterns/go), [Go package design](https://go.dev/blog/organizing-go-code). Use design patterns to name solutions to problems that already exist. The object/class-based example in reference needs to be adapted to the Go composition. Distinguishing Gang of Four (GoF) from a distributed/enterprise system architecture pattern.

### 14.1 GoF patterns that may be useful early

| Pattern | Category | A real need that triggers | Application to Go and Mamony | Risk of overuse |
|---|---|---|---|---|
| **Adapter** | Structural | The C03 contract needs semantics distinct from the C01 API or C05 SDK. | The structure/function adapter in `internal/app/adapters` translates the DTO, error, and result. | Assume every function call needs an adapter. |
| **Facade** | Structural | The subsystem needs a small, stable surface. | `ledger.New(...); ledgerModule.Post(...)` without leaking an internal repo. | Add a tiered façade without boundary rules. |
| **Strategy** | Behavioral | There are some real policies with similar contracts: e.g. FX quote selection or failure mode simulator. | Function type or small interface in consumer; tested as contract. | Creating a registry/plugin engine while implementing one. |
| **Decorator** | Structural | Cross-request behavior: metrics, tracing, logging, auth gate. | `http.Handler` packaging or port implementation with explicit wrapper. | A series of wrappers hide the transaction context or record confidential data. |
| **State** | Behavioral | State payments and transitions are quite complex. | Start with enum + explicit transition function/table + exhaustive tests; object-state only when assigning a value. | One class/struct per status for a simple workflow. |
| **Chain of Responsibility** | Behavioral | HTTP middleware sequences with clear responsibility. | Authentication, body limit, request ID, authorization according to the route policy. | The middleware sequence is wrong so authz can't be executed. |
| **Factory function** / Factory Method sederhana | Creational | Construction needs config validation, dependency, or resource initialization. | `NewService(deps)` or `NewAmount(...) (Money,error)`; not the creator hierarchy of Java. | Factory for every struct without invariant. |
| **Command** | Behavioral | Job/intent needs to be represented in a durable and replayable way. | Struct command + handler; durable commands are only complete if stored in the AC-compliant DB/outbox. | Replace events, commands and transactions with a generic message bag. |

### 14.2 Other patterns and when to consider them

| Pattern | Mamony's current position | When relevant |
|---|---|---|
| Builder | On demand, often enough struct config + `New` validation. | Request/config creation is actually a lot of combinations and there's sequential validation. |
| Abstract Factory | Tunda. | Several adapter families are actually changed together (e.g. one provider provides a client, parser, verifier). |
| Prototype | Tunda. | Cloning expensive/complex objects that appear in profiling, instead of copying simple Go struct. |
| Singleton | Avoid the global mutable singleton; select instance per app in composition root. | The resource of one instance per runtime can still be managed explicitly via constructor. |
| Bridge | There's usually no need for a double hierarchy. | Two independent variations have been realized. |
| Composite | On demand. | Policy tree or recursive rule group. |
| Flyweight | Wait until the profiling. | Data objects are very abundant with a shared immutable state. |
| Proxy | On demand, review security. | Auth/cache/remote access proxy with clear semantics; do not bypass resource authorization. |
| Observer | Watch your step. | Local non-durable events can use callbacks; durable business events use outboxes and deductions, not in-memory callbacks. |
| Iterator | Usually use the `for`/slice/DB cursor with an explicit lifecycle. | Traversal collection is complex and stable. |
| Mediator | Rarely; `app` composition is not a universal domain mediator. | Many components need centralized coordination and real ownership. |
| Memento | Jarang. | Checkpoint states that have size, retention, and semantics are clear; not a replacement for audit ledgers. |
| Template Method | Go doesn't use the inheritance of a GoF class; function/callback composition is often clearer. | Algorithms along with a few steps that do vary. |
| Visitor | Jarang. | Many operations for a stable node/AST structure; not the default fintech domain model. |

Refactoring lists. The Go teachers contain creative, structural, and behavioral patterns, but this catalogue is prioritized rather than porting all the patterns literally. `Repository`, `Unit of Work`, `Transactional Outbox`, `CQRS`, `Saga`, `Circuit Breaker`, and `Bulkhead` are discussed as enterprise/system patterns, not GoFs that should mimic OO classes.

### 14.3 System patterns and initial decisions

| Pattern / approach | Default Mamony | When to use | The main evidence |
|---|---|---|---|
| **Repository** | Yes, the adapter access persistence per owner capability. | SQL needs to be separated from use case policy. | PostgreSQL integration, grants, query/explain when needed. |
| **Unit of Work / Transaction Boundary** | Yes to some write that must be atomic in capability. | Use case needs to combine write and outbox. | Commit/rollback, cancellation, failure tests. |
| **Transactional Outbox** | Yes, for an event that must be durable according to ADR. | DB write should generate an event without the non-atomic dual-write. | The event is present only after commit; retry/dedupe/ordering. |
| **Idempotent Consumer / Inbox** | Yeah, when there's an at-least-once delivery. | The event can be recycled. | Duplicate delivery doesn't have a double effect. |
| **CQRS** | Lightweight read/write separation if complexity demands; don't build two platforms. | The statement/read model is very different from the post immutable. | Consistency and lag read models are measured. |
| **Saga / Process Manager** | Candidates with multi-step external side effects can't have a single transaction. | Payment orchestrates provider unknown, compensation meaning domain. | Failure matrix, reconciliation, timeout and restart. |
| **Circuit Breaker** | Not the baseline for a local simulator. | Downstream remote repeatedly fails and there's a correct fallback/fail fast policy. | Load test, failure simulation, retry budget. |
| **Bulkhead / Worker pool** | On demand with an explicit concurrency limit. | Provider/job overload interferes with other requests. | Queue depth, isolation, goroutine lifecycle. |
| **Cache-Aside** | Delayed for authoritative balance data. | A hot read whose stale tolerance is explicit, key scope correct, invalidation testable. | Consistency/staleness contract, benchmark. |
| **Event Sourcing** | Don't automatically match an immutable ledger with the entire event-sourced app. | There's a need to replay events as the primary state of all aggregates. | Cost model and migration policy. |
| **Hexagonal/Clean Architecture** | Use dependency boundaries, not rigid directory templates. | It needs to separate the core business from the adapter, already filled with modular monoliths. | Import checks, tests, owner graph. |

### 14.4 Template for a pattern decision

```text
Problem: [Real changes that are hard to handle without patterns]
Simplest alternative: [switch/function/struct biasa]
Selected pattern: [name, how to reduce coupling/risk]
Cost: [new types, indirection, tests, ownership]
Proof: [test showing behavior and boundaries]
Revisit trigger: [conditions that change choices]
```

Use a small Go function when it solves the problem. Verify the invariants regardless of whether the design uses a named pattern.

## 15. System design for a financial modular monolith

**Learning references:** [roadmap.sh System Design](https://roadmap.sh/system-design), [roadmap.sh Backend](https://roadmap.sh/backend), [PostgreSQL isolation](https://www.postgresql.org/docs/current/transaction-iso.html), [Go concurrency cancellation](https://go.dev/blog/pipelines), [Google SRE Workbook](https://sre.google/workbook/). The roadmap is a coverage of competence, not evidence that Mamony requires any large-scale component.

### 15.1 Start system design with requirements and failure modes

For each M/L-class boundary/feature, answer briefly:

1. **Functional:** Who is the actor, outcome, command/query, state transition, simulation funding source, owner capability?
2. **Non-functional:** correctness, latency, throughput, concurrency, availability, security, cost, operability, recovery. Unmeasured figures are written as TBD and not presented as actual SLO.
3. **Data:** source of truth, schema, constraints, indexes, migrations, retention, tenant scope, and lifecycle.
4. **Consistency:** When is the state considered committed, who can read out stale data, what invariants cannot be temporarily violated?
5. **Failure:** timeout, duplicate, out-of-order, partial commit, unknown outcome, retry, operator action, restore.
6. **Interfaces:** HTTP/event/query contracts, ownership, authz, versioning, error mapping.
7. **Operations:** metrics/log/trace, alert, diagnostics, rollback, manual recovery.
8. **Trade-off:** At least 1 alternative is simpler; reasons for rejection; cost and revisiting the trigger.

A context diagram (actor → app → DB/simulator), container / process (two listener + jobs / relay), capability graph, then sequence for happy path and failure path. Don't build deployment diagrams that don't match real runtime.

### 15.2 System design fundamentals and Mamony policy

| Concept | The core question | Applications and guardrails |
|---|---|---|
| **Latency vs throughput vs concurrency** | Is the problem slow response, workload per second, or contentment? | Set a budget per dependency; measure p50/p95/p99 and queue at representative loads; don't just rate the average. |
| **Availability vs consistency vs durability** | What happens if the node dies, the message is delayed, or the connection breaks down? | The financial state ledger should not rely on eventual cache as truth. Recovery needs to restore logs and verifiers. |
| **ACID / isolation / locks** | Write where should atomic and can race? | Transaction C01, unique constraint idempotency, lock ordering, deadlock/serialization retry according to the DB invariant. |
| **CAP and PACELC** | What are the assumptions about partitions and replication? | Learn trade-offs for distributed systems; it doesn't mean choosing CP/AP labels for one PostgreSQL and ignoring transaction design. |
| **Scaling vertical/horizontal** | Where's the measured bottleneck? | Start a single process + PostgreSQL; multi-instance only if the session, relay ownership, migration, pool budget, and idempotency are secure. |
| **Sync vs async** | Does the caller have to wait for side effects? | Local DB atomic synchronous; event durable via outbox; provider unknown needs state/reconciliation, not routine fire-and-forget. |
| **Queues / delivery semantics** | Are there any duplicates, changes in order, poison messages? | Assume at least once during relay, dedupe consumer, retry bounded, dead-letter/replay policy if real need be. |
| **Retry, timeout, cancellation** | Is the retry safe? What if the result is remote unknown? | `context.Context`; retry only error safe + idempotency, backoff/jitter and overall deadline; do not retry ambiguous debit provider without inquiry/reconciliation. |
| **Backpressure / rate limit** | What if the traffic exceeds DB/provider's capabilities? | Connection pool budget, bounded workers, overload response; prioritize a clear queue over an unlimited spawn routine. |
| **Caching** | Can data stale and how invalidation? | Do not cache authoritative balance for posting decisions; cache static configuration/FX read when freshness contract is approved. |
| **Partitioning/sharding/replication** | Does the data really go beyond one DB? | Delay; complicate financial transactions and reconciliation. Measure first. |
| **Observability / SLO** | How do you know a system is technically and commercially sound? | Monitor request error/latency, queue/outbox lag, provider unknown, imbalance/reconciliation, retries, restore. |
| **RPO/RTO and restore** | What data can be lost? How long to recover? | Determine the target by need; test restore to an isolated instance with a verifier, not just check for backup files. |
| **Blast radius / graceful degradation** | If the FX/provider simulator fails, what's safe? | Fail closed on authorization and ledger operations. Keep independent status and read endpoints available. Report the actual operation outcome. |

### 15.3 Required failure-mode analysis for money flows

| Point of failure | The risks | Expected design response | The proof |
|---|---|---|---|
| Double request at the same time, same key | Double charge/post | Unique constraint + one replay result according to scope key | DB integration 2+ connection + same-key. |
| Same key, different payload | Reuse is misleading | Reject conflict without changing journal | Fingerprint/unique test. |
| The dead process before commit | Some of the stored data | DB atomic rollback, the outbox does not appear | Fault/transaction test. |
| The responses disappear after commit | The caller repeats | Replay returns committed results without posting again | Simulate the lost response. |
| Outbox relay crash after publishing | Event accepted twice | At-least-once + consumer dedupe | Redelivery test. |
| Provider timeout/unknown | Repeated external transaction errors | Status `unknown/pending`, inquiry and reconciliation | Simulator failure injection. |
| Two posts wrestling balance floor | Negative balance | Lock/constraint/serialization DB, transaction retried safely when correct | The Concurrency DB test is separate from the Go race. |
| Tenant context lost/reuse pool | Cross-tenant leak | Transaction-local RLS and runtime role deny-by-default | Tests tenant A/B/empty + pool reuse. |
| Migration stopped in part | Schema inconsistent | Versioned migration, transactional DDL when possible, staged rollout | Fresh+upgrade test, recovery runbook. |
| Restore from backup | Journal/account mismatch | Restore isolated and recompute verifier | Restore drill with posted journal. |

All final details follow PRD/ADR/TD Mamony; the tables are design review scenarios, not feature claims Go already running.

### 15.4 Frequently overlooked backend fundamentals

| Area fundamental | The minimum that AI needs to understand | Practical check or exercise |
|---|---|---|
| **HTTP/REST semantics** | Method safety/idempotence, status/error, headers, body limits, pagination, content type, caching rules. | `httptest`, OpenAPI contract tests, invalid JSON and duplicate requests. |
| **Networking** | DNS, TLS, timeouts, connection reuse/pooling, request cancellation, failure TCP vs application errors. | Simulated slow/hung provider, TLS errors and shutdown. |
| **Authentication vs authorization** | Caller identity ≠ permission on resource; explicit principal and tenant isolation. | Unauthorized access to another tenant's resource. |
| **SQL & relational modeling** | PK/FK/unique/check, transactions, isolation, MVCC, row locks, indexes, query plans. | `EXPLAIN`, concurrency test real Postgres, migrations. |
| **Data representation** | integer minor units, overflow, currency exponent, timezone UTC, encoding/normalization, IDs, stable ordering. | Edge-case parser, currency mismatch, timestamps near boundaries. |
| **API and event evolution** | Compatibility, schema version, deprecation, consumer-driven risk, backward/forward handling. | oasdiff/contract fixture, event replay with supported schema. |
| **OS/runtime** | Process signals, file descriptors, memory, CPU, goroutines, garbage collection, resource limits. | Graceful shutdown, leak/race tests, load and profiling when needed. |
| **Build & source control** | `go.mod`/`go.sum`, semver, reproducible build, Git discipline, bisectable commits. | Clone-clean `make check`, pinned toolchain, `go mod verify`. |
| **Logging and diagnostics** | Structured logs, correlation, metrics, traces, no PII/secret leakage. | Failure drill produces actionable logs without sensitive fields. |
| **Testing strategy** | Unit vs integration vs contract vs race/fuzz/load/recovery, deterministic fixtures. | Risk-specific evidence matrix; no blanket 100% coverage target. |

### 15.5 Brief design template for M and L changes

```markdown
# [Feature / risk] Design Brief
Owner capability: Cxx | Risk: M/L | Status: Proposed
AC / source links: [...] | Scope / excluded: [...]
Actor → entry point → use case → module owner → DB/outbox/provider: ...
Invariant: ... | Consistency boundary: ...
Data schema/grants/migration: ...
API/event and compatibility: ...
Timeout/retry/idempotency/unknown outcome: ...
Failure matrix + recovery: ...
Observability and rollout: ...
Alternatives and reasons: ...
Verification commands/tests: ...
Open decisions requiring owner approval: ...
```

**YAGNI for system design:** load balancer, CDN, Redis, message broker, Kubernetes, multi-region, sharding, distributed transactions, custom circuit breakers, and service mesh is not a requirement of the foundation Mamony. They become candidates only after the requirement and measurements show benefits beyond complexity.

## 16. DevSecOps, security engineering, and operational fundamentals

**References:** [roadmap.sh DevSecOps](https://roadmap.sh/devsecops) as a competence map, [NIST SSDF SP 800-218 v1.1 (final)](https://csrc.nist.gov/pubs/sp/800/218/final) as a secure SDLC practice, [OWASP ASVS 5.0.0](https://asvs.owasp.org/) as a set of application verification requirements, [GitHub supply-chain security](https://docs.github.com/en/code-security/concepts/supply-chain-security), and [Go vulnerability management](https://go.dev/doc/security/vuln/). It extends SEC/OPS §3 and CI §10; don't make a second pipeline that duplicates the same gate.

### 16.1 Secure-by-design and shift-left policy

| Stage of SDLC | Minimum practice | Evidence | Impact class |
|---|---|---|---|
| **Requirements** | Define the asset, actor, trust boundary, data classification, abuse case and acceptance security. | PRD/TD + security AC. | All M/Ls must be deep. |
| **Design** | Lightweight threat modeling: spoofing, tampering, repudiation, information disclosure, DoS, privilege escalation; control comparison prevent/detect/recover. | Data-flow + threat-to-test mapping. | I auth/ledger/external is mandatory. |
| **Code** | Parameterized SQL, output encoding according to the media, typed config, validations, context deadlines, secure secrets, no hardcoded credentials. | Lint/source review, unit/contract tests. | All of them. |
| **Dependencies** | Update/inventory direct and transitive deps, `go mod verify`, vulnerability scan with triage exploitability. | SBOM or dependency list, govulncheck, reasonable exceptions. | All the dependency changes. |
| **CI** | Least-privilege workflows, SHA pin action, secret scanning according to availability, artifact integrity. | CI logs, permission review, action diff. | All of them. |
| **Build/deploy** | Reproducible-ish pinned toolchain, non-root image, minimal runtime, env isolation, immutable image digest. | Image scan/signature/provenance if the stage is ready, deploy record. | Stage-produced simulator. |
| **Runtime** | TLS, access control, observability safe fields, rate limiting, resource/time limits, backup/restore. | Security integration, smoke, runbook. | According to exposure. |
| **Respond** | Patch/rollback, revoke credentials, incident timeline, postmortem, dependency re-scan terjadwal. | Drill/log incident, restore verification. | Release/operation. |

Verify object-level authorization, amount matching, and prevention of double posting with policy review and domain-specific tests. For ASVS claims, pin the ASVS version, select control IDs, and demonstrate coverage.

### 16.2 Threat review for Mamony

| Asset/boundary | Examples of threats | Primary prevention | test negative |
|---|---|---|---|
| Public API → payment | Caller memanipulasi amount/reference; replay key | Authn/Authz, validation, scoped idempotency, rate budget | Invalid amount, key reuse with different body, unauthorized resource. |
| Private listener → operator | Privilege escalation or maker-checker bypass | Listener network policy + independent authorization + approval bound to exact payload | Approved is the same as maker; payload changed after approval. |
| Application → PostgreSQL | SQL injection, wrong tenant, excessive grants | Parameterized SQL, runtime role, transaction-local RLS, least privilege | RLS empty context, tenant B read, forbidden schema writes. |
| Ledger posting | Rounding/overflow, double post, race | Integer minor units, checked arithmetic, DB uniqueness/atomic transaction | Max amounts, concurrency, cancellation, rollback. |
| Outbox/consumer | Replay, reordering, poison message | Versioned envelope, dedupe, ordering policy, bounded retries | Duplicate/out-of-order event, retry exhaustion. |
| Provider simulator | Unknown outcome, forged webhook, unbounded retry | Authenticate/scope callback if endpoint exposed; idempotent updates, inquiry/reconcile | Forged callback, timeout after remote effect, duplicate callback. |
| Build agent/CI → repository | Malicious skill/plugin, poisoned dependency, secret exfiltration | Pin tools, review third-party scripts, restricted tokens, safe secret handling | Validate permissions and untrusted-input handling. |

ADR-0008 defines this system as a simulator. Do not make a connection to real providers or store real bank/account data to test threat scenarios.

### 16.3 Minimum quality gates by layer

| Gate | Automated or manual | Scope | The Rules |
|---|---|---|---|
| `gofmt`, `go vet`, `golangci-lint` | Automated | Style, bug patterns, some security checks | Required PR baseline, false positive drives for a reason. |
| `go test`, race, DB integration | Automated | Invariant semantik, goroutine race, SQL concurrency | Race and DB concurrency evidence are different. |
| OpenAPI/contract + dependency graph | Automated | API drift, layer/owner import | Mandatory if boundary exists; not declaring contract valid only from build. |
| `govulncheck`, dependency review | Automated + human triage | Advisory & reachable code | Scanning schedule, exception procedures and upgrades. |
| Secret scan, CI permissions, image security | Automated checks and review | Repository/CI/runtime packaging | Activate when capabilities and stages are relevant; do not claim native tools are available on private plans without verification. |
| Domain + threat model review | Humans are aided by AI | Authz, ledger, recovery, error ambiguity | M/L must be at risk; AC→test evidence. |
| Restore/DR, load, chaos/fault injection | Operational testing | Recovery and behavior when dependency fails | Stage by blast radius, not every typo edit. |

### 16.4 Basics that are costly to overlook

- **Time semantics:** use UTC for storage, timezone only on display; inject clock on test; define timestamp of event vs processing time, and retention based on clear timezone.
- **Numeric semantics:** minor units of type; do not use binary floating-point for posting; `int64` still needs overflow checks and clear currency exponents; FX rounding follows ADR named.
- **Error taxonomy:** classify validation, authorization, not found, conflict, retryable, and unknown outcome; do not display internal stacks/SQL in public responses.
- **Resource hygiene:** shut down body/rows, check `rows.Err()`, defer rollback with a clear lifecycle, set HTTP/server/client timeout, pool budget, shutdown goroutine/worker.
- **Determinism:** Tests are not dependent on a wall-clock, locale, random unseeded, internet provider, or random race scheduling.
- **Data ownership:** there is no cross-schema SQL shortcut; query cross-capability through the façade/port/read model is approved.
- **Schema evolution:** The migration that has been released is immutable, test fresh/upgrade, staged backfill, index builds and lock budget explained.
- **Dependency hygiene:** Minimize new libraries and transitive risks; review licenses, maintenance, binary/build size, and supply chain where relevant.
- **Operational documentation:** Quickstart of clean clones, safe env example, make targets, testable runbooks and clear owner decisions.
- **Developer ergonomics:** actionable error messages, concise logs, small test fixtures, `gopls`/symbol search before reading the entire repository.

## 17. Using the engineering principles in AI-assisted coding

### 17.1 Automated checks and engineering judgment

| Rule | Can CI check it? | How to erect |
|---|---|---|
| Formatting, lint, vulnerable dependency | **Yes** | §10 toolchain is pinned. |
| Import boundary and forbidden package | **Yes** | Import the graph checker + tests for positive/negative fixtures. |
| HTTP/event contract & schema compatibility | **Most of it** | OpenAPI/event validation, contract tests. |
| Ledger invariants/idempotency/RLS | **With executable tests** | DB integrations, concurrent fault tests. |
| Refactoring behavior-preserving | **Partly automated** | Regression/contract tests + diff review. |
| SRP, KISS, DRY, YAGNI, correct pattern | **Not entirely** | Review of real change, reason for the formation of abstractions, minimal alternatives. |
| Proper trade-off / business semantics | **Human decisions** | Requirement/ADR owner, failure matrix, risk evidence. |

### 17.2 Suggested short addition to `AGENTS.md`

```markdown
## Engineering design rules
- Correctness, financial invariants, authorization, and recovery outrank brevity.
- Follow KISS/YAGNI: implement only approved behavior; avoid speculative frameworks,
  repositories, interfaces, factories, caches, brokers, and generic abstractions.
- Apply SOLID as Go dependency boundaries, not class hierarchies. Prefer small
  consumer-owned interfaces when a genuine seam is needed.
- Preserve module ownership; domain/usecase may not import concrete HTTP/DB adapters.
- Before adding a design pattern or dependency, state the problem, simplest alternative,
  benefit, cost, and how to verify the decision.
- Refactors must preserve observable behavior; run targeted regression/contract checks.
- For finance/auth/data changes, enumerate failure, duplicate, concurrency, and recovery cases.
- Report actual checks as PASS / FAIL / NOT RUN. Do not infer proof from code inspection.
- Read relevant headings in docs/standards/engineering-principles.md on demand;
  never paste the full handbook into every task context.
```

### 17.3 Routing to one relevant review skill

Add `mamony-engineering-review` as a skill on demand that is active when there's an architecture/refactoring/pattern decision, including when AI plans to add dependency/package. This skill checks the problem → alternative → chosen design → evidence → risk, then calls `mamony-boundary-review` or `mamony-ledger-review` only if the module/risk really touches it. Do not load the entire §12–§16 in the context of each request.

### 17.4 Short review checklists by change class

| Classroom | Basic design review | Additional evidence |
|---|---|---|
| **S** | Small scope, clear naming, not adding abstractions without need, targeted tests, lint. | If the behavior is unaffected: show relevant tests. |
| **M** | AC, module owner, SOLID/SoC, alternatives, HTTP/DB contract and failure path. | Integration for boundary changes; migration/API drift if any. |
| **L** | All the M+ sources of truth, trust boundaries, recovery, idempotency, transaction/lock, data retention, roll-forward. | Real DB concurrency, fault injection, security review, restore/runbook/reconciliation when relevant. |

### 17.5 Small decision record to prevent over-engineering

Before creating a new dependency, package, pattern, interface, cache, broker or worker, the AI must fill in (maximum 5 lines except L):

```text
Need/AC: ...
Simplest valid design: ...
Why it fails / where it creates risk: ...
Chosen change + what it costs: ...
Proof and revisit trigger: ...
```

If the simplest valid design already meets the acceptance criteria and addresses the risks, use it. Don't expand the design just so that the entire roadmap/GoF catalog is used.

### 17.6 Source map and limits of interpretation

| Sources | What are you wearing? | Limitations of application |
|---|---|---|
| [Refactoring.Guru — Design Patterns](https://refactoring.guru/design-patterns), [Go examples](https://refactoring.guru/design-patterns/go) | Vocabulary, intent and trade-off patterns. | The GoF pattern is not a necessity. The OO sample was adapted, not copied raw. |
| [Refactoring.Guru — Refactoring](https://refactoring.guru/refactoring), [Code Smells](https://refactoring.guru/refactoring/smells) | Small change procedures, testing and smell heuristics. | Smells are not automatic bugs; behavioral changes require specs. |
| [roadmap.sh — System Design](https://roadmap.sh/system-design) | Map the topics of scale, reliability, consistency, infrastructure. | Not a product requirement or recommendation to install the entire component. |
| [roadmap.sh — Backend](https://roadmap.sh/backend) | Curriculum for HTTP, APIs, servers, databases, security, and testing. | Priorities are selected from the scope Mamony. |
| [roadmap.sh — DevSecOps](https://roadmap.sh/devsecops) | SDLC + security lens from code to operation. | For concrete controls, use NIST/OWASP and evidence CI. |
| [Go project — Effective Go](https://go.dev/doc/effective_go), [Review Comments](https://go.dev/wiki/CodeReviewComments) | Idioms, names, errors, interface and packaging. | Not all old advice includes all of the latest Go features; reviews on a pined version of the toolchain. |
| [Martin Fowler — YAGNI](https://martinfowler.com/bliki/Yagni.html) | Delaying speculative capability while facilitating change. | Not permission to eliminate the current requirement or integrity control. |
| [OWASP ASVS 5.0.0](https://asvs.owasp.org/), [NIST SSDF SP 800-218 v1.1 final](https://csrc.nist.gov/pubs/sp/800/218/final) | Requirement security verification and secure SDLC. | Select control IDs/specific versions; do not make a compliance claim without an audit. |

**Summary of application:** Apply §12 to every task and select the relevant material from §§12–16. Use CI to enforce testable rules and let human reviewers judge trade-offs. Prioritize correctness under failure.
