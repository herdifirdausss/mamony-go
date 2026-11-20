## Outcome and source
- Issue / acceptance criteria:
- Mamony source PRD/TD/ADR with path and heading:
- Owner capability and risk class (S/M/L):

## Design
- Smallest change that satisfies AC:
- Alternatives considered / why a new abstraction is needed (if applicable):
- Security / money / concurrency / recovery invariants impacted:

## Evidence
| Check | Command + evidence | Status (PASS/FAIL/NOT RUN) |
|---|---|---|
| Format, vet, tests, docs, architecture, skills, v6 research registry, build | `make check` | NOT RUN |
| Race detector (where relevant) | `make race` | NOT RUN |
| Go architecture linter (specialized CI) | `make archlint` / GitHub Actions | NOT RUN |
| Summarized + retained JSON/JUnit Go tests (specialized CI) | `make test-report` / GitHub Actions | NOT RUN |
| oasdiff API compatibility (only with real baseline/current specs) | `make api-compat` | NOT RUN |
| Third-party skill scanning (if added/updated) | [Preflight](../docs/tooling/third-party-security.md) | NOT RUN |
| golangci-lint v2.15.0 (blocking CI job) | `make lint` | NOT RUN |
| govulncheck v1.10.0 (blocking CI job) | `make security` | NOT RUN |
| PostgreSQL integration / contracts / restoration (where relevant) | Targeted test names | NOT RUN |

## Compatibility / rollout
- API/event schema or migration impact:
- Deployment/rollback/roll-forward:
- Open blockers, owner decision or risk acceptance:

## AI-workflow changes (if applicable)
- [v6 E01–E12 plan](../docs/evals/mamony-agent-benchmark.md): matched tasks, model and baseline:
- Accepted AC / hard-gate results:
- Measured input/output/cached tokens, cost, retries, review effort (or NOT RUN):

Do not claim Gate A, benchmark success, token savings or deployment readiness without reproducible evidence.
