# Documentation router

| Task | Start here |
|---|---|
| What to build next | [Implementation sequence](delivery/README.md) |
| Review findings and coverage | [Product and design review](delivery/validation.md) |
| Choices for the current stage | [Decision register](delivery/decisions.md) |
| Product, capabilities, and delivery gates | [Product scope](product.md) |
| System decisions | [Architecture decision index](adr/README.md) |
| Go packages and listeners | [Architecture](architecture.md) |
| Ledger posting, replay, or Gate A | [C01 requirements and acceptance checks](contracts/ledger-gate-a.md) |
| Migrations, database grants, or tenant isolation | [PostgreSQL contract](contracts/postgresql.md) |
| Public routes or amount formats | [Public HTTP contract](adr/0003-public-http-contract.md) |
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

Read the sections for your task. Use the decisions and contracts in this repository. Research and benchmark plans provide optional context.
