# Engineering standards adoption status

| Area | Authority / status | Enforcement |
|---|---|---|
| Original Mamony approved ADRs and PRDs | Source of truth; historical Rust choices translated only with Go ADR | Tracked in `docs/contracts`, relevant tests as implemented |
| [Engineering principles handbook](engineering-principles.md) | **Draft guidance**; does not supersede approved Mamony decisions | Invoked selectively through skills and review |
| Go modular monolith [ADR-0001](../adr/0001-go-modular-monolith.md) | **Proposed** pending owner review | Current import checker tests, does not imply ADR accepted |
| Agent skill files | **Installed as repo files**; discovery and effectiveness depend on agent host | Structural metadata check by `make skills`; behavioral A/B NOT RUN |
| `gofmt`, `go vet`, tests, architecture, doc links, build, race | Baseline deterministic checks | `make check` and `make race` in PR CI |
| golangci-lint/gosec, govulncheck | Configured as optional local commands; **not yet blocking PR CI** | `make lint`, `make security`, missing binaries fail explicitly |
| PostgreSQL, ledger idempotency, RLS, outbox, restore | **NOT IMPLEMENTED** in Go | Database/financial Gate A is NOT RUN |

**Truthful gate reporting:** a green basic CI does not imply the security, finance, or performance gates passed.

## Skill activation

Use the narrowest relevant skill from [registry](../../.agents/skills/). An agent must verify its installed skill discovery behavior; a file's presence in this directory does not itself prove a skill was invoked. Read only matching handbook headings.

## Next adoption steps

1. Decide which v5 recommendations become mandatory Go standards.
2. Add verified linter/vulnerability CI jobs with pinned compatible versions.
3. Build PostgreSQL migrations and real integration/recovery tests before claiming Gate A.
4. Run the [skill evaluation suite](../evidence/skill-evaluation-v5.md); compare AC success, cost, latency and defects.
