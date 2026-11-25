# Current state and active work

Snapshot: **2026-11-25 (WIB)**. Inspected `main` at [`dd0e5453`](https://github.com/herdifirdausss/mamony-go/commit/dd0e5453087ba01e6b325b8e983d9005760416f1), after [#96](https://github.com/herdifirdausss/mamony-go/pull/96). This is an observation, not a new requirement, approval, release or independent technical review. [Open pull requests](https://github.com/herdifirdausss/mamony-go/pulls) and [open issues](https://github.com/herdifirdausss/mamony-go/issues) are live; counts below are point-in-time (before this documentation PR). Contracts, ADRs and issue acceptance criteria remain authoritative.

## What actually runs

- Default `make run` / `cmd/mamony`: public GET /livez, private GET /livez and /readyz; process readiness only. No Identity or Ledger routes are mounted.
- Separate migration, maintenance, reconciliation and Identity worker entrypoints exist; they are not started by the default HTTP process. No production deployment or live delivery is established.
- Profile A trusts its authoritative runtime/database; independent authority and multi-region profiles are not established. Independent technical acceptance for bounded solo experiments is **DEFERRED** and production readiness **NOT ACCEPTED**.

## Merged baseline and remaining gates

| Area | Merged / observed scope | Remaining |
|---|---|---|
| Governance | Solo ruleset baseline tracked by [#13](https://github.com/herdifirdausss/mamony-go/issues/13); bounded synthetic policy [#89](https://github.com/herdifirdausss/mamony-go/pull/89) | Independent human review and remaining admin verification; separate exact-head owner authorization for eligible synthetic merges |
| PostgreSQL | DEC-01, disposable role/RLS fixture, migration integrity and bounded pool ([#15](https://github.com/herdifirdausss/mamony-go/pull/15)–[#19](https://github.com/herdifirdausss/mamony-go/pull/19)); separate Identity migrations | [#14](https://github.com/herdifirdausss/mamony-go/issues/14), [#18](https://github.com/herdifirdausss/mamony-go/issues/18): deployment, role/grant parity, capability authorization and recovery acceptance |
| Recovery | [#88](https://github.com/herdifirdausss/mamony-go/pull/88) RH-01/RH-02 native PITR lab with six scoped synthetic acceptance markers | RH-03 partial in Draft [#95](https://github.com/herdifirdausss/mamony-go/pull/95); recovered application enforcement, independent evidence/custody and production recovery objectives not established |
| Identity | [#22](https://github.com/herdifirdausss/mamony-go/pull/22)–[#77](https://github.com/herdifirdausss/mamony-go/pull/77), through v35: enrollment, email verification, password login, opaque sessions, password change/recovery, security notices and unmounted WebAuthn candidates | [#20](https://github.com/herdifirdausss/mamony-go/issues/20), [#78](https://github.com/herdifirdausss/mamony-go/issues/78), [#18](https://github.com/herdifirdausss/mamony-go/issues/18); public activation, tenant/resource authorization, operational delivery, passkey login and full recovery not accepted |
| Ledger K1 | [#91](https://github.com/herdifirdausss/mamony-go/pull/91): isolated pure monetary domain, balanced-journal validation and deterministic account mutation planning | No durable posting, database, replay, reversal execution, financial authority, outbox or restore; K2–K6 not authorized by this slice |
| Platform FND-02a | [#93](https://github.com/herdifirdausss/mamony-go/pull/93): curated immutable currency/country catalog | Partial FND-02, not a production registry or complete historical data |
| Ledger FND-02b | [#96](https://github.com/herdifirdausss/mamony-go/pull/96): USD-only canonical reference admission, unmounted composition helper | Historical definition identity and full FND-02/LGR-09 outstanding; no public posting, runtime activation or database |
| Financial pilot and later capabilities | Requirements/roadmap; financial design in Draft [#87](https://github.com/herdifirdausss/mamony-go/pull/87) | No accepted end-to-end payments, payouts, FX, provider, screening, settlement or operational pilot |

Identity v35 verifies signed possession assertions only; it does not consume the challenge or activate the pending credential. K1 and FND-02a/b were bounded synthetic experiments before full Foundation/Identity completion, not a change to the delivery order. See [roadmap](roadmap.md), [Identity contract](contracts/identity.md), [Ledger contract](contracts/ledger.md) and [Platform contract](contracts/platform.md).

## Open pull requests (six, all Draft)

| PR | State at inspection | Next gate / limit |
|---|---|---|
| [#79](https://github.com/herdifirdausss/mamony-go/pull/79) | v36 design | Critical design/acceptance under [#78](https://github.com/herdifirdausss/mamony-go/issues/78); not merged |
| [#80](https://github.com/herdifirdausss/mamony-go/pull/80) | v36 abstract Go handoff, stacked on #79 | No concrete durable activation in this slice; retain Draft |
| [#81](https://github.com/herdifirdausss/mamony-go/pull/81) | v36 PostgreSQL candidate, stacked on #80 | No merge or activation authority; historical CI evidence does not transfer to a changed head |
| [#87](https://github.com/herdifirdausss/mamony-go/pull/87) | Ledger/cross-border design; owner G2 direction recorded, G3 handoff | Design review and canonical alignment; not merged, no downstream implementation authority |
| [#95](https://github.com/herdifirdausss/mamony-go/pull/95) | RH-03 bounded synthetic recovery candidate | Partial; RH-03b3 recovered-application enforcement outstanding; no merge authority |
| [#98](https://github.com/herdifirdausss/mamony-go/pull/98) | FND-02c historical currency definition experiment | G5 reported complete; separate exact-head owner G6 decision and G7 required; no database, real history import, conversion or activation |

## Open issues (seven)

| Issue | Remaining responsibility |
|---|---|
| [#13](https://github.com/herdifirdausss/mamony-go/issues/13) | Solo protection baseline active; remaining admin verification/independent review |
| [#14](https://github.com/herdifirdausss/mamony-go/issues/14) | PostgreSQL foundation and recovery; deployment, grants and RH-03/full recovery evidence |
| [#18](https://github.com/herdifirdausss/mamony-go/issues/18) | Tenant-aware capability integration and end-to-end authorization |
| [#20](https://github.com/herdifirdausss/mamony-go/issues/20) | Identity Phase 1 and release blockers |
| [#78](https://github.com/herdifirdausss/mamony-go/issues/78) | v36 one-use passkey possession transition and related acceptance/recovery |
| [#86](https://github.com/herdifirdausss/mamony-go/issues/86) | Financial research, design, Ledger requirements and implementation sequencing |
| [#97](https://github.com/herdifirdausss/mamony-go/issues/97) | FND-02c immutable historical currency definition identity |

Older issue body statements (for example, #20 saying no PR remains or #14 describing branch protection as deferred) are historical; consult current lists, issue comments, later decisions and actual merge state before acting. Closed slice issues #90, #92 and #94 do not close the parent Ledger/Foundation gates.

## Evidence at the inspected main commit

The exact main commit has four successful push-triggered check runs: [foundation](https://github.com/herdifirdausss/mamony-go/actions/runs/45529871215/job/138922210310), [lint](https://github.com/herdifirdausss/mamony-go/actions/runs/45529871215/job/138922210778), [vulnerability](https://github.com/herdifirdausss/mamony-go/actions/runs/45529871215/job/138922210839) and [PostgreSQL](https://github.com/herdifirdausss/mamony-go/actions/runs/45529871245/job/138922210323). These are GitHub-reported conclusions, not a new local test execution. PR-specific checks and scoped results are in the linked PRs; do not transfer results between heads. Full production readiness, real delivery, durable Ledger integration and end-to-end authorization are not proven.

## Work sequencing

Continue already authorized bounded Draft work and obtain separate review/owner decisions according to the [change lifecycle](development-workflow.md). Do not infer authorization to merge a Draft, activate Identity or financial runtime, import production data, change migrations or begin K2–K6 from this snapshot. Refresh this page when a material milestone merges or acceptance changes; keep detailed evidence in the owning issue/PR and stable requirements in contracts.
