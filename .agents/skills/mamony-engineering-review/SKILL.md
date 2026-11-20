---
name: mamony-engineering-review
description: Use for Mamony Go architecture choices, package/interface/dependency proposals, design-pattern selection, or refactoring reviews. Skip for trivial prose-only changes.
---
# Engineering design review (on demand)

1. Read issue/acceptance criteria and only the relevant **heading** of `docs/standards/engineering-principles.md` (or §12–§17 of the v5 workflow until extracted). Do not load the full handbook by default.
2. Establish impact class S/M/L, owner capability, confirmed requirements, invariants, and security/recovery obligations. If money, permission or irreversible state is unclear, report the blocker instead of inventing a decision.
3. Identify **the simplest valid design** in idiomatic Go. Apply KISS/YAGNI/DRY/SoC, small consumer-owned interfaces, composition, and explicit dependencies; never force all SOLID or GoF patterns into a change.
4. If proposing a new package, interface, design pattern, tool, cache, queue, or dependency, report: real problem/AC; smallest alternative; why it fails; chosen approach; added complexity; exact evidence/test; revisit trigger. If a simple function or switch works, choose that.
5. For refactoring: list observable behavior that must remain, baseline tests and contract, then perform small changes. Distinguish a behavior-changing feature/fix from refactoring.
6. For M/L system work: ask which consistency boundary, failure mode, timeout, idempotency, data ownership, tenant isolation, recovery, and observability need verification. Use `mamony-ledger-review` or `mamony-boundary-review` only when their scope is touched.
7. Finish with: **Decision**, **Trade-offs**, **Changed scope**, **Evidence** (`PASS`, `FAIL`, `NOT RUN`), **Open risks**. Never claim success of a command that was not executed.

This skill is advisory. Accepted Mamony PRD/ADR/TD and current executed tests outrank generic style heuristics. Do not read entire roadmap/design-pattern pages into the model context for routine tasks.
