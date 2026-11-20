# Harness v7 — small mechanical benchmark (historical design)

Source: uploaded Mamony Engineering Harness v7 `docs/evals/benchmark.md`. This is a **separate set of harness fixtures**, not the E01–E12 *financial workflow* benchmark in [v6](mamony-agent-benchmark.md). Its E01–E08 labels are scoped to **v7-harness**, to avoid interpreting matching IDs as equivalent tasks.

| v7 harness case | Required local behavior | Current verification status |
|---|---|---|
| H07-E01 | Forbidden module import is rejected | PASS in [local fixture verification](../evidence/engineering-pack-verification.md). Deliberately added direct payment-to-ledger import rejected. |
| H07-E02 | Valid app adapter and local usecase imports are accepted | PASS in [local fixture verification](../evidence/engineering-pack-verification.md). App-to-facade and local handler-to-use-case imports accepted. |
| H07-E03 | Unbalanced debit/credit rejected | Existing pure ledger unit test |
| H07-E04 | Minor-unit total overflow rejected | Existing pure ledger unit test |
| H07-E05 | Public listener does not expose private readiness | PASS in route-isolation test and [local HTTP smoke test](../evidence/engineering-pack-verification.md). |
| H07-E06 | Concurrent duplicate ledger Post yields one durable posting | BLOCKED: PostgreSQL C01 Post not implemented |
| H07-E07 | Unknown provider outcome cannot cause blind resend | BLOCKED: provider simulator not implemented |
| H07-E08 | Tenant A cannot access B after pooled connection reuse | BLOCKED: runtime role/RLS not implemented |

Run controlled baselines at the same commit, agent model/effort, issue brief, fixtures and test harness. Record hard-gate outcomes, actual elapsed time, reviewer corrections, input/output/cached tokens and cost when available. Do not invent metrics or assume RTK makes total agent effort cheaper.

[Empty measurement sheet](results.csv). The source ZIP's eight fixtures are retained here with IDs namespaced and clarified.
