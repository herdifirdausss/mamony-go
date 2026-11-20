# ADR-0002 — Ledger and data ownership

**Status:** Contract baseline

## Ledger ownership

C01 alone writes balances. Each posting creates an immutable double-entry journal in one currency. A reversal creates a new journal. A verifier recomputes balances from entries. See the [C01 acceptance checks](../contracts/ledger-gate-a.md).

C01 does not depend on other capabilities. Other capabilities call its facade. If a workflow needs one local transaction across capabilities, it must use an explicit ledger extension point with a documented grant.

Store ledger amounts as exact integer minor units. An FX trade uses two journals through conversion accounts. When FX work starts, use an integer mantissa and scale for rates. Round once to the target currency's minor unit with a named rule. The FX contract must specify bounds, intermediate arithmetic, and the applicable rounding rule.

## Database boundary

Use one PostgreSQL database with one schema per capability and separate owner and runtime roles. A capability reaches another capability's data through a facade, event, or an explicitly granted database interface.

Use transaction-local row-level security (RLS) for tenant-owned tables. Keep C01 core tables platform-owned. See the [PostgreSQL contract](../contracts/postgresql.md) for roles, isolation, and restore tests.

## Event delivery

Write an event in the owner's outbox in the same transaction as the state change. The relay polls the outbox. A notification can wake the relay. The database remains the durable record.

Delivery is at least once. Consumers deduplicate by event source and ID. For C01, preserve event order for each affected account, including journals that touch more than one account. The relay must not publish an event before its predecessors for all those accounts.

Use a stable producer and event type, such as `c01-ledger-core` and `mamony.journal.posted`. Give a breaking payload change a new event type version. Keep unpublished events until delivery. Set retention for published events when the relay contract defines it.
