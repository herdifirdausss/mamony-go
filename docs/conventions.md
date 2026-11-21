# Go conventions and golden paths

This is a **project-specific implementation contract** for new Mamony Go capabilities, not a universal rule for every Go project. Use the [development lifecycle](development-workflow.md) to approve requirements before writing code, the [architecture](architecture.md) for ownership, and the [testing strategy](testing.md) for verification. Existing architecture checks are enforced by `make architecture`; review rules that cannot yet be enforced automatically.

## Standard capability shape

Introduce only the folders needed by the first accepted vertical slice. Do not pre-generate unused handlers, interfaces, service registries, helpers or empty files.

```text
internal/modules/<capability>/
  module.go                     # exported facade: commands/queries made available to app
  internal/
    domain/                     # aggregates, value types, invariants; no I/O
    usecase/                    # orchestrates domain and consumer-owned ports
    repository/postgres/        # SQL adapter for records owned by capability
    handler/httpapi/            # external contract, validation, mapping to usecase
    handler/worker/             # only when a background job exists
internal/app/                   # composition: wires accepted dependencies
internal/platform/              # low-level reusable technical primitives only
```

A capability can omit an adapter or use feature-specific folders beneath an existing layer once the feature is real. Do not create a `service` and a `usecase` layer for identical responsibilities. Do not create a `utils`, `common` or `shared` capability to bypass ownership; use narrowly scoped types/ports and justify a truly shared primitive.

| Layer | MAY own | MUST NOT do |
|---|---|---|
| domain | Money values, policy decisions, pure state transitions and errors | Import transport, SQL, Redis, queues, configuration/global runtime |
| usecase | A named business operation, orchestration and minimal consumer-owned ports | Import HTTP/framework/DB clients or concrete repository adapters |
| repository | One owner's schema queries and local transaction mechanics | Authorize based on client-supplied tenant alone; access another capability's tables by accident |
| handler | Decode/validate transport, authenticate context, call usecase and map errors | Direct SQL/ledger mutation, bypass ownership or store secrets in errors |
| capability facade | Expose stable operations to application wiring | Export private internals or leak database/HTTP-specific types into domain |
| app | Construct dependencies, register routes and lifecycle | Own business invariants or reach into nested capability internals |
| platform | Reusable transport/config/storage mechanics with explicit boundaries | Import capabilities or make business authorization decisions |

`tools/archcheck` enforces cross-capability, layer and selected forbidden **production** imports. Test-only imports can use integration fixtures. Static rules do not yet prove all of the business obligations above; reviewers must trace the actual data flow.

## Go style and deterministic behavior

- Packages are short lowercase nouns, no underscores or artificial `Manager`/`Helper` layers. Name files by **feature/responsibility**, not generic numbered versions; rename on real ownership changes.
- Prefer Go standard library, explicit constructors and narrow interfaces owned by the consumer. Avoid framework-wide dependency injection, global mutable registries and reflection-heavy patterns unless tested requirements justify them.
- Accept `context.Context` first for request and I/O operations; honor cancellation and deadlines, but never treat cancellation as proof an external money operation failed. Use injected clock/ID generator when timing/identity affects behavior.
- Return errors with stable classification at the owning boundary; wrap with `fmt.Errorf("operation: %w", err)` when context helps. Log once near an operational boundary. Do not return SQL details, secrets, internal stacks or personal information to a caller.
- Model monetary amounts as **integer minor units and currency**, never binary float. Prevent overflows, inconsistent currency, duplicate financial effects and invalid state transitions in domain and database constraints.
- Keep database and network calls outside the same transaction. The repository owns atomic changes to its capability, including outbox and idempotency rows. Unknown provider outcomes remain uncertain until inquiry/reconciliation.
- Never construct `*http.Client` without bounded transport/timeout policy for external calls; never trust arbitrary callback headers or user-supplied destinations. Follow the existing [HTTP](adr/0003-public-http-contract.md), [provider](contracts/provider.md) and [PostgreSQL](contracts/postgresql.md) contracts.
- Do not `panic` for expected business errors or use `recover` to conceal invalid state. If goroutines are introduced, define owner, cancellation, bounded concurrency, result/error propagation and shutdown policy. A process crash is not an acceptable error-handling strategy.
- No `TODO` that hides an unimplemented safety requirement behind a successful return. Do not disable linters, skip tests or lower CI gates to achieve green. Narrow, documented exceptions require independent review.

## Reusable feature implementation sequence

1. Confirm accepted AC IDs in the owning `docs/contracts/` file and all relevant architecture/decision references.
2. Write table-driven **domain tests** for invariant, boundary and denied cases. Implement domain behavior with no infrastructure dependency.
3. Define usecase operations and minimal ports for required side effects. Unit-test success and failures with explicit fakes; do not test persistence behavior only against mocks.
4. Add repository migrations and integration tests against **real** PostgreSQL runtime grants, tenant transactions, concurrency and rollback when used.
5. Add handler/provider adapters with strict input validation, authorization checks and contract tests. Confirm generated API/event documentation matches runtime.
6. Run targeted tests, `make check`, `make test-repeat`, `make race`, lint/security checks and required DB/runtime suites. Record exact `PASS`, `FAIL`, `NOT RUN`.
7. Review minimal diff, owner boundaries, AC-to-test evidence, error paths, deployment/recovery consequences and unchanged security guarantees before merge.

Use the [test-plan template](templates/test-plan.md) as a checklist in a PR or proposal, not as a required extra document for every change.

## Introducing a new rule

When a real agent error recurs, first improve an API or package boundary to make it impossible. Otherwise add a narrow deterministic check with **positive and negative tests**, an actionable error and a justified scope. Add prose only for context that a tool cannot enforce. Never ban a valid Go practice across the whole codebase solely because one instance was wrong.

References for conventions: [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments), [Go Test Comments](https://go.dev/wiki/TestComments), and the [agent-safety guidance](https://cheatsheetseries.owasp.org/cheatsheets/Secure_Coding_with_AI_Cheat_Sheet.html). These references inform the local rules above; they do not make any proposed control implemented.
