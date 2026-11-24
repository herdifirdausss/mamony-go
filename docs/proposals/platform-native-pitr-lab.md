# Platform native PostgreSQL recovery lab

**Status:** OWNER LAB SCOPE ACCEPTED; independent G3 technical acceptance pending; RH-01 verified at fe1a4c1; RH-02b/c candidates pending CI; RH-02a NOT RUN.
**Owner:** Platform/database foundation ([Issue #14](https://github.com/herdifirdausss/mamony-go/issues/14), PGF-06). Identity dependency: #78; separate Draft #81 is unchanged.
**Decision record:** [owner acceptance](https://github.com/herdifirdausss/mamony-go/issues/14#issuecomment-7133295432). Risk path: Critical recovery verification. No production behavior is approved.
**Contracts:** [PostgreSQL](../contracts/postgresql.md), [Platform](../contracts/platform.md) FND-08/FND-12, [recovery runbook](../runbooks/recovery.md), DEC-01/12/17. Checked 2026-11-24.

## G1 — Research and gap

The Identity logical restore and repeat-target tests on Draft #81 (19158ff, PostgreSQL run 45405294442) established bounded hold and review-record binding; they did not exercise physical backup, WAL replay, or an independent archive. Main's PostgreSQL contract requires a fresh restore and observed recovery time. PGF-06 remains partial. Use the accepted PostgreSQL 18.7 rather than a custom event replay engine.

| Primary source (PostgreSQL 18) | Finding and limitation |
|---|---|
| [Continuous archiving/PITR](https://www.postgresql.org/docs/18/continuous-archiving.html) | Physical recovery replays WAL; archive recovery also looks in local pg_wal. Configuration files are not reconstructed by WAL. Specify timeline and target; no silent recovery to the latest available state. |
| [pg_basebackup](https://www.postgresql.org/docs/18/app-pgbasebackup.html) | Native consistent physical backup with manifest; backup-contained WAL may satisfy some recovery needs. |
| [pg_verifybackup](https://www.postgresql.org/docs/18/app-pgverifybackup.html) | Verify plain backup and required backup WAL with matching server tools. Manifest checks do not certify all subsequent archived WAL, completeness, or successful restore. |
| [WAL recovery target settings](https://www.postgresql.org/docs/18/runtime-config-wal.html) | Explicit target and timeline; recovery ending before a specified target is fatal. Distinguish bounded wait, startup failure, and verified recovery. |
| [Administrative functions](https://www.postgresql.org/docs/18/functions-admin.html) | Named restore point returns a WAL location that may be used as a synthetic boundary. |

## G2 — Accepted bounded lab scope

Actor: authorized platform test operator. Outcome: native physical restore reproduces known synthetic transaction state through an explicit, independently recorded test boundary, and missing or mismatched required evidence never qualifies for review. Owner accepted RH-01/RH-02 only; not full PGF-06 or production readiness.

| Case | Acceptance criteria | Status |
|---|---|---|
| RH-01a | Create a fresh PostgreSQL 18.7 fixture with a committed marker before physical backup; verify plain backup manifest with matching version tools. | NOT RUN |
| RH-01b | Commit another marker after backup, keep a separate marker uncommitted then roll it back, quiesce synthetic writers, create a named boundary and independently record source system ID, timeline, target name/LSN and exact expected rows. Confirm required post-backup WAL is archived. | NOT RUN |
| RH-01c | Restore onto a separately owned disposable data directory/container with no application network access or source connection. Replay archive through explicit target and timeline; prove target reached, exact rows match, post-backup committed marker exists, uncommitted marker absent. Observe elapsed backup and recovery times. Remain held. | NOT RUN |
| RH-02a | Remove a specifically required post-backup WAL segment from an isolated copy of the archive (not the original), ensure backup-local WAL cannot satisfy that missing interval, and require native target-not-reached failure or bounded HOLD. No timeout counts as positive evidence. | NOT RUN |
| RH-02b | Reject a mismatched backup/system identity, timeline or target boundary record before review; distinguish an actual native wrong-timeline restore test from a metadata mismatch. | NOT RUN |
| RH-02c | Absent or unknown source boundary, incomplete archive evidence, failed verification or corrupt manifest cannot qualify for review, even if PostgreSQL starts. | NOT RUN |

Non-goals: production backup scheduler, WAL retention and key custody, full grants/RLS/schema parity, real deployment, financial or Identity credential reconciliation, provider notices, generation registry, release approvals, production RTO/RPO or availability guarantees. Duplicate evidence collector is a later RH-02 refinement and is not implemented by this native restore slice.

## G3 — Proposed design for technical review

Use a dedicated CI-only lab script under tools/postgrestest and the existing PostgreSQL integration workflow. Never mutate the existing service container or accept external DSNs, arguments, backup files or live credentials. Own unique disposable containers/volumes with verified labels, image and no published network ports; scope every privileged command and cleanup to resources created by the run. Use a separate test archive directory/volume outside both PGDATA locations, with the target reading its own archive copy. The archive fixture simulates separate custody, not independent production administration. Do not add application dependencies, routes, migrations, persistent roles, or release code.

Sequence: initialize marker fixture; capture verified native backup; commit additional synthetic marker; observe known rolled-back transaction; quiesce writes; record boundary and timeline externally; archive required segments; create isolated recovery copy; inspect exact restored markers and explicit recovery target; repeat from independent copies for failures. Keep positive and negative resources distinct. Bound startup/archival/replay waits; fail closed on unexpected target identity, missing tools, unrecognized state, or missing mandatory evidence. Logs contain synthetic identifiers/counts/elapsed time only, never real backup contents. CI must fail on missing case execution.

Physical restore preserves the source system identifier; do not reuse the logical restore requirement that source and target system IDs differ. Distinguish resource identity (container/volume/attempt) from physical lineage (system ID/timeline). Do not confuse successful startup, a manifest digest, or last archived filename with reaching the selected boundary. A required WAL gap must be beyond backup-contained WAL; preserve an unmodified archive for positive control. No automatic promotion, runtime access, or release. Use explicit target and held/read-only observation; review actual PostgreSQL behavior before selecting configuration. A fatal target miss, timeout, or unavailable evidence stays held; absence of an expected success marker must fail CI for positive case.

Trust model: trusted synthetic lab administrator/runner; source state and known commit boundary are controlled by the fixture. No claim about a malicious privileged writer, archive tampering by administrator, production durability, or acknowledged tail when source is lost. The lab boundary is not a general commit watermark. No production security policy selection.

**Technical acceptance:** pending. Governance PR [#89](https://github.com/herdifirdausss/mamony-go/pull/89) is now merged and the active main lifecycle permits an explicitly owner-approved bounded synthetic Draft experiment. The recorded #14 scope covers this RH-01/RH-02 lab only. Independent technical acceptance is not claimed, and no synthetic merge or production release is authorized.

## G4–G5 — Implementation handoff

Implement the positive native restore first, then required-segment and metadata/boundary negatives, with mandatory case markers and actual CI log verification. Use existing PostgreSQL integration workflow, no new runtime package. Record exact SHA and individual results in Issue #14. All RH-01/RH-02 cases NOT RUN. No merge or release approval. Follow independent review and exact-head CI before merge. Later RH-03–05 remain separate decisions.

### RH-01 candidate — 2026-11-24

- `tools/postgrestest/check-platform-native-pitr.sh` (a83b295, 1a5ab68) implements the bounded synthetic marker experiment in two newly created PostgreSQL 18.7 containers with network mode none. No external database arguments, published ports or application runtime. Local socket trust is restricted to disposable fixtures; not a production authentication pattern.
- Separate committed transactions establish pre/post-backup markers; a rolled-back marker and a later committed marker must be absent at the selected boundary. Verify native backup manifest, post-backup archive segment outside base WAL, source physical lineage, external process-local boundary record, named target pause and exact restored rows. Record backup/restore elapsed seconds; do not claim RTO/RPO or independent custody. The fixture intentionally commits a later marker to distinguish target-specific replay from latest-state recovery, then stops the source before copying the archive.
- CI step e1afee7 requires all three RH-01 case markers and successful script exit; no existing checks removed. RH-01a/b/c candidate **NOT RUN** pending exact-head GitHub CI. RH-02a/b/c **NOT RUN** (not implemented). No release, reconciliation or production changes. The working environment has no local Docker executable; actual execution must be verified in CI. Independent review and exact-head verification remain pending.


### RH-01 observed evidence and RH-02b/c candidate — 2026-11-24

Exact head `fe1a4c10e32a5d81dd14fc4ab9018d6d59433249` passed all five workflows. [PostgreSQL integration 45426748271](https://github.com/herdifirdausss/mamony-go/actions/runs/45426748271) job 138560704492 actually logged RH-01a/b/c PASS, including native backup verification, archived boundary and paused recovery with rows 1,2 only. Coarse shell elapsed counters reported backup 1s and recovery 0s (subsecond resolution not measured); not production RTO/RPO evidence.

Candidate 355089f extends the process-local record to include the backup-manifest SHA-256 and named target. It requires matching seven fields and manifest digest, plus presence of the required archive segment. Separate synthetic record copies exercise individual field mismatches; missing/truncated record, absent archive evidence and corrupt manifest fail the lab evidence predicate. A third disposable network-none container runs native pg_verifybackup against an intentionally invalid manifest on its own backup copy. No original backup, positive target, runtime access or live data is modified. CI 29c6b34 additionally requires RH-02b/c markers. These are candidates, NOT RUN at this revision until exact-head logs confirm them.

RH-02b is limited to metadata mismatch, not a native wrong-timeline recovery. RH-02c tests representative missing/incomplete evidence and corrupt manifest, not all archive continuity or acknowledged-tail completeness. The reference and digest are generated within the same trusted lab process, not independently authenticated custody. RH-02a actual required-WAL-gap recovery remains NOT RUN. The copied archive check is not a substitute for native recovery with an incomplete archive. No production implementation, promotion, release, or merge authorized.
