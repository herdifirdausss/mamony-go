# Mamony Go contributor instructions

Mamony Go is a synthetic simulator. Use test identities, funds, providers, and credentials.

Before making a feature, behavior, API, database or trust-boundary change, use the [change lifecycle](docs/development-workflow.md). Classify the change as Routine, Feature or Critical, validate research and acceptance criteria, and obtain explicit owner/technical acceptance of Critical PRD and technical design before coding. Do not invent missing requirements or treat the agent's own review as independent approval.

1. Read the issue and the relevant [contract](docs/index.md). Use the [roadmap](docs/roadmap.md) for prerequisites and [open decisions](docs/open-decisions.md) for unresolved choices.
2. Keep each capability under `internal/modules/<name>`. Put composition in `internal/app`. See the [architecture](docs/architecture.md).
3. Use the [engineering practices](docs/engineering.md). Load the matching [repository skill](.agents/skills/) when needed.
4. Run targeted tests and `make check`. Run `make race` for shared-memory changes. Use real PostgreSQL tests for grants, transactions, and database contention once the fixtures exist.
5. Record executed checks and remaining risks in the [PR template](.github/pull_request_template.md). For a change to a project decision, propose an [ADR](docs/adr/README.md).
6. For any security-sensitive change, identify the attacker capability, authoritative owner, surviving trust assumptions, denial paths, and concrete negative tests. Use the [security boundaries](docs/architecture.md#security-authority-and-trust-boundaries) and [verification matrix](docs/engineering.md#security-verification). Never claim a planned control works or a scanner establishes financial/identity security.
