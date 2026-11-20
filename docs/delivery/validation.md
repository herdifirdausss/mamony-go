# Product and design review

This review checked the architecture decisions, capability descriptions, requirements, technical designs, and selected event, runbook, and test records. The Go contracts in this directory are the implementation reference.

## Review scope

- Reviewed 29 architecture decisions and the ownership and dependency declarations for 22 capabilities.
- Compared 16 requirements documents with their paired technical designs. All 384 functional requirement IDs appear in their paired design trace sections.
- Compared selected scenarios with test definitions for ledger posting, event ordering, provider recovery, and workflow claims.
- Compared project contracts with JSON interoperability and PostgreSQL isolation behavior. Architecture decisions in this repository define the applicable Go policy.

The trace count measures the reviewed input. Each local check can cover several related requirements. The review used test definitions and existing reports. The Go implementation needs its own test results.

## Findings that affect Go

| Finding | Local treatment |
|---|---|
| Journal checks in the bootstrap run in memory. The ledger design also requires durable replay, account locks, reversals, outbox, and recovery. | [C01 and Gate A](../contracts/ledger-gate-a.md) defines the transaction path, acceptance checks, and recovery evidence. |
| Some tenant isolation wording describes all resources generically. The core ledger and other platform-owned records have distinct access rules. | [PostgreSQL table classes](../contracts/postgresql.md#tenant-isolation) state where to apply RLS, grants, and capability authorization. |
| The JSON integer option and the digit-string option differ in accepted range and client compatibility. | [Public HTTP decision](../adr/0003-public-http-contract.md#open-decision-on-wire-amounts) keeps the choice open before releasing an amount route. |
| The outbox sequence must preserve each account's order, even when a journal affects several accounts. Replay can redeliver an event. | [Ledger data and event ownership](../adr/0002-ledger-and-data-ownership.md#event-delivery) and [consumer tests](08-operations-events.md#c12-event-delivery) preserve both properties. |
| Paid and completed are distinct payout states. An unknown result is neither. A missing ledger posting after provider success also needs recovery. | [Money flows](07-money-flows.md) define those states and the evidence required to finish a flow. |
| FX can stop after the first journal. A second journal can fail after the first commits. | [FX recovery](06-fx.md#execution-and-recovery) records the partial position, fixed keys, and correction path. |
| A retained outbox window can serve replay. Full accounting rebuild needs history beyond that window. | [Accounting and reporting](10-settlement-accounting.md#c16-accounting-and-reporting) requires a rebuild source before release. |
| Disputes, recipient verification, and monitoring need fields and operations that the small provider and event contracts do not all guarantee. | [Decision register](decisions.md#money-and-recovery) requires the producer contracts before these consumers start. |
| Historic performance numbers, retention values, and library choices depend on their environment and workload. | [Decision register](decisions.md) retains the decisions and required evidence. Use Go measurements for release targets. |

## Coverage by subject

| Reviewed subject | Go contract |
|---|---|
| Platform standards | [00 — Platform](00-platform.md) |
| Ledger | [01 — Ledger and Gate A](../contracts/ledger-gate-a.md) |
| Identity and access | [02 — Identity](02-identity.md) |
| Screening and onboarding | [03 — Screening and onboarding](03-screening-onboarding.md) |
| Account and balances | [04 — C02 account views](04-fees-limits-accounts.md#c02-account-views) |
| Fees, limits, and policy | [04 — Fees, limits, and accounts](04-fees-limits-accounts.md) |
| Provider integration | [05 — Simulated provider](05-provider.md) |
| FX | [06 — FX](06-fx.md) |
| Payment orchestration | [07 — Pay-in](07-money-flows.md#c03-pay-in) |
| Payout | [07 — Pay-out](07-money-flows.md#c04-pay-out) |
| Reconciliation | [08 — C10 reconciliation](08-operations-events.md#c10-reconciliation) |
| Back office, audit, and events | [08 — Operations and events](08-operations-events.md) |
| Fraud and AML activity monitoring | [09 — Risk and monitoring](09-risk-monitoring.md) |
| Settlement and accounting | [10 — Settlement and accounting](10-settlement-accounting.md) |
| Disputes, webhooks, and developer experience | [11 — Disputes and developer](11-disputes-developer.md) |
| Platform split payments | [12 — Platform expansion](12-platform-expansion.md) |

Use the [implementation sequence](README.md) to select the next step. Use its acceptance checks to produce new Go evidence.
