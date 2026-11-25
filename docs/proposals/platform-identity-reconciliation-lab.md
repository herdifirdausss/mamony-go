# Synthetic Identity reconciliation lab — RH-03

**Status:** owner-approved bounded Draft implementation; independent technical acceptance DEFERRED; G6 merge NOT AUTHORIZED; production readiness NOT ACCEPTED.
**Owner:** Platform [#14](https://github.com/herdifirdausss/mamony-go/issues/14), Identity [#78](https://github.com/herdifirdausss/mamony-go/issues/78).
**Scope decision:** [owner approval](https://github.com/herdifirdausss/mamony-go/issues/14#issuecomment-7149877826).
**Base inspected:** 3f402ff668d582d774b2fb3494a4703a602b0eb8.

## G0–G3 context

Goal: compare a held recovery target with a known, controlled synthetic source boundary; preserve durable bindings and original audit/notice obligations; reject incomplete evidence. See [native PITR lab](platform-native-pitr-lab.md), [recovery runbook](../runbooks/recovery.md), IAM-03, FND-08/FND-12 and PGF-06. The approved choice is B: bounded known-boundary recovery and reconciliation with HOLD, not restore-and-release or a production registry. PostgreSQL logical dump/restore supports a consistent data snapshot, not proof of the newest acknowledged history. See [PostgreSQL 18 pg_dump](https://www.postgresql.org/docs/18/app-pgdump.html) and [pg_restore](https://www.postgresql.org/docs/18/app-pgrestore.html). Native PITR was exercised separately in #88; this slice deliberately uses existing main Identity integration fixtures and a logical restore, so it does not claim native Identity PITR. The source history and test runner are trusted for this exercise; no independent custody or authenticated completeness.

## G4 slices and AC-to-test map

| Slice | Scope / expected evidence | Status |
|---|---|---|
| RH-03a1 (RH-03 durable bindings) | Actual committed main Identity fixture after mandatory integration tests; snapshot selected real tables with nonempty durable rows; compare stable counts and SHA-256 checksums with a distinct isolated logical restore. Includes original audit and notice rows, staged passkey state, recovery code set, factor keys, password/contact/subject rows. | NOT RUN |
| RH-03b1 (RH-03 temporary authority) | Compare session, recovery challenge/grant, enrollment and passkey challenge rows; prove target is isolated and runtime roles NOLOGIN. This is containment and row-state evidence only, not application-level temporary authority invalidation. | NOT RUN |
| RH-03c1 (RH-03 evidence rejection) | Missing/truncated/contradictory process-local comparison evidence refuses review; even equality remains HOLD. No release mechanism. | NOT RUN |
| Remaining RH-03 | Actual native Identity PITR at known boundary, explicit application-level denial of stale authority and preservation of current durable state across a recovery workflow; v36 possession transition and its notice depend on unmerged #81. | NOT RUN |

No source writes, no schema changes, no new runtime, no live credentials, no provider calls, no release or registry. The runner uses the exact existing synthetic GitHub Actions service, fixed database name, image and service identity; a second container has no network, no published ports, and recreated NOLOGIN role identifiers only. The dump excludes ownership/grants; no ACL or migration parity claim. Keep dump and comparison files in a private ephemeral directory; only case results are logged. Do not export the dump or row hashes. The source is observed before and after dump after the integration gate finishes; this does not establish a generally trustworthy commit watermark. Cleanup is best effort on the disposable runner. Fail closed on missing fixtures, unexpected source/target, or missing evidence.

## G5 verification

Candidate must pass bash syntax, required markers and exact-head CI. All recovery cases remain NOT RUN until actual logs are inspected. Independent technical review and separate owner exact-candidate merge authorization remain mandatory under the adopted policy; the owner performs any merge. No change to #81. PGF-06/FND-08/FND-12 remain PARTIAL.
