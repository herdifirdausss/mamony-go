# Implementation sequence

Start with the current step. Read its contract and its prerequisites. A contract describes the behavior the Go implementation must demonstrate. A suggested package, table name, or tool remains a design choice until the project records it.

The bootstrap runs two HTTP listeners, checks journal shape in memory, and runs engineering checks. The [product and design review](validation.md) records which behavior the following steps carry forward.

## Foundation and Gate A

| Step | Contract | Proceed when |
|---|---|---|
| 00 | [Platform foundation](00-platform.md) | Migrations, configuration, audit, and the minimal event relay are verifiable. |
| 01 | [C01 ledger and Gate A](../contracts/ledger-gate-a.md) | PostgreSQL tests prove posting, recovery, audit, and restore. |

Build only the event delivery and audit needed by Gate A in step 00. Use step 08 for the full catalog, consumer controls, and operator workflows.

## Money flows and Gate B

| Step | Contract | Prerequisites |
|---|---|---|
| 02 | [C08 identity and access](02-identity.md) | PostgreSQL roles and tenant transaction boundaries. |
| 03 | [C19 screening and C09 onboarding](03-screening-onboarding.md) | Tenant identity and approval. Implement screening before onboarding. |
| 04 | [C06 fees, C07 limits, and C02 accounts](04-fees-limits-accounts.md) | C01, tenant identity, and onboarding. Implement policy before account activation. |
| 05 | [C05 simulated provider connector](05-provider.md) | Local configuration, durable jobs, and event delivery. This step can run alongside steps 02–04. |
| 06 | [C14 basic FX](06-fx.md) | C01 posting, account mappings, and fee contracts. |
| 07 | [C03 pay-in and C04 pay-out](07-money-flows.md) | Steps 02–06. Build one end-to-end flow at a time. |
| 08 | [C10 reconciliation, C11 back office, and C12 events](08-operations-events.md) | Build catalog and audit interfaces earlier. Run reconciliation against complete flows before the pilot. |

Gate B requires both flows on one simulated corridor. Exercise screening, ambiguous provider outcomes, reversal or return, daily reconciliation, and a failed-worker recovery. Demonstrate a restore. Use the [pilot behavior](../product.md#pilot-behavior) and [decision register](decisions.md) before selecting the corridor or claiming readiness.

## Business, risk, and expansion

| Step | Contract | Prerequisites |
|---|---|---|
| 09 | [C13 fraud and C19 activity monitoring](09-risk-monitoring.md) | Stable money events, outcome labels, and reviewer roles. |
| 10 | [C15 settlement and C16 accounting](10-settlement-accounting.md) | Reconciled money flows, stable payout results, and rebuildable history. |
| 11 | [C17 disputes and C18 developer sandbox](11-disputes-developer.md) | Provider dispute and evidence interfaces. Tenant-safe API and event contracts. |
| 12 | [C20 split payments](12-platform-expansion.md) | Sub-merchant onboarding, settlement, and reports. |

Gate C covers repeatable settlement, a close without unexplained differences, reviewed monitoring alerts, measured fraud controls, and a hot-account load test. Set the pass thresholds before the test. C21 billing and C22 cards and savings remain deferred.

## Use the contracts

1. Select the requirement IDs for the change.
2. Read the relevant [architecture decisions](../adr/README.md) and [open choices](decisions.md).
3. Write the smallest Go slice that demonstrates the requirement.
4. Record the executed tests and remaining gaps in [PR evidence](../evidence/template.md).
