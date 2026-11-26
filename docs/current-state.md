# Current state and active work

Snapshot inspected on **2026-11-26 (WIB)** at [main `76d417d`](https://github.com/herdifirdausss/mamony-go/commit/76d417d35571b28b7aa748577861fc3a1f9e6dcb). For live status, see [open pull requests](https://github.com/herdifirdausss/mamony-go/pulls) and [open issues](https://github.com/herdifirdausss/mamony-go/issues). The [roadmap](roadmap.md) records delivery order. The owning contracts and ADRs record requirements and design decisions.

## What actually runs

- The default `make run` / `cmd/mamony` serves only health endpoints. The public listener serves GET /livez. The private listener serves GET /livez and /readyz. Readiness covers the HTTP process.
- The repository has separate migration, maintenance, reconciliation and Identity worker entrypoints. The default HTTP process starts only the health listeners. Production deployment and live delivery remain unverified.
- [Profile A](architecture.md#security-authority-and-trust-boundaries) trusts the application runtime and its authoritative database. Independent technical acceptance for the merged bounded synthetic slices is **DEFERRED**. Production readiness is **NOT ACCEPTED**.

## Merged baseline and remaining gates

| Area | Decision and merged scope | Remaining |
|---|---|---|
| Governance | Active solo ruleset and [bounded synthetic policy](https://github.com/herdifirdausss/mamony-go/pull/89). | [#13](https://github.com/herdifirdausss/mamony-go/issues/13) tracks remaining governance verification and independent review. Each eligible synthetic merge requires separate exact-candidate owner authorization. |
| PostgreSQL | Effective DEC-01 uses golang-migrate. The disposable role/RLS fixture, migration integrity checks, bounded pool and separate Identity migrations are merged. [ADR-0042](adr/0042-context-aware-migration-executor.md) was merged as a proposed DEC-01 amendment. | [#105](https://github.com/herdifirdausss/mamony-go/issues/105) tracks migration cancellation and crash-outcome design. [#14](https://github.com/herdifirdausss/mamony-go/issues/14) and [#18](https://github.com/herdifirdausss/mamony-go/issues/18) track deployment, grants, recovery and capability authorization. |
| Recovery | [#88](https://github.com/herdifirdausss/mamony-go/pull/88) merged the disposable native PostgreSQL point-in-time recovery (PITR) lab for RH-01 and RH-02. | The [RH-03 state reconciliation lab](https://github.com/herdifirdausss/mamony-go/pull/95) is partial. Recovered application enforcement, independent evidence and production recovery objectives remain open. |
| Identity | Internal enrollment, login, sessions, recovery and security-notice slices are merged. [#81](https://github.com/herdifirdausss/mamony-go/pull/81) merged one-use passkey possession verification for bounded synthetic use. The transition records `POSSESSION_VERIFIED` and queues a notice. Identity remains unmounted. | [#20](https://github.com/herdifirdausss/mamony-go/issues/20), [#78](https://github.com/herdifirdausss/mamony-go/issues/78) and [#108](https://github.com/herdifirdausss/mamony-go/issues/108) track Phase 1, residual passkey acceptance and recovery-session design. Passkey login, recovery authority, resource authorization, production delivery and public activation remain open. |
| Reference data | [FND-02a–d](contracts/platform.md#configuration-data-and-operations) merged the curated catalog, USD-only Ledger admission, private historical identity fixtures and explicit curated definition IDs. | Full historical lifecycle, provenance and full FND-02 acceptance remain open. |
| Ledger | [K1 — Monetary domain](roadmap.md#ledger-implementation-sequence) merged in [#91](https://github.com/herdifirdausss/mamony-go/pull/91). [#87](https://github.com/herdifirdausss/mamony-go/pull/87) records the accepted ownership and USD definition direction. [K2a — Isolated persistence foundation](proposals/ledger-k2a-isolated-persistence.md) merged in [#104](https://github.com/herdifirdausss/mamony-go/pull/104) as a one-time synthetic sequencing exception with disposable PostgreSQL storage and restore fixtures. | [#86](https://github.com/herdifirdausss/mamony-go/issues/86) tracks remaining design and sequencing. Full K2 account lifecycle, K3 journal posting and later Ledger stages remain outside K2a. Ledger runtime integration and the money API remain pending. |
| Financial pilot | The [roadmap](roadmap.md#simulated-pilot) identifies the planned capabilities and dependencies. | End-to-end payments, payouts, FX, provider, screening, settlement and operational pilot acceptance remain open. |

<a name="open-pull-requests-six-all-draft"></a>

## Open pull requests

At this snapshot, the [Platform state reconciliation lab](https://github.com/herdifirdausss/mamony-go/pull/95), [Identity recovery-fence policy](https://github.com/herdifirdausss/mamony-go/pull/102), [symbolic recovery-policy experiment (E1)](https://github.com/herdifirdausss/mamony-go/pull/106) and [recovery-session design](https://github.com/herdifirdausss/mamony-go/pull/109) were Draft. The [recovery-fence policy PR](https://github.com/herdifirdausss/mamony-go/pull/102) records the scope and prerequisites for the proposed follow-up study (E2).

<a name="open-issues-seven"></a>

## Open issues

Use the [live issue list](https://github.com/herdifirdausss/mamony-go/issues). Follow the links in the baseline table for the open decisions that block each capability.

## Evidence at the inspected main commit

GitHub reported successful push checks at the inspected commit for [foundation](https://github.com/herdifirdausss/mamony-go/actions/runs/45839562201/job/139977340457), [lint](https://github.com/herdifirdausss/mamony-go/actions/runs/45839562201/job/139977340367), [vulnerability](https://github.com/herdifirdausss/mamony-go/actions/runs/45839562201/job/139977340055) and [PostgreSQL](https://github.com/herdifirdausss/mamony-go/actions/runs/45839562241/job/139977339670). See the [passkey possession](https://github.com/herdifirdausss/mamony-go/pull/81#issuecomment-7195023603) and [isolated Ledger storage](https://github.com/herdifirdausss/mamony-go/pull/104#issuecomment-7152708615) merge records for exact candidate evidence and scope limits.

The merge commit tree matches the exact owner-authorized #81 candidate. Identity capability v36 uses SQL migration 000035 (fingerprint version 35); v35 was Go-only assertion verification. This is bounded synthetic merge evidence, not production deployment or independent technical acceptance.

The documentation-only [recovery-session proposal #109](https://github.com/herdifirdausss/mamony-go/pull/109#issuecomment-7195112057) passed all five workflows on head `a85ca8b4fd10947e96f253f7ade6c5c9a2e8a5d2` based on the inspected main. Its [PostgreSQL job](https://github.com/herdifirdausss/mamony-go/actions/runs/45840196811/job/139979516945) recorded passing migration, finisher-role, atomic possession and local epoch tests. IRF-F05 and IRF-F06 remained PARTIAL. This is baseline regression evidence only, not integrated recovery acceptance. The prior #81 merge prerequisite is removed; recovery-session implementation and E2 remain BLOCKED, RF-01–08 integrated evidence NOT RUN, RH-03 PARTIAL. Four technical-design blockers remain: authoritative currentness and affected scope; integration and durable consistency; audit/notice applicability and custody/retention; exact change impact, containment and independent review. No new implementation or release authorization follows.

## Work sequencing

Follow the [roadmap](roadmap.md) and [change lifecycle](development-workflow.md) for prerequisites, acceptance and authorization. Refresh this snapshot when a material milestone merges or acceptance changes. Keep detailed evidence in the owning issue or PR and stable requirements in contracts.
