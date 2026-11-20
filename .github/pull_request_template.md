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
| Format, vet, tests, docs, architecture, skill metadata, build | `make check` | NOT RUN |
| Race detector (where relevant) | `make race` | NOT RUN |
| golangci-lint | `make lint` | NOT RUN |
| Vulnerability scan | `make security` | NOT RUN |
| PostgreSQL integration / contracts / restoration (where relevant) | Targeted test names | NOT RUN |

## Compatibility / rollout
- API/event schema or migration impact:
- Deployment/rollback/roll-forward:
- Open blockers, owner decision or risk acceptance:

Do not claim Gate A or deployment readiness without database and failure-mode evidence.
