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

The [fee-quote boundary](fees-limits-and-accounts.md#fee-quotes) links the open FEE-02 conflict with Ledger-local ownership. Resolve that decision before implementing fee consumption in step 6.

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

Use the [simulated pilot roadmap](../roadmap.md#simulated-pilot) for sequencing. Map each scenario to the owning contract and record the evidence from each affected owner.
This is a reading map for the **planned synthetic pilot**. The roadmap gives dependency order. Each capability contract remains authoritative for its own states and requirements. This map does not claim that an end-to-end runtime or pilot gate has passed.

| Stage | Payment | Payout |
|---|---|---|
| Prerequisites | Start from the [pilot roadmap](../roadmap.md#simulated-pilot), [Identity](identity.md), [screening and onboarding](screening-and-onboarding.md), and [fees, limits, and accounts](fees-limits-and-accounts.md). | Use the same capability prerequisites, then validate the immutable recipient under [Payouts](#payouts). |
| Start and reserve | Follow [Payments steps 1–3](#payments): create the intent, reserve usage, screen parties, and obtain required quotes. | Follow [Payouts steps 1–3](#payouts): validate the recipient, screen, reserve usage, and commit the ledger hold before dispatch. |
| Convert currency when required | Read [FX execution and recovery](fx.md#execution-and-recovery). Check the current sequential-leg behavior and the proposed FX-05/06 atomic-group alternative; do not assume the alternative is accepted. | Apply the same FX contract and decision status when the payout currencies differ. |
| Dispatch and resolve the provider result | Follow [Payments steps 4–5](#payments) and the [provider operation lifecycle](provider.md#operation-lifecycle). An unknown result remains processing and uses inquiry against the original provider. | Follow [Payouts steps 4–6](#payouts). An unknown result keeps funds and usage reserved; use the original provider reference. |
| Record and reconcile money movement | Follow [Payments step 6](#payments), the [Ledger gate](ledger.md#ledger-gate-evidence), and [reconciliation](reconciliation-and-events.md#reconciliation). K3 posting and full Ledger runtime integration remain planned. | Follow the payout settlement, release, and return steps, then trace the journal and provider references through [reconciliation](reconciliation-and-events.md#reconciliation). |
| Check pilot recovery acceptance | Use the [pilot gate](../roadmap.md#simulated-pilot) for daily reconciliation and worker recovery, and the [database restore procedure](../runbooks/recovery.md#restore-a-database) for restore evidence. | Apply the same pilot and restore gate to the payout flow. |

Before implementation, resolve the [FEE-02 fee-consumption boundary](fees-limits-and-accounts.md#fee-quotes) and record the FX-05/06 decision. Read the existing payment and payout step lists for state transitions. The flow map links the contracts; it does not replace their requirements.

| Scenario | Evidence to trace |
|---|---|
| Refusal or race before dispatch | [Transaction authorization](#transaction-authorization-boundary), OUT-01's committed hold, and [provider operation identity](provider.md#operation-lifecycle). |
| Unknown result after dispatch | [Provider inquiry](provider.md#operation-lifecycle) uses the original provider and reference. OUT-02 keeps payout funds and usage reserved. See the [operator procedure](../runbooks/recovery.md#recover-an-unknown-provider-outcome). |
| Provider success with missing local posting | PAY-02 keeps the payment processing and uses the original posting keys on a recoverable error. Payout step 5 pairs paid status with hold settlement. The [reconciliation contract](reconciliation-and-events.md#reconciliation) covers missing or mismatched records. |
| Lost acknowledgement after Ledger commit | [Ledger replay and restore checks](ledger.md#recovery-tests) use the same key and verify a single journal and outbox effect. |
| Incomplete currency conversion | [FX execution and recovery](fx.md#execution-and-recovery) defines the current sequential-leg checks and links the proposed atomic alternative. |
| Duplicate or late provider evidence | [Provider inbound messages](provider.md#inbound-messages) retain conflict evidence. PAY-03 and OUT-03 handle duplicates and late results under their owning workflow. See [reconciliation](reconciliation-and-events.md#reconciliation) for tracked differences. |

## Transaction authorization boundary

A login session, API key or generic second-factor success is **not** permission to move money. The owning payment or payout use case must authorize the current subject/capability, operation, tenant, resource and account ownership and enforce the required assurance and policy. Where user-specific transaction approval is required, bind the challenge/receipt to immutable amount, currency, recipient, tenant, operation and intent version. Store and consume it at a documented consistency point with expiry, single-use behavior, revocation/freeze handling and no approval of changed content.

| ID | Requirement | Acceptance check |
|---|---|---|
| PAY-05 | For actions requiring approval, associate a canonical, immutable approved intent with exactly one executable operation. Reauthorize and fail closed if current subject, tenant, beneficiary or approval binding is invalid. | Reuse or change an approval; swap recipient, amount or currency; race revocation and two confirms; stop after consumption and retry. |
| OUT-05 | Dispatch an authorized payout only for the immutable verified recipient and current account owner, with a stable provider reference and applicable approval. A changed recipient requires a new authorization decision. | Try cross-tenant destination substitution, beneficiary replacement after confirmation, expired/frozen authority, duplicated dispatch and unknown external result. |

Record which operations are service-initiated and do not need a user's transaction approval; their service permission and business policy must still be explicit. Independent approval/execution trust domains are an advanced option, not an assumed property of the initial monolith. See [identity](identity.md#security-state-and-recovery-threat-controls) and [ledger](ledger.md#financial-authority-before-posting).

