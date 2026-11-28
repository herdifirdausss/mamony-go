# Recovery procedures

Run these procedures in the synthetic test environment before claiming a gate. Record the first signal, affected references, action, and result. Use authorized application commands and queries.

## Restore a database

Use this procedure to assess full restore acceptance for the affected capability. The PostgreSQL [evidence scope](../contracts/postgresql.md#implementation-and-evidence-scope) links the bounded PITR, Identity reconciliation, and Ledger K2a drills. Apply each result to its documented fixture scope. Full Ledger restore still requires the [Ledger verifier and gate evidence](../contracts/ledger.md#ledger-gate-evidence). Keep the recovery gate open when a required check or implementation is missing.

1. Select a backup and record its recovery point.
2. Restore onto a fresh server with the documented roles and migrations.
3. Compare journal counts, balances, sequences, outbox state, and grants.
4. Run the read-only ledger verifier.
5. Check that consumers can resume from their progress without a second effect.
6. Record elapsed time, missing data, and the decision to resume traffic.

## Investigate a ledger difference

1. Record the verifier run, affected account or journal IDs, and the difference.
2. Restrict affected money operations through the approved control if the difference is unexplained.
3. Compare entries, balances, and per-account sequences. Review the relevant audit and outbox records.
4. Request a correction through the ledger if the investigation requires one. Obtain approval for the exact change.
5. Run the verifier and reconciliation again. Resume affected work after they agree.

## Recover an unknown provider outcome

1. Locate the original operation and provider reference.
2. Check the durable operation and message history. Use an inquiry to the same provider.
3. Keep the payout hold while the provider outcome remains unknown.
4. Record the decision before the review deadline. Apply the definitive result once through the owning workflow.
5. Verify provider, ledger, and usage records. Open a reconciliation case for any disagreement.

## Recover a blocked consumer

1. Find the consumer, aggregate, and first event that blocks progress.
2. Check its attempt history, error, and deduplication record.
3. Resolve the cause. Replay the original event with approval where required.
4. Check the consumer effect and progress for the aggregate. Check that later events follow in order.

## Recover compromised identity or approval state

This procedure covers operator recovery after authority compromise or restore. See the [account-recovery contract](../contracts/identity.md#account-recovery-and-saved-codes) for customer recovery and the [recovery-session proposal](../proposals/identity-recovery-session-capability.md) for post-restore session policy.

1. Declare the attacker scope: application runtime, tenant/business tables, authoritative identity data, session store, keys, approval state, migration/deploy administration and backups. Record which independent authorities, if any, are still trusted.
2. Restrict affected authentication and financial actions using the owning authority. Do not report a freeze complete until the required deny state and propagation barrier are acknowledged.
3. Revoke compromised credentials and sessions, pause risky recovery and key actions, and preserve audit evidence outside the suspect writer's permissions where possible.
4. Verify trusted subject-to-credential and account-owner bindings. A row signature or a restored snapshot alone does not prove latest authorized state when its current-head authority was compromised.
5. Restore into an isolated environment. Compare credential generations, revoked sessions, consumed approvals, financial ownership, outbox/notice obligations and ledger journals against surviving trusted evidence. If independent evidence is unavailable, treat the result as an unresolved security hold.
6. Resume only after authorization negative tests, reconciliation and independent review of the scope and residual risk. Record measured recovery time, lost evidence and affected references.

This procedure has **not** been exercised in the HTTP bootstrap. The initial Profile A cannot claim containment of an arbitrary write to its own authoritative database.

## Recover a suppressed mandatory security notice

1. Find the original committed security command and its durable notification obligation, if the feature exists.
2. Compare the destination/policy generation and event ID against the owning authority, not against mutable marketing preferences.
3. Determine whether the failure is an unsent obligation, provider acknowledgement, or unconfirmed user receipt; do not treat these as equivalent.
4. Replay with the original event identity and bounded retries. Audit the repair and the verified dispatch result without duplicating the logical obligation.
