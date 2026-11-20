# Agent skill A/B evaluation v5

Source: user-provided `Mamony_Engineering_Standards_Pack_v5`, `skill-eval-cases.md`. This is a **benchmark plan**, not executed evidence.

| Task | Independent expected assertion |
|---|---|
| S: change a nonfunctional error description | No unwanted changes; documentation/targeted checks as applicable |
| M: propose an unnecessary provider interface | Reject over-abstraction without a real contract seam; justify simplest alternative |
| M: refactor payment handler into usecase | Behavior-preserving contract tests; SQL/permission semantics unchanged |
| M: add a payment API field | Match AC, OpenAPI, contract test, and capability imports |
| L: concurrent C01 replay | One journal/outbox; different-payload rejected; real PostgreSQL with >= 2 connections |
| L: runtime tenant context | Tenant A cannot read B, missing tenant fails closed, pool reuse safe |
| L: migrate payment attempt schema | Fresh and upgrade migrations pass; grants retained; recovery plan tested |

Record for every trial: model/agent version; skill name + commit; input/output/cached tokens if available; tool calls; duration; cost; AC pass count; executed tests and outcomes; reviewer findings. Compare matched tasks and repeat trials. Output compression is **not** equal to overall session token savings.
