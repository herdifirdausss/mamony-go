# Documentation

## Start by task

| Task | Read in this order |
|---|---|
| Understand what Mamony Go does | [Product scope](product.md) → [current state](current-state.md) → [running packages](architecture.md#running-packages) |
| Run the application or local checks | [Run locally](../README.md#run-locally) → [verification commands](engineering.md#verification) |
| Plan or implement a change | Tracking issue → [change lifecycle](development-workflow.md#choose-a-change-path) → owning contract below → relevant ADR or proposal → [testing strategy](testing.md) |
| Check a decision or prerequisite | [Decision lookup by capability](open-decisions.md#decision-lookup-by-capability) → linked decision or ADR for its status and scope |
| Record review evidence or approval | [Verification commands](engineering.md#verification) → [PR template](../.github/pull_request_template.md) |
| Report a security vulnerability | [Security policy](../SECURITY.md) |
| Review security or recovery evidence | [Trust boundaries](architecture.md#security-authority-and-trust-boundaries) → owning contract → [security verification](engineering.md#security-verification) → linked PR and test results |
| Run a procedure | Choose a runbook below and check its scope and prerequisites first. |

## Read by capability

| Scope | Requirements and decisions | Design, evidence, or procedure |
|---|---|---|
| Platform foundation and reference data | [Platform contract](contracts/platform.md), [configuration data and operations](contracts/platform.md#configuration-data-and-operations) | [Current state](current-state.md) for merged slices and remaining scope |
| PostgreSQL and migrations | [PostgreSQL contract](contracts/postgresql.md), [DEC-01](open-decisions.md#foundation-and-access) | [Migration runbook](runbooks/platform-migrations.md), [proposed native executor](proposals/native-pgx-migration-executor.md), [ADR-0042](adr/0042-context-aware-migration-executor.md) |
| Identity enrollment and contact verification | [Accepted contact-verification scope](contracts/identity.md#internal-contact-verification--accepted-narrow-phase-1-scope-idf-02), [Phase 1 owner decision](open-decisions.md#owner-approval-record--narrow-identity-phase-1-direction) | [Enrollment design](proposals/identity-enrollment-auth.md), [first slice](proposals/identity-enrollment-first-slice.md), [delivery](runbooks/identity-delivery.md), [reconciliation](runbooks/identity-reconciliation.md), [browser evidence](runbooks/identity-browser-e2e.md) |
| Identity password login and sessions | [Authentication and lifecycle](contracts/identity.md#authentication-and-lifecycle), [Identity ADRs](adr/README.md) | [Login performance evidence](runbooks/identity-login-performance.md), [browser session evidence](runbooks/identity-browser-e2e.md) |
| Authenticated password change and security notices | [Identity contract](contracts/identity.md), [ADR-0013](adr/0013-authenticated-password-change.md), [ADR-0014](adr/0014-password-change-security-notices.md) | [Delivery runbook](runbooks/identity-delivery.md), [browser evidence](runbooks/identity-browser-e2e.md) |
| Account recovery and saved codes | [Identity contract](contracts/identity.md), [ADR-0021](adr/0021-password-recovery-authority-foundation.md), [ADR-0022](adr/0022-password-recovery-issuance-outbox.md) | The recovery ADRs are proposals; check the contract and decision status before implementation. |
| Identity passkey possession | [Bounded possession contract](contracts/identity.md#bounded-synthetic-passkey-possession), [ADR-0041](adr/0041-passkey-atomic-activation.md) | [Synthetic possession design and evidence](proposals/identity-passkey-atomic-activation.md) |
| Database restore, Identity reconciliation, and recovery fencing | [Platform restore requirements](contracts/platform.md#edge-and-recovery), [Identity recovery controls](contracts/identity.md#security-state-and-recovery-threat-controls) | [Recovery runbook](runbooks/recovery.md), [native PITR lab](proposals/platform-native-pitr-lab.md), [RH-03 reconciliation lab](proposals/platform-identity-reconciliation-lab.md), [Profile A fencing design](proposals/identity-restore-fencing.md), [recovery-fence proposal](proposals/identity-recovery-fence.md), [recovery-session proposal](proposals/identity-recovery-session-capability.md) |
| Ledger | [Ledger contract](contracts/ledger.md), [ADR-0002](adr/0002-ledger-and-data-ownership.md) | [Ledger stages](roadmap.md#ledger-implementation-sequence), [K2a persistence record](proposals/ledger-k2a-isolated-persistence.md), [broader financial design proposal](proposals/ledger-cross-border-payments.md) |
| Screening and onboarding | [Screening and onboarding contract](contracts/screening-and-onboarding.md) | [Pilot order and prerequisites](roadmap.md#simulated-pilot) |
| Fees, limits, and accounts | [Fees, limits, and accounts contract](contracts/fees-limits-and-accounts.md) | [Pilot order and prerequisites](roadmap.md#simulated-pilot) |
| Provider connector | [Provider contract](contracts/provider.md) | [Pilot order and prerequisites](roadmap.md#simulated-pilot) |
| FX | [FX contract](contracts/fx.md) | [Pilot order and prerequisites](roadmap.md#simulated-pilot) |
| Payments and payouts | [Payments and payouts contract](contracts/payments-and-payouts.md) | [Pilot order and prerequisites](roadmap.md#simulated-pilot) |
| Reconciliation and events | [Reconciliation and events contract](contracts/reconciliation-and-events.md) | [Pilot order and prerequisites](roadmap.md#simulated-pilot) |
| Fraud and monitoring | [Fraud and monitoring contract](contracts/fraud-and-monitoring.md) | [Operations and expansion order](roadmap.md#operations-and-expansion) |
| Settlement and accounting | [Settlement and accounting contract](contracts/settlement-and-accounting.md) | [Operations and expansion order](roadmap.md#operations-and-expansion) |
| Disputes and sandbox | [Disputes and sandbox contract](contracts/disputes-and-sandbox.md) | [Operations and expansion order](roadmap.md#operations-and-expansion) |
| Split payments | [Split payments contract](contracts/split-payments.md) | [Operations and expansion order](roadmap.md#operations-and-expansion) |
| Public HTTP | [Public HTTP decision](adr/0003-public-http-contract.md), [running packages](architecture.md#running-packages) | [Current state](current-state.md) records the default listener behavior. |

The roadmap records dependencies and stage order. Read the [decision lookup by capability](open-decisions.md#decision-lookup-by-capability) before implementation to find unresolved choices.

For Identity, **account recovery** covers user account recovery and saved factors. The database recovery row covers restore, reconciliation, and stale-authority controls across Platform and Identity. Each scope has its own contract and release gates.

For recovery design review, start with the Platform lab for database observations, then read the Identity fence proposal for temporary-authority policy, and the recovery-session proposal for the session-specific design. Use each proposal's current disposition for blocker status.

## Find the source for each claim

| Question | Source |
|---|---|
| What should a capability do, and how do we accept it? | The applicable section of its owning contract under `contracts/`. Some sections describe proposed or future behavior. Read their status before using them. |
| Why did we choose this design? | The applicable [ADR](adr/README.md), with its decision status and scope. |
| What work is next, and what is unresolved? | The [roadmap](roadmap.md) for dependencies, the [decision lookup](open-decisions.md#decision-lookup-by-capability) for unresolved choices, and the tracking issue for live coordination. |
| What merged, what runs, and what was tested? | [Current state](current-state.md) for the dated baseline. Use the linked code, PR, and test results for exact evidence and scope. |
| Where is the research, PRD, or technical design? | The linked proposal in `proposals/` for Critical work. Use the issue or PR for smaller changes. [Development history](history.md) links earlier research and verification records. |
| How should contributors write and verify changes? | [Go conventions](conventions.md), [engineering practices](engineering.md), [testing strategy](testing.md), and [tooling](tooling.md). |

## Follow a change

Use the [lifecycle gates](development-workflow.md#lifecycle-and-quality-gates) for the applicable change path. Start with the issue and research the uncertain claims. Record the required behavior and acceptance IDs in the owning contract. Link the accepted plan, proposal or ADR when the path requires one. In the implementation PR, map acceptance IDs to tests and record the executed checks. Update the dated baseline when a material milestone or acceptance status changes.

Use the [proposal template](templates/feature-proposal.md), [test-plan checklist](templates/test-plan.md), and [PR template](../.github/pull_request_template.md) at the corresponding gates. For AI-assisted work, follow [AGENTS.md](../AGENTS.md) and prepare a small [task context](../.agents/skills/mamony-context-pack/SKILL.md) with the relevant headings, accepted IDs, code paths, tests, and blockers.
