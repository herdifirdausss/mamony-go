---
name: mamony-engineering-review
description: Use for Mamony Go architecture choices, package/interface/dependency proposals, design-pattern selection, or refactoring reviews. Skip for trivial prose-only changes.
---
# Engineering design review (on demand)

1. Read the issue and the relevant section of [engineering practices](../../../docs/engineering.md).
2. Follow the [change lifecycle](../../../docs/development-workflow.md). Identify the change path, impact, owning capability, *accepted* PRD/contract requirements, invariants, and security/recovery obligations. For Critical work, require documented product/technical acceptance before implementation. If money, permission or irreversible state is unclear, report the blocker instead of inventing a decision.
3. Identify **the simplest valid design** in idiomatic Go. Apply KISS/YAGNI/DRY/SoC, small consumer-owned interfaces, composition, and explicit dependencies; never force all SOLID or GoF patterns into a change.
4. If proposing a new package, interface, design pattern, tool, cache, queue, or dependency, report: real problem/AC; smallest alternative; why it fails; chosen approach; added complexity; exact evidence/test; revisit trigger. If a simple function or switch works, choose that.
5. For refactoring: list observable behavior that must remain, baseline tests and contract, then perform small changes. Distinguish a behavior-changing feature/fix from refactoring.
6. For changes to money, access, or recovery: ask which consistency boundary, failure mode, timeout, idempotency, data ownership, tenant isolation, recovery, and observability need verification. Use `mamony-ledger-review` or `mamony-boundary-review` only when their scope is touched.
7. Compare proposed architecture against each accepted acceptance criterion and its planned proof. If a requirement is unknown or a trust boundary changed, propose a decision and block implementation instead of silently designing around it.
8. Finish with: **Decision**, **Trade-offs**, **Changed scope**, **Evidence** (`PASS`, `FAIL`, `NOT RUN`), **Open risks**. Never claim success of a command that was not executed.

This skill is advisory. Mamony Go contracts, accepted ADRs, and actual test results outrank generic style heuristics. Do not read entire roadmap/design-pattern pages into the model context for routine tasks.
