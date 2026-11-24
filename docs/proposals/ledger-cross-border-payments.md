# Ledger and cross-border payments: financial architecture proposal

- Status: **PROPOSED — Critical; G0/G1 research consolidated; G2/G3 unaccepted.**
- Intake: [#86](https://github.com/herdifirdausss/mamony-go/issues/86); review: [PR #87](https://github.com/herdifirdausss/mamony-go/pull/87).
- Research baseline: main f82b47c5a198e731f652053bc050aace3d91f58c. Consolidation: 2026-11-24.
- Owner: repository owner; independent technical reviewer not designated.
- Scope: synthetic funds, identities, provider and evidence only. No active requirements, API, schema, implementation, deployment or Identity-first direction changed.
- Governing: [product](../product.md), [roadmap](../roadmap.md), [change lifecycle](../development-workflow.md), [decisions](../open-decisions.md), [ADR-0002](../adr/0002-ledger-and-data-ownership.md), [contracts](../index.md). Existing accepted decisions and contracts win over every candidate below until explicitly amended.

## How to use this proposal

This is the review entry point, not a parallel normative contract. The [research evidence appendix](ledger-cross-border-evidence.md) preserves the complete A–F research, original G1–G4 candidate material, source register and abstract experiments. Its alternatives and historical wording may conflict; use the conflict/decision registers below to decide what to promote. Once accepted, stable requirements and acceptance checks belong only in the owning contracts; enduring cross-capability decisions belong in ADRs. Do not create duplicate numbered PRDs, new DEC IDs or packages simply because an appendix mentions them. Candidate labels FIN-A through FIN-L below are local review labels, not active contract IDs. G/H/I/J/K labels used in conversation are work phases, not repository G0–G7 gates.

## G1 — Research synthesis and evidence index

| Topic | Research evidence | Reusable conclusion (not new authority) |
|---|---|---|
| Accounting and ledger | [A](ledger-cross-border-evidence.md#deep-dive-a--financial-accounting-and-ledger-domain-model) | Model legal book, currency, normal side and exact economic facts; distinguish available/pending/in-transit, assets and liabilities. |
| PostgreSQL and concurrency | [B](ledger-cross-border-evidence.md#deep-dive-b--ledger-database-transactions-and-concurrency) | Known locked rows, database constraints, exact replay, outbox and explicit caller authority; no blind retry. |
| Payment and settlement | [C](ledger-cross-border-evidence.md#deep-dive-c--payment-lifecycle-clearing-and-settlement) | Separate workflow, dispatch, cash movement, fulfillment and return evidence; settlement finality is rail-specific. |
| Cross-border and treasury | [D](ledger-cross-border-evidence.md#deep-dive-d--cross-border-payments-fx-and-treasury) | Currency-specific balanced journals, local group atomicity candidate, separate actual target cash and market exposure. |
| Reconciliation and recovery | [E](ledger-cross-border-evidence.md#deep-dive-e--reconciliation-financial-reporting-and-recovery) | Five independent control layers, exact allocations and coverage; external facts survive local restore. |
| Financial security | [F](ledger-cross-border-evidence.md#deep-dive-f--financial-security-fraud-and-compliance) | Exact action/intent authority, screening and hard guards, controlled approvals, residual Profile A risk. |

Key source spot-checks on 2026-11-24 (not a full hyperlink or legal audit):
- [PostgreSQL 18 isolation](https://www.postgresql.org/docs/18/transaction-iso.html), [explicit locking](https://www.postgresql.org/docs/18/explicit-locking.html), [application consistency](https://www.postgresql.org/docs/18/applevel-consistency.html): READ COMMITTED needs explicit protection of business invariants; complex predicates may require additional guards or Serializable. Tests must use pinned repo version, not documentation alone.
- [Wise transfer statuses](https://docs.wise.com/guides/product/send-money/tracking/transfer-statuses): sent is not a beneficiary credit guarantee and later bounce-back exists.
- [Stripe Ledger](https://stripe.com/blog/ledger-stripe-system-for-tracking-and-validating-money-movement): clearing, timeliness and completeness are separate measurements.
- [BIS 2025 survey analysis, June 2026](https://www.bis.org/publications/qr-202606/uncovering-fx-settlement-risk-new-measures-2025-bis-triennial-survey): 36% of average daily FX settlement in April 2025 via PvP; not turnover, not Mamony target, not equivalent to SQL atomicity.
- [FATF R.16 2025 revision](https://www.fatf-gafi.org/content/fatf-gafi/en/publications/Fatfrecommendations/update-Recommendation-16-payment-transparency-june-2025.html), [June 2026 consultation](https://www.fatf-gafi.org/content/fatf-gafi/en/publications/Fatfrecommendations/R16-Public-Consultation-June-2026.html): end-2030 implementation expectation; consultation is not final guidance or domestic law. Legal applicability and all remaining source URLs/versions require review when used for a specific implementation. No compliance conclusion.

## G2 — Candidate product boundary

Actors: synthetic customer/merchant, permitted service capability, provider simulator, authorized finance/risk operator and auditor. Goal: explainable money movement with reliable local posting and external reconciliation, not a full banking product. Retain Identity Phase 1 before ledger; no live funds, real identities, public money endpoint, real sanctions checks/filings, real provider credentials, card data, lending, crypto, multibook/intercompany, direct network membership or independent-authority Profile B by default. Use same-currency control flow before one approved synthetic cross-currency corridor. USD to SGD is a candidate, not accepted. Funding and chart assume a synthetic single intermediary book; real legal ownership and revenue recognition not decided.

## Consolidated decision register

Every row is **OPEN / proposed**, unless it explicitly states an existing accepted baseline. Owner accepts product scope and risk; independent technical reviewer accepts design. Dependencies below are approval blockers for the affected slice, not permission to implement. Existing DEC-01 (PostgreSQL/Go/pgx/migration design) and Identity-first direction remain accepted and are not reopened.

| Review label | Existing decisions / contracts | Candidate to review | Evidence required before acceptance / first affected slice |
|---|---|---|---|
| FIN-A: accounting | LGR-01/02/09/10, ACC-01, FIN-02 | One book, per-currency accounts, explicit normal-side/floor, versioned chart and named liability balances; no real statutory GL claims | Owner approves economic ownership, fee recognition, account types/floors, correction and shortfall policy; independent accounting oracle; ledger domain |
| FIN-B: money | DEC-06, FND-02, FX-01/03, LGR-01 | int64 minor units, exact bounded intermediate arithmetic, immutable exponent version, explicit adapter units; wire candidate decimal minor-unit string | Decide wire format before public amount route; bounds, rounding, reference version and adapter tests; domain/API/FX |
| FIN-C: posting | DEC-02/05, LGR-01–05/09–12, ADR-0002, PostgreSQL contract | READ COMMITTED with known guard/account locks and exact canonical replay; narrow SQL posting boundary preferred over broad Go DML, no arbitrary participant code | Choose Go DML vs restricted SQL; prove actual caller binding, privileges, lock graph, replay identity, floor and close race, known/unknown commit; ledger |
| FIN-D: authority | DEC-07/13/14/16/17/18, IAM-06/07/12, LGR-12, PAY-05/OUT-05 | Exact server-owned intent, one-time receipt, current authority and freeze/dispatch fences; role binding and residual Profile A explicit | Actor/approval matrix, linearization and trust assumptions; adverse tests, actual role and security review; ledger/money admission |
| FIN-E: holds and limits | DEC-07, ACC-02, LIM-01–04, OUT-01/02, PAY-04 | Operation-scoped hold, exact used/reserved capacity, partial refund/return caps, release only proof of unused/no-send or approved resolution | Decide consumption/release milestones and exceptional frozen/in-flight accounting; race and lost-response tests; policy/payout |
| FIN-F: FX | DEC-02/08, FX-01–06, FEE-01/02, ADR-0002 | Fixed source/target with approved rounding, immutable quote; atomic local two-journal group candidate; separate external settlement recovery | Decide quote expiry, fee/rate, local grouping and positions; exact oracle and second-leg rollback; FX |
| FIN-G: corridor and evidence | DEC-09/11, PAY/OUT/PRV | Versioned synthetic profiles, immutable instruction and reference scope, business completion criterion separate from bank/beneficiary evidence | Owner approves corridor, calendar, grace, fee, actual simulator milestones/return capability and unknown handling; same-currency then cross-currency |
| FIN-H: treasury | DEC-09/12, FX/LIM/OUT/REC | Separate customer balance, FX exposure and partner liquidity allocation; minimal owning guard, no automatic trading or new service | Approve eligible cash/ownership, pool scope, guard, reservation and rebalancing authority; concurrent funding/cash debit proof; prefunded payout |
| FIN-I: recon | DEC-09/10/11, REC-01–05, OPS-01/02 | Complete versioned reports, exact scoped matching/allocations, five control layers, suspense/case controls | Approve source coverage, matching/correction/aging rules and restricted evidence; missing/duplicate/ambiguous/late fixture; pilot |
| FIN-J: events and recovery | DEC-10/12/17, LGR-06–08, EVT, FND-08 | Per-account predecessor order, dedup + progress, rebuild from immutable history, actual restore and external-effect recovery | Define retention/rebuild, checkpoint, RPO/RTO and load objectives; outbox/restore/old-worker drill; ledger gate and pilot gate |
| FIN-K: reporting | DEC-10/12, STL-01–04, FIN-01–04 | Separate provider cash batch and merchant batch, manifest-based allocation; close fence and immutable reporting snapshot | Approve dates/eligibility/net basis, exceptions and accounting close policy; actual snapshot and close race; operations |
| FIN-L: screening/risk | DEC-04/07/09/11/13–18, SCR/ONB/RSK/AML | Synthetic eligibility/screening and hard controls before money, advanced risk and activity monitoring later; no compliance assertion | Approve list freshness, tiers, review/failure policies, data/access/retention and synthetic labels; no real filings; money gate/operations |

## Conflict and amendment register

No active contract edits in this PR. Every row must have an explicit disposition before its affected implementation. Distinguish actual conflicts from gaps; do not rewrite valid existing requirements merely to restate research.

| Priority | Baseline and issue | Proposed disposition after acceptance |
|---|---|---|
| Blocker — actual conflict | [FX-05/06](../contracts/fx.md) require separately recoverable local legs; candidate local atomic group cannot also expose first-leg-only committed state | Supersede sequential local rule with group all-or-none, stable group replay and two independent currency journals; retain recovery for truly external legs. Amend FX contract and ADR-0002 explicitly (FIN-F). |
| Blocker — design choice | [LGR posting design](../contracts/ledger.md) describes Go use case/repository; mutation grants not selected | Preserve facade and Go workflow; decide whether repository invokes narrow SQL function or vetted direct DML. Resolve DEC-02 and role/grant model; not a requirement to put all business logic in SQL (FIN-C). |
| Blocker — scope change | LGR-03 caller/key uniqueness versus proposed book/tenant/kind scope | Decide whether extending scope creates distinct operations or accidental duplicate money. Preserve existing behavior until approved; define unique economic fact independently from transport key and authorized replay read (FIN-C). |
| Blocker — authority gap | IAM-12, LGR-12, PAY-05, OUT-05 exact intent already required; current caller role/approval participation not proven | Specify real authenticated capability binding (not current_user inside SECURITY DEFINER), lock/receipt protocol, freeze linearization and no-send proof. Do not assume same-process pools isolate full compromise (FIN-C/D). |
| Blocker — freeze ambiguity | ACC-02 refuses new movement; unknown external work may already have effect | Explicitly distinguish new admission/dispatch from restricted recognition/settlement and permitted recovery. No automatic release or bypass of legal hold; recheck current authority (FIN-D/E). |
| Blocker — lifecycle ambiguity | PAY-02 / OUT payout success and PRV-02 generic outcome | Define method-specific recognition/fulfillment evidence and independently visible cash and beneficiary state; retain provider raw status. A success event missing required journal stays processing (FIN-G). |
| Blocker — finality ambiguity | OUT-03 and payout completion after return window | Operational closure only, valid late return handled as linked fact/correction; no claim of universal legal finality; no silent discard or auto refund (FIN-G). |
| Blocker — risk/cash gap | FX position limit not partner liquidity; [accounts/policy](../contracts/fees-limits-and-accounts.md) no cash allocation guard | Specify treasury ownership and eligible cash guard before prefunded dispatch; avoid double subtraction and unapproved new module (FIN-H). |
| High — completeness gap | REC-01/02 reference match/report totals | Add scoped semantic fact, report version/coverage, allocation guard and per-item checking; same net total is insufficient (FIN-I). |
| High — recovery gap | [recovery runbook](../runbooks/recovery.md) restores local records and roles | Extend to external effects, stale authority and dedup, old worker fencing and no blind restart. Ledger restore first, external recovery before pilot release (FIN-J). |
| High — close gap | FIN-03/04 immutable closed periods but no explicit concurrency fence | Define close/effective-date coordination, source coverage and approved timing exceptions; preserve late corrections in open period (FIN-K). |
| Roadmap clarification | [roadmap](../roadmap.md) advanced fraud/monitoring after pilot; F suggests pre-admission risk controls | Keep order: required screening/onboarding/authorization, limits and basic failure posture before pilot; advanced RSK/AML rules, model evaluation and alerting in operations unless owner explicitly changes roadmap (FIN-L). |
| Scope clarification | Earlier illustrative combined payout cash/fulfillment journal vs staged J5/J6 | Combined evidence allowed only where approved profile proves both; staged separate evidence otherwise. No universal posting template or duplicate cash movement (FIN-A/G). |
| Scope clarification | First research narrative could imply quote fixed before slow external funding | First pilot quote executable only with settled source funds and current expiry; delayed collection recognized separately; no silent reprice. Alternative guaranteed quote requires DEC-08 approval (FIN-F). |
| Documentation gap | Stage A–F appendices contain duplicate recommendations, examples and candidate tables | Preserve as historical evidence only; canonical decision register here; accepted requirements in contracts and ADR, not appendices. |

## G3 — Candidate architecture summary (not accepted)

Retain Go modular monolith, one PostgreSQL database, per-capability schemas, explicit facades and grants. Ledger owns accounts/journals/entries/balance and posting transaction; it has no dependency on other capabilities. Accounts owns tenant/owner mappings; Identity owns subject/credential/approval authority; Payments/Payouts own workflow and immutable intent/hold obligations; FX owns rates/quotes/position; provider owns external operation/evidence; reconciliation owns cases, not balances; settlement/accounting own batch/report/close. Treasury capacity ownership is a decision, not a new package requirement. Outbox relay and consumers use explicit contracts, not unrestricted DB access. No public arbitrary posting endpoint, Redis authoritative balance, message broker dependency, generic orchestration platform or independent service split required.

### Consistency boundaries to prove

| Boundary | Candidate local commit | Explicitly outside |
|---|---|---|
| Journal | scoped replay, journal/entries/account steps/balance, event, allowed participant effects | remote calls, provider state |
| FX group (after approval) | both currency journals, immutable group/quote, fees/position and approved capacity participation | wholesale FX settlement, external cash |
| Payout admission | exact intent authority/receipt, customer hold, usage and applicable funding reservation, operation identity | actual provider send |
| Dispatch claim | current admission fence, stable provider ref and potential-send state | remote result; stale worker cannot be fenced by DB lease alone |
| Evidence application | normalized semantic fact, guarded transition, required posting/participant effects or durable unresolved recovery | bank/counterparty true state beyond evidence |
| Matching / correction | guarded allocations and case transitions; approved exact command via ledger owner | arbitrary automatic journal or fictitious cash |
| Close | effective-date fence and pinned manifest; finalize only after validated checks | future adverse events, statutory restatement |

Local atomic cross-capability writes require explicit grant and tested narrow interface; no assumption all owners share transaction merely because database is shared. Unresolved authority/participant protocol blocks the affected action. Stage H must choose concrete boundaries, lock classes and interface, without coding.

### Cross-stage invariants

- Every journal balances within one book/currency; amount positive exact int64 minor units, bounded exact intermediate math; no summing across currencies.
- Every financial effect has authorized immutable economic identity; transport retries and events cannot create duplicate effects.
- Available balances, usage, position and cash reservations are different authorities; never double deduct a posted/reserved amount.
- Ledger history append-only for runtime. Corrections are authorized facts; stored balance drift is integrity incident, not an excuse for balancing plug.
- Unknown external outcomes retain applicable holds/usage and use original reference; no deadline/lease-based blind resend, fallback or release.
- Provider signature, accepted status, beneficiary credit, cash settlement and reconciliation are different facts. Missing evidence is not zero activity.
- Restore cannot undo an external debit; safe resume requires evidence, fencing, current authority and reconciliation.
- Profile A trusts its authoritative database/process. No independent compromise resistance, real compliance, external exactly-once or production SLO claim.

## G4 — Candidate dependency and review sequence (not authorization to implement)

| Slice | Prerequisites | Contract trace / minimum verification |
|---|---|---|
| 0. Identity and platform gates | Accepted roadmap, relevant active issues; no work diverted | Identity authority, runtime/grants, migrations, audit and event foundation, current gate evidence |
| 1. Ledger domain and currency | FIN-A/B, relevant G2/G3 acceptance | LGR-01/09, FND-02; exact arithmetic, domain oracle, account semantics |
| 2. Ledger schema and lifecycle | Slice 1, FIN-C/D | LGR-09/10/12, PostgreSQL; roles, migrations, close/post and authority guards |
| 3. Atomic single posting and replay | Slice 2, FIN-C/D | LGR-01–04/12, EVT; actual two-session races, rollback, ambiguous commit, direct grant denials |
| 4. Reversal/read/order/verifier/restore | Slice 3, FIN-J | LGR-05–08/11; immutable reverse, ordered event, stable statement, read-only drift, actual restore; ledger gate |
| 5. Synthetic screening/onboarding/policy/account mapping | Identity and ledger gates, FIN-D/E/L | SCR/ONB/ACC/LIM/FEE; no stale pass, cap/floor races, exact approval, freeze |
| 6. Simulator and evidence intake | platform jobs/outbox, FIN-G; may be designed in parallel | PRV; isolated external-state fixture, unknown and duplicate dispatch, signature/mapping/egress negative checks |
| 7. Exact FX group and liquidity | accepted FIN-F/H, ledger/policy | FX/FEE/LIM; exact oracle, atomic second-leg failure, quote once, concurrent partner funding |
| 8. Same-currency control flow then one cross-currency | slices 5–7 as applicable; FIN-G | PAY/OUT, PRV; collection, hold, dispatch, debit/fulfillment, returns and unknown recovery |
| 9. Pilot reconciliation and external recovery | slices 4/6/8, FIN-I/J | REC/OPS/EVT; source coverage, allocation, case and restore after external send; pilot gate |
| 10. Operations | pilot gate, FIN-K/L | RSK/AML/STL/FIN/DSP as approved; shadow, restricted cases, settlement, close and measured hot-account load |

These are planning slices, not actual issues or PR authorizations. Map each accepted AC to code path, executable tests and reviewer in a small context pack when that slice is approved. No bulk creation of empty packages, speculative interfaces, migrations, new DEC/AC IDs or issues before dependency review. This preserves roadmap FX-before pilot payment work while using same-currency fixture before cross-currency activation.

## G5–G7 — Evidence status and review exit criteria

Historical abstract experiments: A illustrative journal arithmetic; B two-withdrawal interleavings; C six state histories; D 10,800 fixed-buy integer checks; E five batch matching fixtures; F 14 decision-precedence cases. These were standalone toy models, not repository tests or proof of implementation. Full historical source material in appendix. Stage G reviewed repo documents and spot-checked sources only. No Go, PostgreSQL, CI, backup restore, provider, fraud, penetration or performance tests run for financial features. No independent reviewer or owner approval recorded.

Stage G exit: proposal condensed, full original research preserved, explicit conflicts and decision groups indexed. Remaining H: concrete architecture alternatives and accepted boundary choices; I: proposed contract/ADR amendments, after acceptance promoted into owning sources; J: scoped engineering design; K: dependency-reviewed issue/PR plan; L: explicit G2 product and G3 technical acceptance before code; M: implementation under repo G5–G7. Labels are descriptive, not extra mandatory documentation gates. Source audit beyond listed spot-checks, exact contract IDs for new requirements, reviewer designation, approval of candidate decisions and real fixtures remain open. Do not merge as approved specification or start critical financial implementation from this proposal alone.
