# ADR-0042 — Proposed context-aware forward-only migration executor

**Status: PROPOSED DEC-01 AMENDMENT / OWNER-ACCEPTED DETAILED DESIGN CANDIDATE ONLY.** 2026-11-25. [Foundation issue #105](https://github.com/herdifirdausss/mamony-go/issues/105), [C1–C6 review](https://github.com/herdifirdausss/mamony-go/issues/105#issuecomment-7152858127), [proposal](../proposals/native-pgx-migration-executor.md), [proposed contract](../contracts/postgresql.md#proposed-native-migration-contract-not-effective).

The owner accepted C1–C6 for this documentation-only Draft. This is not an effective DEC-01 change, implementation or cutover approval. Current DEC-01 retains golang-migrate v4.20.1. Independent technical acceptance DEFERRED; production readiness NOT ACCEPTED. No actual native execution, database migration or fault test is evidenced.

## Context

At main f6b9a2d1f46413e7c87db0cc09d13acc4261c438, the shared runner uses an outer native pgx session for target lock and verification and a separate golang-migrate driver session for execution. The pinned driver uses independent/background contexts for initialization, lock, version, and metadata operations; its SQL batch has a separate 45-second timeout. The CLI's 180-second caller deadline is not a proven end-to-end bound. Legacy dirty marker, body+fingerprint transaction, and clean marker are distinct transactions. Existing preflight checks the applied manifest prefix, not necessarily exact requested final version. Connection failure does not establish whether COMMIT occurred. See the active [runbook](../runbooks/platform-migrations.md).

## Proposed decision

Retain PostgreSQL 18.7, Go 1.27, native pgx/v5, immutable forward-only SQL files, existing fingerprint format, separate capability ownership and administrative-only invocation. Subject to separate authorization and actual compatibility evidence, replace golang-migrate execution with a minimal native pgx executor, not a general migration framework.

- A single dedicated session owns the fixed target lock throughout verification and mutation. No app pool, runtime auto-migrate, generic target selection or reconnect-to-resume.
- Explicit transaction per version includes the unchanged reviewed SQL file (body and existing trailer) and clean version metadata. Confirm expected prefix and next fingerprint before commit. Stop on cancellation before starting another version. Do not claim all versions commit together or all possible SQL effects are transactional.
- Preserve existing metadata schema and historical hashes/timestamps; reject legacy dirty or inconsistent history. Only verified exact target and settled cleanup yield success. Distinguish cause, attempted-version effect, last verified durable prefix (including a proven fresh empty prefix), and resource cleanup; a confirmed commit remains committed even if the run cannot report success. Unresolved COMMIT or cleanup blocks further execution.
- Immutable bounded manifest snapshot, role/schema/database identity, and bounded work/cleanup/observation. No automatic retry, force, down, repair, engine fallback or metadata rewriting.
- Durable, exclusive admission and execution intent must be recorded before the first database mutation, including metadata bootstrap. Unresolved intent survives process death and prevents new execution until explicitly authorized settled reconciliation; fail closed if the admission authority is unavailable. Unknown outcomes require durable operational quarantine, not merely a record written after an error is returned. The concrete authority, store and enforcement mechanism remain unresolved; an in-memory error is not a restart fence.

## Alternatives

A: retain golang-migrate and add strict final verification/graceful stop. Low implementation change but no direct in-flight caller context or atomic clean metadata.

B: context-aware driver adapter. Retains orchestration but split metadata and driver lifecycle remain complex.

C: native pgx per-version transaction (recommended candidate). More direct control and fewer session boundaries, but repository owns migration engine correctness, compatibility, fault evidence and cutover.

D: new framework or subprocess-only wrapper. Extra dependency/lifecycle cost; process termination is not proof of DB rollback.

## Consequences and gates

Legacy failed-body dirty metadata is existing tested behavior; proposed native confirmed rollback preserves the previous clean prefix. This is a behavioral contract change, not a refactor. Preserve legacy characterization and add distinct native acceptance rather than silently rewriting assertions. Neither history hashes nor locking protect against compromised administrators. Time-derived fields may differ because explicit transaction start differs from legacy batch; verify semantics with explicit oracles.

The current engine remains default until a separately accepted implementation and controlled cutover. Static 36-file audit is not real engine equivalence. Fresh-state admission requires approved provisioning provenance and target identity, including restore/incarnation disposition, expected object inventory and owner/grants; absence of metadata alone cannot establish freshness. Require actual isolated old/new compatibility, populated historical fixtures, cancellation/crash/commit ambiguity/restore tests and mandatory regression. No mixed writers, live migration, production cutover or activation. #105 remains open under #14 and is a prerequisite to Ledger runtime/production. Independent technical acceptance mandatory before production.

## Proposed DEC-01 amendment wording

Retain the accepted PostgreSQL/Go/pgx versions, per-schema roles, immutable forward-only SQL and canonical fingerprint verification. After separate accepted engine implementation and cutover gates, replace golang-migrate orchestration with a dedicated native pgx executor using context-aware per-version atomic SQL/fingerprint/clean-version transactions, bounded lifecycle and explicit durable outcome classification; preserve legacy dirty refusal and historical metadata. This proposed wording does not supersede the current DEC-01 today.

See the [detailed decision and acceptance matrix](../proposals/native-pgx-migration-executor.md).
