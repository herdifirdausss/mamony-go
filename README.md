# Mamony Go

Mamony Go is a project for simulating cross-border payments and payouts with synthetic data. The current Go binary serves health endpoints on two HTTP listeners. The [roadmap](docs/roadmap.md) describes planned ledger and payment capabilities.

Start with the [documentation index](docs/index.md) or the [roadmap](docs/roadmap.md). For new capabilities, follow the [research-to-implementation workflow](docs/development-workflow.md) before coding.

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

This creates an ephemeral PostgreSQL 18.7 container bound to a random **loopback-only** port, tests separate synthetic runtime/admin roles and tenant RLS, then removes the container. The equivalent CI job runs on every PR and push to `main`. Without a disposable database, `make integration` fails closed rather than reporting PASS. **No production migrations, ledger code, provider calls or real identities are exercised.**

## Platform SQL migration runner

Accepted DEC-01 uses PostgreSQL 18.7, native pgx v5, and golang-migrate v4 with a **database-committed SHA-256 fingerprint per applied migration**. The executable is `cmd/mamony-migrate` and must be invoked **outside the HTTP process** with separate migration credentials. The public HTTP binary never receives migration credentials.

The dedicated [platform migration runbook](docs/runbooks/platform-migrations.md) describes bootstrap grants, startup, integrity evidence, and dirty-state recovery. `make migrations-check` checks committed migration files without accessing PostgreSQL. `make integration-local` runs the disposable PostgreSQL integration suite, including negative migration tests, when Docker and `psql` are installed. Ledger, tenant runtime adapters, and restore remain future work.
