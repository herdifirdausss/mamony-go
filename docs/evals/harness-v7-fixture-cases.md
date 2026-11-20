# Harness v7 — small mechanical benchmark (historical design)

Source: uploaded Mamony Engineering Harness v7 `docs/evals/benchmark.md`. This is a **separate set of harness fixtures**, not the E01–E12 *financial workflow* benchmark in [v6](mamony-agent-benchmark.md). Its E01–E08 labels are scoped to **v7-harness**, to avoid interpreting matching IDs as equivalent tasks.

| v7 harness case | Required local behavior | Current verification status |
|---|---|---|
| H07-E01 | Forbidden module import is rejected | Architecture policy unit test covers the rule; deliberate source mutation is NOT RUN on this PR |
| H07-E02 | Valid app adapter and local usecase imports are accepted | Architecture policy unit test, not a complete adapter integration |
| H07-E03 | Unbalanced debit/credit rejected | Existing pure ledger unit test |
| H07-E04 | Minor-unit total overflow rejected | Existing pure ledger unit test |
| H07-E05 | Public listener does not expose private readiness | Existing HTTP route isolation test |
| H07-E06 | Concurrent duplicate ledger Post yields one durable posting | BLOCKED: PostgreSQL C01 Post not implemented |
| H07-E07 | Unknown provider outcome cannot cause blind resend | BLOCKED: provider simulator not implemented |
| H07-E08 | Tenant A cannot access B after pooled connection reuse | BLOCKED: runtime role/RLS not implemented |

Run controlled baselines at the same commit, agent model/effort, issue brief, fixtures and test harness. Record hard-gate outcomes, actual elapsed time, reviewer corrections, input/output/cached tokens and cost when available. Do not invent metrics or assume RTK makes total agent effort cheaper.

[Empty measurement sheet](results.csv). The source ZIP's eight fixtures are retained here with IDs namespaced and clarified.
