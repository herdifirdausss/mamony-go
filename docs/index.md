# Documentation

## Start by task

| Task | Read in this order |
|---|---|
| Understand what Mamony Go does | [Product scope](product.md) → [current state](current-state.md) → [running packages](architecture.md#running-packages) |
| Run the application or local checks | [Run locally](../README.md#run-locally) → [verification commands](engineering.md#verification) |
| Plan or implement a change | Tracking issue → [change lifecycle](development-workflow.md#choose-a-change-path) → owning contract below → relevant ADR or proposal → [testing strategy](testing.md) |
| Review security or recovery evidence | [Trust boundaries](architecture.md#security-authority-and-trust-boundaries) → owning contract → [security verification](engineering.md#security-verification) → linked PR and test results |
| Run a procedure | Choose a runbook below and check its scope and prerequisites first. |

## Read by capability

| Scope | Requirements and decisions | Design, evidence, or procedure |
|---|---|---|
| Platform and PostgreSQL | [Platform contract](contracts/platform.md), [PostgreSQL contract](contracts/postgresql.md), [DEC-01](open-decisions.md#foundation-and-access) | [Migration runbook](runbooks/platform-migrations.md), [proposed native executor](proposals/native-pgx-migration-executor.md), [ADR-0042](adr/0042-context-aware-migration-executor.md) |
| Identity enrollment, login, and sessions | [Identity contract](contracts/identity.md), [authentication and lifecycle](contracts/identity.md#authentication-and-lifecycle), [Identity ADRs](adr/README.md) | [Phase 1 design record](proposals/identity-enrollment-auth.md), [first enrollment slice](proposals/identity-enrollment-first-slice.md), [email delivery](runbooks/identity-delivery.md), [enrollment reconciliation](runbooks/identity-reconciliation.md), [browser verification](runbooks/identity-browser-e2e.md), [login benchmark](runbooks/identity-login-performance.md) |
| Identity passkey possession | [Bounded possession contract](contracts/identity.md#bounded-synthetic-passkey-possession), [ADR-0041](adr/0041-passkey-atomic-activation.md) | [Synthetic possession design and evidence](proposals/identity-passkey-atomic-activation.md) |
| Platform and Identity recovery | [Platform recovery requirements](contracts/platform.md#edge-and-recovery), [Identity recovery controls](contracts/identity.md#security-state-and-recovery-threat-controls) | [Recovery runbook](runbooks/recovery.md), [native PITR lab](proposals/platform-native-pitr-lab.md), [RH-03 reconciliation lab](proposals/platform-identity-reconciliation-lab.md), [Profile A fencing design](proposals/identity-restore-fencing.md), [recovery-fence proposal](proposals/identity-recovery-fence.md), [recovery-session proposal](proposals/identity-recovery-session-capability.md) |
| Ledger and money movement | [Ledger contract](contracts/ledger.md), [ADR-0002](adr/0002-ledger-and-data-ownership.md), [payments and payouts](contracts/payments-and-payouts.md), [FX](contracts/fx.md) | [Ledger stages](roadmap.md#ledger-implementation-sequence), [K2a persistence record](proposals/ledger-k2a-isolated-persistence.md), [broader financial design proposal](proposals/ledger-cross-border-payments.md) |
| Pilot and later capabilities | [Simulated pilot](roadmap.md#simulated-pilot) and [operations and expansion](roadmap.md#operations-and-expansion) link each owning contract. | Use the roadmap for ordering and the [decision register](open-decisions.md) for unresolved choices. |
| Public HTTP | [Public HTTP decision](adr/0003-public-http-contract.md), [running packages](architecture.md#running-packages) | [Current state](current-state.md) records the default listener behavior. |

For recovery design review, start with the Platform lab for database observations, then read the Identity fence proposal for temporary-authority policy, and the recovery-session proposal for the session-specific design. Use each proposal's current disposition for blocker status.

## Find the source for each claim

| Question | Source |
|---|---|
| What should a capability do, and how do we accept it? | The applicable section of its owning contract under `contracts/`. Some sections describe proposed or future behavior. Read their status before using them. |
| Why did we choose this design? | The applicable [ADR](adr/README.md), with its decision status and scope. |
| What work is next, and what is unresolved? | The [roadmap](roadmap.md) for dependencies, the [decision register](open-decisions.md) for unresolved choices, and the tracking issue for live coordination. |
| What merged, what runs, and what was tested? | [Current state](current-state.md) for the dated baseline. Use the linked code, PR, and test results for exact evidence and scope. |
| Where is the research, PRD, or technical design? | The linked proposal in `proposals/` for Critical work. Use the issue or PR for smaller changes. [Development history](history.md) links earlier research and verification records. |
| How should contributors write and verify changes? | [Go conventions](conventions.md), [engineering practices](engineering.md), [testing strategy](testing.md), and [tooling](tooling.md). |

## Follow a change

Use the [lifecycle gates](development-workflow.md#lifecycle-and-quality-gates) for the applicable change path. Start with the issue and research the uncertain claims. Record the required behavior and acceptance IDs in the owning contract. Link the accepted plan, proposal or ADR when the path requires one. In the implementation PR, map acceptance IDs to tests and record the executed checks. Update the dated baseline when a material milestone or acceptance status changes.

Use the [proposal template](templates/feature-proposal.md), [test-plan checklist](templates/test-plan.md), and [PR template](../.github/pull_request_template.md) at the corresponding gates. For AI-assisted work, follow [AGENTS.md](../AGENTS.md) and prepare a small [task context](../.agents/skills/mamony-context-pack/SKILL.md) with the relevant headings, accepted IDs, code paths, tests, and blockers.
