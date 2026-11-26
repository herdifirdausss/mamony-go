# ADR-0002 — Ledger and data ownership

**Status:** Owner-accepted bounded design direction. [#87](https://github.com/herdifirdausss/mamony-go/pull/87) merged the canonical documentation alignment. [K2a](../proposals/ledger-k2a-isolated-persistence.md) was a separately approved synthetic persistence slice. Independent technical acceptance is DEFERRED. Production readiness is NOT ACCEPTED.

## Ledger ownership

The ledger alone writes balances. Each posting creates an immutable double-entry journal in one currency. A reversal creates a new journal. A verifier recomputes balances from entries. See the [Ledger acceptance checks](../contracts/ledger.md).

Ledger owns its book, account, admitted currency snapshot, journal, entry, balance, replay, and Ledger outbox records and commits its own effects in one local ACID transaction. Other capabilities use its authorized facade or events. Each independent capability owns its local transaction and durable outcomes. Normal workflows do not share transactions or directly query or mutate another owner's private schema. Use typed ports, durable operation identities/progress, idempotent commands and explicit recovery across owners. Place an invariant that requires one atomic boundary within one owner. Any exceptional cross-owner ACID boundary or private database access requires a separately accepted ADR, scope, threat model, grants, extraction cost and actual verification. No exception is authorized by this alignment.

Store ledger amounts as exact integer minor units. An FX trade uses two journals through conversion accounts. When FX work starts, use an integer mantissa and scale for rates. Round once to the target currency's minor unit with a named rule. The FX contract must specify bounds, intermediate arithmetic, and the applicable rounding rule.

## Database boundary

Use one PostgreSQL database with one schema per capability and separate owner and runtime roles. Use authorized facades and events by default. Narrow technical relay or verification access requires separately reviewed named grants and catalog evidence. No routine private cross-schema joins, foreign keys, function calls, business DML, or shared transaction handles across owners. One physical database and Platform migration tooling do not imply shared business ownership.

Use transaction-local row-level security (RLS) for tenant-owned tables. Platform owns canonical reference data and infrastructure primitives; Ledger owns its financial records and any future immutable local admitted-definition snapshots, not a second canonical catalog. Accounting book is not tenant; enforce resource authorization and appropriate tenant isolation separately. See the [PostgreSQL contract](../contracts/postgresql.md) for roles, isolation, and restore tests.

## Event delivery

Write an event in the owner's outbox in the same transaction as the state change. The relay polls the outbox. A notification can wake the relay. The database remains the durable record.

Delivery is at least once. Consumers deduplicate by event source and ID. For ledger events, preserve event order for each affected account, including journals that touch more than one account. The relay must not publish an event before its predecessors for all those accounts.

Use a stable producer and event type, such as `ledger` and `mamony.journal.posted`. Give a breaking payload change a new event type version. Keep unpublished events until delivery. Set retention for published events when the relay contract defines it.
