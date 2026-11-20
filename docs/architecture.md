# Architecture

The bootstrap runs one Go process. See [ADR-0001](adr/0001-go-modular-monolith.md) for the proposed architecture.

## Packages

| Package | Responsibility |
|---|---|
| `cmd/mamony` | Start and stop the process. |
| `internal/app` | Compose capability facades and translate between capability contracts. |
| `internal/modules/<capability>` | Own the capability facade and business rules. |
| `internal/modules/<capability>/internal` | Hold private domain, use case, repository, and handler packages as needed. |
| `internal/platform` | Hold shared technical primitives without capability dependencies. |

A capability calls another capability through an app adapter. Keep the domain independent of HTTP and SQL. Use cases depend on their own ports. Handlers and repositories implement the edges. Add a feature subpackage when it has a distinct responsibility. Run `make architecture` after an import change.

## Listeners

| Listener | Bootstrap routes | Later surface |
|---|---|---|
| Public | `GET /livez` | External API. |
| Private | `GET /livez`, `GET /readyz` | Internal API, operator console, and metrics. |

The private listener binds to loopback. The readiness route reports HTTP process readiness. Add checks for PostgreSQL and required jobs before using it as a traffic gate for money flows.

See the [data ownership decision](adr/0002-ledger-and-data-ownership.md) and [public HTTP contract](adr/0003-public-http-contract.md).
