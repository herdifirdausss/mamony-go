# Mamony Go

Go modular-monolith implementation of the [Mamony](https://github.com/herdifirdausss/mamony) **synthetic** cross-border payment simulator. **Bootstrap only**: no ledger persistence, payouts, real money or real provider credentials. This is not production-ready.

- Original Mamony is the source of truth for accepted product requirements, ledger invariants and ADRs.
- [Seev](https://github.com/herdifirdausss/seev) supplies relevant Go engineering patterns, not business semantics.
- The Go-specific topology is Proposed: see [ADR-0001](docs/adr/0001-go-modular-monolith.md).

## Getting started

Requires Go 1.23+ for the stdlib-only foundation. CI selects the pinned Go version in `.go-version`. Python 3 is required for the repository skill metadata check.

```sh
make check
make race
make run
curl -i http://127.0.0.1:8080/livez
curl -i http://127.0.0.1:8081/readyz
```

The private listener binds loopback only. Readiness means **HTTP process readiness**, not PostgreSQL or financial readiness.

## What is present

- One Go binary and separate public/private HTTP listeners with route-isolation tests.
- C01 pure journal-shape validation (positive integer minor units, one currency, debits equal credits, overflow rejection); **not real posting**.
- Architecture checker, documentation-link checker, unit tests, vet, race and build targets.
- Engineering [handbook v5](docs/standards/engineering-principles.md), [v6 AI-agent production engineering](docs/standards/ai-agent-production-engineering.md), [integrated workflow v6](docs/research/engineering-workflow-v6.md), [31-source register](docs/research/source-register.md), [adoption status](docs/standards/adoption-status.md), and eight project-scoped [agent skills](.agents/skills/) with structural metadata tests.
- [Skill A/B evaluation v5](docs/evidence/skill-evaluation-v5.md) and [v6 E01–E12 matched-task benchmark](docs/evals/mamony-agent-benchmark.md) with acceptance criteria, hard gates, cost and token metrics (**NOT RUN**).
- `make research` checks the v6 document routing, 31 reference entries and 12 benchmark case IDs; this is a metadata gate, **not** independent research verification.
- [11-candidate adoption matrix](docs/tooling/shortlist.md): pinned `go-arch-lint` and `gotestsum` in a specialized CI workflow; others are independent Mamony skill adaptations, opt-in tools, or explicitly unrun experiments.
- [Third-party skill security preflight](docs/tooling/third-party-security.md) and [RTK token-economics protocol](docs/tooling/token-economics.md).
- CI baseline and a source-derived [Gate A trace](docs/contracts/ledger-gate-a.md).

## What is absent

No PostgreSQL migration, RLS, role grants, journal posting, idempotency, reversal, outbox, reconciliation, backup or verified financial behavior. General `golangci-lint` and `govulncheck` have local commands but are **not yet blocking PR CI**. `oasdiff` cannot check the absent payment OpenAPI contract, and `gopls MCP` / RTK / SkillSpector are not connected or installed here. Skills are **not benchmarked**; files existing does not establish that any coding agent discovers or follows them. Historical Rust tests do not count as Go runtime evidence.

Start with [documentation index](docs/index.md) for task-specific references. Do **not** paste full v5/v6 research or handbooks into every AI prompt. Literature conclusions and token benefits remain unverified for this Go repository until tested.

## Merge policy

Review as a draft PR. Green basic CI does not equal production-readiness or Gate A acceptance. Keep `mamony` and `seev` unchanged.
