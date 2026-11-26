# Foundation #105 — Proposed native pgx migration executor

**Status:** C1–C6 (native executor design), DA-A (durable admission option A), IP-D1–D4 (identity and provisioning decisions), and EB-D1–D4 (execution budgets and evidence decisions) owner-accepted detailed design candidates; documentation-only consolidation, not implementation approval. [Issue #105](https://github.com/herdifirdausss/mamony-go/issues/105) under [#14](https://github.com/herdifirdausss/mamony-go/issues/14). [Research decision](https://github.com/herdifirdausss/mamony-go/issues/105#issuecomment-7152752304), [detailed review](https://github.com/herdifirdausss/mamony-go/issues/105#issuecomment-7152858127), [ADR-0042](../adr/0042-context-aware-migration-executor.md), [proposed contract](../contracts/postgresql.md#proposed-native-migration-contract-not-effective). Existing DEC-01 and runtime unchanged. Independent technical acceptance DEFERRED; production readiness NOT ACCEPTED.

## C1 — Compatibility inventory

Initial 36-file audit baseline f6b9a2d1f46413e7c87db0cc09d13acc4261c438. Historical 37-file reviewed corpus baseline 76d417d35571b28b7aa748577861fc3a1f9e6dcb (not a claim about the current main HEAD). After #81 merged, the 36 historical blobs remained identical and Identity v35 was added (blob 49441878e45cd251a7a2a96d4f42388cb2ad89d6; recomputed body SHA-256 329725f31728911adddb53780e5827e761ed713a003a2cbbd9385ba3fc845157 matches trailer). Static screening found no obvious nontransactional top-level operation in v35. Recorded static integrity coverage at the reviewed corpus baseline: 37/37. Actual native equivalence NOT RUN. All 36 committed files were fetched at the initial audit revision; all returned blob IDs matched the complete tree, and all recomputed SHA-256 body hashes matched exact target/version trailers. Static lexical screening is not PostgreSQL parsing or function-body semantic proof. No obvious top-level nontransactional operation, explicit transaction control, sequence mutation or external database operation detected. No execution or old/new parity evidence. Inventory (T tables/constraints; X ordinary index; F function; G grant/revoke; Tr trigger; B backfill; excludes trailer):

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
| Identity 35 | T/F/Tr/G |

Identity v12 and v27 require populated historical backfill fixtures. Function/trigger/privilege behavior must be compared, including SECURITY DEFINER functions. Preserve exact historical SQL bytes and previously stored digests/timestamps. Newly generated transaction timestamps may differ; verify time-derived semantics with bounded oracles, not blanket omission. Existing v1 Platform/Ledger are fresh/no-op baselines; test-only upgrade fixture is not supported deployed history. The bounded proposed supported inventory is H1–H7 below, not every historical prefix. #81 merged; #95 and #106 remain open at the current baseline. Reconcile changed main and any new migration before implementation.

## C2 — Metadata and outcome details

Retain legacy schema_migrations (BIGINT version PK NOT NULL, BOOLEAN dirty NOT NULL; verify singleton) and per-schema migration_fingerprints (existing version, sha256, recorded_at). New metadata table creation, if absent on positively verified fresh baseline, is transactional with first version. Preflight classification:

| State | Required handling |
|---|---|
| Fresh | Both metadata tables absent, approved provisioning provenance and positively verified expected fresh schema/object/privilege inventory and target identity; allow first version |
| Legacy empty | Compatible empty version metadata, no fingerprint/business history and the same positively established fresh provenance/inventory; allow |
| Clean prefix | One positive clean manifest version, exact hash prefix, expected metadata shape/owner/grants; allow next version |
| Exact target | Verified no-op |
| Legacy dirty | Refuse, regardless of fingerprint |
| Inconsistent | Refuse missing/extra/conflicting history, unknown version, multiple rows, unexpected shape/owner or business objects without history |

No auto-normalization of nil/negative/zero version records. The logical empty prefix is a distinct verified state, not a persisted version 0. The fresh-state oracle must use independently approved provisioning evidence tied to the expected environment, database identity and current incarnation (including restore provenance), and an expected inventory of schema objects including non-table objects, owners, effective privileges and metadata presence. Compare the observed inventory to the approved bootstrap; refuse unexpected or missing objects, unexplained absent history, unknown provenance or identity mismatch. An empty catalog alone does not prove no historical data. The proposed external identity and positive fresh-state oracle are defined below; exact fixture inventory, catalog visibility and approved legacy baseline evidence remain blocking before applicable implementation approval. Existing manifest order is preserved; no arbitrary caller target. Execute reviewed immutable full SQL file as one batch with explicit transaction; pinned pgx argument-free Exec uses simple protocol. No naive delimiter splitting. Parameterize metadata operations, use fixed identifiers. A lexical guard is not an SQL sandbox. Check expected next fingerprint and prefix before COMMIT, then verify settled outcome. On known rollback previous clean prefix remains; no native dirty marker. Legacy Platform and Ledger tests explicitly require dirty after failed body: preserve legacy characterization while adding distinct native tests, not weakening checks.

Outcome model defined in proposed contract. Record termination cause, requested target, last independently verified durable prefix, attempted version (or none), attempted-version effect (not started, confirmed rolled back, confirmed committed, or unknown), phase, verification/cleanup status and correlation ID as separate facts. A verified prefix may be empty after a confirmed rollback of the first version, or may include a newly committed version before a subsequent cancellation. STOPPED_AT_VERIFIED_PREFIX requires a settled verified prefix short of target and no unresolved execution/cleanup; it does not imply the last attempted version rolled back. A confirmed target commit followed by failed postflight or unsettled cleanup cannot return success; preserve confirmed commit/prefix evidence even when the overall result is OUTCOME_UNKNOWN. Success only exact verified target plus settled cleanup and durable terminal resolution; prior prefix is not success. Ambiguous COMMIT remains UNKNOWN until exclusive settled observation; no blind replay.

## C3 — Ownership, budgets and restart safety

One dedicated session per run, fixed existing target advisory lock key, immediate refusal on contention, no shared application pool or implicit reconnect-to-resume. Verify trusted database/environment and effective schema owner boundary before writes; locks are cooperative, not ACLs. Hold lock through preflight, transactions and final verification; bounded confirmed unlock/close. If unsettled, stop and quarantine. Independent bounded read-only observation may classify outcome only after prior execution settled, trusted target verified and exclusive ownership re-established. No repair from observer.

The consolidated EB-D1 budget below supersedes the initial hypotheses. It is a proposed permission-to-start-work and observation contract, not a hard wall-clock exit guarantee. No global setting or CI timeout change.

Immutable bounded manifest: read once, validate canonical bytes/name/version/digest/trailer, execute that snapshot; no untrusted SQL interface. Exact size budget, effective-role evidence and fixture-specific fresh inventory remain open. Logs redacted; no DSN/raw SQL/secret. Operational quarantine must survive crash/restart and block subsequent jobs until explicit reviewed reconciliation. Before any database mutation (including metadata bootstrap), the trusted admission authority must exclusively admit a run for the positively identified target/incarnation and durably record its unresolved execution intent. A crash before outcome reporting leaves the intent unresolved; a new process must refuse admission while any such intent remains, or if the gate/store is unavailable or untrusted. Do not clear intent on mere process exit, timeout, or connection loss. Only explicit authorized reconciliation with settled database and lock evidence may resolve it; a successful run also requires durable resolution after settled verification/cleanup. The ordering and enforcement must prevent overlapping attempts and stale or restored identity from bypassing the gate. DA-A is the accepted bounded design candidate; its authority, persistent journal and global gate are specified below. Actual credential enforcement, storage format, durability and atomic publication feasibility remain open, not a new metadata registry approval.

## C4 — Architecture and DEC-01

[ADR-0042](../adr/0042-context-aware-migration-executor.md) is proposed and not effective. Retain current runner and accepted decisions until separately authorized implementation, tested compatibility, and explicit cutover. No dependency removal or engine flag introduced. Preserve legacy dirty refusal; separately accept changed native rollback semantics.

## C5 — Controlled cutover

1. Reconcile exact main, corpus, pinned engine, concurrent Identity/recovery PRs. Accept supported upgrade fixtures and G4 path/containment plan.
2. Compare legacy vs native in separate owned disposable environments with equivalent independently provisioned roles/data. Fresh all targets; clean legacy latest -> native no-op; supported prior prefix -> native remaining versions. Identity v11→12, v26→27 and v34→35 with populated fixtures under H1–H7. Compare schema, data, grants, ownership, functions, constraints, historical fingerprints/timestamps and effective role behavior. No fake committed Ledger upgrade.
3. Pass actual fault/recovery/regression criteria; retain default legacy engine. No dual writer or automatic fallback.
4. Separately approve synthetic cutover. Quiesce old admin invocations/jobs, verify settled backend and trusted target, compatible clean history, backup and recovery evidence. Same outer lock does not prove the legacy inner session stopped. If uncertain, stop.
5. Verify new exact history and outcome. Binary rollback is not database rollback; executable rollback requires separate compatible clean-state decision, no version rewind or digest rewriting. No production cutover authorized.

## Actual fault-test acceptance

All AC105-01–11 are NOT RUN for proposed native engine. Actual fault experiments require separate implementation and execution approval. Record exact SHA/engine, independently observed phase, target identity, timing, termination cause and durable effect, prior/after metadata/schema/data/ACL oracle, settled sessions and locks, no unexpected next version, sanitized evidence. No arbitrary sleep as sole phase evidence; mocks cannot substitute actual fault evidence.

| ID | Required evidence |
|---|---|
| AC105-01 | Pinned legacy graceful-stop/partial target and failed-body dirty behavior; preserve existing characterization; actual legacy split-window evidence. |
| AC105-02 | Pre-cancel, wrong role/target, inconsistent metadata and actual lock contention refused before mutation; positive owner control. |
| AC105-03 | Independently observed in-flight cancellation, settled rollback retains previous prefix including verified empty first-version case; cancellation after a committed intermediate version retains that version and starts no next version. Do not assert rollback if commit already occurred. |
| AC105-04 | Actual statement, lock, transaction and cumulative overall budgets; bounded cleanup/observation and durable result. No production SLO. |
| AC105-05 | Actual controlled process interruption during transaction, before commit and after commit (including before outcome reporting); independent settled empty/prefix/new-version evidence; distinguish legacy split windows. |
| AC105-06 | Independently established committed and noncommitted outcomes with ambiguous response; exact-target commit followed by failed postflight or unsettled cleanup is not success and retains known durable facts. Prove durable intent precedes mutation, crash before reporting preserves unresolved intent, new invocation refuses until authorized settled reconciliation, and unavailable/untrusted admission fails closed. UNKNOWN until settled verified observation, no replay. |
| AC105-07 | Legacy dirty before/after body, conflicting/missing history, lost metadata with business objects, unexpected metadata shape/owner, lost session and changed manifest source; verified fresh and legacy-empty positive controls versus unexpected non-table objects, missing expected objects, absent provenance or identity mismatch; preserve state, no repair. |
| AC105-08 | All 37 static file checks at the reviewed baseline; actual old/new fresh and supported prefix equivalence, Identity v12/v27 populated backfills and v34→35 preservation, exact historical bytes/ACL/function/data/time semantics; real Platform/Ledger upgrade remains NOT RUN until meaningful next release. |
| AC105-09 | Positively identified disposable target/incarnation, approved bootstrap object inventory and effective roles, reject wrong or unproven identity/provenance, cross-owner denial, redaction, settled cleanup and no shared/live database. |
| AC105-10 | Independent clean-prefix and legacy-dirty restore, ownership/data/ACL/history parity, dirty refusal; restore/incarnation provenance must be explicitly reconciled with trusted admission state, not treated as fresh or allowed to bypass unresolved intent. Coordinate #14, not financial restore. |
| AC105-11 | Retain existing legacy characterization; independent native cases, additive reviewed mandatory registration, existing checks and exact candidate evidence. No weakening or unauthorized CI changes. |

## Consolidated admission, identity and execution design

Owner-approved design directions (not implementation or effective contract approval): [DA-A research](https://github.com/herdifirdausss/mamony-go/issues/105#issuecomment-7194850544), [identity and restore design](https://github.com/herdifirdausss/mamony-go/issues/105#issuecomment-7194979796), [execution and compatibility research](https://github.com/herdifirdausss/mamony-go/issues/105#issuecomment-7195098899). All new native, DA, IP and EB acceptance cases remain NOT RUN. D identifies a design decision, R a requirement, and AC an acceptance criterion; M105 identifies the proposed parent contract for issue #105, and AC105 its native acceptance cases. H1–H7 identify historical compatibility scenarios. These are local traceability identifiers, not implementation stages or effective contracts.

### Durable admission — DA-A

The scope is one positively owned Linux administrative host, one preprovisioned persistent local control directory outside the target DB backup/restore set, and one trusted controller/credential path. DA-D1–D7: select the bounded local journal; separate it from target recovery; preserve stable logical slots across incarnations; establish exclusive durable intent before mutation; no expiry of unresolved intent; explicit settled reconciliation without replay; stop and reevaluate DA-C when assumptions fail. A registry in the target cluster can be rewound by physical recovery. An independent control store (DA-C) is a future option for multi-host or insufficient continuity, not selected infrastructure. An orchestrator may wrap the contract but scheduling/job status alone is not durable admission. No extra database table, network service or dependency is authorized.

Use one global administrative gate (IP-D3), not simultaneous per-capability admission. Keep the existing fixed target DB advisory locks. Order: global gate → target lock → version transaction. All managed migration, restore and reconciliation mutations in the lab pass through this sole authority; no raw legacy or alternative execution path in the experiment. Different eligible slots may progress sequentially; no required parallelism. Global serialization is distinct from slot-specific unresolved intent and the restore fence that covers every affected slot. Unavailable or untrusted global authority blocks all admission. The owner selected Option A — global quarantine — on 2026-11-26 for the bounded synthetic design (R112-2). Any unresolved intent or restore fence in the managed scope blocks new migration admission for every slot, including unrelated slots and H2 no-op requests. The active authorized attempt may complete normally while authority and work budget remain valid; uncertainty stops new mutation. A restore fence still records its actual affected slots and preserves their history; its admission effect is global. Only separately authorized reconciliation of the blocking state may proceed under the global gate; no unrelated migration or restore work while blocked. A separately approved restore that forms part of reconciliation must retain the existing fence and evidence requirements. New admission requires every blocking state durably resolved, verified continuity and authority, and fresh authorization. No automatic replay, TTL, takeover or partial clearance. Global quarantine applies to administrative migration admission only, not runtime shutdown, and does not alter the current engine. Scoped quarantine is not selected; any future change requires separate design and isolation evidence. No nested target locks, automatic takeover, timeout-based clearing or lock deletion/replacement. Locks are cooperative, not security authorization. Runtime lacks migration/journal authority. Trusted host/controller/root/migrator are assumptions, not protection against their compromise. Existing default engine outside the isolated experiment is unchanged.

Logical journal record: format/version, independently provisioned journal identity and trusted control storage binding, stable logical DB/slot, approved incarnation/receipt reference, revision within trusted history, attempt ID, release/manifest digest, requested version, starting verified prefix, known effect/cause/cleanup and resolution evidence/authorization. Retain attempt evidence and never delete unresolved intent. No raw SQL, DSN, credentials or sensitive business data. Missing/corrupt/untrusted journal or lock is BLOCKED; do not initialize on runner startup. Exact encoding, capacity, retention, ownership and protected credential path remain open. If bounded capacity is exhausted, refuse; no automatic pruning.

Admission state: READY means trusted continuity, verified authority and no unresolved attempt or restore fence in any managed slot; OPEN_INTENT is durably recorded before any mutation (including metadata bootstrap); QUARANTINED is global refusal of new migration admission derived from unresolved intent, restore fence or uncertain authority/state (no post-crash write needed); RESOLVED retains terminal facts and does not itself authorize a new attempt. No TTL or retry. Under the global gate, validate receipt, manifest and journal, and obtain a read-only initial identity/role/prefix observation against the independent approved inventory. Record the observed verified starting prefix and binding in durable intent; obtain the target lock and revalidate the same identity, role and prefix against the recorded facts before any target mutation. On mismatch, refuse execution; finalize only with proved no mutation, settled resources and verified authority, otherwise retain unresolved intent. Check continued authority and work budget before each new version. Execute immutable versions, verify exact outcome and settled backend/lock/cleanup, durably publish terminal resolution, then release the gate. H2 clean adoption/no-op follows this admission and resolution sequence without target database mutation, including no metadata rewrite. A known no-mutation or settled stopped prefix may be finalized by the authorized normal path; unknown/dirty/uncertain authority requires explicit owner-reviewed reconciliation. If intent persistence is uncertain, do not mutate; if terminal persistence is uncertain, do not report success. A released host/DB lock or process exit alone is not proof of backend settlement. Read-only observer only after verified quiescence and exclusive authority; no automatic replay, repair or new mutation.

### Provisioning and restore — IP-D1–D4

Stable environment + logical database + fixed capability is the admission slot, independent of endpoint/incarnation. A trusted lab provisioner issues an opaque incarnation and approved receipt outside DB recovery before first admission: environment/logical DB, incarnation and parent/restore reference, owned host/daemon/container/storage, endpoint binding, expected database/session role/schema, image/version/encoding, bootstrap recipe and expected inventory digests, provenance FRESH / RESTORED / ADOPTED_CLEAN, admitted targets, authorization and journal binding. Collect actual owned resource/session/catalog observations independently and compare to the expected receipt; do not derive expected state from observed data. Database name, OID, cluster system identifier, timeline, endpoint or copied UUID alone do not prove currentness. Ordinary worker restart retains incarnation only with verified continuity; restore/replacement/clone/reinitialization requires explicitly approved new incarnation. Verify original and effective role, memberships, owner/grants, expected writable primary and connection binding on the locked session. No privilege expansion for observation. Unknown identity refuses.

FRESH requires positive approved provisioning history, no other writer/restore/adoption, trusted journal continuity showing never-applied or separately approved empty baseline, exact independently specified expected inventory and absent metadata. LEGACY_EMPTY requires the same provenance plus approved compatible zero-row version table and absent fingerprint history. A persisted negative/zero record is not a logical empty prefix. Verify relations (including views/indexes/sequences), routines, types/domains, triggers/policies, dependency/extension objects, roles/memberships, ownership, schema/database/object/default privileges and relevant configuration; account for expected system objects and explicitly allowed other capability objects. Unexpected/missing objects, incomplete inspection authority or missing metadata on previously applied/restored/adopted target refuse. Empty replacement or restored empty target is not fresh by appearance. Exact fixture catalog/role matrix and fresh oracle remain open.

Preprovision protected journal/control identity, stable lock and storage binding independently of DB. No auto-init, lock inode replacement or unknown path. A checksum/revision verifies integrity or ordering within trusted observed history, not currentness against indistinguishable silent rollback. Known stale, missing, corrupt or replaced storage/host, or unproven continuity, blocks. DA-A excludes undetected valid journal rollback, arbitrary administrator compromise, host loss/HA and actual power-loss assurance. If continuity or sole credential authority cannot be proven, STOP and reevaluate DA-C.

Restore is separately authorized. Under the global gate durably record RESTORE_OPEN with stable DB/cluster scope, affected slots, prior incarnation, outstanding intents and approved recovery reference before restore/replacement mutation; physical cluster recovery fences every affected managed slot. Preserve all old attempt records. Prove old workers/backends settled and old execution path cannot mutate the admitted target. If original target is unreachable, no replacement admission without trusted evidence and explicit disposition. Verify independently approved backup/manifest/lineage/recovery point and actual restored schema, fingerprints, data, ACL and relevant domain evidence. Existing RH-01/02 or RH-03 mechanisms do not prove this admission fence. Publish approved new incarnation binding to the SAME slots and fence resolution atomically and durably only after verified reconciliation. A crash or uncertain publication blocks all affected slots. Restoring an earlier clean prefix does not establish previous commit/rollback. Preserve unknown historical facts; irrecoverable historical uncertainty needs a separate explicit owner recovery disposition, not authorized here. No automatic repair, rebind or replay; new execution needs new admission.

### Execution budgets and persistence — EB-D1 and EB-D2

Use one monotonic normal work budget of 180s or earlier caller deadline from entry, including manifest, admission, connection, preflight, versions and final verification. Phase ceilings are clipped to remaining budget, not additive; do not reset per version.

| Phase or guard | Candidate ceiling |
|---|---|
| Manifest, identity and admission validation | 15s including intent publication target ≤5s |
| Authorized connection | 5s |
| Global and target lock contention | Immediate refusal |
| Locked preflight | 15s |
| Per-version transaction | 45s including verification and commit |
| Server statement / lock / transaction / idle-in-transaction | 40s / 5s / 45s / 10s |
| Final verification | 10s reserved before next version |
| Additional cleanup, observation and terminal publication | Combined 10s target (independent cleanup context; no new migration work) |

Use one independent bounded cleanup context for the combined additional 10s target; it permits cleanup, settled observation and terminal publication only. Conservatively require remaining normal budget ≥55s before starting another version, plus applicable admission, identity and cancellation checks. A shorter test-only budget may be approved for fault fixtures. Restore/reconciliation need separate finite approved budgets; neither can expire a fence. Verify expected writable primary, fsync, synchronous_commit for migration transaction and full_page_writes enabled; no global settings change or failover claim. Statement/lock/transaction limits have different scopes; timeout is not proof of rollback. Preserve known committed facts even if cleanup/reporting fails.

Ordinary file write/sync and kernel/storage stalls are not generally context-cancellable. Do not promise a 190s hard return or kill completion. A proposed 200s controller reporting/escalation threshold (180+10+10) is not a DB/OS termination guarantee or CI timeout change. Stop new mutations after observed expiry/loss of authority, do not detach a writer and report completion, retain unresolved intent and verify actual settlement. If cleanup or observation cannot establish outcome, UNKNOWN, no replay.

Use a bounded versioned control record and retained attempt evidence under the stable gate. Candidate durability protocol: complete same-filesystem replacement record, file synchronization, atomic publication, containing-directory synchronization and verified result before acknowledgment. Retain prior evidence before resolving; no automatic pruning. Atomic visibility differs from crash durability. A visible terminal record after uncertain final sync is not proof that prior execution acknowledged durable resolution. Reconcile interrupted/uncertain runs under the trusted controller before new authorization. Actual filesystem/mount semantics, descriptor lifetime, storage continuity and persistence failure tests remain open. No NFS/shared volume or DB-owned/ephemeral control storage for a continued target. A hosted CI job tests child restarts on one owned host; job/host loss abandons that disposable fixture, never continues it from artifacts. No claim of cross-job/host currentness, silent rollback detection or power-loss survival.

### Supported synthetic historical upgrade inventory — EB-D3

| ID | Required compatibility boundary |
|---|---|
| H1 | Fresh Platform v1, Identity v1–35, Ledger v1, legacy/native comparison with approved independent bootstrap and full relevant catalog/data/ACL oracle |
| H2 | Clean legacy latest adoption/no-op for all three targets; preserve existing hashes/timestamps and data, verified outcome and durable resolution |
| H3 | Populated Identity v11→12: last_seen_at equals issued_at, no artificial activity or expiry revival, existing role/function behavior; separately authorized remainder to v35 |
| H4 | Populated Identity v26→27: ACCEPTED→SUBMITTED, other notices UNCONFIRMED, no implied delivery, preserve provider/event history; separately authorized remainder to v35 |
| H5 | Populated Identity v34→35: preserve ordinary data and approved pending credential/challenge/registration history, no possession or activation notice effect from migration alone; role/constraint/function parity and repeat clean no-op |
| H6 | Test-only next Platform/Ledger version for failure machinery, with separately approved fixture provenance and authorization; NOT actual committed release upgrade. The source decision calls this fixture signed; exact signing/verification authority and fixture representation remain open, and no new signing system is authorized |
| H7 | Legacy dirty/inconsistent refusal and independent clean/dirty restore parity, no repair |

Historical bytes immutable. Verify deterministic data exactly and time-derived fields against bounded semantic oracles. Existing mandatory v34→35 legacy test passed on [main PostgreSQL CI](https://github.com/herdifirdausss/mamony-go/actions/runs/45839562241/job/139977339670), not native equivalence or all pending-credential fixtures. Identity capability v36 corresponds to Identity SQL migration v35; the capability and migration versions are distinct identifiers. Other starting prefixes are not included in this bounded supported claim. Real Platform/Ledger version-to-version upgrade remains NOT RUN until meaningful next release.

### Isolated fault infrastructure — EB-D4

Current ubuntu-24.04 PostgreSQL job cap 15m; three existing fixed testgate commands each have 3m Go test timeout. One observed main run 137s; package durations 37.029s, 0.469s, 12.505s. Existing database crash/restart and restore tests are scoped legacy evidence. Prefer one additive fixed mandatory native fault test group/package with separate 3m cap, initial repeated measured target ≤120s including setup/cleanup. Keep existing commands, assertions, named tests, timeouts and workflow unchanged. New group, exact names/subtests/parser allowlist and registration require separate G4 approval. No implicit timeout increase, skipped cases or weaker gate. If duration or containment cannot be demonstrated, stop for revised scope.

Trusted controller owns isolated disposable DBs and separate control directory; positive resource/image/network/storage/host receipt, generated synthetic credentials, no caller DSN or shared service faults. Independent observer, deterministic phase barriers and durable-state oracle. Test actual child interruption, cancellation, settlement, independent restore and only safely evidenced filesystem errors. Mock/injected errors are logic tests, not power-loss or actual sync failure proof. Both committed and noncommitted ambiguous response cases need a separately reviewed deterministic safe observation mechanism; this is unresolved and blocking. No new privileged fault injector, host power-off, external service or dependency authorized. Record exact candidate, phase, time, journal/receipt/intent/fence, durable DB outcome, catalog/data/ACL, settled resources and explicit PASS/FAIL/NOT RUN. Existing CI artifacts are evidence, not authority.

### Supplemental requirement traceability

These requirements refine M105-R1–R8, not new active contracts. The owner accepted DA-R1–8, IP-R1–7 and EB-R1–6 as candidate requirements.

| Requirement | Required boundary | Parent contract | Native acceptance | Supplemental acceptance |
|---|---|---|---|---|
| DA-R1 | Stable slot, approved incarnation, journal identity and affected-slot restore fencing | M105-R1/R5/R6 | AC105-07/09/10 | DA-AC01/07; IP-AC01/02/06–08 |
| DA-R2 | Sole trusted administrative and credential path | M105-R1/R6/R8 | AC105-02/09/11 | DA-AC02/08/09; IP-AC04/05/10 |
| DA-R3 | Exclusive durable immutable pre-mutation intent; recorded starting facts | M105-R3/R5/R6 | AC105-02/06/07 | DA-AC02/03/05; IP-AC02/05 |
| DA-R4 | Non-expiring intent, continued authority before each version, global quarantine and restart refusal, no replay | M105-R2/R3/R6 | AC105-03/05/06 | DA-AC02/03/06/09; IP-AC04/05; EB-AC01/06 |
| DA-R5 | Separate cause/effect, verified prefix, settled cleanup and durable resolution | M105-R3/R6 | AC105-03/04/05/06 | DA-AC04/05/06; EB-AC01–03/06 |
| DA-R6 | Controlled restore, preserved attempt facts and authorized reconciliation | M105-R5/R6/R7 | AC105-06/09/10 | DA-AC06/07/08; IP-AC06–08; EB-AC07 |
| DA-R7 | Bounded minimal protected storage and redacted evidence | M105-R1/R6/R8 | AC105-06/09/11 | DA-AC05/08/09; EB-AC03 |
| DA-R8 | Actual isolated evidence and retained regression gate | M105-R8 | AC105-05/06/09/11 | DA-AC09/10; IP-AC10; EB-AC06/08 |
| IP-R1 | Stable logical slot and external incarnation binding | M105-R1/R5/R6 | AC105-07/09/10 | DA-AC01/07; IP-AC01/06–08 |
| IP-R2 | Independent owned resource/session/role evidence and immutable receipt | M105-R1/R5 | AC105-02/07/09 | IP-AC01/02/09 |
| IP-R3 | Positive fresh/legacy-empty provenance and approved inventory | M105-R5/R7 | AC105-07/08/09 | IP-AC02/03/09; EB-AC04 |
| IP-R4 | Journal continuity and sole host/credential path; no auto-init | M105-R1/R6 | AC105-02/06/09 | DA-AC01/08/09; IP-AC04/05/10 |
| IP-R5 | Global serialization, durable affected-slot restore fence and global admission refusal while unresolved | M105-R5/R6 | AC105-02/05/06/10 | DA-AC02/06/07; IP-AC04/06/07; EB-AC07 |
| IP-R6 | Explicit verified rebind preserving old outcome facts | M105-R3/R5/R6/R7 | AC105-06/09/10 | IP-AC06–08; EB-AC07 |
| IP-R7 | Actual disposable evidence, no new registry/dependency or production claim | M105-R1/R8 | AC105-09/11 | IP-AC09/10; DA-AC10; EB-AC08 |
| EB-R1 | Shared monotonic budget and phase ceilings | M105-R2/R3 | AC105-02/03/04 | EB-AC01/02 |
| EB-R2 | Durable publication and unknown handling; no hard OS/storage deadline claim | M105-R3/R6 | AC105-04/05/06 | DA-AC05/08; EB-AC02/03/06 |
| EB-R3 | Bounded historical compatibility inventory and immutable bytes | M105-R4/R7 | AC105-01/07/08/10 | EB-AC04/05/07; IP-AC09 |
| EB-R4 | Actual isolated fault harness, containment and deterministic evidence | M105-R6/R8 | AC105-05/06/09/10 | DA-AC03/04/09; IP-AC06/07/10; EB-AC06/07 |
| EB-R5 | Additive mandatory CI gate and unchanged legacy guarantees | M105-R8 | AC105-01/11 | DA-AC10; EB-AC08 |
| EB-R6 | Explicit nonclaims and fail closed on unresolved storage, identity or fault feasibility | M105-R3/R5/R6/R8 | AC105-04/06/09/11 | DA-AC05/08/09; IP-AC05/10; EB-AC03/06/08 |

Mappings are documentation traceability, not evidence of execution. Slash-separated numbers inherit the identifier prefix in the same cell. All parent and supplemental acceptance cases retain their original required conditions. Option A refines DA-R4 and IP-R5 under M105-R6; DA-AC02/06/07, IP-AC04/07 and EB-AC07 must demonstrate global admission refusal and authorized recovery. Sequential eligible-slot progress is required only after all blocking state is durably resolved; unrelated slots may not bypass quarantine.

### Supplemental acceptance criteria

All cases below are NOT RUN for the proposed native candidate; they supplement AC105-01–11. Actual fault execution needs separate approval. Preserve independently observed phase, before/after durable state and containment evidence; a skipped case or parent PASS cannot substitute its required evidence.

| ID | Required evidence |
|---|---|
| DA-AC01 | Positive provisioned journal/target; missing, corrupt, wrong identity, unapproved host or unknown continuity refuses before mutation |
| DA-AC02 | Independent concurrent contenders; one durable admission, sequential eligible distinct-slot progress under IP global gate; alternate invocation refused. Verify same-slot and unrelated-slot requests (including H2 no-op) refuse while any unresolved intent or restore fence remains; prove sequential progress after all blocking state is durably resolved and fresh admission is authorized |
| DA-AC03 | Actual interruption before/after durable intent, during execution and before reporting; no mutation without intent, restart refusal |
| DA-AC04 | Committed, rolled-back and ambiguous effects distinguished; lock release/death cannot clear intent |
| DA-AC05 | Safely evidenced persistence/permission/space/unavailable/terminal publication failures; no admission/success on uncertainty |
| DA-AC06 | Restart, stale or duplicate attempt and unauthorized resolution refusal; global refusal persists across restart; explicit settled resolution of all blocking state and fresh admission, no replay |
| DA-AC07 | Target restore retains external unresolved intent and fences all affected slots; unrelated migration admission also refuses while any fence remains; no automatic incarnation rebind |
| DA-AC08 | Known stale/lost/replaced journal or host blocked, no automatic reinit; no indistinguishable rollback claim |
| DA-AC09 | Bounded admission and cleanup, redacted evidence, authority loss refuses, no orphan writer |
| DA-AC10 | Exact-head containment, invariants and existing regression evidence |
| IP-AC01 | Owned target positive control; wrong endpoint/container/storage/role, recovery/read-only or unknown provenance refuses |
| IP-AC02 | Fresh and approved legacy-empty positive controls; no metadata mutation before intent. Verify initial observed facts recorded in intent and locked revalidation; mismatched facts refuse before target mutation |
| IP-AC03 | Unexpected non-table objects, grants/memberships, missing expected objects/history/receipt or incomplete catalog visibility refused |
| IP-AC04 | Actual competing same/different slots and restore serialize through global gate; unrelated admission refuses during global quarantine; separately authorized reconciliation can resolve the blocking state under the gate, and ordinary admission resumes only with verified continuity, all blockers resolved and fresh authorization |
| IP-AC05 | Actual worker restart, known stale or invalid control state, continuity unknown and persistence errors refuse |
| IP-AC06 | Actual owned restore under separately approved fixture; preserved intent despite same/different identity, all affected slots fenced |
| IP-AC07 | Interrupted restore and incarnation publication; unresolved fence survives restart, no partial eligibility in affected or unrelated slots |
| IP-AC08 | Stale/mismatched attempt, insufficient lineage, unquiesced backend and unauthorized rebind refused; old unknown facts preserved |
| IP-AC09 | Actual relevant inventory/role/dependency parity across targets including Identity v35 |
| IP-AC10 | Bounded trusted host, storage and credential containment, exact-head evidence; DA-C reevaluation if assumptions fail |
| EB-AC01 | Pre-cancel, exhausted budget, contention and cumulative versions; no new mutation after observed expiry |
| EB-AC02 | Actual database timeout/commit/cleanup semantics and durability settings with independently settled effects |
| EB-AC03 | Process-restart journal persistence, stable gate and evidenced storage errors; distinguish injected errors, no power-loss claim |
| EB-AC04 | H1/H2 old/new fresh and adoption parity, exact history preservation; H2 durable intent and terminal resolution without target mutation |
| EB-AC05 | H3/H4/H5 populated upgrade oracle and privilege/data invariants |
| EB-AC06 | Actual worker interruption and both ambiguous response outcomes; restart refusal and authorized reconciliation |
| EB-AC07 | Isolated restore/fence/identity/continuity and dirty refusal; global admission refusal while any blocking state remains, no auto-rebind/replay |
| EB-AC08 | Exact named mandatory tests, repeated CI feasibility and retained original gate; stop if infeasible |

## Open implementation gates

Architecture directions are accepted candidates, not proven feasibility. Before applicable Gate C: exact global quarantine enforcement and acceptance feasibility (Option A selected); exact journal format/size/retention and atomic publication semantics; trusted single-host storage/mount continuity and exclusive credential/entrypoint enforcement; independently approved fixture-specific inventory, provenance and catalog visibility; exact legacy characterization and supported upgrade seeds/oracles, including H6 fixture signing/authorization semantics; measured execution/cleanup and filesystem behavior; deterministic safe committed/noncommitted ambiguous-response mechanism; isolated restore and fault harness feasibility; exact G4 path/test allowlist, additive named registration and repeated CI budget evidence; separate owner implementation and execution approval. If single-host assumptions fail, stop for DA-C research. No unresolved blocker is waived by documentation merge. Current scope does not authorize coding, database execution, migrations, dependencies, runtime or merge. #105/#14/#18/#86 remain open; K2a limitations unchanged; K2b/K3 not authorized.

## Sources

Pinned sources reviewed: [golang-migrate v4.20.1 driver](https://github.com/golang-migrate/migrate/blob/v4.20.1/database/pgx/v5/pgx.go), [orchestration](https://github.com/golang-migrate/migrate/blob/v4.20.1/migrate.go), [pgx v5.13.0 connection](https://github.com/jackc/pgx/blob/v5.13.0/conn.go), [transaction](https://github.com/jackc/pgx/blob/v5.13.0/tx.go). PostgreSQL 18 reference material: [protocol flow](https://www.postgresql.org/docs/18/protocol-flow.html), [locking](https://www.postgresql.org/docs/18/explicit-locking.html), [client settings](https://www.postgresql.org/docs/18/runtime-config-client.html), [date/time functions](https://www.postgresql.org/docs/18/functions-datetime.html). Additional research: [Linux fsync](https://man7.org/linux/man-pages/man2/fsync.2.html), [rename](https://man7.org/linux/man-pages/man2/rename.2.html), [flock](https://man7.org/linux/man-pages/man2/flock.2.html), [Go os](https://pkg.go.dev/os), [PostgreSQL WAL settings](https://www.postgresql.org/docs/18/runtime-config-wal.html), [physical recovery](https://www.postgresql.org/docs/18/continuous-archiving.html), [catalogs](https://www.postgresql.org/docs/18/catalogs.html), [GitHub-hosted runners](https://docs.github.com/en/actions/reference/runners/github-hosted-runners). Research consolidated 2026-11-26; no actual native test execution.
