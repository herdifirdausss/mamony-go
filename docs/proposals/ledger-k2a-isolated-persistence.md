# Ledger K2a — isolated persistence foundation

**Status:** Merged for bounded synthetic use in [#104](https://github.com/herdifirdausss/mamony-go/pull/104). Independent technical acceptance is DEFERRED. Production readiness is NOT ACCEPTED.

[#103](https://github.com/herdifirdausss/mamony-go/issues/103) records this slice under [#86](https://github.com/herdifirdausss/mamony-go/issues/86). The owner accepted the [detailed design](https://github.com/herdifirdausss/mamony-go/issues/86#issuecomment-7152124974), the [one-time sequencing exception](https://github.com/herdifirdausss/mamony-go/issues/86#issuecomment-7152185146), and the [bounded implementation plan](https://github.com/herdifirdausss/mamony-go/issues/86#issuecomment-7152222312). In this workstream, [Gate C](https://github.com/herdifirdausss/mamony-go/issues/86#issuecomment-7152265856) authorized Draft implementation and verified disposable database testing. The owner later authorized the [exact synthetic merge](https://github.com/herdifirdausss/mamony-go/pull/104#issuecomment-7152639690). The [Ledger implementation sequence](../roadmap.md#ledger-implementation-sequence) records the stages that follow.

## Bounded design

Ledger owns four local business tables: currency_definitions, books, account_types and accounts. Exact USD definition and original curated provenance, single book, three fixed account type mappings, opaque bytewise UTF-8 identifiers, ACTIVE zero-balance/sequence fixtures. Immediate local foreign keys; no cross-owner reference. No seeded business rows in migration. The initial schema and migration fingerprint are separate from fixture provisioning. A fixed Ledger migration target reuses existing runner behavior, with distinct owner, schema and lock. No production command or runtime callsite.

Tests create dedicated disposable PostgreSQL 18.7 containers on a local Docker daemon, with generated synthetic credentials and explicit loopback ports. Inspect container identity, image, mounts and port boundary before starting; verify database and role identity, UTF-8 and server version before provisioning. No external DSN or existing application database. Bootstrap admin and schema owner are trusted; loader is temporarily insert-only, reader is read-only, loader privileges revoked and sessions terminated before restore. No ordinary financial writer. Other-owner access is tested with a disposable canary. Back up a sealed fixture and restore into a distinct independently provisioned destination, preserving schema ownership and ACLs.

## Acceptance evidence

The [exact-candidate reconciliation](https://github.com/herdifirdausss/mamony-go/pull/104#issuecomment-7152589123) and [post-merge verification](https://github.com/herdifirdausss/mamony-go/pull/104#issuecomment-7152708615) record the executed cases and accepted limits. The [PostgreSQL check on the merge commit](https://github.com/herdifirdausss/mamony-go/actions/runs/45545410875/job/138973718282) passed all six named K2a test groups.

| AC | Evidence | Bounded result |
|---|---|---|
| 01, 09 | TestPostgresLedgerK2aContainment and diff review | PASS within the hosted disposable fixture and diff. External deployment inspection was NOT RUN. |
| 02 | TestPostgresLedgerK2aCurrency | PASS for the frozen USD definition. |
| 03, 04 | TestPostgresLedgerK2aStorage | PASS for the synthetic storage fixture. A direct private K1 domain round trip was NOT RUN. |
| 05 | TestPostgresLedgerK2aPrivileges | PASS for the fixture roles and grants. |
| 06 | TestPostgresLedgerK2aMigration | Executed cases PASS. In-flight cancellation, actual process kill in the committed-body/pre-clean window and Ledger version-to-version upgrade were NOT RUN. |
| 07 | TestPostgresLedgerK2aRestore | PASS for the sealed fixture restored into a distinct disposable database. Financial recovery and production recovery objectives remain unverified. |
| 08 | Repository checks and named integration groups | PASS on the reviewed candidate. |
| 10 | Exact-candidate review and retained scope limits | Reconciled for bounded synthetic merge. |

[#105](https://github.com/herdifirdausss/mamony-go/issues/105) tracks the remaining migration cancellation and outcome requirements before Ledger runtime or production use. The modeled dirty fixture demonstrates refusal of the modeled state. Actual crash behavior in that window remains unverified. [PR #123](https://github.com/herdifirdausss/mamony-go/pull/123) merged the pure migration journal model for that follow-up. Its unit evidence covers model behavior. Actual native execution, storage, and fault-test evidence remain open.

## Limits

The slice excludes account lifecycle, journal posting, replay, balance mutation, runtime integration, tenant mapping, real users and funds, production databases and activation. Full requirement and Ledger gate acceptance remains open. The fixture trusts its schema owner and administrator and uses the curated reference as its provenance boundary. See the [Ledger implementation sequence](../roadmap.md#ledger-implementation-sequence) and [current state](../current-state.md) for remaining work.
