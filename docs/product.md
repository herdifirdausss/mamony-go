# Product scope

Mamony Go is a simulator for cross-border pay-in and pay-out. Use synthetic identities, funds, providers, banks, and FX rates. Use test credentials for all integrations.

- **Pay-in.** A merchant accepts a payment from a payer in another country.
- **Pay-out.** A merchant sends funds to a recipient in another country.
- **Deferred scope.** Consumer wallets, billing, cards, savings, and split payments are outside the first pilot.

## Capability ownership

| Capability | Owns |
|---|---|
| C01 Ledger | Journals, account balances, reversals, and the ledger verifier. |
| C02 Accounts | Merchant account ownership and balance views. Monetary balances come from C01. |
| C03 Pay-in and C04 Pay-out | Payment and payout states. Each requests ledger postings through C01. |
| C05 Provider connector | Calls to simulated providers and the status of those calls. |
| C06 Fees and C07 Policy | Prices, fees, limits, and policy decisions. |
| C08 Identity and C09 Onboarding | Authentication, access decisions, and synthetic identity checks. |
| C10 Reconciliation and C11 Back office | Reconciliation findings and operator workflows. Corrections go through C01. |
| C12 Events | Event contracts and delivery. The capability that changes data owns the event. |
| C14 FX and C19 Compliance | Rates, FX positions, screening, and later activity monitoring. |
| C13 Fraud | Rules, scores, review, and risk evidence. |
| C15 Settlement and C16 Accounting | Merchant settlement batches, reserves, ledger projections, and period close. |
| C17 Disputes and C18 Developer tools | Dispute evidence, refund policy, API docs, and sandbox operations. |
| C20 Platform | Sub-merchant allocation rules and split payments. |
| C21 Billing and C22 Cards and savings | Deferred scope. |

## Pilot behavior

- Keep a provider call in an unknown state if the provider may have received it. Query its status or reconcile before retrying. Retry only if the provider contract makes it safe.
- Route each money correction through C01 as a new journal. Record the operator and approval for the correction.
- For operator changes to pricing, limits, or routing, bind the approval to the exact payload. Use a different person to approve. Check the approval and apply the payload once.

## Delivery gates

1. **Current bootstrap.** Run one Go process with two health listeners. Validate journal structure in memory.
2. **Gate A.** Demonstrate durable ledger posting, recovery, and audit with PostgreSQL. See the [C01 requirements and checks](contracts/ledger-gate-a.md).
3. **Gate B.** Run a simulated pilot on one corridor. Cover pay-in and pay-out, sanctions checks, provider failures and unknown outcomes, and daily reconciliation. Match FX positions to the ledger. Exercise recovery and record incident procedures.

The pilot corridor, observation window, and performance targets require a decision before Gate B. See the [implementation sequence](delivery/README.md) for the prerequisites and acceptance checks for each capability.
