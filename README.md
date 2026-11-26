# Mamony Go

Mamony Go is a Go modular-monolith simulator for cross-border payments and payouts using synthetic data. It is under active development, not a production payment service.

## Current state

The default HTTP process exposes **health endpoints only**. The repository also contains substantial internal, unmounted synthetic implementations.

| Capability | Merged baseline |
|---|---|
| Platform | PostgreSQL migration tooling, restricted runtime pool, disposable recovery/PITR lab, curated reference catalog |
| Identity | Internal enrollment, password login, sessions, recovery, security notices and one-use passkey possession verification. The default HTTP process mounts health endpoints only. |
| Ledger | [Monetary domain and isolated persistence](docs/roadmap.md#ledger-implementation-sequence), with USD-only reference admission and disposable PostgreSQL fixtures. Durable journal posting and the money API remain planned. |
| Payments, payouts, FX and providers | Planned; no money movement or live payment provider integration |

See [current state and active work](docs/current-state.md) for the dated main snapshot, open PRs/issues, evidence and remaining gates. [Documentation index](docs/index.md) · [Roadmap](docs/roadmap.md) · [Change lifecycle](docs/development-workflow.md).

Synthetic verification and merge do not establish production readiness. Do not use real funds, identities or live credentials.

## Run locally

Use Go 1.27 or later. CI uses the version in [`.go-version`](.go-version). Install Python 3.9 or later for the repository's metadata checks.

```sh
make check
make run
```

From a second terminal, use the default listener addresses to check the process.

```sh
curl -i http://127.0.0.1:8080/livez
curl -i http://127.0.0.1:8081/readyz
```

The private listener binds to loopback. Use `MAMONY_PUBLIC_ADDR` and `MAMONY_PRIVATE_ADDR` to set listener addresses. The private readiness endpoint checks the HTTP process. See [engineering practices](docs/engineering.md#verification) for the test and CI commands.

## Disposable PostgreSQL integration tests

The [PostgreSQL foundation proposal](docs/proposals/postgresql-foundation.md) describes the intentionally limited synthetic fixture. With Docker and the PostgreSQL `psql` client installed:

```sh
make integration-local
```

This creates an ephemeral PostgreSQL 18.7 container bound to a random **loopback-only** port, tests separate synthetic runtime/admin roles and tenant RLS, then removes the container. The equivalent CI job runs on every PR and push to `main`. Without a disposable database, `make integration` fails closed rather than reporting PASS. **No production database, live provider calls, real identities or durable Ledger posting are exercised.** The suite also includes synthetic Identity database tests and recovery fixtures; see [current state](docs/current-state.md).

## Platform SQL migration runner

Accepted DEC-01 uses PostgreSQL 18.7, native pgx v5, and golang-migrate v4 with a **database-committed SHA-256 fingerprint per applied migration**. The executable is `cmd/mamony-migrate` and must be invoked **outside the HTTP process** with separate migration credentials. The public HTTP binary never receives migration credentials.

The dedicated [platform migration runbook](docs/runbooks/platform-migrations.md) describes bootstrap grants, startup, integrity evidence, and dirty-state recovery. `make migrations-check` checks committed migration files without accessing PostgreSQL. `make integration-local` runs the disposable PostgreSQL integration suite, including negative migration tests, when Docker and `psql` are installed. Separate Identity migration tooling and disposable recovery tests also exist; production migrations, deployment grant parity and full Ledger recovery are not accepted.

## Runtime PostgreSQL primitive (not yet connected to HTTP)

`internal/platform/postgres` provides an optional bounded native `pgxpool` wrapper that checks the real runtime DB role, requires verified TLS outside isolated loopback test databases, and uses **transaction-local** tenant context for an already-authorized tenant ID. Internal Identity uses dedicated PostgreSQL adapters; the default HTTP process does not mount those handlers or a tenant-aware capability. End-to-end authorization integration remains open under [#18](https://github.com/herdifirdausss/mamony-go/issues/18). Tenant authority must come from Identity and the owning use case, **never from a client-supplied tenant string or from RLS alone**.

The disposable `make integration-local` suite now also verifies connection reuse, negative tenant reads/writes, rollback, cancellation and elevated-role refusal against PostgreSQL itself. This does not require a production database or credentials.
