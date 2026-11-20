# Recovery procedures

Run these procedures in the synthetic test environment before claiming a gate. Record the first signal, affected references, action, and result. Use authorized application commands and queries.

## Restore a database

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
4. Request a correction through C01 if the investigation requires one. Obtain approval for the exact change.
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
