# Product scope

Mamony Go simulates cross-border payments and payouts. Use synthetic identities, funds, providers, banks, and FX rates. Use test credentials for all integrations.

A merchant accepts money from a payer with a payment. A merchant sends money to a recipient with a payout. The first pilot uses one corridor. Consumer wallets, billing, cards, savings, and split payments are outside the first pilot.

## Capability ownership

| Capability | Owns |
|---|---|
| Ledger | Journals, balances, reversals, and verification. |
| Accounts | Merchant account ownership, ledger account mappings, and balance views. |
| Payments and payouts | Business state and requests for ledger postings. |
| Provider connector | Requests to simulated providers and the outcome of each request. |
| Fees and policy | Prices, fees, limits, and policy decisions. |
| Identity and onboarding | Authentication, access decisions, and synthetic identity checks. |
| Reconciliation and back office | Reconciliation findings and operator workflows. The ledger records approved money corrections. |
| Events | Event contracts and delivery. The capability that changes data owns its event. |
| FX and compliance | Rates, FX positions, screening, and activity monitoring. |
| Fraud | Rules, scores, review, and risk evidence. |
| Settlement and accounting | Merchant settlement batches, reserves, ledger projections, and period close. |
| Disputes and developer sandbox | Dispute evidence, refund policy, API docs, and sandbox operations. |
| Split payments | Sub-merchant allocation rules. |

See the [roadmap](roadmap.md) for the build order and delivery gates. The contracts define the money, approval, and recovery rules for each capability.
