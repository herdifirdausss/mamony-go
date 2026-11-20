# Engineering standards adoption status

| Area | Authority / status | Enforcement |
|---|---|---|
| [Local decisions](../adr/README.md) and [task contracts](../index.md) | Mamony Go baseline and proposed choices | Apply the contract checks for changes in scope. |
| [Engineering principles handbook](engineering-principles.md) | **Draft guidance**; does not supersede approved Mamony decisions | Invoked selectively through skills and review |
| [v6 agent engineering](ai-agent-production-engineering.md) and [integrated research](../research/engineering-workflow-v6.md) | **Draft**; guidance, not Accepted product ADR | Read task-relevant sections, not all context |
| [31-source research register](../research/source-register.md) | Review-date literature index; links/versions need revalidation when promoted to policy | CI verifies presence/count, NOT live source correctness |
| [v6 E01–E12 agent benchmark](../evals/mamony-agent-benchmark.md) | **Plan only; NOT RUN**; no measured cost/token improvement | CI checks case registry, not agent quality |
| Go modular monolith [ADR-0001](../adr/0001-go-modular-monolith.md) | **Proposed** pending owner review | Current import checker tests, does not imply ADR accepted |
| Eight Mamony agent skill files | **In-repo native guidance**; upstream Ponytail/Superpowers/Trail of Bits/STE100 are NOT installed | Metadata checks via `make skills`; host discovery and behavioral A/B NOT RUN |
| [11 external tool shortlist](../tooling/shortlist.md) | Actual integration vs opt-in/not-run tracked per candidate | `make tools-policy` verifies registry and required files only |
| go-arch-lint v1.19.2 + gotestsum v1.13.0 | Specialist Go CLI pinned in [CI](../../.github/workflows/engineering-tools.yml) | Separate PR job; verify actual workflow outcome, do not extrapolate from basic CI |
| gopls MCP, ast-grep, RTK, SkillSpector, upstream skills | **OPT-IN / NOT INSTALLED**; activation requires host setup and trust review | Manual per [preflight](../tooling/third-party-security.md) and matched task eval |
| oasdiff breaking API check | Script prepared but no approved Go API contract exists | **NOT RUN** on actual released-vs-candidate spec |
| `gofmt`, `go vet`, tests, architecture, doc links, build, race | Baseline deterministic checks | `make check` and `make race` in PR CI |
| v6 research and evaluation metadata | Machine-checkable document completeness only | `make research` in `make check` (not literature verification) |
| golangci-lint v2.15.0 and govulncheck v1.10.0 | [CI](../../.github/workflows/ci.yml) now defines separate blocking lint and vulnerability jobs; latest execution must be inspected | `make lint` / `make security`, **PASS only on successful Actions runs** |
| [Harness v7 integration map](../research/harness-v7-provenance.md) | V7 ZIP audited against branch; historical test results archived, not rerun as current evidence | `make harness` checks routing and expected jobs, not design safety |
| PostgreSQL, ledger idempotency, RLS, outbox, restore | **NOT IMPLEMENTED** in Go | Database/financial Gate A is NOT RUN |

**Truthful gate reporting:** a green basic CI does not imply the security, finance, or performance gates passed.

## Skill activation

Use the narrowest relevant skill from [registry](../../.agents/skills/). An agent must verify its installed skill discovery behavior; a file's presence in this directory does not itself prove a skill was invoked. Read only matching handbook headings.

## Next adoption steps

1. Review v6 research and decide which recommendations become mandatory Go standards; record any baseline change in a local ADR.
2. Verify and maintain the added pinned lint/vulnerability CI jobs; address failures before approving the PR.
3. Build PostgreSQL migrations and real integration/recovery tests before claiming Gate A.
4. Run the [v6 matched-task E01–E12 evaluations](../evals/mamony-agent-benchmark.md) before claiming quality improvements or token savings; compare accepted AC, cost, latency and defects.
