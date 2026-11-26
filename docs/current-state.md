# Current state and active work

Snapshot: **2026-11-26 (WIB)**. Inspected [main 76d417d3](https://github.com/herdifirdausss/mamony-go/commit/76d417d35571b28b7aa748577861fc3a1f9e6dcb) after [#81](https://github.com/herdifirdausss/mamony-go/pull/81). This is a point-in-time repository observation, not a requirement, release approval or independent technical review. See live [pull requests](https://github.com/herdifirdausss/mamony-go/pulls) and [issues](https://github.com/herdifirdausss/mamony-go/issues). Stable requirements remain in the owning contracts; historical Draft references describe historical decisions.

## Runtime and trust boundary

- Default HTTP composition mounts health/readiness routes only; Identity and Ledger are not public releases.
- Separate migration, maintenance, reconciliation and Identity worker entrypoints are not started by the default HTTP process. No live provider or production environment acceptance is established.
- Profile A does not demonstrate resistance to arbitrary writes to its authoritative database. Bounded synthetic acceptance and independent review deferrals are candidate-specific.

## Merged milestones and remaining gates

| Area | Merged / observed scope | Remaining |
|---|---|---|
| Governance | [#89](https://github.com/herdifirdausss/mamony-go/pull/89) bounded synthetic policy | #13 independent review and separate exact-candidate authorization |
| Foundation | DEC-01, bounded PostgreSQL pool and migration integrity; [#107](https://github.com/herdifirdausss/mamony-go/pull/107) documents proposed native executor | Current DEC-01 remains effective; #105 research; no engine cutover |
| Recovery | [#88](https://github.com/herdifirdausss/mamony-go/pull/88) RH-01/RH-02 bounded native PITR lab | RH-03 PARTIAL; #95 Draft; independent completeness and release not established |
| Identity | #22–#77, #79, #80 and [#81](https://github.com/herdifirdausss/mamony-go/pull/81): bounded internal Profile A through atomic passkey possession capability v36 | #20/#78/#18; no passkey login, public activation, real delivery, full recovery or end-to-end financial authorization |
| Monetary domain / reference | #91, #93, #96, #98 bounded monetary and historical reference slices | Synthetic scope only; not complete historical dataset, conversion or production registry |
| Ledger | #87 design alignment; [#104](https://github.com/herdifirdausss/mamony-go/pull/104) isolated bounded synthetic persistence foundation | #86; no public money API, real funds or runtime activation |
| Recovery-session capability | #108 and Draft #109 | E2 BLOCKED; implementation-ready design incomplete; no implementation authority |

Identity capability v36 uses SQL migration 000035 (fingerprint version 35); capability v35 was Go-only assertion verification. No renumbering is required. #81 merged on bounded synthetic owner authorization, with independent technical acceptance DEFERRED and production readiness NOT ACCEPTED. Historical unmerged statements in [Identity contract](contracts/identity.md), [open decisions](open-decisions.md), [ADR-0041](adr/0041-passkey-atomic-activation.md) and [atomic activation proposal](proposals/identity-passkey-atomic-activation.md) must not be treated as current merge status. Their security requirements and historical decision records are not changed by this snapshot.

## Open pull requests at inspection (four, all Draft)

| PR | Scope | Next gate |
|---|---|---|
| [#95](https://github.com/herdifirdausss/mamony-go/pull/95) | Platform RH-03 bounded state reconciliation | Partial overall; separate acceptance and exact-head merge decision required |
| [#102](https://github.com/herdifirdausss/mamony-go/pull/102) | Identity recovery-fence policy research | No implementation or merge authorization |
| [#106](https://github.com/herdifirdausss/mamony-go/pull/106) | Bounded symbolic E1 evidence | Prior bounded acceptance not automatically transferable to changed head; no merge authorization |
| [#109](https://github.com/herdifirdausss/mamony-go/pull/109) | Critical recovery-session capability proposal | RF-08 merge prerequisite removed; integrated evidence NOT RUN; E2 BLOCKED |

## Open issues at inspection (eight)

| Issue | Responsibility |
|---|---|
| [#13](https://github.com/herdifirdausss/mamony-go/issues/13) | Branch protection and independent review |
| [#14](https://github.com/herdifirdausss/mamony-go/issues/14) | PostgreSQL foundation and recovery |
| [#18](https://github.com/herdifirdausss/mamony-go/issues/18) | Capability integration and authorization |
| [#20](https://github.com/herdifirdausss/mamony-go/issues/20) | Identity Phase 1 and release blockers |
| [#78](https://github.com/herdifirdausss/mamony-go/issues/78) | Passkey possession acceptance and recovery |
| [#86](https://github.com/herdifirdausss/mamony-go/issues/86) | Ledger and financial design |
| [#105](https://github.com/herdifirdausss/mamony-go/issues/105) | Migration cancellation and crash outcomes |
| [#108](https://github.com/herdifirdausss/mamony-go/issues/108) | Recovery-session design and acceptance prerequisites |

## Evidence boundary

The #81 merge tree matches the exact owner-approved pre-merge candidate (428 non-tree entries). No dedicated exact-main workflow runs were returned during inspection. On the merged baseline, documentation-only #109 HEAD a85ca8b4fd10947e96f253f7ade6c5c9a2e8a5d2 passed all five workflows; its [PostgreSQL job](https://github.com/herdifirdausss/mamony-go/actions/runs/45840196811/job/139979516945) records migration, passkey finisher-role, atomic possession and local epoch tests PASS. IRF-F05/F06 remain PARTIAL. This is post-merge regression evidence on a separate PR revision, not dedicated main-commit execution or integrated recovery acceptance. RF-01–08 NOT RUN for the proposed recovery policy; RH-03 PARTIAL, E2 BLOCKED. See the [#109 checkpoint](https://github.com/herdifirdausss/mamony-go/pull/109#issuecomment-7195112057).

## Sequencing

Use the [change lifecycle](development-workflow.md). Keep detailed evidence in the owning issue/PR and stable requirements in contracts. No Draft merge, runtime activation, migration execution, registry or production approval follows from this snapshot.
