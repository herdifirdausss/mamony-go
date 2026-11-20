# Ledger requirements and Gate A

C01 is the sole writer of balances. See the [ledger and data ownership decision](../adr/0002-ledger-and-data-ownership.md).

The bootstrap checks journal structure and arithmetic in memory. Durable ledger operations and Gate A evidence are pending.

## Posting and replay

| ID | Requirement | Go acceptance check |
|---|---|---|
| LGR-01 | Post at least two entries. Use positive integer minor units. All entries and accounts must use the journal currency. Debit and credit totals must match. | Reject empty, unbalanced, zero, mixed-currency, closed-account, and unknown-account cases. |
| LGR-02 | Commit the journal, entries, balances, idempotency record, and one outbox row as one transaction. Reject overflow and enforce the floor of each account that requires it. | Race withdrawals against the same balance. Inject a failure after a write. Check for no partial changes and no event after rollback. |
| LGR-03 | Scope the key to the calling capability. Keep the key for the life of the journal. Equal content returns the original result. Different content with the same key fails without a write. | Start concurrent requests with the same key. Check one journal and one event. Repeat after a lost response following commit. The same key for a different caller is independent. |
| LGR-04 | Derive balances from entries. A read after an acknowledged posting includes that posting. | Compare the stored balance against the entries after commit, retry, and process restart. |

Use the signed 64-bit range for stored minor units. A posting amount must be positive. Compute intermediate totals without overflow. Keep key lookup and balance checks in the posting transaction. Lock affected accounts in a stable order. Test the chosen isolation level with real PostgreSQL connections and deterministic barriers.

Before implementation, record which fields are part of replay identity. Include the financial content. Decide how correlation metadata affects replay. For public requests, use the [HTTP idempotency contract](../adr/0003-public-http-contract.md#http-idempotency).

## Reversal, events, and verification

| ID | Requirement | Go acceptance check |
|---|---|---|
| LGR-05 | Protect journal and entry rows from updates and deletes. A reversal offsets the full original journal, refers to it, and follows all posting rules. Allow one reversal per journal. | Race two reversals with different keys. Check one winner and no changes to the original. Check that the floor can refuse a reversal. Attempt updates and deletes as the runtime and owner roles. |
| LGR-06 | Deliver the durable event at least once and in order per affected account. Deduplicate consumer work. Record audit entries for account and currency administration. | Exercise a multi-account journal and replay after an interrupted delivery. Check order, deduplication, and audit records. |
| LGR-07 | Run a daily read-only verifier. Recompute balances and validate journal totals, currency, and entry sequences. Report drift. | Plant controlled drift in an isolated database. Check the report and alert. Check that verification changes no ledger data. |
| LGR-08 | Restore the ledger, its grants, and its outbox from a backup. | Restore onto a fresh database. Compare the committed records and privileges. Run the ledger verifier on the restored data. |

## Gate A evidence

1. Exercise the posting, replay, and reversal checks against real PostgreSQL.
2. Run property tests against an independent ledger model.
3. Exercise event delivery, audit, and recovery in a synthetic test environment.
4. Record the restore drill and verifier result.

Use the [PostgreSQL contract](postgresql.md) for schema ownership and runtime roles. Apply its tenant-isolation tests to tenant-owned tables. Check C01 through its own database grants and its calling capability's authorization.

Record the commit, PostgreSQL version, migration version, test commands, and observed results in the [PR evidence](../evidence/template.md). All database checks above are **NOT RUN** in the bootstrap.
