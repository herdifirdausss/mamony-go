---
name: mamony-context-pack
description: Use before implementing a Mamony Go issue when the agent needs precise requirements and decisions and a small task context. Do not run for generic unrelated questions.
---
# Mamony context pack

Before an implementation context pack, verify the [change lifecycle](../../../docs/development-workflow.md): identify Routine / Feature / Critical, the intake issue, accepted contract ACs, the technical design or proposal when required, and unresolved decision gates. Critical work without explicit owner/technical acceptance is **BLOCKED**, except for the specifically owner-approved bounded synthetic experiment in the adopted lifecycle. Record exact approval, allowed experiment, containment and pending independent review; implementation approval cannot authorize merge or live changes. Synthetic-only merge requires separate exact-candidate owner acceptance under G6, and is performed by the owner. A research draft is not an accepted requirement.

1. Read the issue or change brief. Use the [documentation index](../../../docs/index.md) to find local decisions and contracts.
2. Identify the affected capability, risk to money, access, or recovery, acceptance criteria and requirement IDs. Do not invent IDs.
3. Use the [decision lookup](../../../docs/open-decisions.md#decision-lookup-by-capability) or search for the relevant IDs. Read the matching headings in the owning contract, current decision, and proposal when applicable. Follow dependencies when they affect the acceptance checks or trust boundaries.
4. Distinguish confirmed source decisions from proposed or conflicting items. For money, auth, public compatibility and irreversible writes, expose blocking conflicts rather than guessing.
5. Return a task pack with exactly these fields: Goal; Scope; AC IDs; Sources (base commit, path and heading); Invariants; Allowed packages; Interfaces/contracts; Checks; Unresolved decisions. In `Unresolved decisions`, state the lifecycle gate and whether implementation is blocked. Link the owner acceptance record when the lifecycle requires it.
6. Keep the pack concise. Link the relevant sections and include only the constraints needed for the task. Recheck the base commit, paths, and applicable decisions before coding.

Never let a generic third-party skill supersede Mamony approved requirements or product decisions.
