# Ledger

The ledger is the sole writer of balances. See the [ledger and data ownership decision](../adr/0002-ledger-and-data-ownership.md).

K1 and FND-02a–d are merged, isolated synthetic implementations. Durable Ledger posting and integration evidence are pending; see [current state](../current-state.md).

## K1 isolated monetary domain (synthetic experiment)

The owner accepted the limited pure-domain experiment in [#90](https://github.com/herdifirdausss/mamony-go/issues/90), following [#86](https://github.com/herdifirdausss/mamony-go/issues/86). This experiment may precede completion of Foundation and Identity; it does not change capability activation order or authorize K2–K6. No runtime integration, database, public posting, or real funds.

- LGR-01: validate 2–64 ordered positive int64 entries, one book and immutable currency definition, exact debit/credit equality. K1 domain tests use structural fixture values; separate FND-02a–d slices provide the unmounted curated reference and admission boundary, not persistent or production authority.
- LGR-09: three initial types (cash asset/debit, capital equity/credit, customer liability/credit), all zero floor; valid snapshots with nonnegative balances and sequences, active only for new posting plans. Identifiers and definition versions are nonempty valid UTF-8 up to 128 bytes, no NUL, compared bytewise.
- Arithmetic portion of LGR-02: exact wide intermediate net per touched account, final floor/range and sequence bounds, deterministic sorted steps, one sequence increment per account even for net zero; reject invalid/missing/duplicate/unreferenced snapshots and mismatches, no partial plan or input mutation.
- Acceptance: actual Go unit and independent model tests; persistence, replay, authority, reversal execution, outbox, restore, FND-02 reference integration and all database tests remain outside this slice. Synthetic experiment approval does not authorize merge or release.

## FND-02b isolated currency admission (synthetic experiment)

The owner accepted [#94](https://github.com/herdifirdausss/mamony-go/issues/94) after FND-02a post-merge verification. Application composition resolves the canonical USD definition from the Platform catalog and passes its code, exponent and definition version as Ledger-owned values to the unmounted facade. Ledger owns the USD-only enablement policy (FND-02 partial and LGR-09 enabled subset only). The facade admits exactly one valid USD definition from trusted composition, retains a private domain CurrencyRef, and refuses missing, duplicate, incompatible or disabled configuration, unknown/disabled lookup and nonmatching metadata. No default exponent, coercion, mutable configuration or domain import of Platform. A matching value is not authentication or authorization; a malicious trusted in-process constructor caller could supply a structurally valid false definition. The release ID is not the definition version. FND-02d [#100](https://github.com/herdifirdausss/mamony-go/issues/100) adds explicit allocated definition and monetary-unit IDs to the Ledger-owned DTO through the same unmounted composition. USD remains the sole enabled currency. Admission validates shape and code-aligned namespaces, not catalog authenticity; RequireCurrency compares the entire admitted tuple including unit ID. Legacy v1 and release IDs are refused. K1 uses the explicit definition ID as its structural version without changing the domain or arithmetic. The immutable baseline binds unit/exponent to that ID; effective dates, real history, correction and retirement policy still require review before persistence. No posting API, account creation, database, K2, startup wiring, runtime integration or real funds in this experiment.

## Canonical ownership and bounded USD definition freeze (future design)

Owner-approved documentation-only direction in [#87](https://github.com/herdifirdausss/mamony-go/pull/87): Ledger owns its local financial transaction and financial records; Platform owns the canonical reference. Future Ledger persistence would keep an immutable local admitted-definition snapshot with exact code, exponent, definition ID and monetary unit ID, sourced from trusted composition. It is evidence bound to stored amounts, not a second canonical currency authority. Dataset release and checksum are provenance, not definition identity. No cross-owner foreign key or routine shared transaction is required.

Option B freezes only USD / 2 / `mamony-currency-v2:USD:1` / `mamony-unit-v1:USD:1` for any separately approved bounded synthetic K2. Refuse other enabled currencies, missing or mismatched metadata, legacy identity and currency evolution until a separately reviewed lifecycle design permits transitions. Never reinterpret existing amounts or silently replace a stored definition. Real historical provenance, retirement, correction and conversion remain out of scope; exact snapshot schema, keys, constraints, grant matrix, restore and authority design require separate acceptance. No persistence or K2 authorization follows from this direction.

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

## Ledger gate evidence

1. Exercise the posting, replay, and reversal checks against real PostgreSQL.
2. Run property tests against an independent ledger model.
3. Exercise event delivery, audit, and recovery in a synthetic test environment.
4. Record the restore drill and verifier result.

Use the [PostgreSQL contract](postgresql.md) for schema ownership and runtime roles. Apply its tenant-isolation tests to tenant-owned tables. Check the ledger through its database grants and its calling capability's authorization.

Record the commit, PostgreSQL version, migration version, test commands, and observed results in the [PR evidence](../../.github/pull_request_template.md). All Ledger database checks above remain **NOT RUN**; isolated domain/reference test evidence does not satisfy them.

## Account lifecycle and reads

| ID | Requirement | Go acceptance check |
|---|---|---|
| LGR-09 | Store account type, normal side, negative-balance rule, currency, and status. Use an opaque owner reference. Open with a stable account ID. | Repeating the same open returns one account. Reject incompatible reuse and a currency outside the enabled list. |
| LGR-10 | Close a ledger account only when its balance is zero and close guards allow it. Keep the account readable. | Reject postings to a closed account. Test the race between close and post. |
| LGR-11 | Read an account balance, journal, or account entries through the ledger interface. Sort entries by a stable key. Keep personal data and secrets out of all records. | Page through equal timestamps without overlaps or omissions. Check that a second query can reconstruct the balance. |

## Financial authority before posting

**LGR-12.** A ledger request must come from an explicitly permitted calling capability and carry its authenticated service authority, tenant/resource reference, canonical financial content and applicable posting policy. The business owner resolves the current subject, account ownership, action, beneficiary and approval when required; the ledger enforces its registered posting gate and protects monetary invariants independently. A client-provided `approved` flag, valid user session, known account ID or provider callback cannot directly authorize a journal.

**Acceptance:** Try calling the ledger facade from a disallowed capability, substituting a tenant/account/recipient, replaying an approval for a different amount/currency, using it twice or after expiry/revocation, and racing a frozen account with posting. Require one durable effect or a safe refusal. For a protected high-impact action, the approval receipt must be bound to the exact intent and consumed under an explicitly documented consistency boundary; do not claim an independent approval domain exists in the current monolith.

Money that has left the application through an external provider is not covered by a local database transaction. Keep unknown outcomes unresolved, inquire using the original stable reference, and reconcile. See the [payments workflow](payments-and-payouts.md) and [provider connector](provider.md).

## Posting design to test

1. Validate the request and identify its caller and journal key. If a matching posting already committed, check its content and return the stored result.
2. Run registered posting gates for new work. A gate can refuse the request. Record the refusal reason without a ledger write.
3. Start a local transaction. Enforce key uniqueness in PostgreSQL. Handle the concurrent-claim path even if the initial lookup found no row.
4. Lock all affected account rows in a stable order. Read currency, status, floor, and balances under those locks. Check the final signed 64-bit balance bounds.
5. Write the journal, entries, balance changes, and per-account sequence. Store the replay result and one event. Any atomic participant effects must be Ledger-owned and within the same local transaction; independent capabilities record their own durable outcomes through explicit orchestration.
6. Commit. If a Ledger-owned participant fails, roll back the whole local Ledger transaction; cross-owner progress is recovered separately, not rolled back by sharing this transaction. Return the recorded result.

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
| Admitted currency snapshot (future) | Exact immutable canonical code/exponent/definition ID/unit ID; release provenance separate; initial synthetic USD freeze only. | Ledger (Platform canonical source) |
| Accounts and account types | Stable account ID. One currency per account. Store the normal side and floor policy. Check status before posting. | Ledger |
| Journals | Unique calling capability and journal key. Store a content fingerprint and the immutable result. Allow at most one reversal of an original. | Ledger |
| Entries | Unique journal position. Reference the account and journal. Store a positive amount and the account order. | Ledger |
| Account order and outbox | Transactional counters scoped to each affected account. Advance them under stable locks. A rollback leaves no account-order record for the rejected post. | Ledger producer, event relay |
| Verifier runs | Store when the verifier ran, the results, and any differences. Keep the balance data read-only during the verification pass. | Ledger |

Implement journal shape and arithmetic checks in the ledger domain package. A ledger use case owns the posting sequence. Its repository applies the atomic operation and returns the durable result. The composition root supplies approved gates and Ledger-owned participants. Use one Ledger-local transaction for its effects; no foreign-owner transaction handle, private schema mutation, or network call inside it. Cross-owner work uses separately accepted authorized commands and durable recovery. Exact authority and currentness remain open.
