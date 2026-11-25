# Platform foundation

The current binary reads listener addresses at startup and shuts down on cancellation. The database, event relay, and recovery procedures below are planned. The [package rules](../architecture.md), [PostgreSQL contract](postgresql.md), and [public HTTP contract](../adr/0003-public-http-contract.md) apply.

## Configuration, data, and operations

| ID | Requirement | Acceptance check |
|---|---|---|
| FND-01 | Load typed configuration at startup. Validate required values and bounds. Read test credentials from the configured secret source. | Invalid configuration stops startup. Logs and errors omit secrets. |
| FND-02 | Keep one reference list for country codes and currency exponents. Represent each amount as currency plus integer minor units. Each capability can enable a subset of currencies. | Reject undefined exponents and unsupported currencies. Test zero-, two-, and three-decimal currencies. |
| FND-03 | Version migrations by schema. Protect applied migration files from edits. Separate owner and runtime grants. | Run bootstrap and migrations on an empty database. Upgrade a supported fixture. Check owners and grants in the catalog. |
| FND-04 | Use structured logs with a correlation ID. Record the actor, action, target, outcome, time, and capability for controlled changes. | Follow one request across a job and a consumer. Check redaction and trace continuity. |
| FND-05 | Provide one local startup command once PostgreSQL joins the application. Give the process bounded startup and shutdown. | Start from a clean clone with synthetic fixtures. Cancel the process during a job. Check that work can resume. |

### FND-02a — curated reference experiment

The owner accepted the bounded synthetic slice in [#92](https://github.com/herdifirdausss/mamony-go/issues/92). Platform owns one immutable reference catalog for the curated currency and country codes; see [source manifest](../../internal/platform/reference/README.md). Lookup must fail for unknown or malformed codes; undefined exponents must not become zero. Validate canonical bytes, reject duplicates and conflicting definitions, preserve minor-unit definition identity across unrelated release updates, and refuse a mismatched code/version/exponent. Initial scope covers IDR/2, JPY/0, KWD/3, SGD/2, USD/2 and ID, JP, KW, SG, US. Membership does not enable a currency or authorize a financial operation. No shared Money domain, provider exponent, public wire semantics, database, or runtime wiring in this experiment. Capability integration, supported-currency policy, broader reference coverage and historical identity decisions remain outstanding. Test lookup, invalid data, version identity, checksum, and concurrent read isolation; no claim of full FND-02 acceptance.

### FND-02c — isolated historical identity experiment

The owner accepted [#97](https://github.com/herdifirdausss/mamony-go/issues/97) after FND-02b post-merge verification. Private pure-Go synthetic history validates explicitly allocated immutable definition IDs, distinct monetary unit IDs, code/exponent and fixture evidence, independently of release identity. Effective selection and exact pinned lookup are separate. UTC calendar dates use half-open intervals; activation requires no active selection, retirement ends selection on its effective date, and a definition cannot be activated again. Strict per-code effective-date order rejects same-date competing events; this bounded model requires a gap before a replacement, so gapless/same-day transitions require further design. Events have separate canonical UTC acceptance timestamps, cannot precede their recorded UTC date or follow release acceptance, and are globally nondecreasing by recorded time. New releases preserve all definition/event prefixes, increase release revision and acceptance time, and refuse new events recorded at/before previous acceptance or effective at/before its UTC day. Future scheduled events can be selected in the accepted snapshot; this is not a knowledge-time projection. Old definitions remain readable by exact ID regardless of current selection. No automatic equivalence or conversion, real historical data, external installation interface, persistence or authorization. Fictional ZZZ fixtures only; existing v1 definitions, catalog, Ledger admission and startup wiring remain unchanged. Actual identity promotion, real effective-date provenance, corrections, retired-unit posting/reversal policies and K2 need separate review.

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

## Security operations and evidence

| ID | Requirement | Acceptance check |
|---|---|---|
| FND-09 | Give build automation, application runtime, migration tooling and cross-tenant jobs separate least-privilege credentials and permissions. Limit cryptographic operations by caller, purpose and key version. | Check actual workflow-token privileges, DB role grants, runtime access to migration secrets and an unauthorized signing/decryption request. |
| FND-10 | Keep credentials, secrets, token bodies, password verifiers and sensitive identity payloads out of logs, tracing, error responses, analytics and committed fixtures. Have a documented rotation/revocation path for enabled secrets. | Exercise application errors and request tracing with synthetic sentinel secrets; verify no disclosure, and review revocation of a compromised test credential. |
| FND-11 | Define resource budgets for public requests and expensive authentication operations; enforce trustworthy proxy headers and an egress policy where external calls exist. Keep health listeners' network exposure independent of future payment routes. | Test oversized input, concurrency exhaustion, spoofed forwarding headers, forbidden outbound address and the deployed public/private route inventory. |
| FND-12 | Keep security-event obligations durable with their state changes and audit evidence restricted from ordinary business writers. Choose and test recovery checkpoints before claiming rollback or tamper resistance. | Fail event/outbox write and worker dispatch, deny unauthorized audit modification, restore a stale snapshot, and detect revoked-session/approval resurrection. |

Passing lint, `gosec` or `govulncheck` proves only the checks run on that commit. It does **not** prove authorization, RLS isolation, ledger concurrency, event durability, cryptographic trust boundaries or regulatory compliance. See the [engineering verification matrix](../engineering.md#security-verification).

