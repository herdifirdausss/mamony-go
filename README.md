# Mamony Go

Go modular-monolith implementation of the [Mamony](https://github.com/herdifirdausss/mamony) **synthetic** cross-border payment simulator. **Bootstrap only**: no ledger persistence, payouts, real money or real provider credentials. This is not production-ready.

- Original Mamony is the source of truth for accepted product requirements, ledger invariants and ADRs.
- [Seev](https://github.com/herdifirdausss/seev) supplies relevant Go engineering patterns, not business semantics.
- The new Go-specific package topology is Proposed and can be reviewed in [ADR-0001](docs/adr/0001-go-modular-monolith.md).

## Getting started

Requires Go 1.23+ to run the stdlib-only foundation locally. CI requests the pinned Go version in `.go-version`.

```sh
make check
make race
make run
curl -i http://127.0.0.1:8080/livez
curl -i http://127.0.0.1:8081/readyz
```

The private listener is loopback-only in this bootstrap. Readiness checks only HTTP-process readiness, **not** PostgreSQL or financial readiness.

## What is present

- One Go binary and separate public/private HTTP listeners with route-isolation tests.
- C01 **pure** journal-shape validation (positive integer minor units, one currency, debit equals credit, overflow rejection); not real posting.
- Architecture dependency checker, documentation-link checker, tests, `go vet`, race test and build targets.
- CI baseline, source-derived Gate A trace, and compact instructions for coding agents.

## What is absent

No PostgreSQL migration, tenant RLS, role grants, journal posting, idempotency, reversal, outbox, reconciliation, backup or verified financial behavior. These require implementation and real DB tests against the accepted original source; historical Rust tests do not count as Go evidence.

Read [documentation index](docs/index.md), [architecture](docs/architecture.md) and [Gate A requirements](docs/contracts/ledger-gate-a.md).

## Merge policy

Review this bootstrap as a draft PR. Do not equate a passing Go unit test with production or Gate A acceptance. Keep `mamony` and `seev` read-only.
