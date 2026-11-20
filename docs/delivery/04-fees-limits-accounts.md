# 04 — Fees, limits, and accounts

Use [C01](../contracts/ledger-gate-a.md) for balances. Use [onboarding](03-screening-onboarding.md) for subject eligibility and tier.

## C06 fee quotes

| ID | Requirement | Acceptance check |
|---|---|---|
| FEE-01 | Price a command from a versioned rule for its kind, method, corridor, and currency. Compute the fee in exact minor units. Apply one named rounding rule to the total. | Test fixed, percentage, and tiered rules. Check boundary values and the total of split legs. |
| FEE-02 | Bind the quote to the tenant and command. Record its rule version, fee, legs, and expiry. Consume it once in the posting transaction through a ledger participant. | Reject expiry and changed input. Race two uses. Check that a rolled-back posting leaves the quote available. A replay returns the original posting. |
| FEE-03 | Apply a rule change only after an independent approval of its exact content. Keep the version that priced each quote. | Change a rule while an existing quote is active. Check that its recorded amount and version remain stable. |

## C07 policy and usage

| ID | Requirement | Acceptance check |
|---|---|---|
| LIM-01 | Return allow or deny with a reason and rule version. Deny unknown tiers, unavailable rules, disabled corridors, and active kill switches. | Test each refusal. No reservation exists after a denial. |
| LIM-02 | Check per-operation, daily, and monthly limits. Include used and reserved amounts. Create a reservation in the same transaction as the check. Use UTC calendar boundaries. | Race two requests against the remaining amount. Check that they cannot spend the same allowance. |
| LIM-03 | Confirm or release a reservation once for the applicable final outcome. Keep it active while a payout result is unknown. | Duplicate a confirm and release. Run expiry processing during an unknown payout. Check that the amount stays reserved. |
| LIM-04 | Bind an allow decision to the same tenant, amount, currency, kind, and command reference. Enforce the allow decision on applicable ledger postings. | Reject a different amount, caller context, expired allow, and a missing reference. Record the reason and rule version. |

The fee, usage, and money state have different owners. The workflow must state which ledger milestone confirms a reservation. See the [open choices](decisions.md#money-and-recovery).

## C02 account views

| ID | Requirement | Acceptance check |
|---|---|---|
| ACC-01 | Give an eligible owner one balance account per currency. Assign stable C01 account IDs to its named balances. Keep tenant ownership in C02. | Repeat an open after a partial failure. Check one mapping and one ledger account per named balance. |
| ACC-02 | Support active, suspended, and closed accounts. Refuse new movements on suspended or closed accounts. Close only when balances are zero and registered guards allow it. | Test onboarding and tenant suspension events. Race close with posting. Test the permitted in-flight behavior. |
| ACC-03 | Read balances from the ledger. Build statements from ordered entries with opening, movements, and closing. Mark unfinished periods provisional. Use the same lines for CSV. | Rebuild a statement with equal timestamps. Check exact totals, reversal references, and safe pagination. |
| ACC-04 | Apply tenant checks to account mappings and statements. Use C01 only through its authorized capability interface. | Attempt cross-tenant lookup and export. A ledger ID alone grants no access. |

Merchant balances can include available and pending funds. Pay-out adds an in-transit balance where its contract requires one. Define all named balances as configuration with explicit C01 account types.
