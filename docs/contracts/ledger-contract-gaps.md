# C01 ledger contract gaps — block premature implementation

Adapted from v7 harness. Source of truth is the original Mamony accepted [PRD-01](https://github.com/herdifirdausss/mamony/blob/main/docs/prd/PRD-01-ledger-core.md), [TD-01](https://github.com/herdifirdausss/mamony/blob/main/docs/design/TD-01-ledger-core.md), relevant approved ADRs, and the [Go Gate A trace](ledger-gate-a.md).

The existing Go `ValidateJournal` is **only a pure shape/arithmetic check** (positive integer minor amounts, one currency, balanced debits/credits and checked addition). It is **not** C01 Post, balance mutation, idempotency, reversal or persistence.

Before implementing financial posting, resolve and verify:
- PRD-01 FR-003/004/005/006/007: account and journal validity, balance floor, idempotency scope and response.
- PRD-01 FR-008/009/013: single reversal, verifier and immutable journals.
- PRD-01 FR-011 and related event/outbox ADRs: atomic outbox, ordering, replay and consumer deduplication.
- ADR-0010 and ADR-0021: per-capability schema ownership, actual runtime role, and transaction-local tenant RLS.
- ADR-0014 versus TD-01/OpenAPI: wire amount shape inconsistency; **do not resolve silently**.
- FX precision/rounding policy: no `float64` monetary arithmetic.

Acceptance evidence requires real PostgreSQL posting, concurrent barriers, replay after committed response loss, rollback, cross-tenant pool reuse negative tests, upgrade migration and verifier after backup restoration. **None of those scenarios are implemented in this starter.**
