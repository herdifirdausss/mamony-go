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

## Deep dive A — Financial accounting and ledger domain model

**Reviewed 2026-11-24; candidate G2/G3, not accepted.** This expands sections G1 and G3, rather than introducing a parallel normative specification. The earlier simplified USD example is illustrative; the more precise cash-debit/beneficiary-finality and return fixtures below show why the timing matters.

### A.1 — Source-backed boundaries and accounting policy

| Primary source, checked 2026-11-24 | Finding and use | Limit |
|---|---|---|
| [IFRS Conceptual Framework (2018)](https://www.ifrs.org/issued-standards/list-of-standards/conceptual-framework/) | Distinguish economic resources, obligations, equity, income and expenses; define reporting entity and recognition. | Framework is not itself an IFRS Standard; legal/economic role determines actual reporting. |
| [IFRS 15](https://www.ifrs.org/issued-standards/list-of-standards/ifrs-15-revenue-from-contracts-with-customers/), [2016 clarifications](https://www.ifrs.org/news-and-events/news/2016/04/amendment-issued-ifrs-revenue-from-contracts-with-customers/) | Revenue is linked to satisfaction of performance obligations; principal/agent assessment matters to gross/net treatment. | Capture-time fee revenue below is a synthetic policy, not a conclusion for a real PSP. |
| [IAS 21](https://www.ifrs.org/issued-standards/list-of-standards/ias-21-the-effects-of-changes-in-foreign-exchange-rates/) | Functional currency and foreign-currency accounting require separate measurement policy. | Operational minor units and per-currency zero-sum are not a statutory consolidated GL. |
| [TigerBeetle financial accounting](https://docs.tigerbeetle.com/coding/financial-accounting/), [data model](https://docs.tigerbeetle.com/coding/data-modeling/), [two-phase transfers](https://docs.tigerbeetle.com/coding/two-phase-transfers/) | Normal side, currency partition, reservation and resolution primitives. | Compare semantics, not implementation or throughput. |
| [Modern Treasury account model](https://docs.moderntreasury.com/ledgers/docs/ledger-accounts-overview), [transaction status](https://docs.moderntreasury.com/ledgers/docs/transaction-status-and-balances), [guarantees](https://docs.moderntreasury.com/ledgers/docs/ledgers-guarantees), [version locking](https://docs.moderntreasury.com/ledgers/docs/lock-on-account-balance-or-version) | Separate pending/posted/available balances, atomicity, version guards, posted immutability. | Mamony can model named balance accounts instead; never mix reservation models without a proof. |
| [Stripe Ledger](https://stripe.com/blog/ledger-stripe-system-for-tracking-and-validating-money-movement), [currency rules](https://docs.stripe.com/currencies) | Financial flow completeness, provider-specific amount restrictions. | Provider amount format is not canonical ledger scale. |
| [Adyen accounting report](https://docs.adyen.com/platforms/reports-and-fees/balance-platform-accounting-report/), [reporting](https://docs.adyen.com/platforms/quickstart-guide/reporting/) | Transfer/transaction IDs, booking and value dates, currency and balance evidence. | Provider terminology is specific to the selected contract. |
| [FCA safeguarding](https://www.fca.org.uk/firms/emi-payment-institutions-safeguarding-requirements), [CASS 15](https://handbook.fca.org.uk/handbook/cass15) | UK-specific separation/protection of relevant funds and associated obligations. | Not a universal legal rule, not approval for a UK or Indonesian business, and not established by separate database rows. |

**Product assumption to approve:** one synthetic payment intermediary legal entity, one operational book partitioned by currency. The platform recognizes contractual customer claims as liabilities and cash/receivables as assets for the following fixtures. Actual agency/trust/safeguarding arrangements may require other treatment; real accounting and jurisdiction-specific advice are outside the pilot. No customer funds are company revenue by default.

### A.2 — Vocabulary, chart and ownership

- Book: reporting/ownership boundary, not a tenant. Tenant: authorization/resource scope. Currency: unit-of-account partition. Ledger account: one immutable book/currency/type/opaque owner identity. Account type: reusable category and normal side. Balance account: business-facing grouping of named ledger accounts owned by accounts capability. Journal: one atomic financial fact, with at least two entries, one book and one currency. Entry: positive amount and debit/credit direction. External bank account: separate external cash/evidence identity, not a customer account.
- Operational financial ledger is authoritative for transaction-level balances and movements. Later finance reporting projects immutable entries to versioned general-ledger codes, effective periods and functional currency; a report may not post, repair, or authorize spending. Do not maintain two independently writable financial truths.
- Debit-normal asset/expense balances increase by debit; credit-normal liability/equity/revenue balances increase by credit. Contra-accounts, sign exceptions and negative policy are explicit future chart decisions. Never infer debit/credit from inbound/outbound labels. Define normal-side balance as sum of signed effects, opening = zero (seed funding via posted journals), closing = opening + movements. Trial balance compares debit and credit totals within the same currency/book; never sum different currencies without a measurement policy.

Candidate codes are classifications, not unique IDs, and not migrations. Floor is the normal-side lower bound; the synthetic pilot defaults to zero on all listed classes (later exceptions require explicit approval).

| Code/type | Category/normal side | Scope / intent |
|---|---|---|
| 1100 collection_cash | Asset/debit | per provider bank/account/currency; not automatically safeguarded |
| 1110 operating_cash | Asset/debit | per operating bank/currency; separate from customer cash |
| 1120 prefunded_partner_cash | Asset/debit | per partner account/currency; externally confirmed cash/claim classification required |
| 1200 provider_receivable | Asset/debit | per provider/account/currency; evidenced claim, not spendable cash |
| 1210 payout_clearing | Asset/debit | per provider/account/currency; cash sent or return receivable supported by evidence and accounting policy |
| 1300 merchant_receivable | Asset/debit | per merchant/currency; approved recoverable shortfall, not available funds; disabled initially |
| 2100 customer_available | Liability/credit | per authorized owner/currency; spendable within current policy |
| 2110 customer_pending | Liability/credit | per owner/currency; recognized but not spendable |
| 2120 customer_in_transit | Liability/credit | per owner/currency; obligation held for dispatch/return |
| 2130 customer_reserve | Liability/credit | per owner/currency; separately governed release |
| 2140 dispute_held | Liability/credit | per owner/currency; disabled until dispute design |
| 2200 provider_payable | Liability/credit | per provider/currency; explicit fee/settlement obligation |
| 3100 platform_capital | Equity/credit | book/currency; initial operating funding, never user-facing |
| 4100 payment_fee_revenue | Revenue/credit | book/currency; approved recognition milestone |
| 4110 payout_fee_revenue | Revenue/credit | book/currency; approved recognition milestone |
| 5100 provider_fee_expense | Expense/debit | book/currency; supported provider cost |
| 5200 operational_loss | Expense/debit | book/currency; approved write-off, not automatic |
 
FX position/bridge accounts need explicit sign and exposure limits; suspense asset/liability types need evidence and aging; deferred revenue, taxes, FX gains/losses and financial close entries need separate policy. An operational FX bridge is a control account, not automatically a recognized statutory asset or liability. No generic signed clearing bucket or balancing plug. Account type/normal side/currency exponent are immutable once referenced. Display names and external mappings can be versioned without reinterpreting past journals. External cash account ownership, fiduciary segregation and bank statement evidence must be separately verified. Tenant-facing reads expose only authorized account mappings, never a global bank balance.

### A.3 — Compare balance representations

A: separate named liability accounts (existing ACC contract). Every reserve, release, consume and eligibility change is an immutable journal. Available balance already excludes held amount; do not subtract a reservation a second time. B: one ledger account with native pending and posted balance dimensions (Modern Treasury). Strong abstraction but introduces mutable pending lifecycle/version semantics. C: native pending transfer primitive (TigerBeetle), reserve/resolve with invariant protection; implementing it locally is additional complexity. Recommend A for first slice, while preserving per-operation hold identity in the owning workflow. Pending is an economic availability category, not permission to mutate a posted journal. A mere attempted payment or authorization without a recognized asset/obligation remains workflow state and does not fabricate a journal.

Aggregate in-transit is not proof that a particular payout is funded. Each payout has one immutable hold ID and exact amount/currency/account/intent version, stable hold journal, lifecycle and resolved amount. Two distinct payouts must not consume each other's funds. Hold creation, release/consume and relevant local workflow fence require one specified atomic authority boundary; if not established, do not enable dispatch. Definitive failure before debit releases once; timeout after dispatch keeps hold; evidence of partial settlement/return needs an accepted amount allocation policy. No expiration based solely on wall time after external send.

### A.4 — Worked posting fixtures (USD cents)

Policy assumptions: single synthetic book, no tax, captured claim enforceable, 300 fee earned at capture, 100 provider fee deducted from settlement and borne by Mamony, customer funds become available only after cash evidence, payout without additional fee. All values integer minor units. These do not prescribe real revenue recognition or safeguarding. Positive entries shown as D/C; no zero entries. Journal key is derived from business operation and transition, not retry attempt or external webhook delivery ID.

| Journal / trigger | Debit | Credit |
|---|---|---|
| J1: confirmed pay-in; recognized receivable | provider_receivable 10000 | customer_pending 9700 + payment_fee_revenue 300 |
| J2: report and bank credit 9900; provider cost 100 | collection_cash 9900 + provider_fee_expense 100 | provider_receivable 10000 |
| J3: availability evidence | customer_pending 9700 | customer_available 9700 |
| J4: payout hold 4000 | customer_available 4000 | customer_in_transit 4000 |
| J5: evidenced external cash debit; obligation not yet extinguished | payout_clearing 4000 | collection_cash 4000 |
| J6: definitive fulfillment under selected rail policy | customer_in_transit 4000 | payout_clearing 4000 |
| J7: later valid partial return 1000; recognized return receivable and renewed obligation | payout_clearing 1000 | customer_in_transit 1000 |
| J8: returned cash credited | collection_cash 1000 | payout_clearing 1000 |
| J9: return funds become eligible | customer_in_transit 1000 | customer_available 1000 |

After every journal, these are normal-side balances. R = revenue (300 after J1), E = expense (100 after J2); both zero initially.

| State | Receivable | Cash | Clearing | Pending | Available | In-transit | R | E |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| J1 | 10000 | 0 | 0 | 9700 | 0 | 0 | 300 | 0 |
| J2 | 0 | 9900 | 0 | 9700 | 0 | 0 | 300 | 100 |
| J3 | 0 | 9900 | 0 | 0 | 9700 | 0 | 300 | 100 |
| J4 | 0 | 9900 | 0 | 0 | 5700 | 4000 | 300 | 100 |
| J5 | 0 | 5900 | 4000 | 0 | 5700 | 4000 | 300 | 100 |
| J6 | 0 | 5900 | 0 | 0 | 5700 | 0 | 300 | 100 |
| J7 | 0 | 5900 | 1000 | 0 | 5700 | 1000 | 300 | 100 |
| J8 | 0 | 6900 | 0 | 0 | 5700 | 1000 | 300 | 100 |
| J9 | 0 | 6900 | 0 | 0 | 6700 | 0 | 300 | 100 |

Cash + receivable + clearing = customer liabilities + revenue - expense at every state. All nine journals and running balances were independently recomputed with a small arithmetic script during research; no Go code or PostgreSQL tests executed. This verifies the illustrative numbers only. J5 is valid only if the clearing asset satisfies the accepted economic definition and supported evidence; a cash debit is not beneficiary credit. J7 similarly requires an evidenced enforceable return claim, not merely an untrusted return message. J7–9 can be combined only when the necessary evidence and policy are simultaneous.

Alternative branches (not appended to J1–J9):
- After J4 and definitive failure with no external debit: D customer_in_transit 4000 / C customer_available 4000. Cash remains 9900; available becomes 9700. Unknown outcome does not post this journal.
- If J5 has happened and the payout fails but money has not returned, keep the relevant obligation and clearing asset visible; do not mark funds available based on timeout. On evidenced cash return: D collection_cash 4000 / C payout_clearing 4000; then release the hold with the preceding failure journal when eligible.
- Internal transfer from customer A to customer B within the same book/currency: D A_available 2000 / C B_available 2000; platform cash unchanged. Authorization, product scope and owner checks required; no cross-currency journal.
- Reserve: D customer_available 1000 / C customer_reserve 1000; release reverses those economic effects with a new authorized journal, not mutation. Release may be partial subject to total remaining amount.
- Initial prefunding from platform operating cash: D prefunded_partner_cash / C operating_cash only when the corresponding asset/evidence policy applies; opening capital uses D operating_cash / C platform_capital. Never fund a balance by unbalanced UPDATE or fictitious opening entries.
- Partial refund of a captured payment: refund workflow owns refundable cap and fee policy; reserve funds before external send, and use the same cash/clearing/fulfillment separation when applicable. If insufficient funds, refuse initially; debt recovery/chargebacks require approved receivable/loss accounting and may not be silently dropped or bypass balance floors.

### A.5 — Posting authority, templates and time

The ledger domain checks math and account classes; allowed business owners determine economic meaning and evidence. Candidate registered, versioned posting templates: payin_recognized, payin_cash_settled, availability_release, payout_reserve, payout_cash_debit, payout_fulfilled, payout_release, payout_return_recognized, return_cash_received, return_availability, internal_transfer, reserve_hold/release, full_reversal and approved_adjustment. First ledger gate uses synthetic funding/transfer/reversal templates only; others remain disabled until owning capabilities and contract checks exist. A posting template constrains allowed account types, book/currency/owner binding, direction, amount relationships, operation ID, evidence/approval policy and stable key. No client arbitrary debit/credit, even if balanced. Each economic fact has one stable identity independent of delivery ID; multiple distinct facts can reference one payment.

Journal immutability includes financial reference, amounts, account, direction and committed sequence. Record recorded_at (database recording time, not a guaranteed commit-order clock), effective date under accepted accounting policy, and optional external event/booking/value times in provider evidence or authorized mapping. Reject arbitrary backdating by clients. Account sequence + entry position is authoritative account order; statements pin a sequence watermark; cross-account reports use a consistent snapshot and explicit cutoff. Closed-period correction policy remains FIN-04. A journal reversal offsets the whole original and must pass current gates; a partial refund, return, fee adjustment and dispute is a distinct financial fact, not reuse of a full-reversal ID. For FX, balance USD and SGD independently and keep linked group identity, rates and source evidence; accounting valuation and real liquidity are separate.

### A.6 — Candidate acceptance and decision records

| Trace | Testable candidate (not accepted, NOT RUN in repo) |
|---|---|
| LGR-01/09 | Reject mismatched book, currency, invalid direction or normal side, zero/negative entries, immutable classification changes, overflow and arbitrary opening balance. |
| LGR-02/04, ACC-03 | Reproduce nine journal fixture and each state; rebuild from entries; independently prove balance equation and per-currency debit/credit totals. |
| LGR-12 | Reject balanced but unauthorized posting template, wrong account owner/type, missing or substituted evidence and duplicate economic identity. |
| ACC-02, OUT-01/02 | Available excludes holds once; two concurrent payouts cannot consume the same funds or another payout's hold. Unknown/failure-after-debit cannot prematurely release. |
| PAY-04, LGR-05 | Full reversal distinct from partial refund and return; concurrent cumulative refund/return cap, duplicate callbacks, original journal unchanged. |
| REC-02/05, FIN-02 | J2 reconciles gross 10000, fee 100 and bank net 9900; missing fee or cash event creates case despite balanced journal. J5 and J6 cannot be collapsed without evidence. |
| FIN-01/03/04 | Statements preserve entry sequence, immutable close watermark, booking/value provenance and separate functional-currency mapping; later correction not silent historic mutation. |

Proposed choices requiring owner/technical review: (1) synthetic intermediary and single accounting book, customer balances as liabilities; (2) named balance accounts and operation-specific holds; (3) chart/type immutability and default zero floors, exception governance; (4) economic posting templates and evidence authority, not generic user journal writes; (5) fee recognition, cash clearing, return and availability policy; (6) GL mapping and functional-currency policy deferred to accounting stage. No change to accepted LGR/ACC/FIN or ADR-0002 before explicit acceptance. Next deep dive B: Go/PostgreSQL transaction model, database enforcement, key and sequence semantics, actual concurrency/failure test design (DEC-02). No runtime work authorized.

## Deep dive B — Ledger database, transactions and concurrency

**G1/G3 candidate; reviewed 2026-11-24; not accepted.** DEC-02/06/07/10/12/16, LGR-01–12, PostgreSQL and EVT contracts. No financial code, migration, public API, dependency or accepted ADR changes. Stage A defines candidate economic semantics; this section defines an enforcement design to prove before implementation.

### B.1 — Current foundation and primary evidence

The current main ledger contract has no implementation evidence. The existing platform migration v1 creates migration fingerprints; no ledger migration is present. The platform postgres pool verifies the actual unprivileged runtime role, bounds connections to 1–8, sets statement and idle-in-transaction timeouts to 5 seconds and a restrictive search_path, uses pgx transactions, and treats commit errors as possibly ambiguous. Its tenant context is not an authority. Reuse the existing pool and migration workflow, not another generic DB framework. The helper currently begins with default transaction options; explicitly review whether a narrow ledger transaction wrapper must enforce isolation, lock budget, synchronous commit and typed commit outcomes. Do not modify this shared primitive until a reviewed slice requires it. Existing synthetic role and migration evidence does not prove ledger correctness.

| Primary source, checked 2026-11-24 | Observation / implication |
|---|---|
| [PostgreSQL 18 isolation](https://www.postgresql.org/docs/18/transaction-iso.html), [application consistency](https://www.postgresql.org/docs/18/applevel-consistency.html) | READ COMMITTED has statement snapshots; known locked rows can be guarded, but predicate rules may need additional guards or Serializable. |
| [Explicit locking](https://www.postgresql.org/docs/18/explicit-locking.html), [failure handling](https://www.postgresql.org/docs/18/mvcc-serialization-failure-handling.html) | Acquire locks consistently; deadlocks can still occur. Retry complete aborted transactions, not isolated SQL statements. |
| [INSERT](https://www.postgresql.org/docs/18/sql-insert.html), isolation section on ON CONFLICT | Unique-key claims serialize contenders; conflicting rows may not be visible to the INSERT snapshot. Read result with a new statement under READ COMMITTED. |
| [Constraints](https://www.postgresql.org/docs/18/ddl-constraints.html), [constraint triggers](https://www.postgresql.org/docs/18/sql-createtrigger.html) | CHECK is not an aggregate cross-row invariant; use declarative constraints plus trusted procedure and reviewed deferred verification. |
| [CREATE FUNCTION](https://www.postgresql.org/docs/18/sql-createfunction.html), [privileges](https://www.postgresql.org/docs/18/ddl-priv.html) | SECURITY DEFINER executes with owner privileges; secure search path and revoke PUBLIC execution atomically. |
| [pgx v5 API](https://pkg.go.dev/github.com/jackc/pgx/v5) | Commit can return ErrTxCommitRollback on known rollback; other errors require classification, not assumption. |
| [Modern Treasury concurrency](https://docs.moderntreasury.com/ledgers/docs/handle-concurrency), [balance/version locking](https://docs.moderntreasury.com/ledgers/docs/lock-on-account-balance-or-version), [guarantees](https://docs.moderntreasury.com/ledgers/docs/ledgers-guarantees) | Hot accounts and strong balance guards have costs; versioned balance reads and atomic entries are valuable. Do not import asynchronous balance semantics into current strong-read contract. |
| [TigerBeetle linked events](https://docs.tigerbeetle.com/coding/linked-events/) | Explicitly linked financial operations succeed or fail together; useful comparison for local FX group. |
| [Stripe Ledger](https://stripe.com/blog/ledger-stripe-system-for-tracking-and-validating-money-movement), [AWS transactional outbox](https://docs.aws.amazon.com/prescriptive-guidance/latest/cloud-design-patterns/transactional-outbox.html) | Durable ledger and event publication are distinct; consumers must deduplicate and reconcile missing facts. |
| [WAL settings](https://www.postgresql.org/docs/18/runtime-config-wal.html), [asynchronous commit](https://www.postgresql.org/docs/18/wal-async-commit.html), [PITR](https://www.postgresql.org/docs/18/continuous-archiving.html) | Atomic transactions are not a substitute for durable commit, restore, and external reconciliation. |

Sources establish mechanisms, not a benchmark, schema proof, or production approval. Verify behavior on repository-pinned PostgreSQL 18.7 and actual pinned pgx dependency before G5.

### B.2 — Architecture options and selected candidate

| Option | Advantages | Limitation |
|---|---|---|
| A: Go orchestrates all DML in pgx transaction | Idiomatic, easy business composition and debugging | Broad ledger runtime INSERT/UPDATE grants can bypass checks on a compromised or buggy path; independent DB guards must cover all mutations. |
| B: Go owns workflow; narrow database posting command owns financial mutation | Restricted EXECUTE boundary; one SQL round trip for posting; checks live next to durable state | SQL migration/review complexity; function owner and authorized caller remain trusted; multi-capability atomic participants need explicit protocol. |
| C: dedicated financial database | Financial primitives and specialized performance characteristics | New operational dependency, separate consistency boundary, migration/reconciliation and external atomicity questions. |

Recommend B as the first candidate for DEC-02, without moving payment workflows into SQL or prematurely building a ledger microservice. Go owns domain types, orchestration, ports, error mapping and observability. SQL owns only immutable journal validation, exact locked balance changes, durable replay and outbox. A is a valid alternative only if it demonstrates equivalent restricted mutation enforcement. No external calls inside the financial transaction. No arbitrary callbacks, dynamic SQL or general-purpose cross-module DML.

Candidate modules: internal/modules/ledger/{module.go,internal/domain,internal/usecase,internal/repository/postgres}, db/migrations/ledger/000001_initialize.up.sql (future), and composition under internal/app. Respect existing architecture and migration fingerprint conventions; filenames and signatures remain design candidates.

### B.3 — Relational model and constraints

All table names below are logical, proposed under mamony_ledger. Single book initially, currency-version identity retained. UUID internal IDs are opaque, not authorization or order keys. Fields required by a key or invariant are NOT NULL unless explicitly optional.

| Record | Key / enforcement candidate |
|---|---|
| books, currency_versions, account_types | PKs; immutable economic classification/normal side/currency scale after first use; enabled state separate from historic definition. |
| accounts | PK account_id; unique (account_id, book_id, currency_version_id); FKs to book/currency/type; stable owner identity; status; balance bigint; account_seq bigint >= 0; explicit nonnegative floor when applicable; initial balance zero. |
| posting_keys | UNIQUE (authenticated capability, book_id, scope_id, operation kind, idempotency_key); nonnull normalized scope (tenant or explicit platform scope); canonical version and trusted fingerprint; committed journal/group reference. Use owner-derived scope, not an arbitrary request field. |
| journals | PK journal_id; unique (journal_id, book_id, currency_version_id); immutable business reference, template/version, recorded/effective time, optional reverses; unique partial index on nonnull reverses; no self reversal; same-book/currency composite FK to original. |
| journal_account_steps | PK (account_id, account_seq); UNIQUE (journal_id, account_id); composite FK binds journal/account/book/currency; one sequence allocation per account per journal; net delta and resulting balance optional immutable verification fields. |
| entries | PK (journal_id, position); positive bigint amount; direction constrained; composite FKs to journal and account using same book/currency and to journal_account_steps using journal/account/sequence; no cascade delete. Entries of one account in one journal share account_seq but distinct positions. |
| outbox, outbox_account_steps | UNIQUE event per journal/versioned event kind; immutable payload derived from committed facts; dependency edges to each touched account step. Delivery lease/cursor metadata separate and narrowly mutable. |
| posting_groups | Later only if approved; unique scoped group identity; immutable child manifest and order; all journal keys tied to group; no independent re-execution. |
| verification_runs / discrepancies | Separate restricted evidence writer; verification reads financial records but never edits them. |

Unique index and composite FKs prevent cross-book/currency mismatch and duplicate identity; a CHECK cannot enforce sum(debits)=sum(credits) across rows. Validate journal count, exact aggregate totals, templates, positive amounts and delta bounds inside posting function; consider a deferred constraint trigger on journal creation to catch an empty/incomplete journal at commit. A trigger solely on entries misses an empty journal; merely checking a journal INSERT before entries exist also fails. Validate exact reversal shape, unique original and reverse-of-reversal rule. No direct runtime journal/entry UPDATE/DELETE/TRUNCATE or owner membership; enforce append-only through grants and optionally defensive triggers, not claims of immunity from superusers. Floor check uses normal-side balance, not debit/credit sign guessing. Net per-account delta is computed before any update; decide whether no-op netting/same-account-only journals are allowed (recommend refuse economically empty templates). Use numeric or bounded big.Int intermediates, check int64 conversion, reject sequence overflow. Bound max entries/amounts before locks; proposed test candidates only, not approved defaults.

### B.4 — Authority, roles and hostile-path tests

Candidate privileges: mamony_ledger_owner NOLOGIN owns schema/tables; separate controlled migrator; mamony_ledger_runtime can execute specifically granted posting and read entry points but no unrestricted DML or CREATE; verifier read access plus separate result writer; relay can claim/ack only delivery state, cannot create journal/change payload; business runtime roles may execute only explicitly approved template/authority interface. Exact role names, grants and deployment provisioning require G3 review. No role with ownership, BYPASSRLS, CREATEROLE or inherited privileged membership in runtime. Create SECURITY DEFINER with qualified objects, fixed trusted search_path (pg_catalog, trusted schemas, pg_temp last), no unsafe dynamic SQL, REVOKE EXECUTE FROM PUBLIC in same migration, grant only named roles, review overloaded signatures and default privileges. Prohibit public writable schemas and runtime owner/role escalation. Test with actual distinct logins including hostile temporary/schema shadowing and direct DML.

Important: current_user inside SECURITY DEFINER is the function owner. Do not use it to infer caller or accept a caller string/tenant GUC as proof. A dedicated login may use validated session_user under a deployment where SET SESSION AUTHORIZATION/role switching and shared credentials cannot falsify the binding. If all modules share one login, database cannot distinguish Go capabilities from that identity alone; use one explicitly trusted orchestrator with constrained interface and acknowledge the remaining trust, or provide distinct capability-specific credentials. G3 must select and prove a concrete binding. No claim of isolated authority if same compromised process holds all pools. Tenant mapping and exact approval receipt are checked by owning authority and, where required, fenced atomically at posting. Database permissions defend restricted credentials; privileged owner, migrator, superuser and valid-but-malicious service remain residual risks. Hashes without independent trusted anchor do not prevent privileged rewriting or rollback.

### B.5 — Posting and lock protocol (DEC-02 candidate)

1. Before transaction: bound request, canonicalize syntactic fields, authenticate capability, establish authorized scope. Generate stable operation ID before retry. Precheck may reject early but cannot authorize a stale state.
2. Begin explicit READ COMMITTED on authoritative primary. Claim replay key first with INSERT ... ON CONFLICT DO NOTHING RETURNING. If conflict, issue a new SELECT statement for stored record; never use a one-statement INSERT/SELECT fallback assuming same snapshot sees winner. Compare canonical version/fingerprint and authorized scope. Matching committed result returns original IDs without new writes; changed content refuses. Concurrent failed claimant rolls back; waiting contender can claim. No committed in-progress row in this atomic local design. If result not readable due to current authorization, deny read without re-posting.
3. For new work acquire all participating durable guard rows in documented order (authority/intent/quote/hold and policy rows by class then immutable ID); all workflows including revoke/freeze/close must use compatible order. Acquire ledger account rows in sorted immutable ID order, explicitly one-by-one in the procedure or prove equivalent actual query/plan. Lock modes must be compatible with FK checks; account and journal insertion ordering must be reviewed for implicit KEY SHARE and lock upgrades. No new lock class late in the transaction. Discover complete account/guard set before acquiring locks; abort and retry if changed. Account membership and policies are re-read under locks. A close/suspend/revoke path shares the same fence, so a precheck is not sufficient; if an owning authority cannot join safely, disable the affected action until the consistency boundary is defined.
4. Validate committed current status, authorization evidence, remaining per-operation hold, exact journal shape, book/currency, normal-side delta and floor; update each account once per journal and allocate sequence under lock. Persist journal, steps, entries, replay result, outbox and explicitly approved participant effects in one local transaction. Do not dispatch provider operations here. Grouped FX later locks union of accounts and quote/position guards before either leg; both journals balance in their own currency, quote and group key consumed once, all effects commit or none.
5. Commit with durable settings; return success only after confirmed commit or authoritative matching replay. Do not expose a result computed before commit as success. If transaction aborts, no financial state/outbox/claim remains.

READ COMMITTED suffices as a candidate only for invariant decisions made on known locked rows or declarative unique constraints. An invariant over a changing set (e.g., count/sum of active holds) needs an authoritative counter/guard row shared by all writers, or a proven Serializable protocol; Repeatable Read alone can permit write skew. SERIALIZABLE may abort on conflicts; retry the complete transaction. The ordering above reduces known deadlock paths, not a proof all lock cycles are impossible. Include foreign keys, unique conflicts, close/freeze, fee/limit consumption, relay and later group paths in lock graph. Prefer bounded lock/statement/transaction deadlines measured on fixtures, with no lock held while calling a remote provider. No SKIP LOCKED on spendable balance or authorization checks; use only for jobs with explicit lease/reclaim.

Abstract model result: two withdrawals of 80 from balance 100, each with read/check then subtract, exhaustive six valid interleavings without mutual exclusion: four end at -60 and two at 20. With exclusive lock around read/check/write: two schedules, both end at 20. This tiny enumerated model validates the reasoning only, not PostgreSQL MVCC or Go code. Real PostgreSQL executable and Docker were unavailable in the research environment.

### B.6 — Idempotency identity and recovery

A journal key is not an HTTP response cache or external dispatch key. Candidate canonical fingerprint uses a versioned deterministic encoding of authenticated scope, book, currency version, operation/template version, stable business reference, ordered entry positions/account/direction/amount, reversal/group identity and immutable approval intent binding where relevant. Exclude trace IDs, request timestamps and retry metadata. Exact string normalization, length-prefix/serialization and field bounds require review; do not hash unordered JSON maps or use user-supplied digest as authority. Use stable binary or documented canonical encoding plus SHA-256 and compare actual canonical identity when needed; preserve version across upgrades. Distinct business operations also need unique economic fact IDs to stop different transport keys duplicating the same effect. Internal key retained for journal life; public API and provider idempotency horizons separate.

| Result / failure | Response and retry rule |
|---|---|
| Domain/authorization rejection | Known no new committed financial effect; refusal is not a posted replay. Audit per policy; do not retry unchanged deterministic invalid inputs. |
| Same key / same canonical content | Return original committed result if authorized; do not repeat approval consumption or outbox. |
| Same key / changed content | Conflict, no write; never overwrite fingerprint. |
| SQLSTATE 40001 or 40P01 with known abort | Roll back entire transaction, bounded retry with jitter and same identity; recheck policy; expose exhaustion safely. |
| 55P03/57014 lock or statement timeout | Abort/cleanup, classify exact phase; retry only when rollback known and policy permits. No universal retry on every SQL error. |
| pgx ErrTxCommitRollback | Known transaction rollback; distinguish from transport loss. |
| Disconnect or deadline during COMMIT | UNKNOWN: resolve on primary by same scoped key, or safely reattempt identical command/unique claim within bounded policy. Absence from a lagging replica is not proof of rollback. Never switch key, regenerate business intent, or release an external hold. |
| External provider timeout | Local posting replay cannot guarantee external exactly once; retain original provider identity, inquiry, reconcile; no blind resend. |

If primary unavailable, return/retain unresolved result; don't claim failure. Durable restart reuses same keys. Keep commit phase separate from precommit error in Go mapping; never use generic automatic retry middleware for arbitrary transaction callbacks. No irreversible side effect in retried closure. For primary failover, acknowledge durability depends on replication guarantees and RPO; do not claim zero data loss with async replicas. Verify fsync, full_page_writes, synchronous_commit settings and failover configuration; runtime can alter some session settings, so trust and enforcement of durability configuration need explicit review. For financial commands do not deliberately disable durable commit.

### B.7 — Reads, event sequencing and hot accounts

Authoritative available read uses primary (or proven causal read mechanism), no Redis/replica/projection for floor decisions. A single statement can read balance and sequence; a stable statement page uses fixed account high-water mark and (account_seq, position), not recorded_at alone. Use REPEATABLE READ READ ONLY for verifier and consistent multi-account report; report cutoff/progress; avoid unbounded long snapshots. Verify count, totals, sequence and balance from independent query/model; read-only verifier cannot repair; operational evidence writer separately persists discrepancies.

Outbox order: one event per journal and dependency on every affected account step. For first implementation use one logical serialized relay per consumer with durable progress; event eligible only when all touched accounts' previous sequence deliveries are acknowledged for that consumer. Each consumer's inbox dedup (source,event_id), applied effects and progress commit atomically; replay identical payload/ID on loss of acknowledgement. A delivery receipt without application progress is not evidence of consumer correctness. A poison event blocks dependent paths for that consumer; quarantine/alert with controlled replay, no silent skip or global head-of-line requirement. Other independent consumers continue. Sequence assigned under account locks gives per-account order, not global commit order; global sequence values, UUIDs and timestamps are not sufficient cursor watermarks for concurrent inserts. A journal touching multiple accounts forms a partial order. Later parallel relay requires atomic eligibility/claim and crash fencing. Rebuild from durable immutable history beyond outbox retention, not ephemeral queue.

Hot accounts: shared collection/clearing rows serialize updates even across unrelated merchants. Baseline synchronous exact balance read/write stays unchanged. Measure lock wait, transaction duration, pool saturation, p50/p95/p99, retries, deadlocks, committed throughput and invariant drift for uniform and skewed distributions. Candidate test workloads: 1/2/4/8 concurrent workers per runtime pool and multiple process/pool fixtures; 100 contenders may be queued across bounded pools; 1,000/10,000 journals, 2/10/50 entries (experimental sizes, not product limits). Compare no shared account vs single shared account vs approved sharded clearing. Determine SLOs and dataset before gate (DEC-12). Do not choose batching/sharding or relaxed reads on benchmark alone. Possible later options: batch independent complete journals while preserving identities, shard only accounts with proven economic partition and exact aggregation, separate write-only reporting projection; each changes concurrency/visibility and needs approval. A common debit authority or liquidity floor cannot be split arbitrarily.

### B.8 — Deterministic verification and rollout plan

Use existing synthetic PostgreSQL integration infrastructure, actual runtime/owner/migrator roles, distinct sessions, barriers on observed lock/wait state rather than sleeps, exact pgx and migration versions. All tests below NOT RUN; no DB code has been introduced.

| Scenario / trace | Required durable assertion |
|---|---|
| LGR-01: mixed currency/book, nonpositive amount, total overflow, unknown/closed account, malformed template | no journal, entry, step, balance, replay or outbox writes |
| LGR-02: seed 10 units, 100 one-unit competing spends with distinct keys | exactly 10 successful commits in fixture without injected transient failure; balance 0; no negative state; rejected requests no effect |
| LGR-03: same key concurrent same/different payload, winner rollback | one committed result; identical replay; conflict no mutation; waiting claimant succeeds after known rollback if still valid |
| LGR-02: reverse-order accounts A/B and B/A; multiple entries per account | sorted lock protocol, exact net deltas, unique step/position; bounded retry if deadlock; no partial write |
| LGR-10/12: close/post, freeze/post, receipt reuse/revoke, hold ownership | one specified linearization; stale authority refused; wrong tenant/account/template cannot post/read |
| LGR-05: concurrent full reversal with distinct keys; floor refusal | at most one reversal, original unchanged, exact inverse and no bypass |
| LGR-02/03: fault after key claim, journal insert, balance update, participant write, outbox insert, immediately before commit | all writes rollback together; retry same key produces one complete effect |
| LGR-03/04: suppress response after commit; kill client while commit uncertain | resolve original result with same key, one journal/event; no duplicate |
| LGR-06/EVT: A+B then B+C and independent D; relay crash after consumer commit | predecessor order per touched account; repeated delivery one effect; independent consumers progress |
| LGR-07/11: concurrent writes during verifier and statement paging; planted drift | snapshot has no false drift; controlled drift detected; stable watermark and no gaps/duplicates |
| PostgreSQL contract: actual login role, unauthorized SQL/EXECUTE, PUBLIC, role switch, search path shadowing, cross-tenant pool reuse | no unauthorized change or read; catalog evidence and unchanged financial facts |
| LGR-08: backup/WAL restore plus crash/restart; restore role/grants and outbox | matching committed history and privilege boundary; record RPO/RTO and reconcile external facts before resuming dispatch |
| FX future: fault on second journal, concurrent quote use and group replay | both journals and participant/outbox commit or none; reject independent constituent re-execution |

Use independent exact arithmetic model, Go fuzz/property corpus for repeated journal lines, int64 bounds, template and canonicalization. Record seed, schedule, actual SQLSTATE/phase, balances, journal/entry/outbox counts, role, schema version, server/driver version, command and commit. Verify required tests are discovered/executed by integration gate. make race is shared-memory evidence only; mocked pgx is not database evidence. Use fault injection in isolated fixtures, no runtime bypasses or weakened constraints.

Candidate G4 split: (1) approve DEC-02 transaction/role and Stage A chart/scope; (2) domain oracle and canonical identity tests; (3) ledger migration/roles and lifecycle, fresh/upgrade/fingerprint tests; (4) atomic posting plus replay and outbox with real role/lock tests; (5) reversal/read/verifier; (6) relay/restore and load proof; (7) group extension only after accepted FX design. Forward-only migrations; no destructive rollback of financial history. Rollback application capability by disabling new money admission while preserving read/reconciliation and unresolved operations, never by deleting journals.

**Decisions still pending:** restricted function vs Go DML, exact caller identity/role layout, key scope and fingerprint encoding, lock/guard protocol across owning modules, bounds/floor policy, commit resolution/durability, event dependency model, load/RPO/RTO. G2/G3 owner/independent technical acceptance required before code. No database, Go suite, CI or load test executed for this deep dive. Next Stage C: clearing, settlement and payment finality, including evidence-driven state transitions and rail-specific return/recall rules.

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
