# Identity login performance baseline

This opt-in test measures the internal password login candidate on a disposable PostgreSQL fixture. It runs the existing PostgreSQL admission transaction, credential lookup under the authenticator role, and Argon2id verification. Every measured request must return the expected synthetic subject. A wrong-password control must return a generic denial after the measured batch.

## Reproduce

Use Linux, Go from `.go-version`, PostgreSQL 18.7 binaries, Python 3, and `taskset`. Run as an unprivileged user. Run the cases sequentially. The runner creates and removes a fresh loopback-only cluster per invocation. It verifies that the server uses the new data directory before provisioning the fixture.

```sh
bash tools/postgrestest/run-identity-login-perf.sh 1
bash tools/postgrestest/run-identity-login-perf.sh 4
MAMONY_IDENTITY_PERF_PROFILE=1 bash tools/postgrestest/run-identity-login-perf.sh 4
```

Use `MAMONY_IDENTITY_PERF_CPU_SET` to choose the same two available logical CPUs for all runs. The default is the first two available CPUs. PostgreSQL and the test process share that affinity. The test uses `GOMAXPROCS=2`, a **soft** Go memory target of 256 MiB, one KDF slot, and two connections per runtime pool. PostgreSQL uses 32 MiB shared buffers, a maximum of 20 connections, and JIT disabled. The memory target does not impose a process or cluster memory cap.

Each run uses 16 verified synthetic actors and measures 64 valid internal login attempts in a closed loop. Provisioning, warm-up, and the denial control are outside the timed batch. Case setup and requests use the existing role-separated adapters and effective admission limits. The runner builds the suite with `integration,identity_perf`. Session issuance, HTTP ingress, providers, restart, and open-loop traffic are outside the measured path.

## Read the metrics

| Metric | Meaning |
|---|---|
| `login` | Time through the internal login operation, including admission, SQL, and KDF. |
| `admission_db_tx` and `credential_db_tx` | Transaction wall time in each adapter, including checkout, BEGIN, work, and COMMIT. |
| `db_checkout_and_begin` | Sum of the pre-callback time for the measured transactions in one operation. This combines checkout and BEGIN. The runner has no separate pool-wait metric. |
| `db_callbacks` | Sum of the callback wall times. Includes queries, scans, and callback work. |
| `kdf_including_slot_wait` | Wall time inside the actual verifier. It includes slot wait, input checks, and hashing. |
| `process_peak_rss_kib` | Peak RSS of the Go test process over its lifetime, including fixture setup. PostgreSQL RSS is outside this value. |

Percentiles use nearest rank. At 64 samples, p99 is the maximum observed request for that run. Treat it as a tail observation. Compare several unprofiled runs with the same machine, CPU set, Go version, PostgreSQL version, data shape, and batch settings.

Run the opt-in block profile separately. The cumulative value for `PasswordHasher.acquire` sums blocked time across goroutines. It can exceed elapsed wall time and cannot supply a per-request percentile. The standard test binary is built from the same sources in both modes.

## Initial observation

Measured on 2026-11-27 WIB on Linux amd64, Intel Xeon Platinum 8573C, CPUs 0 and 1, Go 1.27.4, and PostgreSQL 18.7. Each range spans two unprofiled runs per case. All four runs returned 64 expected candidates and passed the wrong-password control.

| Callers | Successful operations per second | Login p50 | Login p95 | Login p99 observed | Go peak RSS |
|---|---:|---:|---:|---:|---:|
| 1 | 7.53–7.67 | 118–125 ms | 165–213 ms | 190–255 ms | 144.4–144.6 MiB |
| 4 | 8.41–9.00 | 438–458 ms | 482–618 ms | 500–646 ms | 144.8–144.9 MiB |

At four callers, admission transaction p95 was 15.0–25.1 ms, credential transaction p95 was 3.2–7.4 ms, and KDF including wait p95 was 479–612 ms. The separate profiled run attributed 23.12 seconds of cumulative blocking to acquisition of the KDF slot across goroutines. This workload points to slot waiting as the main contributor to the parallel-case tail. Keep admission, verifier parameters, and authorization semantics fixed when comparing performance changes. Measure aggregate CPU and memory under deployment resource limits before setting any capacity objective.
