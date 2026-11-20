# C01 Gate A — requirement trace

**Current:** only pure in-memory journal shape validation, not Post, Reverse, persistence, authorization or a working ledger.

**Approved Mamony source:**
- [ADR-0005](https://github.com/herdifirdausss/mamony/blob/main/docs/adr/0005-ledger-invariants.md): only C01 writes balances; immutable balanced journals; one currency per journal; idempotent Post; reversal and daily verifier.
- [PRD-01](https://github.com/herdifirdausss/mamony/blob/main/docs/prd/PRD-01-ledger-core.md): approved functional requirements; pay attention to FR-003/004/005/006/007, FR-008/009/013 and FR-011.
- [TD-01](https://github.com/herdifirdausss/mamony/blob/main/docs/design/TD-01-ledger-core.md): approved technical design; its Rust library layout must be adapted, not copied literally.
- [Gate A historical evidence](https://github.com/herdifirdausss/mamony/blob/main/docs/09-gate-a-evidence.md): historical Rust test names and gaps, not evidence that Go Gate A passed.
- [ADR-0010](https://github.com/herdifirdausss/mamony/blob/main/docs/adr/0010-one-database-schema-per-capability.md): database permissions and schema ownership.
- [ADR-0021](https://github.com/herdifirdausss/mamony/blob/main/docs/adr/0021-row-level-security-for-tenant-isolation.md): transaction-local tenant context and RLS.

**Gate A required Go evidence (NOT RUN):** migration bootstrap with roles/grants, atomic DB posting, concurrent replay with same key, different-payload rejection, balance floor under contention, one reversal under concurrent requests, outbox ordering and rollback, RLS tenant A/B with pooled connection reuse, backup/restore/recompute verifier.

## Open decision on wire amounts

| Source at Mamony commit `776744a` | Representation |
|---|---|
| [ADR-0014 §Decision, item 5](https://github.com/herdifirdausss/mamony/blob/776744aa36882328a9669382c82a241d139ff437/docs/adr/0014-rest-json-openapi-api-style.md#decision) | JSON integer counts of minor units within the interoperable safe-integer range (±(2^53 − 1)). |
| [TD-01](https://github.com/herdifirdausss/mamony/blob/776744aa36882328a9669382c82a241d139ff437/docs/design/TD-01-ledger-core.md) | Internal ledger amounts are positive minor-unit integers up to `i64::MAX`. PostgreSQL uses `bigint`. |
| [Original OpenAPI](https://github.com/herdifirdausss/mamony/blob/776744aa36882328a9669382c82a241d139ff437/docs/api/openapi.json) | Payment creation (`CreateRequest.amount`) and balance responses (`BalanceResponse.amount`) use strings of digits in minor units. |

Before publishing a Go API, the owner must reconcile the JSON type and range by route. Test the agreed wire format and its boundaries independently of the internal ledger representation.
