# Split payments

Split payments add a platform and verified sub-merchants to the [tenant model](identity.md). This capability depends on the [payment flow](payments-and-payouts.md) and [settlement](settlement-and-accounting.md). Billing, cards, and savings remain deferred.

| ID | Requirement | Acceptance check |
|---|---|---|
| PLT-01 | Each sub-merchant has its own tenant scope, verification, screening, and accounts. Permit platform visibility only through a delegated rule. | Test sibling isolation, revoked delegation, and a suspended sub-merchant. |
| PLT-02 | Apply an approved, versioned split rule. Allocate exact minor units to sub-merchants and the platform fee. Record the rule and allocations with the payment. | Check rounding remainders and multiple destinations. Race duplicate work. Check the total and one set of postings. |
| PLT-03 | Settle and report each sub-merchant independently. Apply limits and monitoring at each permitted scope. | Reject a split that fails an applicable policy. Reconcile sub-merchant totals against the platform view. |
| PLT-04 | Record who bears refunds, disputes, and negative balances for the platform. Use that liability rule for recovery. | Test the chosen liability model with partial refunds and a dispute. Confirm no balancing error or cross-tenant action. |

The ledger still owns all monetary writes. The platform contract must specify the liability model before the first split payment. The pilot does not include this stage.
