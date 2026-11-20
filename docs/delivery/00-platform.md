# 00 — Platform foundation

Build the shared contracts that later capabilities use. The [package rules](../architecture.md), [PostgreSQL contract](../contracts/postgresql.md), and [public HTTP contract](../adr/0003-public-http-contract.md) apply.

## Configuration, data, and operations

| ID | Requirement | Acceptance check |
|---|---|---|
| FND-01 | Load typed configuration at startup. Validate required values and bounds. Read test credentials from the configured secret source. | Invalid configuration stops startup. Logs and errors omit secrets. |
| FND-02 | Keep one reference list for country codes and currency exponents. Represent each amount as currency plus integer minor units. Each capability can enable a subset of currencies. | Reject undefined exponents and unsupported currencies. Test zero-, two-, and three-decimal currencies. |
| FND-03 | Version migrations by schema. Protect applied migration files from edits. Separate owner and runtime grants. | Run bootstrap and migrations on an empty database. Upgrade a supported fixture. Check owners and grants in the catalog. |
| FND-04 | Use structured logs with a correlation ID. Record the actor, action, target, outcome, time, and capability for controlled changes. | Follow one request across a job and a consumer. Check redaction and trace continuity. |
| FND-05 | Provide one local startup command once PostgreSQL joins the application. Give the process bounded startup and shutdown. | Start from a clean clone with synthetic fixtures. Cancel the process during a job. Check that work can resume. |

## Local transactions

- Each write has one owner. Use the owner role only for migrations. Use a limited runtime role for the application.
- Retry the whole local transaction after an explicitly retryable abort. Bound the attempt count and use the request deadline. Keep retries at one layer.
- Keep network calls outside database transactions. After an ambiguous commit, look up the durable record or replay its key.
- Use the same transaction for a state change and its outbox row. Use the [event delivery rules](../adr/0002-ledger-and-data-ownership.md#event-delivery).

## Edge and recovery

| ID | Requirement | Acceptance check |
|---|---|---|
| FND-06 | Keep liveness independent of the database. Before routing money traffic, readiness checks the database and required job startup. | Simulate database loss. Check that liveness stays available and readiness reports the failure. |
| FND-07 | Use a stable problem code and a documented retry policy for each published error. | Check expected status and body for validation errors, policy refusals, and temporary unavailability. |
| FND-08 | Create backups on a schedule. Restore them in a fresh environment. Check data, migrations, grants, and the ledger verifier. | Recover a committed journal and its outbox row. Record elapsed time and the recovery point. |

Run `make check` for each PR. It covers the current Go and documentation foundation. Add database and recovery commands when their fixtures exist. Keep those checks in the CI evidence for each later gate.
