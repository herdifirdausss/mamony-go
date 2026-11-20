# Mamony Go

Mamony Go is a synthetic simulator for cross-border pay-in and pay-out. The current bootstrap checks journal structure in memory and runs two HTTP listeners.

Read the [implementation sequence](docs/delivery/README.md) to find the contract for the next step. The [documentation index](docs/index.md) links the product scope, architecture decisions, and engineering checks.

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
- CI foundation and specialized Go checks, plus v7-derived **lint and vulnerability scanning jobs** ([GitHub workflow](.github/workflows/ci.yml)); check their latest Actions run before claiming PASS.
- [V7 integration map](docs/research/harness-v7-provenance.md) and [engineering pack verification](docs/evidence/engineering-pack-verification.md).
- [V7 mechanical benchmark](docs/evals/harness-v7-fixture-cases.md) and [empty results sheet](docs/evals/results.csv), separate from the v6 E01–E12 evaluation.
- Local [ledger requirements](docs/contracts/ledger-gate-a.md) and [PostgreSQL contract](docs/contracts/postgresql.md).

## What is absent

No PostgreSQL migration, RLS, role grants, journal posting, idempotency, reversal, outbox, reconciliation, backup or verified financial behavior. General `golangci-lint v2.15.0` and `govulncheck v1.10.0` are configured as separate **blocking PR CI jobs**; their actual results are tracked in GitHub Actions, not assumed from the configuration. `oasdiff` cannot check the absent payment OpenAPI contract, and `gopls MCP` / RTK / SkillSpector are not connected or installed here. Skills are **not benchmarked**; files existing does not establish that any coding agent discovers or follows them.

Start with [documentation index](docs/index.md) for task-specific references. Do **not** paste full v5/v6 research or handbooks into every AI prompt. Literature conclusions and token benefits remain unverified for this Go repository until tested.

## Merge policy

Review the draft PR and its CI results. Record Gate A acceptance only after the database and recovery checks pass.
