# 10 — Settlement and accounting

C15 owns settlement schedules, reserves, and batches. C16 owns reporting projections and period close. C01 retains the authoritative journal. C04 executes pay-out requests.

## C15 settlement

| ID | Requirement | Acceptance check |
|---|---|---|
| STL-01 | Use a versioned schedule with the business-day calendar. Derive one batch ID from merchant account, schedule, and period. | Re-run a period and race two workers. Check one batch. Test a holiday and a missed run. |
| STL-02 | Select final, eligible records. Use the settleable balance and the applicable reserve and dispute holds. Confirm the reconciliation policy before including unresolved items. | Check excluded holds, unsettled payments, and missing reconciliation. Verify exact totals and carry-forward below the minimum. |
| STL-03 | Hold and release reserves through keyed C01 journals. Send the batch as a C04 payout with a stable reference. Follow the payout status. | Stop after the reserve journal and after dispatch. Resume with one reserve effect and one payout request. |
| STL-04 | Report the batch's payments, fees, disputes, reserves, and net payout. Mark a preview as an estimate. | Compare the exported totals to the batch, journals, and payout. Test operator pause and resume. |

A payout that is unknown keeps the settlement in progress. Apply its definitive result once. Review the reserve release and dispute accounting before closing the batch.

## C16 accounting and reporting

| ID | Requirement | Acceptance check |
|---|---|---|
| FIN-01 | Build a read projection from C01 events or a supported ledger read. Record its progress and lag. Provide a complete rebuild path. | Clear and rebuild an isolated projection. Compare report totals and handle duplicate events. |
| FIN-02 | Produce a daily trial balance by currency and account type. Export exact balanced journals with a stable batch ID. Keep exports immutable. | Compare debits and credits for each currency. Repeat export and check the same content. |
| FIN-03 | Close a period only when the trial balance matches the ledger and the required reconciliation and operations checks pass. Record refusal reasons. | Inject a discrepancy. Check no close or partial result. Resolve it, close once, and record the audit event. |
| FIN-04 | Record a later correction in the current open period. Keep closed figures unchanged. Limit merchant access to its own reports. | Apply a correction after close. Verify the old export remains stable and the new report has the adjusting journal. |

Commit to a replay or rebuild source before shipping a report that needs complete history. A short outbox retention window cannot rebuild a long-lived projection.
