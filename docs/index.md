# Documentation router

| Task | Start here |
|---|---|
| Product and capabilities | [Mamony vision](https://github.com/herdifirdausss/mamony/blob/main/docs/00-vision-and-scope.md) and [boundaries](https://github.com/herdifirdausss/mamony/blob/main/docs/03-service-boundaries.md) |
| Accepted architecture | [Mamony ADR-0004](https://github.com/herdifirdausss/mamony/blob/main/docs/adr/0004-modular-monolith-first.md) and [ADR-0027](https://github.com/herdifirdausss/mamony/blob/main/docs/adr/0027-first-runtime-and-http-layer.md) |
| C01/DB preparation blockers | [Ledger contract gaps](contracts/ledger-contract-gaps.md) and [database contract gaps](contracts/database-contract-gaps.md) — **not implemented** |
| Ledger implementation | [PRD-01](https://github.com/herdifirdausss/mamony/blob/main/docs/prd/PRD-01-ledger-core.md), [TD-01](https://github.com/herdifirdausss/mamony/blob/main/docs/design/TD-01-ledger-core.md), [ledger constraints](contracts/ledger-gate-a.md) |
| Go package changes | [Architecture](architecture.md), [Go ADR](adr/0001-go-modular-monolith.md) |
| Design principles / refactoring / patterns | [Engineering principles handbook](standards/engineering-principles.md), read only relevant heading |
| Architecture and security standards policy | [Adoption status](standards/adoption-status.md), [security baseline](standards/security.md), [AI execution workflow](standards/agent-workflow.md) |
| AI-agent harness, token efficiency and secure tooling | [v6 production engineering §§18–24](standards/ai-agent-production-engineering.md) — read task-relevant sections only |
| Full workflow/history | [integrated v6 workflow](research/engineering-workflow-v6.md), [v5 predecessor](research/engineering-workflow-v5.md), [31-source register](research/source-register.md), [reader notes](research/README.md); **not required initial context** |
| AI skills, context and efficiency | [8 Mamony skills](../.agents/skills/); [11 external tool adoption statuses](tooling/shortlist.md); [security preflight](tooling/third-party-security.md); [v6 E01–E12 benchmark](evals/mamony-agent-benchmark.md) |
| Go architecture and test-output tooling | [Tool matrix](tooling/shortlist.md): pinned `go-arch-lint` and `gotestsum` in specialized CI |
| OpenAPI compatibility / future contract work | [oasdiff integration](tooling/openapi-compat.md); actual API contract not yet implemented |
| Go symbol navigation / AST search | [gopls MCP](tooling/gopls-mcp.md), [ast-grep](tooling/structural-search.md) — opt-in |
| Engineering pack verification | [v5, v6, and v7 verification report](evidence/engineering-pack-verification.md) |
| Harness v7 provenance and historical test evidence | [Integration map](research/harness-v7-provenance.md), [historical standalone test](evidence/archived-harness-v7-local-run.md) |
| Harness v7 mechanical evaluation | [Eight scoped fixtures](evals/harness-v7-fixture-cases.md) and [empty metrics sheet](evals/results.csv) — not the same IDs as v6 E01–E12 |
| Review / completion | [Evidence template](evidence/template.md) |
| Reference Go project | [Seev](https://github.com/herdifirdausss/seev), engineering patterns only |

Read only sections relevant to the task. A code sample in Seev or generic advice does not override a Mamony accepted ADR or requirement.
