# Mamony Go — compact AI execution contract

Source: Mamony Engineering Harness v7. This is a practical task router; it does not replace Accepted Mamony PRDs/ADRs.

1. **Intake:** capture issue, concrete outcome, acceptance criteria, S/M/L risk and owning capability.
2. **Focused retrieval:** read `AGENTS.md`, the relevant row in [docs index](../index.md), changed packages and the precise source PRD/TD/Accepted ADR headings. Avoid loading full v5/v6 research.
3. **Design when required:** for M/L work state invariants, tenant boundary, failure matrix, data owner, smallest alternative and the evidence needed to justify a new abstraction.
4. **Implement:** smallest testable vertical slice, with behavior-changing and behavior-preserving changes separated.
5. **Verify:** targeted tests, `make check`, `make race` if concurrency matters, and CI lint/security/contract/database/recovery checks where applicable. Never report NOT RUN as PASS.
6. **Review/handoff:** write evidence using the [PR template](../evidence/template.md), remaining owner decisions and residual risks.

Load project [agent skills](../../.agents/skills/) on demand only. Use [controlled benchmarks](../evals/mamony-agent-benchmark.md) before making claims about token savings or better completion rates.
