# Payments and payouts

Build one vertical flow at a time. Both use [account mappings and policy](fees-limits-and-accounts.md), [screening](screening-and-onboarding.md), [the provider connector](provider.md), and [the ledger](ledger.md). Use [FX](fx.md) when the currencies differ.

The intent or payout owns business state. The provider connector owns the provider result. The ledger owns the money movement. Store their references together for reconciliation. Each accepted state change writes its event in the same local transaction. Use stable expected-state checks when two transitions compete.

## Payments

1. Create an intent with the tenant, exact amount, method, corridor, and merchant reference. Use an HTTP idempotency key.
2. On confirmation, reserve usage through the policy module. Screen the required parties. For later risk work, apply its decision here. Obtain a fee quote and, if needed, an FX quote.
3. If screening or risk requires review, keep the intent on hold. Do not start a provider attempt. Release unused resources when the intent ends without a successful posting.
4. Persist one active attempt and its provider reference. Call the provider connector outside a database transaction.
5. If the provider outcome is unknown, keep the attempt processing. Schedule inquiry to the same provider. Process later results once.
6. After provider success, make the required ledger postings with stable keys. Consume the fee quote in the posting transaction. Record the ledger result. Only then mark the intent succeeded.

| ID | Requirement | Acceptance check |
|---|---|---|
| PAY-01 | Enforce valid transitions and at most one active attempt. An intent can require action, enter a hold, process, or end in success, failure, cancellation, or expiry. | Race two confirmations. Race cancellation with a result. Check one accepted transition and one durable attempt. |
| PAY-02 | A provider success with missing ledger work stays processing. Retry recoverable posting errors with the same keys. Report a rule refusal to an operator. | Stop after provider success. Resume without a second charge. Check one ledger effect and one terminal success. |
| PAY-03 | Final or stale provider results cannot undo an intent's result. Record disagreements. Release or confirm reservations at their specified milestones. | Deliver a duplicate result and late conflict. Verify the event, reservation, and journal counts. |
| PAY-04 | Allow a refund only against a succeeded payment. Enforce the total amount still refundable. Give each refund a separate reference, lifecycle, and journal. | Race two partial refunds against the remaining amount. Check the ledger floor, configured fee treatment, and dispute restrictions. |

A processing intent can cancel only when the provider confirms cancellation under its method contract. A deadline schedules inquiry, review, or an allowed expiry transition. A deadline alone gives no proof of a failed charge.

## Payouts

1. Validate the tenant's recipient, account verification result, and screening decision. Store immutable destination versions. Replace a destination with a new recipient record.
2. Check policy and reserve usage. Apply later fraud policy if enabled. Obtain fee and FX quotes as required.
3. Hold the amount and fee in the ledger before sending. Move funds from available to in-transit with a stable reservation key.
4. Queue one operation for the selected provider. Respect its cutoff calendar. Record the expected dispatch time for a pending payout.
5. On definitive success, settle the hold and mark the payout paid. On definitive failure, release the hold and mark it failed. Each transition has a separate stable journal key.
6. When the return window closes, complete the payout. A return inside the window posts a corrective journal. Send a late return to reconciliation.

| ID | Requirement | Acceptance check |
|---|---|---|
| OUT-01 | A payout cannot dispatch without a committed hold and a valid recipient. | Race two payouts against the same available balance. Check the balance floor and provider dispatch count. |
| OUT-02 | Keep funds and usage reserved while the outcome is unknown. Query the original provider. Alert the operator before its deadline. | Stop the worker after send. Advance the clock. Check no blind resend, fallback, release, or second hold. |
| OUT-03 | Cancel locally before send when the state permits. After send, require the provider's outcome. Apply returns and final results once. | Race cancel with dispatch. Deliver duplicate and late return events. Check funds, fees, and status. |
| OUT-04 | Publish an event with each state change. Expose the provider reference, trace reference when present, and journal references to authorized reconciliation. | Rebuild a payout history from the references. Check one event per accepted transition. |

Use durable jobs with claims, expiry, and bounded retry intervals. A restarted worker must resume with the same operation and ledger keys. Use the [provider contract](provider.md) to determine whether another dispatch is safe.

## Pilot evidence

Exercise a successful payment and payout on the chosen corridor. Exercise refusal before dispatch, a lost response after dispatch, a lost response after ledger commit, and duplicate provider results. Reconcile the resulting journal, usage, fee, and provider records.
