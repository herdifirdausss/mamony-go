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
- Engineering [handbook v5](docs/standards/engineering-principles.md), [research workflow v5](docs/research/engineering-workflow-v5.md), [adoption status](docs/standards/adoption-status.md) and five draft [agent skills](.agents/skills/) with structural metadata tests.
- [Skill A/B evaluation](docs/evidence/skill-evaluation-v5.md) with acceptance criteria, cost and token metrics (not run).
- CI baseline and a source-derived [Gate A trace](docs/contracts/ledger-gate-a.md).

## What is absent

No PostgreSQL migration, RLS, role grants, journal posting, idempotency, reversal, outbox, reconciliation, backup or verified financial behavior. Go lint/security scanners have local commands but are **not yet blocking PR CI**. Skills are **not benchmarked**; files existing does not establish that any coding agent discovers or follows them. Historical Rust tests do not count as Go runtime evidence.

Start with [documentation index](docs/index.md) for task-specific references. Do **not** paste the full research or handbook into every AI prompt.

## Merge policy

Review as a draft PR. Green basic CI does not equal production-readiness or Gate A acceptance. Keep `mamony` and `seev` unchanged.
