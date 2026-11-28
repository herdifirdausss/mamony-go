# Reconciliation and events

The reconciliation module explains differences. The back office owns operator workflows and access to the audit log. The event relay delivers events from the outbox of each producer. Capabilities keep their own business state.

## Reconciliation

Use the [pilot evidence scenarios](payments-and-payouts.md#pilot-evidence) to connect an operation to its original provider result, workflow state, committed journal references, and usage records. Retrieve facts through their owning interfaces and apply the matching rules below.

| ID | Requirement | Acceptance check |
|---|---|---|
| REC-01 | Ingest a provider report as a batch with source, period, file fingerprint, counts, and totals. Store each provider item and reject inconsistent totals before matching. | Ingest the same file twice. Test an invalid total and a duplicate external reference. |
| REC-02 | Match by provider reference and operation kind. Compare exact amount, currency, and status. Read internal records through the owners' interfaces. | Re-run a batch against the same inputs. Check stable results for matched, internal only, external only, duplicate, and mismatch. |
| REC-03 | Keep an internal item pending before its expected report time and grace period. Open a tracked case for a difference after the applicable wait. | Advance a test clock across the cutoff. Check that each unresolved item produces one case. |
| REC-04 | Record a case owner, state, age, evidence, and resolution reason. Apply an approved correction through the ledger. Keep a link to its journal. | Reapply the same approval. Verify one correcting journal and an auditable resolution. |
| REC-05 | Reconcile in-transit and pending balances with workflow records. Open a case for ledger verifier drift. Report suspense and unexplained differences daily. | Inject a missing report, ledger drift, and a nonzero unexplained total. Check the case and alert. |

A suspense account holds recorded differences under an explicit posting contract. Its balance must agree with its open items. Do not auto-correct a difference that needs operator approval.

## Operator workflow and audit

- Carry the operator's identity to the owning capability. Check permissions for the action and resource at the owner.
- Require a reason for each controlled action. Use [independent approval](identity.md#authorization-and-approvals) for designated changes. Store the approved content and outcome.
- Keep audit entries append-only. Record actor, target, time, action, result, and correlation ID. Test gap or tamper detection. Review where to hold the protected audit checkpoint.
- Define separate permissions for read, action, approval, and audit access. Attach a [recovery procedure](../runbooks/recovery.md) to each pilot alert.

**OPS-01.** Test view-only access, a refused action, independent approval, duplicate apply, and audit search. Verify that the console does not receive unrestricted access to a capability's data.

## Security evidence and mandatory notices

**OPS-02.** Security-sensitive account, role, recovery, approval, freeze and financial-administration changes must create a durable audit and, where applicable, a mandatory notification obligation in the same owning transaction as the accepted change. A marketing opt-out or change to a business read model cannot remove an already required obligation. Notification dispatch uses a restricted destination/policy history, retries without duplicating the logical notice, and separates `enqueued`, `provider accepted`, `delivered` and `user read` where observable.

**Acceptance:** Force the state write to succeed while the outbox write fails and assert the entire transaction rolls back. Tamper with marketing preference, suppress a worker, replay an event, alter a notice destination, attempt audit deletion as the runtime role and restore older backup state. Record gaps explicitly rather than claiming append-only storage prevents a fully privileged administrator from altering it. See [platform evidence](platform.md#security-operations-and-evidence), [identity lifecycle](identity.md#security-state-and-recovery-threat-controls) and [recovery procedures](../runbooks/recovery.md).

## Event delivery

| ID | Requirement | Acceptance check |
|---|---|---|
| EVT-01 | Give each event an ID, stable producer, type, version, aggregate, time, and correlation information. Keep an example and schema for each published version. | Validate an example and the actual producer output. Reject a breaking change without a new version. |
| EVT-02 | Keep delivery progress per consumer. Preserve ordering for each affected aggregate. Retry with bounds. Keep failures for review after the limit. | Stop one consumer during a multi-account sequence. Check that independent consumers continue. Check ordering on resume. |
| EVT-03 | Replay retained events with their original source and ID. Record operator identity and reason. Let the consumer deduplicate. | Replay a batch after an interrupted acknowledgement. Check one effect and an auditable replay. |

For the ledger gate, prove the producer transaction, minimal relay, audit, and recovery. For the pilot gate, add a catalog and the consumer controls used by the pilot flows. Merchant delivery and notices follow [merchant webhooks and notices](disputes-and-sandbox.md).
