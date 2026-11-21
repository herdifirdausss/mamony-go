# Mamony Go

Mamony Go is a project for simulating cross-border payments and payouts with synthetic data. The current Go binary serves health endpoints on two HTTP listeners. The [roadmap](docs/roadmap.md) describes planned ledger and payment capabilities.

Start with the [documentation index](docs/index.md) or the [roadmap](docs/roadmap.md).

## Run locally

Use Go 1.23 or later. CI uses the version in [`.go-version`](.go-version). Install Python 3.9 or later for the repository's metadata checks.

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
