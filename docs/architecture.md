# Architecture

## Running packages

| Package | Responsibility |
|---|---|
| `cmd/mamony` | Read listener addresses and handle process signals. |
| `internal/app` | Start and stop the two HTTP listeners. |
| `internal/platform/httpedge` | Serve liveness and readiness. |

| Listener | Default | Routes |
|---|---|---|
| Public | `127.0.0.1:8080` | `GET /livez` |
| Private | `127.0.0.1:8081` | `GET /livez`, `GET /readyz` |

Set `MAMONY_PUBLIC_ADDR` and `MAMONY_PRIVATE_ADDR` to override the addresses. The private listener accepts `127.0.0.1` and `::1`. It maps `localhost` to `127.0.0.1`. Readiness reports only HTTP process readiness.

## Capability packages

[ADR-0001](adr/0001-go-modular-monolith.md) describes the planned package structure. Add each capability as work reaches it.

- Place the capability facade at `internal/modules/<capability>`.
- Keep the domain, use cases, repositories, and handlers private under the capability's `internal/` directory.
- Place cross-capability adapters in `internal/app`. Keep domain rules independent of HTTP and SQL. Give use cases their own ports.
- Add a feature subpackage when it has a distinct responsibility. Run `make architecture` after an import change.

See the [data ownership decision](adr/0002-ledger-and-data-ownership.md), [public HTTP contract](adr/0003-public-http-contract.md), and [roadmap](roadmap.md) for planned work.
