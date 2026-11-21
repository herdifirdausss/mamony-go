# Development history

This review checked the architecture decisions, capability descriptions, requirements, technical designs, and selected event, runbook, and test records. Use the [roadmap](roadmap.md) to find the current contracts.

## Review scope

- Reviewed 29 architecture decisions and the ownership and dependency declarations for 22 capabilities.
- Compared 16 requirements documents with their paired technical designs. All 384 functional requirement IDs appear in their paired design trace sections.
- Compared selected scenarios with test definitions for ledger posting, event ordering, provider recovery, and workflow claims.
- Compared project contracts with JSON interoperability and PostgreSQL isolation behavior. Architecture decisions in this repository define the applicable Go policy.

The trace count measures the reviewed input. Each local check can cover several related requirements. The review used test definitions and existing reports. The Go implementation needs its own test results.

## Findings that affect Go

| Finding | Local treatment |
|---|---|
| Journal checks in the bootstrap run in memory. The ledger design also requires durable replay, account locks, reversals, outbox, and recovery. | [Ledger contract](contracts/ledger.md) defines the transaction path, acceptance checks, and recovery evidence. |
| Some tenant isolation wording describes all resources generically. The core ledger and other platform-owned records have distinct access rules. | [PostgreSQL table classes](contracts/postgresql.md#tenant-isolation) state where to apply RLS, grants, and capability authorization. |
| The JSON integer option and the digit-string option differ in accepted range and client compatibility. | [Public HTTP decision](adr/0003-public-http-contract.md#open-decision-on-wire-amounts) keeps the choice open before releasing an amount route. |
| The outbox sequence must preserve each account's order, even when a journal affects several accounts. Replay can redeliver an event. | [Ledger data and event ownership](adr/0002-ledger-and-data-ownership.md#event-delivery) and [consumer tests](contracts/reconciliation-and-events.md#event-delivery) preserve both properties. |
| Paid and completed are distinct payout states. An unknown result is neither. A missing ledger posting after provider success also needs recovery. | [Money flows](contracts/payments-and-payouts.md) define those states and the evidence required to finish a flow. |
| FX can stop after the first journal. A second journal can fail after the first commits. | [FX recovery](contracts/fx.md#execution-and-recovery) records the partial position, fixed keys, and correction path. |
| A retained outbox window can serve replay. Full accounting rebuild needs history beyond that window. | [Accounting and reporting](contracts/settlement-and-accounting.md#accounting-and-reporting) requires a rebuild source before release. |
| Disputes, recipient verification, and monitoring need fields and operations that the small provider and event contracts do not all guarantee. | [Decision register](open-decisions.md#money-and-recovery) requires the producer contracts before these consumers start. |
| Historic performance numbers, retention values, and library choices depend on their environment and workload. | [Decision register](open-decisions.md) retains the decisions and required evidence. Use Go measurements for release targets. |


## Research packs and bootstrap evidence

`v5`, `v6`, and `v7` were the standards, research, and scaffold pack labels. The [pack audit](https://github.com/herdifirdausss/mamony-go/blob/16f1d484ca50c0e8b90754df52246d7e8d96d768/docs/evidence/engineering-pack-verification.md) records checksums and integration. The [documentation snapshot](https://github.com/herdifirdausss/mamony-go/tree/16f1d484ca50c0e8b90754df52246d7e8d96d768/docs) retains the full drafts, bibliography, and original reports.

The [bootstrap CI run](https://github.com/herdifirdausss/mamony-go/actions/runs/44839152957) at `16f1d48` passed. Its [specialized test report](https://github.com/herdifirdausss/mamony-go/actions/runs/44839152898/artifacts/14680007485) recorded 30 passing Go tests. Use the current commit's CI for new verification. Use the [ledger contract](contracts/ledger.md#ledger-gate-evidence) for database acceptance.
