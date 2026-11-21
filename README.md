# Mamony Go

Mamony Go simulates cross-border payments and payouts with synthetic data. The bootstrap runs public and private health listeners and checks ledger journal structure in memory. Durable ledger posting and the simulated pilot are planned.

Start with the [documentation index](docs/index.md) or the [roadmap](docs/roadmap.md).

## Run locally

Use Go 1.23 or later. CI uses the version in [`.go-version`](.go-version). Install Python 3 for the repository's metadata checks.

```sh
make check
make run
curl -i http://127.0.0.1:8080/livez
curl -i http://127.0.0.1:8081/readyz
```

The private listener binds to loopback. Its readiness endpoint checks the HTTP process. See [engineering practices](docs/engineering.md#verification) for the test and CI commands.
