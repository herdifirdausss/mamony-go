# Ledger K2a — isolated persistence foundation

Status: bounded synthetic Draft candidate; independent technical acceptance DEFERRED, production readiness NOT ACCEPTED.

Tracks [#103](https://github.com/herdifirdausss/mamony-go/issues/103) under [#86](https://github.com/herdifirdausss/mamony-go/issues/86). Owner accepted P01–P07 / AC01–AC10, SEQ-01–06, exact ten-path G4 plan and Gate C for Draft implementation and verified disposable database testing only. See [design packet](https://github.com/herdifirdausss/mamony-go/issues/86#issuecomment-7152124974), [sequencing review](https://github.com/herdifirdausss/mamony-go/issues/86#issuecomment-7152185146), [G4](https://github.com/herdifirdausss/mamony-go/issues/86#issuecomment-7152222312) and [reconciliation](https://github.com/herdifirdausss/mamony-go/issues/86#issuecomment-7152265856). Baseline main f46459615189ea0657c630f589219b082aee9b71; #81 and #95 are open. No roadmap or governance change. G6 exact-head and target authorization remains separate.

## Bounded design

Ledger owns four local business tables: currency_definitions, books, account_types and accounts. Exact USD definition and original curated provenance, single book, three fixed account type mappings, opaque bytewise UTF-8 identifiers, ACTIVE zero-balance/sequence fixtures. Immediate local foreign keys; no cross-owner reference. No seeded business rows in migration. The initial schema and migration fingerprint are separate from fixture provisioning. A fixed Ledger migration target reuses existing runner behavior, with distinct owner, schema and lock. No production command or runtime callsite.

Tests create dedicated disposable PostgreSQL 18.7 containers on a local Docker daemon, with generated synthetic credentials and explicit loopback ports. Inspect container identity, image, mounts and port boundary before starting; verify database and role identity, UTF-8 and server version before provisioning. No external DSN or existing application database. Bootstrap admin and schema owner are trusted; loader is temporarily insert-only, reader is read-only, loader privileges revoked and sessions terminated before restore. No ordinary financial writer. Other-owner access is tested with a disposable canary. Back up a sealed fixture and restore into a distinct independently provisioned destination, preserving schema ownership and ACLs.

## Acceptance evidence

All candidate-specific execution is NOT RUN at authoring time. Six named PostgreSQL groups are mandatory in the existing testgate:

| AC | Candidate test | Status |
|---|---|---|
| 01, 09 | TestPostgresLedgerK2aContainment and diff review | NOT RUN |
| 02 | TestPostgresLedgerK2aCurrency | NOT RUN |
| 03, 04 | TestPostgresLedgerK2aStorage | NOT RUN |
| 05 | TestPostgresLedgerK2aPrivileges | NOT RUN |
| 06 | TestPostgresLedgerK2aMigration | NOT RUN |
| 07 | TestPostgresLedgerK2aRestore | NOT RUN |
| 08 | Existing repository checks and actual named integration evidence | NOT RUN |
| 10 | Exact-candidate review and nonclaims | Pending |

No real version-to-version Ledger upgrade (first baseline only), complete crash/recovery proof, private K1 domain roundtrip, production RPO/RTO or actual authorization claim. The test-only failure migration is not a supported upgrade. The restore candidate also rejects missing role definitions before a successful independent restore. Review exact privilege matrix, remaining negative recovery cases, and actual scenario execution before accepting ACs; passing top-level tests alone is not sufficient. Any failed or missing required scenario blocks G5 completion.

## Limits

No K2b or K3, account lifecycle, journal, posting, replay, balance mutation, runtime integration, tenant mapping, real user/funds, production database or activation. No independent authentication of reference source. Schema-owner/administrator compromise is outside the synthetic privilege proof. No full LGR-09/10/12, FND-02/03/08/09/12 or Ledger gate closure. #14/#18/#20/#78 remain open. No CI workflow, permissions, timeout or dependency change; if the lab is unavailable or required tests cannot run under the existing gate, stop for owner review. Merge is not authorized.
