# Ledger and cross-border payments: financial architecture discovery

- Status: PROPOSED — G0/G1 research and candidate G2/G3 decisions; no approval to implement.
- Intake: [#86](https://github.com/herdifirdausss/mamony-go/issues/86).
- Baseline: main f82b47c5a198e731f652053bc050aace3d91f58c; reviewed 2026-11-24.
- Owner: repository owner; independent technical reviewer not yet designated.
- Critical: monetary semantics, authorization, external settlement and irreversible records.
- Governing references: [ADR-0002](../adr/0002-ledger-and-data-ownership.md), [ledger](../contracts/ledger.md), [FX](../contracts/fx.md), [payments](../contracts/payments-and-payouts.md), [provider](../contracts/provider.md), [accounts](../contracts/fees-limits-and-accounts.md), [reconciliation](../contracts/reconciliation-and-events.md), [settlement](../contracts/settlement-and-accounting.md), [decisions](../open-decisions.md), [workflow](../development-workflow.md).
- Scope is synthetic only. This proposal changes no accepted contract, API, migration, deployment or active Identity work.

## G1 — Findings and evidence

At baseline, the tree has Identity and platform migrations, but no ledger module or ledger migrations. LGR-01–12, FX-01–06, PAY/OUT, PRV, REC, STL and FIN are contracts, not evidence of working financial controls. Existing contracts are strong on double entry, durable replay, reversal, balance floors, external uncertainty and recovery. Earlier design material was reviewed for journal schema, account lifecycle, pay-in, payout, FX, reconciliation and settlement; its algorithms are candidates, not Go verification. The earlier design also identifies a close-guard race and nonmonotonic wall timestamps versus account sequence. Do not reproduce these weaknesses or carry language-specific architecture forward.

### Primary evidence register

All links reviewed 2026-11-24; external products have different semantics and operating envelopes. No external benchmark is an acceptance result.

| Source / section | Observation | Application / limit |
|---|---|---|
| [Stripe Ledger engineering: semantic store, immutability, completeness, correction](https://stripe.com/blog/ledger-stripe-system-for-tracking-and-validating-money-movement) | Records fund flows and monitors incomplete or unexplained events across systems. | Separate mathematical consistency from external completeness. Their reported scale is not a Mamony target. |
| [Uber: Zero-Sum by Design, immutable money order, consistency, hot entity](https://www.uber.com/bo/en/blog/ubers-payments-platform/) | Financial intent, entries and balance mutations have distinct representations; shared accounts create throughput bottlenecks. | Preserve business reference and account-level contention metrics; don't copy the storage architecture. |
| [Modern Treasury: ledger guarantees](https://docs.moderntreasury.com/ledgers/docs/ledgers-guarantees) | Per-currency balance, atomic writes, account versions and posted immutability. | Retain exact local posting and versioned statement reads. |
| [Modern Treasury: concurrency](https://docs.moderntreasury.com/ledgers/docs/handle-concurrency) | Differentiates synchronous guard-dependent balances and asynchronous shared accounts. | An eventual projection must never authorize spending; current Mamony strong read-after-write contract remains intact. |
| [TigerBeetle data modeling](https://github.com/tigerbeetle/tigerbeetle/blob/main/docs/coding/data-modeling.md), [two-phase transfers](https://github.com/tigerbeetle/tigerbeetle/blob/main/docs/coding/two-phase-transfers.md), [transfer reference](https://github.com/tigerbeetle/tigerbeetle/blob/main/docs/reference/transfer.md) | Currency partition, reserve/resolve, linked atomic operations. | Compare atomic internal FX and explicit hold lifecycle; do not confuse pending transfer expiry with external failure. |
| [PostgreSQL 18 explicit locking](https://www.postgresql.org/docs/18/explicit-locking.html) (major-version reference) | Row locks and deadlock behavior. | Validate exact installed version, lock order and privileges with real sessions; no claim of deadlock freedom. |
| [Wise API](https://docs.wise.com/api-reference), [tracking](https://docs.wise.com/guides/product/send-money/tracking/track-transfers) | Distinct quote, recipient, transfer resources; asynchronous tracking; sent to recipient bank is not necessarily beneficiary credited. | Model separate evidence milestones and durable provider references. |
| [Adyen reconciliation](https://docs.adyen.com/platforms/reconciliation-use-cases), [accounting report](https://docs.adyen.com/platforms/reports-and-fees/balance-platform-accounting-report/) | Webhooks/API for near-real-time and report reconciliation for daily completeness. | Compare internal workflow, ledger, provider and bank cash separately. |
| [ISO 4217](https://www.iso.org/iso-4217-currency-codes.html), [SIX maintenance agency](https://www.six-group.com/en/products-services/financial-information/market-reference-data/data-standards.html) | Currency reference data includes minor units and changes over time. | Version reference data; prevent reinterpretation of prior amounts. |
| [Stripe currencies](https://docs.stripe.com/currencies) | Provider operation-specific amount rules can differ from canonical currency units (e.g. HUF payout). | Adapter validates mapping and divisibility, not ledger-wide scale overrides. |
| [BIS payments without borders](https://www.bis.org/publications/payments-without-borders), [CPMI programme](https://www.bis.org/committees/cpmi/cross-border-payments/overview), [BCBS FX risk](https://www.bis.org/committees/bcbs/basel-consolidated-guidelines/module/rma/20) | Correspondents, liquidity, operating windows and FX principal risk are independent of internal posting; PvP reduces settlement risk. | Simulate prefunded payout and distinguish internal atomicity from real PvP. |
| [CPMI harmonised ISO 20022 updated report](https://www.bis.org/publications/harmonised-iso-20022-data-requirements-enhancing-cross-border-payments-updated-report), [Swift CBPR+](https://www.swift.com/standards/iso-20022/iso-20022-financial-institutions-focus-payments-instructions), [Swift UETR](https://www.swift.com/payments/what-unique-end-end-transaction-reference-uetr) | Structured cross-border payment data and stable end-to-end reference. | Internal typed canonical instruction plus adapter mapping; no claim of Swift access, certification or universal UETR. |
| [FATF R.16 June 2025 revision](https://www.fatf-gafi.org/content/fatf-gafi/en/publications/Fatfrecommendations/update-Recommendation-16-payment-transparency-june-2025.html), [June 2026 guidance consultation](https://www.fatf-gafi.org/en/publications/Fatfrecommendations/R16-Public-Consultation-June-2026.html) | Payment transparency and fraud/error mitigation; consultation distinct from adopted text. | Model data availability, screening and jurisdiction-specific controls; implementation dates/legal applicability need separate review. |

### Gap analysis and candidate disposition

| Baseline | Finding | Proposal (not accepted) |
|---|---|---|
| LGR-01/02/04: one-currency journals and balance | Retain; clarify book/legal-entity boundary and net per-account delta before floor. | Account and journal book identity; no direct cross-book post. Currency/book invariants in database write boundary. |
| LGR-03/12: capability-scoped replay and posting gates | A key is not authorization; pre-lock freeze/close check can go stale; tenant context and original result confidentiality matter. | Bind authenticated caller, book/tenant where applicable, operation and canonical payload; reauthorize result read; fenced gate and close obligations inside same transaction. DEC-02/07/16. |
| LGR-05: full reversal | Not equivalent to partial refund, return, dispute or correction; reversing a reversal and refusal on closed accounts need policy. | Separate business operations and approved adjustment types; do not bypass floor or edit history. |
| LGR-06/07/08 + EVT | Per-account ordering and read-only verifier already specified; incomplete upstream activity can still be balanced. | Add consistent snapshot/sequence watermark, missing-reference and bank cash checks, multi-account ordering tests and restore replay checks. |
| ACC-01–04 | Named balances exist; chart/economic ownership and spendability are not fully specified. | Explicit liabilities, assets, revenue, clearing, in-transit, reserves, FX bridge and suspense; distinguish spendable from physical cash. |
| FX-01–04 | Exact arithmetic present; canonical currency metadata and provider amount constraints incomplete. | Version ISO reference and adapter units; define signed range, intermediate sums, fixed-side rounding and quote expiry. DEC-06/08. |
| FX-05/06 and ADR-0002 | Sequential local sell and buy create avoidable partial state; independent external legs can be inherently non-atomic. | Consider linked single-transaction multi-journal posting for internal conversion; retain external saga only where truly external. Requires explicit superseding decision. |
| PAY/OUT, PRV | Unknown handling strong; generic success can obscure finality. | Track submission, acceptance, debit, beneficiary credit, settlement and return evidence per rail. Never equate sent with received. |
| REC-01–05 | Exact match is a good first step, but reference scope and aggregate/net cash lines need modeling. | Reconcile transaction and bank/settlement levels with provider-account-scoped IDs, one-to-many links, per-currency totals, durable evidence and aging. |
| STL/FIN | Batch/report and period close exist; cutoff and cash timing not fully explicit. | Separate authorization, capture, clearing, bank settlement, merchant availability and payout. Define value/booking dates, calendars, close watermark. |
| Screening/provider | No actual jurisdiction/rail mapping approved. | Synthetic corridor profile, typed party data, screening hold and privacy constraints; no live compliance claim. |

## G2 — Proposed scope and acceptance

Actors: merchant/customer, authorized financial capability, provider simulator, finance/reconciliation operator and auditor. First goal is demonstrably correct local money recording; next goal is explainable end-to-end synthetic cross-border payment. No live money, cards, acquiring network membership, banking license, direct Swift connection, lending, crypto, multi-region or public money endpoints. No roadmap reordering: the accepted Identity-before-ledger direction remains.

Candidate acceptance requirements are identified below by existing contract IDs and descriptive names; assign new stable IDs in owning contracts only after G2 acceptance.

| Trace | Proposed falsifiable acceptance | Status |
|---|---|---|
| LGR-01/02, book boundary | Reject mixed currency/book, invalid account, zero/negative entry, overflow of totals or balance, unbalanced journal; no persistent effect. | NOT RUN |
| LGR-02/03/04 | On 100 contenders for 10 units using 1-unit debits, exactly ten committed debits if fixture has no other failure; retries do not add entries. Same-key different content refused; ambiguous commit returns original journal. | NOT RUN |
| LGR-09/10/12, DEC-07/16 | Race posting against close/freeze/revocation; outcome respects one specified linearization boundary. Cross-tenant or forged caller cannot post or read replay. | NOT RUN |
| LGR-05 | Two concurrent full reversals produce at most one; floor refusal leaves original immutable. Distinct partial refunds sum at most original refundable amount. | NOT RUN |
| FX-01–04 | Compare conversion against independent rational oracle including zero/two/three decimal currencies, provider divisibility and range boundaries; quote once, immutable amounts. | NOT RUN |
| FX-05/06 candidate | Local grouped FX with second-leg fault leaves neither journal nor consumed quote; ambiguous commit replay returns both. External partial evidence is visible and cannot be falsely marked settled. | NOT RUN |
| OUT-01/02, PRV-01–08 | Concurrent reserve, cancel/send and timeout-after-acceptance preserve hold, one external operation, no blind retry/fallback; late return becomes linked correction. | NOT RUN |
| REC/FIN | Omit a provider line while all journals balance; raise a completeness case after grace; duplicate/corrected report, net fee, multi-line settlement and bank mismatch are separately explainable. | NOT RUN |
| LGR-06–08, EVT, DEC-10/12 | Interrupted relay, duplicate events, per-account order, snapshot rebuild, backup restore and privilege checks; distinguish local restore from external history. | NOT RUN |

## G3 — Candidate technical design and threat review

### 1. Monetary model and accounting boundary

Define a book as an accounting boundary for a single synthetic legal entity. Accounts carry book ID, currency, immutable account classification/normal side, floor policy, owner reference and status. Assets/expenses normally debit; liabilities/equity/revenue normally credit. The signed normal-side balance is the sum of net entry effects, not a freely editable field. Tenant visibility is not the same as book membership: platform shared accounts may span multiple merchant flows, while only explicitly authorized capabilities can post specific templates for an authorized tenant. Multiple entities and intercompany accounting are deferred; no accidental cross-book journals.

Canonical amounts are positive int64 minor units, no float. Use exact bounded intermediate arithmetic (candidate Go math/big and PostgreSQL numeric with explicit limits). Validate max entries, total sizes and resulting balances before cast. Store currency/exponent identity or immutable version; never reinterpret historical entries. Wire-format DEC-06 remains open (candidate canonical decimal string of minor units plus currency; reject ambiguous numbers). Provider units and operation minima/maxima are validated separately. FX rate direction and scale are explicit.

### 2. Illustrative accounting lifecycle (not an approved chart of accounts)

One platform book. Assume synthetic USD prefunded/settled cash, a merchant owns a USD liability, USD fee income recognized on capture for illustration. All amounts in USD cents. No taxes, chargeback, bank fees or revenue-recognition advice.

| Event / journal | Debit | Credit |
|---|---|---|
| Pay-in confirmed; provider receivable created | Provider receivable 10000 | Merchant pending liability 9700; fee revenue 300 |
| Provider cash received | Bank cash 10000 | Provider receivable 10000 |
| Merchant funds eligible | Merchant pending liability 9700 | Merchant available liability 9700 |
| Payout hold | Merchant available liability 4000 | Merchant in-transit liability 4000 |
| External cash debit evidenced; payout obligation extinguished under rail policy | Merchant in-transit liability 4000 | Bank cash 4000 |
| Alternative definitive pre-debit failure | Merchant in-transit liability 4000 | Merchant available liability 4000 |

After successful payout: bank cash 6000, available liability 5700, revenue 300; assets = liabilities + recognized revenue in this simplified example. The failure row is an alternative to success, not an additional journal. For debit before beneficiary settlement, use a separate payout-clearing asset or obligation as appropriate to the accepted accounting policy; do not prematurely extinguish a customer obligation. Provider accepted status alone does not prove cash received. Pending liability is not spendable. Suspense is an explicit recorded exception, never an automatic balancing plug. Decide how returns, losses, chargebacks, negative merchant balances and fee reversals affect chart and authority before implementation.

FX example (illustrative quote, not a market rate): USD 100.00 -> SGD 130.00; USD exponent 2 and SGD exponent 2. Two journals:
- USD debit customer USD liability 10000, credit USD FX bridge 10000.
- SGD debit SGD FX bridge 13000, credit customer SGD liability 13000.
Each journal balances separately; bridge accounts encode position, not free money or proof of bank cash. Provision and reconcile target-currency liquidity before payout; define inventory/bridge normal sides, permissible floors, risk limits and valuation/P&L with finance review. No summing unlike currencies.

### 3. Transaction and persistence candidates (DEC-02/07/16)

Keep existing Go modular monolith, one PostgreSQL database and capability schemas. Candidate package layout follows current conventions: internal/modules/ledger with public capability facade and internal/domain, internal/usecase, internal/repository/postgres. No public journal write route. Candidate schema includes books, currency versions, account types, accounts, journals, entries, posting replay record (or journal unique key), group identity if approved, outbox and per-account ordering metadata. Balance snapshots are derived and checked; journals/entries immutable for runtime role. Use composite foreign keys/constraints to bind account/book/currency, unique journal position and account journal sequence plus position, unique reversal reference and idempotency scope. No raw PII in ledger references or events.

Compare A: Go-orchestrated SQL in one pgx transaction, easier idiomatic orchestration; direct runtime DML grants could bypass invariants if compromised. B: narrowly granted PostgreSQL posting function with fixed search_path, qualified objects, non-login owner, explicit EXECUTE only, no PUBLIC execution, database validation and restricted caller identities; stronger database mutation boundary at higher SQL complexity. C: dedicated ledger engine (TigerBeetle), strong primitives but adds operational and cross-store atomicity costs. Recommend evaluating B and A against the actual PostgreSQL role model; B preferred if preventing arbitrary DML is required. No final choice until G3. A SECURITY DEFINER function is not protection against its owner/superuser or a valid but malicious posting capability; validate caller identity, policy and allowed templates independently.

Candidate transaction:
1. Authenticate capability, resolve exact intent/tenant/operation and authorized result access. Canonicalize a versioned payload of book, currency, reference, operation kind, account IDs, entry directions/amounts and relevant policy/approval binding. Specify whether ordered lines are semantic; candidate preserve positions, exclude trace/correlation transport metadata. Bound size. Derive fingerprint in trusted boundary; caller cannot supply arbitrary hash as authority.
2. Begin local transaction, claim unique scoped key. A concurrent claimant waits; matching committed record returns authorized original result; changed content refuses. Rollback releases claim. Treat unknown commit as unresolved and query/retry same key, never a new operation. A denied posting is not a posted replay result; retain separate audit policy.
3. Lock required authority/obligation rows and all affected accounts by global documented order, using deterministic sorted identifiers and explicit row locks. Read status, floor and gate policy under locks; freeze/close/approval consumption share this consistency boundary. No network calls, no arbitrary participant code/SQL, no stale external checks.
4. Validate exact per-currency debit/credit and net deltas. Assign account sequence once per account per journal; group operations lock union of accounts first. Write journals, entries, balances, replay result, transactional outbox and authorized participant effects. Check immutable accounting shape at commit and deny unauthorized direct DML. Atomic account-opening/close guard or obligation fence required; a pre-check alone is insufficient.
5. Commit; return IDs. Retry bounded deadlock/serialization failures with same business identity only when known safe; ambiguous result requires replay lookup. Read authoritative primary for guaranteed post-ack visibility. Stable statement pagination uses account sequence and entry position plus fixed high-water mark, not timestamp or random IDs as commit order. Multi-account snapshot requires one consistent transaction.

READ COMMITTED plus explicit locking is candidate for known finite rows; predicate constraints require unique/exclusion constraints, locked guard rows or SERIALIZABLE with whole-transaction retry. Measure actual lock waits, deadlocks, SQLSTATE handling and unknown-commit behavior. Ledger event delivery must wait for predecessor events on all touched accounts; event ID deduplication and consumer progress commit together. No global exactly-once claim. Use immutable durable source for rebuild beyond outbox retention.

### 4. FX: local atomicity versus external settlement

| Option | Benefit | Risk / decision |
|---|---|---|
| Existing sequential two-journal FX | Independently recoverable legs; compatible with separate stores | Avoidable partial state for local conversion. |
| One local transaction with two independently balanced journals and group key | All-or-none visible balances and quote consumption | Needs ledger group API, union lock ordering, replay/event semantics, position participation. Recommended for internal conversion, subject to owner acceptance and ADR change. |
| Real external settlement / two institutions | Represents reality of asynchronous rails | Cannot use one local DB transaction to guarantee counterparties settle. Keep durable evidence, reservations, inquiry, compensating journals and exposure limits. PvP is a real settlement arrangement, not a local SQL property. |

For local group replay, persist group fingerprint, constituent journal IDs and deterministic keys in same transaction, reject independent replay of constituent legs as new work, lock quote/position and accounts consistently, commit both outbox records or none. Decide group reversal and per-account event semantics before code. Do not implement group posting before single-journal proof. External quote expiry after funds arrive (DEC-08) must choose guaranteed quote funding, explicit re-quote/customer consent or approved exception; no silent worse rate.

### 5. Cross-border payment model

Separate messaging, payment workflow, clearing, settlement and internal accounting. Correspondent banking can use nostro/vostro accounts (same relationship viewed from opposite banks); a fintech can prefund destination currency with a partner and use local domestic rails; direct/interlinked fast-payment systems and wholesale PvP have distinct membership and finality rules. Swift transports messages, not balances by itself. Compare these options without implementing live connectivity.

Candidate pilot: USD -> SGD, synthetic prefunded local payout via simulated partner; not an approved corridor. Alternative USD -> IDR or same-currency control fixture. Corridor config records source/destination jurisdiction, currencies, provider-account identity, rail kind, calendar/timezone, cutoffs, limits, supported parties, fees/FX policy, evidence rules and return policy. No assumption all corridors are 24/7 or share finality. DEC-09 owner decision required.

Lifecycle: create immutable versioned instruction -> authenticate/authorize/screen -> fix quote & fees -> collect funds with provider evidence -> post pending and availability according to cash/risk policy -> reserve source and target capacity -> local FX group (if accepted) -> hold payout -> durable operation dispatch -> inquiry/webhook evidence -> record cash/beneficiary outcome under rail semantics -> reconcile -> settle/close or linked return/correction.
Keep workflow status separate from dispatch, provider evidence, journal group, bank settlement and beneficiary delivery. A simulated rail can define explicit states not_sent/accepted/sent/credited/rejected/returned/unknown with permitted transitions and evidence requirements. "Sent" cannot mean beneficiary credited unless proven by selected rail. Unknown locks funds/capacity and routes to inquiry; no blind retry, failover or expiry release. Use stable operation ID and provider-account-scoped reference; optional external UETR only for applicable rail; store original external statuses, timestamps, value date and provenance. Model cancellation/recall as requests with outcome, not guaranteed reversal.

Canonical instruction: internal transfer ID, idempotency identity, end-to-end reference, source/target amounts and currencies, immutable beneficiary version, payer/payee references, quote and fee versions, purpose and required party data under corridor policy. Store sensitive party data in authorized owning module, pass opaque IDs to ledger and minimized event payload; no raw account numbers or personal details in audit/logs. Map canonical model to selected provider/ISO 20022 adapter only when requirements are verified. Investigate pacs.008/pacs.002/pacs.004 and camt.053/.054 in the rail-specific design; do not claim schema conformity from a field name. Actual FATF/local licensing, safeguarding, AML/sanctions, reporting, privacy and retention are jurisdiction dependent; synthetic tests cannot establish compliance.

### 6. Reconciliation, cash and operational risk

Reconcile independently:
- Internal: journal balances, account sequence, snapshot projection, holds versus workflow, quote/fee/limit consumption, outbox completeness.
- Transaction: internal intent/attempt and journal reference versus provider operation and signed webhook/inquiry/report; identify missing/duplicate/conflict/amount/currency.
- Cash: provider clearing and payout settlement totals versus actual simulated bank statements, opening + movements = closing by provider account, currency and value date; allow explicit one-to-many/many-to-one matching, separately recorded fees and net settlements. Raw evidence fingerprint, ingestion version, provenance and correction/replacement chain; unmatched items are not silently discarded.
- Finance: trial balance by book/currency, suspense items versus suspense balance, ledger history watermark, immutable closed-period snapshot, subsequent approved adjusting entries.

Late/duplicate reports, missing files, cutoff/timezone, partial settlements, provider cash debit without confirmed beneficiary, returns after close, restore after external send and stale webhook are distinct test cases. Case owner, aging, reason, linked correction approval and reproducible matching-rule version required. A zero-sum ledger can still be wrong economically. Reconciliation is necessary across external boundaries even when all internal writes are atomic.

### 7. Threats, failure matrix and verification

| Threat/failure | Control candidate | Required evidence |
|---|---|---|
| Untrusted customer or compromised ordinary handler | No ledger DML, capability allowlist, tenant/intent/freeze binding | Use actual runtime grants; cross-tenant substitution, direct call and stale approval rejection. |
| Compromised permitted capability | DB template restrictions, limited account class and bounded approvals; residual legitimate-authority abuse | Try arbitrary accounts, amount and replay; explicitly state trusted service and DB owner limits. |
| Same-key race, response lost, process killed | Unique scope, stable request and durable replay | Independent connections, barriers and kill/restart at pre/post commit. |
| Wrong accounting or integer overflow | Independent exact oracle; DB invariants and verifier | Random sequences, boundary values, reversals and drift fixtures. |
| Hot shared account | First measure synchronous locked baseline; only then investigate serialized batches or shard clearing accounts with exact liquidity rules | Contention distribution, throughput, p50/p95/p99, lock waits, fairness, deadlocks and invariant checks. Do not weaken floor/strong reads silently. |
| Forged or reordered external evidence | Provider-specific verification, durable inbox, state transitions and reconciliation | Wrong signature/key, duplicate, unknown, late conflicting success/return. |
| Operator tampering or restore gap | Least privilege, append-only correction, reviewed audit checkpoints, backup and external replay | Runtime UPDATE/DELETE denial, restore onto fresh DB, reconcile external operations before resuming dispatch; privileged tampering is residual risk. |

Candidate metrics: unbalanced journal count, balance drift count, negative restricted balance count, duplicate financial effect count (all must be zero in verified fixture); unknown payout age, missing provider/ledger references, unexplained cash by currency, suspense difference, reserve/position exposure, quote expiry, outbox lag, account lock wait and posting latency. Define performance/RPO/RTO thresholds before testing (DEC-12); do not derive them from Stripe/Uber numbers. No real infrastructure tests run in this research.

## G4 — Reviewable implementation sequence (blocked until G2/G3 acceptance)

| Slice | Scope / trace | Evidence |
|---|---|---|
| 0 | Accept chart/book, DEC-02/06/07/16 and proposed first ledger scope; verify dependencies #14/#18/#20/#13 as applicable | owner acceptance and independent design review |
| 1 | Pure amount, currency, journal and account domain; LGR-01/09 | unit + independent property oracle |
| 2 | Ledger migration, grants, chart and account lifecycle; LGR-09/10/12 | actual PostgreSQL roles, upgrade/fresh install, close race |
| 3 | Posting, replay, balance, ordering and outbox; LGR-01–04/06/12 | concurrency, failure injection, privileges and ambiguous commit |
| 4 | Reversal, statements, read-only verification, restore; LGR-05/07/08/11 | rebuild, drift, recovery and event tests |
| 5 | Account named balances and hold lifecycle, limits/fees; ACC/LIM/FEE | reserve/release/consume, races and freezes |
| 6 | Internal FX decision and grouped posting; FX | exact oracle, second-leg rollback, position limits |
| 7 | Provider simulator and one approved corridor; PAY/OUT/PRV | timeout after send, inquiry, callbacks and returns |
| 8 | Reconciliation, bank settlement and reporting; REC/STL/FIN | end-to-end cash fixtures, partial/net settlements and close |

The roadmap still governs dependencies, including screening and onboarding before money activation. These are candidate slices, not authority to skip Identity work or implement live operations.

## G5–G7 — Status and decisions

Research reviewed; repository tests, PostgreSQL execution, benchmarks, provider tests and CI: NOT RUN at authoring. No financial runtime change. No G2 or G3 approval. Before advancing: confirm first book and chart, floor semantics, replay scope and canonicalization, exact lock/role boundary, currency wire format, hold/finality and quote expiry, internal FX grouping, corridor/rail, retained evidence and operational thresholds. Promote accepted requirements into owning contracts and amend ADR-0002 only with explicit acceptance. No reference to an earlier repository is required to implement this proposal.
