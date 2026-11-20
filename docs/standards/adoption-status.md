# Engineering standards adoption status

| Area | Authority / status | Enforcement |
|---|---|---|
| Original Mamony approved ADRs and PRDs | Source of truth; historical Rust choices translated only with Go ADR | Tracked in `docs/contracts`, relevant tests as implemented |
| [Engineering principles handbook](engineering-principles.md) | **Draft guidance**; does not supersede approved Mamony decisions | Invoked selectively through skills and review |
| [v6 agent engineering](ai-agent-production-engineering.md) and [integrated research](../research/engineering-workflow-v6.md) | **Draft**; guidance, not Accepted product ADR | Read task-relevant sections, not all context |
| [31-source research register](../research/source-register.md) | Review-date literature index; links/versions need revalidation when promoted to policy | CI verifies presence/count, NOT live source correctness |
| [v6 E01–E12 agent benchmark](../evals/mamony-agent-benchmark.md) | **Plan only; NOT RUN**; no measured cost/token improvement | CI checks case registry, not agent quality |
| Go modular monolith [ADR-0001](../adr/0001-go-modular-monolith.md) | **Proposed** pending owner review | Current import checker tests, does not imply ADR accepted |
| Agent skill files | **Installed as repo files**; discovery and effectiveness depend on agent host | Structural metadata check by `make skills`; behavioral A/B NOT RUN |
| `gofmt`, `go vet`, tests, architecture, doc links, build, race | Baseline deterministic checks | `make check` and `make race` in PR CI |
| v6 research and evaluation metadata | Machine-checkable document completeness only | `make research` in `make check` (not literature verification) |
| golangci-lint/gosec, govulncheck | Configured as optional local commands; **not yet blocking PR CI** | `make lint`, `make security`, missing binaries fail explicitly |
| PostgreSQL, ledger idempotency, RLS, outbox, restore | **NOT IMPLEMENTED** in Go | Database/financial Gate A is NOT RUN |

**Truthful gate reporting:** a green basic CI does not imply the security, finance, or performance gates passed.

## Skill activation

Use the narrowest relevant skill from [registry](../../.agents/skills/). An agent must verify its installed skill discovery behavior; a file's presence in this directory does not itself prove a skill was invoked. Read only matching handbook headings.

## Next adoption steps

1. Review v6 research and decide which recommendations become mandatory Go standards; preserve existing Mamony ADR authority.
2. Add verified linter/vulnerability CI jobs with pinned compatible versions.
3. Build PostgreSQL migrations and real integration/recovery tests before claiming Gate A.
4. Run the [v6 matched-task E01–E12 evaluations](../evals/mamony-agent-benchmark.md) before claiming quality improvements or token savings; compare accepted AC, cost, latency and defects.
