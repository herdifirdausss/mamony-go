---
name: mamony-context-pack
description: Use before implementing a Mamony Go issue when the agent needs precise requirements and decisions and a small task context. Do not run for generic unrelated questions.
---
# Mamony context pack

1. Read the issue or change brief. Use the [documentation index](../../../docs/index.md) to find local decisions and contracts.
2. Identify the affected capability, risk class S/M/L, acceptance criteria and requirement IDs. Do not invent IDs.
3. Read the task-specific headings in local decisions and contracts. Expand to neighboring modules only when the change crosses a boundary.
4. Distinguish confirmed source decisions from proposed or conflicting items. For money, auth, public compatibility and irreversible writes, expose blocking conflicts rather than guessing.
5. Return a task pack with exactly these fields: Goal; Scope; AC IDs; Sources (path and heading); Invariants; Allowed packages; Interfaces/contracts; Checks; Unresolved decisions.
6. Keep the pack concise, link rather than paste long documents. Re-check paths against the active tree before coding.

Never let a generic third-party skill supersede Mamony approved requirements or product decisions.
