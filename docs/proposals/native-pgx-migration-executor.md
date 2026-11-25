# Foundation #105 — Proposed native pgx migration executor

**Status:** C1–C6 owner-accepted detailed design candidate; documentation-only Draft, not implementation approval. [Issue #105](https://github.com/herdifirdausss/mamony-go/issues/105) under [#14](https://github.com/herdifirdausss/mamony-go/issues/14). [Research decision](https://github.com/herdifirdausss/mamony-go/issues/105#issuecomment-7152752304), [detailed review](https://github.com/herdifirdausss/mamony-go/issues/105#issuecomment-7152858127), [ADR-0042](../adr/0042-context-aware-migration-executor.md), [proposed contract](../contracts/postgresql.md#proposed-native-migration-contract-not-effective). Existing DEC-01 and runtime unchanged. Independent technical acceptance DEFERRED; production readiness NOT ACCEPTED.

## C1 — Compatibility inventory

Baseline f6b9a2d1f46413e7c87db0cc09d13acc4261c438. All 36 committed files fetched at this revision; all returned blob IDs matched the complete tree, and all recomputed SHA-256 body hashes matched exact target/version trailers. Static lexical screening is not PostgreSQL parsing or function-body semantic proof. No obvious top-level nontransactional operation, explicit transaction control, sequence mutation or external database operation detected. No execution or old/new parity evidence. Inventory (T tables/constraints; X ordinary index; F function; G grant/revoke; Tr trigger; B backfill; excludes trailer):

| Target/version | Statement classes |
|---|---|
| Platform 1 | T/G |
| Ledger 1 | T/G |
| Identity 1 | T/G |
| Identity 2 | T/X/G |
| Identity 3 | T/X/G |
| Identity 4 | X |
| Identity 5 | T/X/G |
| Identity 6 | T/F/G |
| Identity 7 | T |
| Identity 8 | G |
| Identity 9 | T |
| Identity 10 | T/F/G |
| Identity 11 | T/X/F/G |
| Identity 12 | T/B/F |
| Identity 13 | X/F |
| Identity 14 | F/Tr/G |
| Identity 15 | T/F/G |
| Identity 16 | T/X/F/Tr/G |
| Identity 17 | T/X/F/G |
| Identity 18 | G |
| Identity 19 | T/X/F/G |
| Identity 20 | F/G |
| Identity 21 | T/F/G |
| Identity 22 | T/X/F/G |
| Identity 23 | T/X/F/G |
| Identity 24 | T/X/F/G |
| Identity 25 | T/X/F/Tr/G |
| Identity 26 | T/X/F/G |
| Identity 27 | T/B/X/F/G |
| Identity 28 | T/X/F/G |
| Identity 29 | T/F |
| Identity 30 | T/F/G |
| Identity 31 | F/G |
| Identity 32 | T/F/G |
| Identity 33 | T/X/F/Tr/G |
| Identity 34 | T/F/G |

Identity v12 and v27 require populated historical backfill fixtures. Function/trigger/privilege behavior must be compared, including SECURITY DEFINER functions. Preserve exact historical SQL bytes and previously stored digests/timestamps. Newly generated transaction timestamps may differ; verify time-derived semantics with bounded oracles, not blanket omission. Existing v1 Platform/Ledger are fresh/no-op baselines; test-only upgrade fixture is not supported deployed history. Supported prior-version list is unresolved, not inferred from filenames. #81/#95 were open during review; reconcile changed main and any new migration before implementation.

## C2 — Metadata and outcome details

Retain legacy schema_migrations (BIGINT version PK NOT NULL, BOOLEAN dirty NOT NULL; verify singleton) and per-schema migration_fingerprints (existing version, sha256, recorded_at). New metadata table creation, if absent on positively verified fresh baseline, is transactional with first version. Preflight classification:

| State | Required handling |
|---|---|
| Fresh | Both metadata tables absent and independently verified empty business baseline; allow first version |
| Legacy empty | Compatible empty version metadata, no fingerprint/business history and proven fresh; allow |
| Clean prefix | One positive clean manifest version, exact hash prefix, expected metadata shape/owner/grants; allow next version |
| Exact target | Verified no-op |
| Legacy dirty | Refuse, regardless of fingerprint |
| Inconsistent | Refuse missing/extra/conflicting history, unknown version, multiple rows, unexpected shape/owner or business objects without history |

No auto-normalization of nil/negative/zero versions. Exact fresh-state and supported legacy baseline oracle must be settled before Gate C. Existing manifest order is preserved; no arbitrary caller target. Execute reviewed immutable full SQL file as one batch with explicit transaction; pinned pgx argument-free Exec uses simple protocol. No naive delimiter splitting. Parameterize metadata operations, use fixed identifiers. A lexical guard is not an SQL sandbox. Check expected next fingerprint and prefix before COMMIT, then verify settled outcome. On known rollback previous clean prefix remains; no native dirty marker. Legacy Platform and Ledger tests explicitly require dirty after failed body: preserve legacy characterization while adding distinct native tests, not weakening checks.

Outcome model defined in proposed contract. Record cause, requested/attempted/verified versions, phase, verification/cleanup status and correlation ID. Success only exact verified target plus settled cleanup; prior prefix is not success. Ambiguous COMMIT remains UNKNOWN until exclusive settled observation; no blind replay.

## C3 — Ownership, budgets and restart safety

One dedicated session per run, fixed existing target advisory lock key, immediate refusal on contention, no shared application pool or implicit reconnect-to-resume. Verify trusted database/environment and effective schema owner boundary before writes; locks are cooperative, not ACLs. Hold lock through preflight, transactions and final verification; bounded confirmed unlock/close. If unsettled, stop and quarantine. Independent bounded read-only observation may classify outcome only after prior execution settled, trusted target verified and exclusive ownership re-established. No repair from observer.

Initial research budget hypotheses (not configuration): work max 180s or earlier caller deadline; connect 5s; immediate target lock refusal; per version 45s; server statement 40s, lock 5s, transaction 45s, idle-in-transaction 10s; combined extra cleanup/observation 10s. Check remaining budget before starting new version. No global setting or CI timeout change; commit cancellation can be ambiguous. Verify actual driver/server lifecycle before claiming wall-clock limits; no production SLO. Validate durability and expected primary assumptions, no failover guarantee.

Immutable bounded manifest: read once, validate canonical bytes/name/version/digest/trailer, execute that snapshot; no untrusted SQL interface. Size budget, effective-role and fresh oracle still open. Logs redacted; no DSN/raw SQL/secret. Operational quarantine must survive crash/restart and block subsequent jobs until explicit reviewed reconciliation. Trusted operator/orchestrator mechanism remains unresolved, not a new metadata registry approval.

## C4 — Architecture and DEC-01

[ADR-0042](../adr/0042-context-aware-migration-executor.md) is proposed and not effective. Retain current runner and accepted decisions until separately authorized implementation, tested compatibility, and explicit cutover. No dependency removal or engine flag introduced. Preserve legacy dirty refusal; separately accept changed native rollback semantics.

## C5 — Controlled cutover

1. Reconcile exact main, corpus, pinned engine, concurrent Identity/recovery PRs. Accept supported upgrade fixtures and G4 path/containment plan.
2. Compare legacy vs native in separate owned disposable environments with equivalent independently provisioned roles/data. Fresh all targets; clean legacy latest -> native no-op; supported prior prefix -> native remaining versions. Identity v11->12 and v26->27 with populated fixtures. Compare schema, data, grants, ownership, functions, constraints, historical fingerprints/timestamps and effective role behavior. No fake committed Ledger upgrade.
3. Pass actual fault/recovery/regression criteria; retain default legacy engine. No dual writer or automatic fallback.
4. Separately approve synthetic cutover. Quiesce old admin invocations/jobs, verify settled backend and trusted target, compatible clean history, backup and recovery evidence. Same outer lock does not prove the legacy inner session stopped. If uncertain, stop.
5. Verify new exact history and outcome. Binary rollback is not database rollback; executable rollback requires separate compatible clean-state decision, no version rewind or digest rewriting. No production cutover authorized.

## Actual fault-test acceptance

All AC105-01–11 are NOT RUN for proposed native engine. Actual fault experiments require separate implementation and execution approval. Record exact SHA/engine, independently observed phase, target identity, timing, termination cause and durable effect, prior/after metadata/schema/data/ACL oracle, settled sessions and locks, no unexpected next version, sanitized evidence. No arbitrary sleep as sole phase evidence; mocks cannot substitute actual fault evidence.

| ID | Required evidence |
|---|---|
| AC105-01 | Pinned legacy graceful-stop/partial target and failed-body dirty behavior; preserve existing characterization; actual legacy split-window evidence. |
| AC105-02 | Pre-cancel, wrong role/target, inconsistent metadata and actual lock contention refused before mutation; positive owner control. |
| AC105-03 | Independently observed in-flight cancellation, settled rollback retains previous prefix where applicable; no next version after observed cancellation. Do not assert rollback if commit already occurred. |
| AC105-04 | Actual statement, lock, transaction and cumulative overall budgets; bounded cleanup/observation and durable result. No production SLO. |
| AC105-05 | Actual controlled process interruption during transaction, before commit and after commit, independent settled prefix/new-version evidence; distinguish legacy split windows. |
| AC105-06 | Independently established committed and noncommitted outcomes with ambiguous response; UNKNOWN until settled verified observation, no replay, restart-safe quarantine gate. |
| AC105-07 | Legacy dirty before/after body, conflicting/missing history, lost metadata with business objects, unexpected metadata shape/owner, lost session and changed manifest source; preserve state, no repair. |
| AC105-08 | All 36 static file checks; actual old/new fresh and supported prefix equivalence, I12/I27 populated backfills, exact historical bytes/ACL/function/data/time semantics; real Ledger upgrade remains NOT RUN until meaningful next release. |
| AC105-09 | Positively identified disposable target and roles, cross-owner denial, redaction, settled cleanup and no shared/live database. |
| AC105-10 | Independent clean-prefix and legacy-dirty restore, ownership/data/ACL/history parity, dirty refusal; coordinate #14, not financial restore. |
| AC105-11 | Retain existing legacy characterization; independent native cases, additive reviewed mandatory registration, existing checks and exact candidate evidence. No weakening or unauthorized CI changes. |

## Open implementation gates

Before Gate C: explicit legacy/native failure contract, exact metadata/fresh oracle, bounded budgets, durable quarantine mechanism scope, supported upgrade inventory, exact allowlist, isolated old/new and fault test feasibility, additive mandatory registration and separate owner approval. Current scope does not authorize coding, database execution, migrations, dependencies, runtime or merge. #105/#14/#18/#86 remain open; K2a limitations unchanged; K2b/K3 not authorized.

## Sources

Pinned sources reviewed: [golang-migrate v4.20.1 driver](https://github.com/golang-migrate/migrate/blob/v4.20.1/database/pgx/v5/pgx.go), [orchestration](https://github.com/golang-migrate/migrate/blob/v4.20.1/migrate.go), [pgx v5.13.0 connection](https://github.com/jackc/pgx/blob/v5.13.0/conn.go), [transaction](https://github.com/jackc/pgx/blob/v5.13.0/tx.go). PostgreSQL 18 reference material: [protocol flow](https://www.postgresql.org/docs/18/protocol-flow.html), [locking](https://www.postgresql.org/docs/18/explicit-locking.html), [client settings](https://www.postgresql.org/docs/18/runtime-config-client.html), [date/time functions](https://www.postgresql.org/docs/18/functions-datetime.html). Research checked 2026-11-25; no actual native test execution.
