---
name: mamony-boundary-review
description: Use when changing Mamony Go module structure, imports, repositories, transactions, adapters or composition. Not for prose-only edits.
---
# Mamony boundary review

1. Read the actual Go package tree, ADR-0027, and the package policy in the engineering standards.
2. Determine the owner capability and list direct new dependencies; check `go list -json ./...` and the configured architecture checker where present.
3. Confirm: capability internals are protected by nested `internal`; other capabilities do not import private internals; cross-capability integration is through app adapters/facades/ports; domain never imports HTTP/SQL/pgx; usecase never imports concrete repository.
4. Confirm repository SQL only accesses owner schema via runtime role unless an approved extension point says otherwise. For transactional work, inspect unit-of-work ownership and RLS tenant context.
5. Report for each finding: import or data flow, precise path, violated rule/ADR, effect, smallest fix, and verification command/result.
6. Do not invent missing repo structures or claim automated checks succeeded if they have not run.
