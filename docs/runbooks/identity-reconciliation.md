# Identity enrollment reconciliation — operator runbook (Draft PR #33)

**Status: candidate only.** This standalone tool is intentionally **not deployed, scheduled, invoked by HTTP, or connected to a real Mailgun account**. No public Identity routes are mounted. This document describes an operator-controlled primitive, **not** a production monitoring or incident response deployment.

## Problem and authority

Draft PR #31 registers an UNVERIFIED subject and then issues its first email proof in a second transaction. A crash between the commits leaves an account with ENROLLMENT_PENDING but without an email challenge. Draft PR #32 added an unmounted one-shot reconciler for subjects **2 minutes to 24 hours** old with no historical verification challenge at all. The verifier database role, not the business application/HTTP body, resolves subjects. FirstIfMissing verifies the no-challenge condition again under the subject/alias row lock. Other pending users with an existing, expired, revoked or lost proof must use independently admitted resend; automatic rotation is not permitted.

This PR adds **observation and a guarded command entry point**, not delivery. It reuses the verifier role, encrypted AES-256-GCM proof outbox and previously approved issuance quotas. A running Mailgun worker is a **separate deployment decision**.

## Command contract

`go run ./cmd/mamony-identity-reconcile --inspect`

- Read-only. Opens only the verifier-role connection.
- Reports only `eligible_sampled` (0–26, **26 means at least 26**) and `stale_without_first_proof` (boolean, at least one older than 24 hours), including a static operational status.
- No subject IDs, contact addresses, token digests, ciphertext, password verifiers, database strings or provider responses are printed.

`go run ./cmd/mamony-identity-reconcile --repair-once`

- Requires an explicit approved operator action and separate encrypted-proof keyring; never runs in CI or by default.
- Reads the initial backlog, executes **one** at-most-25-subject reconciliation pass, and reads a final backlog. It does not loop, schedule, or call Mailgun.
- Reads an operator-provisioned verifier DSN from `MAMONY_IDENTITY_RECONCILE_DSN_FILE` (a restricted, regular, owner-readable file). Connection must use role `mamony_identity_verifier_runtime`, TLS `sslmode=verify-full`, a real non-test database and no libpq query overrides.
- Additionally requires `MAMONY_IDENTITY_RECONCILE_APPROVAL=I_APPROVE_BOUNDED_IDENTITY_FIRST_PROOF_REPAIR` and `MAMONY_IDENTITY_ENVELOPE_KEYS_FILE`, a separate private JSON keyring file using `{"active":"key-id","keys":{"key-id":"<base64-encoded-32-byte-key>"}}` shape. The active key and up to 8 key versions are checked before issuing anything. Actual secret values must come from an approved secret manager and **must not** be committed or logged.

The acknowledgement string is **not an authentication mechanism**. Operator identity, access control, change approval and secure file mounts must be provided by infrastructure outside this repository. The test suite uses fake in-memory secrets and disposable loopback PostgreSQL. Do not run production repair simply because this candidate passes CI.

## Exit codes and alert semantics

| Exit code | JSON `status` | Meaning | Operator action |
|---|---|---|---|
| 0 | `OK` | Snapshot has no eligible first-proof gap and no stale gap | Record the successful check; does not establish delivery |
| 1 | Static error message | Invalid configuration, DB/error, partial reconciliation or failed post-check | Page operator via **separately configured** supervisor, inspect restricted logs; never assume retry is harmless without checking current state |
| 2 | `ALERT` | At least one eligible unissued first proof remains after the check, or at least one unissued subject is older than 24 hours | Escalate. Approved repair handles only the 2m–24h group; older items require manual policy review, never automatic sending |

The CLI emits structured count-only output and an exit code; **no pager, email alert, webhook, Prometheus exporter, cron, SLO, external monitor or notification destination is installed**. A production scheduler/supervisor must separately alert on error exit, persistent 2, and **absence of expected successful executions** (missing-run alert). If an inspection query fails, the tool exits 1 and must not return a misleading zero count.

## Operational sequence (subject to approval)

1. Verify application, database/roles, trusted key source and prior migrations; verify backups and that public routes are still disabled.
2. Execute `--inspect` with a restricted, operator-injected verifier DSN file. Record only count/status evidence. A valid public HTTP `202` receipt does not prove email delivery or proof issuance.
3. If eligible first-proof gaps remain, review global messaging/abuse budgets and the change ticket. Only then provision the keyring and explicit repair approval, execute `--repair-once`, and record the post-run status. The default at-most-25 limit is hard coded; there is no `--all` option.
4. If exit 2 persists, consider additional **separately approved** one-shot runs only after rate/quota/backlog review. A 24-hour-old account cannot be silently re-enrolled by this tool. Do not bypass subject-level cooldown or token ownership checks.
5. Examine delivery status with an independent authorized delivery operations process. An outbox row or Mailgun HTTP 200 indicates issuance/submission, not inbox delivery or recipient reading.
6. On any ambiguous error, do not assume an issuance failed: the transaction may already have committed. Re-inspect authority before action. Treat unexpected success/failure counts as a security/availability signal, and do not log PII while investigating.

## Threat and test scope

The browser, BFF and business DB cannot choose subjects for repair or set a new contact address; the query operates on restricted Identity data only. Subject+alias locks and a final under-lock first-only test address scan/commit TOCTOU. Redacted operational status mitigates log leakage. **This does not protect against a compromised verifier runtime, its database administrator, shared key administrator, or arbitrary control-plane access.** It is a Profile A candidate with its acknowledged DB trust assumption; never claim Profile B independent authority containment.

Required tests include command denial (missing approval, unsafe DSN/TLS, symlink, world-readable file, malformed keyring, CI repair), inspect-only separation and alert classification, as well as real PostgreSQL `TestIdentityZEnrollmentBacklogDB` verifying eligible/stale/recent boundaries, zero mutation on inspect and strict verifier-role use. All gates must PASS on the exact PR SHA.

## Production release blockers

Safe real deployment requires reviewer-approved supervisor and monitoring integration; missing-run/error/backlog alerts; production DSN/secret source and operator authentication; retention and manual remediation policy for stale accounts; capacity and query-plan measurement, especially with large subject tables; global provider quotas and bounce/complaint handling; TLS reverse-proxy and browser request security; end-to-end email verification proof consumption; and password blocklist/KDF admission measurements. A successful CI test is not a production deployment approval.
