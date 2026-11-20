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

## Account lifecycle and reads

| ID | Requirement | Go acceptance check |
|---|---|---|
| LGR-09 | Store account type, normal side, negative-balance rule, currency, and status. Use an opaque owner reference. Open with a stable account ID. | Repeating the same open returns one account. Reject incompatible reuse and a currency outside the enabled list. |
| LGR-10 | Close a ledger account only when its balance is zero and close guards allow it. Keep the account readable. | Reject postings to a closed account. Test the race between close and post. |
| LGR-11 | Read an account balance, journal, or account entries through the C01 interface. Sort entries by a stable key. Keep personal data and secrets out of all records. | Page through equal timestamps without overlaps or omissions. Check that a second query can reconstruct the balance. |

## Posting design to test

1. Validate the request and identify its caller and journal key. If a matching posting already committed, check its content and return the stored result.
2. Run registered posting gates for new work. A gate can refuse the request. Record the refusal reason without a ledger write.
3. Start a local transaction. Enforce key uniqueness in PostgreSQL. Handle the concurrent-claim path even if the initial lookup found no row.
4. Lock all affected account rows in a stable order. Read currency, status, floor, and balances under those locks. Check the final signed 64-bit balance bounds.
5. Write the journal, entries, balance changes, and per-account sequence. Store the replay result and one event. Run registered transaction participants on the same transaction.
6. Commit. If a participant fails, roll back its writes and all ledger writes. Return the recorded result.

The posting procedure chooses its own UTC record time. The journal stores the business reference and correlation ID. Use a database constraint for caller and key uniqueness. Use a second constraint for the original journal of a reversal.

Test `Read Committed` with explicit locks as a candidate. A Go design must prove the actual schema, queries, and transaction path under contention. Record a different choice if these tests fail.

## Recovery tests

| Failure | Expected state after recovery |
|---|---|
| The process stops before commit. | No journal, balance change, or outbox row survives. A repeat can post. |
| The transaction commits and the caller loses the response. | A repeat returns the same journal ID. Balances and outbox change once. |
| A consumer receives the event and the relay stops before acknowledgement. | The event can arrive again. The consumer applies one effect. |
| A backup restores. | Journal and entry totals, account sequences, permissions, and outbox records agree. The verifier detects any drift. |

## Data and package plan

| Record | Key or constraint | Owner |
|---|---|---|
| Accounts and account types | Stable account ID. One currency per account. Store the normal side and floor policy. Check status before posting. | C01 |
| Journals | Unique calling capability and journal key. Store a content fingerprint and the immutable result. Allow at most one reversal of an original. | C01 |
| Entries | Unique journal position. Reference the account and journal. Store a positive amount and the account order. | C01 |
| Account order and outbox | Transactional counters scoped to each affected account. Advance them under stable locks. A rollback leaves no account-order record for the rejected post. | C01 producer, C12 delivery |
| Verifier runs | Store when the verifier ran, the results, and any differences. Keep the balance data read-only during the verification pass. | C01 |

Use the existing journal domain package for shape and arithmetic checks. A ledger use case owns the posting sequence. Its repository applies the atomic operation and returns the durable result. The composition root supplies approved gates and participants. Use one transaction for participant effects and do not make network calls inside it.
