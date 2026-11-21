# Implementation roadmap

The bootstrap checks journal shape and runs two health listeners. Each contract below defines the intended behavior, its prerequisites, and its acceptance checks.

## Foundation and ledger

| Order | Contract | Prerequisites and outcome |
|---:|---|---|
| 1 | [Platform foundation](contracts/platform.md) | Verify migrations, configuration, audit, and the minimal event relay. |
| 2 | [Ledger](contracts/ledger.md) | Use PostgreSQL to prove durable posting, replay, reversal, delivery, and restore. |

Pass the ledger gate before building pilot money flows. Extend the catalog and operator controls under [reconciliation and events](contracts/reconciliation-and-events.md).

## Simulated pilot

| Order | Contract | Prerequisites and outcome |
|---:|---|---|
| 3 | [Identity and access](contracts/identity.md) | Use PostgreSQL runtime roles and tenant transaction boundaries. |
| 4 | [Screening and onboarding](contracts/screening-and-onboarding.md) | Use identity and approval. Implement screening before onboarding. |
| 5 | [Fees, limits, and accounts](contracts/fees-limits-and-accounts.md) | Use the ledger, identity, and onboarding. Implement policy before account activation. |
| 6 | [Provider connector](contracts/provider.md) | Use configuration, durable jobs, and event delivery. This can run in parallel with identity, screening, and policy work. |
| 7 | [FX](contracts/fx.md) | Use ledger posting, account mappings, and fee contracts. |
| 8 | [Payments and payouts](contracts/payments-and-payouts.md) | Use the preceding pilot contracts. Build one end-to-end flow at a time. |
| 9 | [Reconciliation and events](contracts/reconciliation-and-events.md) | Build audit and catalog interfaces earlier. Reconcile the completed flows before the pilot. |

Pass the pilot gate on one simulated corridor. Exercise both money flows, screening, unknown provider outcomes, reversals or returns, daily reconciliation, worker recovery, and restore. Use the [open decisions](open-decisions.md) to set the corridor, observation window, and performance targets.

## Operations and expansion

| Order | Contract | Prerequisites and outcome |
|---:|---|---|
| 10 | [Fraud and monitoring](contracts/fraud-and-monitoring.md) | Use stable money events, outcome labels, and reviewer roles. |
| 11 | [Settlement and accounting](contracts/settlement-and-accounting.md) | Use reconciled flows, durable payout results, and rebuildable history. |
| 12 | [Disputes and sandbox](contracts/disputes-and-sandbox.md) | Define provider dispute and evidence interfaces and tenant-scoped API and event contracts. |
| 13 | [Split payments](contracts/split-payments.md) | Use sub-merchant onboarding, settlement, and reporting. |

The operations gate covers repeatable settlement, a close without unexplained differences, reviewed monitoring alerts, measured fraud controls, and a hot-account load test. Set the pass thresholds before the test. Billing, cards, and savings remain deferred.

Use requirement IDs from the relevant contract in the [PR template](../.github/pull_request_template.md). Follow the [architecture decisions](adr/README.md) when choosing a design.
