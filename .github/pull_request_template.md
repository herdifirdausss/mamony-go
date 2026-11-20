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
| golangci-lint | `make lint` | NOT RUN |
| Vulnerability scan | `make security` | NOT RUN |
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
