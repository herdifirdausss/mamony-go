# Engineering pack verification

The three attached archives were checked against the draft PR #1 tree at [`0858099`](https://github.com/herdifirdausss/mamony-go/commit/08580993fec66139e5880c1314e1eb75cd43a967) on 2026-11-21 (Asia/Jakarta). All 60 archive members passed ZIP integrity checks. All 38 entries in the v7 member checksum list matched.

## Archive reconciliation

| Archive | Files | SHA-256 |
|---|---:|---|
| `Mamony_Engineering_Standards_Pack_v5(2).zip` | 9 | `11ecb28246e16b5c22cf1cfc144d98248ea04dec0b11f05d997b4dcb330f8eb2` |
| `Mamony_Engineering_Research_Pack_v6(2).zip` | 12 | `ba6026634ba8746c188f5cbc96a5bbca875e086bf28e975f0fd14baf4b8ae202` |
| `Mamony_Engineering_Harness_v7(2).zip` | 39 | `538990b9fe7c8321f6fa2d2e5c8890a0d54bcfe75b9a9df77f6d9090fe563c88` |

- **v5.** The main [workflow](../research/engineering-workflow-v5.md) and [handbook](../standards/engineering-principles.md) matched their ZIP members byte for byte before translation. The five [agent skills](../../.agents/skills/) match their originals. The ZIP README's adoption intent is reflected in the [adoption register](../standards/adoption-status.md), and all seven source cases are represented in the [skill evaluation plan](skill-evaluation-v5.md).
- **v6.** The main [workflow](../research/engineering-workflow-v6.md) and [addendum](../standards/ai-agent-production-engineering.md) matched their ZIP members byte for byte before translation. The [benchmark plan](../evals/mamony-agent-benchmark.md) remains an exact match. The [source register](../research/source-register.md) retains all 31 URLs and their evidence classes. The [v6 manifest](../research/v6-integration-manifest.md) maps the proposed instructions, shared v5 files, and ZIP README to their repository destinations.
- **v7.** The [39-file integration map](../research/harness-v7-provenance.md) accounts for the scaffold, executable checks, adopted CI jobs, original documentation, and historical evidence. The repository carries the standalone findings in an [archived report](archived-harness-v7-local-run.md). Local verification below used the current PR code and its pinned Go release.

The two English workflows, the handbook, and the addendum are translations of the original drafts. The original bytes remain available in the verified source archives and the audited Git commit. The [adoption register](../standards/adoption-status.md) records which guidance is enforceable today.

## Executed checks

| Check | Result | Evidence |
|---|---|---|
| Foundation and concurrency | PASS | Local `make check` and `make race` on Go 1.27.4. `make check` includes Go format, vet, unit, architecture, documentation, skill metadata, research metadata, tooling metadata, harness metadata, and build checks. |
| Static analysis | PASS | Local `make lint` using golangci-lint v2.15.0 reported zero findings. The configured analyzer set includes gosec. |
| Reachable vulnerabilities | PASS | Local `make security` using govulncheck v1.10.0 found no reachable vulnerabilities. |
| Specialized Go tools | PASS | Local `make archlint` with go-arch-lint v1.19.2 and `make test-report` with gotestsum v1.13.0. The latter produced JSON and JUnit reports for 30 test cases. |
| Architecture fixture | PASS | In an isolated source copy, `make architecture` accepted app-to-facade and local handler-to-use-case imports. Adding a direct payment-to-ledger import caused the expected nonzero exit and cross-capability diagnostic. |
| HTTP runtime smoke | PASS | Started the compiled binary. Public `/livez` and private `/readyz` returned 200. Public `/readyz` returned 404. The process exited cleanly after SIGTERM. |
| GitHub Actions at the audited commit | PASS | [Engineering baseline](https://github.com/herdifirdausss/mamony-go/actions/runs/44804363194) and [Specialized Go tools](https://github.com/herdifirdausss/mamony-go/actions/runs/44804363168) both completed successfully. The latter uploaded the [JSON/JUnit artifact](https://github.com/herdifirdausss/mamony-go/actions/runs/44804363168/artifacts/14661374914). The downloaded archive matched the published digest. Both reports recorded 30 passing tests. |

## Pending evidence

- **Blocked by missing implementations or decisions.** PostgreSQL migrations, runtime role grants and RLS, durable C01 Post/Reverse, idempotency, outbox, provider recovery, restore drills, and OpenAPI compatibility await their approved contracts and implementation. See the [C01 requirement trace](../contracts/ledger-gate-a.md).
- **Not run.** Agent skill discovery in a named host, v5 A/B trials, v6 E01–E12 trials, and RTK cost/token trials. Harness fixtures H07-E06–E08 require the runtime components above.
- **Unverified.** Branch protection requirements and independent review of the 31 cited sources. Workflow job success verifies execution at the named commit. The research gate verifies registry structure.
