# Decisions to resolve before implementation

Use the local [architecture decision index](../adr/README.md) for baseline requirements. Record each choice below when its stage starts. A candidate needs tests in Mamony Go before it becomes an implementation claim.

## Foundation and access

| ID | Choice | Decision evidence |
|---|---|---|
| DEC-01 | Pin the PostgreSQL release, Go driver, migration runner, and role bootstrap. | Clean boot, upgrade, real runtime roles, and restore on the same target. |
| DEC-02 | Choose the Go transaction and locking design for posting. Define which metadata belongs in the replay fingerprint. | Same-key contention, floor race, close and post race, and lost response after commit. |
| DEC-03 | Choose Go password and factor storage, recovery, credential rotation, and revocation timing. | Threat review and tests for operator second factor, revocation, and recovery. |
| DEC-04 | Choose a screening matcher and rule version policy. Choose list age and review limits. | Synthetic false-positive and miss fixtures. A missing or stale list always causes hold. |
| DEC-05 | Approve a job role only when its access cannot use an existing service boundary. Define its exact grants. | Runtime catalog test, least privilege, and no access from a request handler. |

## Money and recovery

| ID | Choice | Decision evidence |
|---|---|---|
| DEC-06 | Choose the [wire amount format](../adr/0003-public-http-contract.md#open-decision-on-wire-amounts) before publishing an amount route. | Exact round trip at the wire and storage limits. Publish OpenAPI and compare it with the routes. |
| DEC-07 | Define when each C07 reservation becomes used or released. Define the account-suspension boundary for work already in a ledger transaction. | Pay-in, pay-out, cancel, unknown, expiration, and concurrent suspension fixtures. |
| DEC-08 | Choose the FX action when a slow provider succeeds after its quote expires. | Specify the rate and approval policy. Test late success and a partial FX execution. |
| DEC-09 | Set pilot corridor, cutoffs, deadlines, return windows, and reporting grace periods. | Synthetic provider fixtures cover both flows and all valid transitions. |
| DEC-10 | Set event retention, replay access, and the complete rebuild source for C16. | Test a replay inside retention and a rebuild older than the outbox window. |
| DEC-11 | Confirm provider ports and event fields for recipient checks, disputes, evidence, and monitoring. | Consumer contract tests cover missing data and supported event versions. |
| DEC-12 | Choose restore cadence, recovery target, and load thresholds for each gate. | Measure results in Go with the selected data size and job workload. |

## Later scope

Decide the dispute shortfall policy, protected audit checkpoint, sandbox isolation, and C20 liability before their affected features start. Read the named contract for each decision. Keep unchosen values out of runtime defaults.
