# Implementation roadmap

The Go binary runs two health listeners; internal Platform, Identity and Ledger synthetic slices also exist. See [current state](current-state.md) for merged implementation and open work, and [architecture](architecture.md) for runtime composition. The contracts specify intended behavior, prerequisites and acceptance checks, not blanket completion.

Before beginning any new feature or architecture change, use the [change lifecycle](development-workflow.md) to validate research, requirements, design, risks and acceptance checks. The roadmap determines build order; it does not replace product acceptance, technical review or PR evidence.

## Security prerequisites by stage

Security is integrated into the owning capability's contract rather than a separate feature milestone. Before implementing a non-health endpoint, resolve the applicable [edge](adr/0003-public-http-contract.md#input-admission-and-browser-boundary) and [security decisions](open-decisions.md#security-architecture-decisions). The current health-only process mounts no user authentication or payment API; unmounted internal Identity implementations do not satisfy this release gate.

- **Foundation gate:** enforce bounded input and trusted routing on introduced endpoints; keep build/runtime/migration permissions narrow; state the attacker model. Existing static checks do not prove future identity or ledger safety.
- **Identity gate:** establish subject/session authority, recovery and revocation policy, resource-level authorization and negative tenant tests. Prove [PostgreSQL runtime isolation](contracts/postgresql.md) when database tables exist.
- **Money gate:** verify an authorized exact financial intent when required; test atomic ledger replay, balance-floor races, approval reuse, uncertain results, and restore with actual database fixtures.
- **Provider/operations gate:** verify provider-specific callback signatures and replay protection, destination/egress restrictions, durable audit/notice obligations, and reconciliation after failure.

Use the [security verification matrix](engineering.md#security-verification) and report unimplemented controls as `NOT RUN`. Defer the independent-authority and multi-region profiles until their risk and operating assumptions justify them.

## Foundation, Identity, and ledger

| Order | Contract | Prerequisites and outcome |
|---:|---|---|
| 1 | [Platform foundation](contracts/platform.md) | DEC-01 accepted; synthetic migration tooling, runtime pool, recovery lab and reference catalog merged. Full recovery, capability authorization integration, operational configuration, audit/event relay and production evidence remain open. |
| 2 | [Identity Phase 1](contracts/identity.md) | Internal enrollment, sessions, recovery and one-use passkey possession verification are merged for synthetic use. Public activation requires resource authorization, recovery and operational acceptance. |
| 3 | [Ledger](contracts/ledger.md) | Monetary domain, reference admission and isolated persistence are merged for bounded synthetic use. See the [Ledger implementation sequence](#ledger-implementation-sequence) for the remaining stages. |

K1 and the reference experiments preceded full Foundation and Identity acceptance. The owner separately approved a one-time sequencing exception for K2a. The existing capability delivery order and Ledger gate still apply. Pass the Ledger gate before building pilot money flows. Extend the catalog and operator controls under [reconciliation and events](contracts/reconciliation-and-events.md).

### Ledger implementation sequence

| Stage | Scope | Status at the documented baseline |
|---|---|---|
| K1 | Monetary domain and exact journal invariants | Merged as a [bounded domain experiment](contracts/ledger.md#k1-isolated-monetary-domain-synthetic-experiment). |
| K2 | Ledger-owned schema, role separation and account lifecycle | Full stage remains planned. |
| K2a | Isolated persistence foundation within K2 | Merged as a [bounded synthetic exception](proposals/ledger-k2a-isolated-persistence.md). It covers the initial schema, fixed USD definition and disposable storage and restore fixtures. Account lifecycle and runtime wiring remain outside this slice. |
| K3 | Atomic authorized journal posting, replay and outbox creation | Planned after K2 and accepted posting authority. |
| K4 | Full reversal and authorized statement reads | Planned after K3. |
| K5 | Per-account outbox delivery and consumer replay | Planned after K3 and verified relay prerequisites. |
| K6 | Independent Ledger verification and restore acceptance | Planned after posting, reversal and delivery evidence. |

Older K2a records use `K2b` to refer to follow-on work outside the synthetic exception. Its scope requires a separate design and approval. The [detailed design proposal](proposals/ledger-cross-border-payments.md#ledger-implementation-plan-stage-k) records dependencies and candidate acceptance checks. See [current state](current-state.md) for merge and activation status.

## Simulated pilot

| Order | Contract | Prerequisites and outcome |
|---:|---|---|
| 4 | [Screening and onboarding](contracts/screening-and-onboarding.md) | Use identity and approval. Implement screening before onboarding. |
| 5 | [Fees, limits, and accounts](contracts/fees-limits-and-accounts.md) | Use the ledger, identity, and onboarding. Implement policy before account activation. |
| 6 | [Provider connector](contracts/provider.md) | Use configuration, durable jobs, and event delivery. This can run in parallel with identity, screening, and policy work. |
| 7 | [FX](contracts/fx.md) | Use ledger posting, account mappings, and fee contracts. |
| 8 | [Payments and payouts](contracts/payments-and-payouts.md) | Use the preceding pilot contracts. Build one end-to-end flow at a time. |
| 9 | [Reconciliation and events](contracts/reconciliation-and-events.md) | Build audit and catalog interfaces earlier. Reconcile the completed flows before the pilot. |

Pass the pilot gate on one simulated corridor. Exercise both money flows, screening, unknown provider outcomes, reversals or returns, daily reconciliation, worker recovery, and restore. Use the [open decisions](open-decisions.md) to set the corridor, observation window, and performance targets.

## Operations and expansion

| Order | Contract | Prerequisites and outcome |
|---:|---|---|
| 10 | [Fraud and monitoring](contracts/fraud-and-monitoring.md) | Use stable money events, outcome labels, and reviewer roles. |
| 11 | [Settlement and accounting](contracts/settlement-and-accounting.md) | Use reconciled flows, durable payout results, and rebuildable history. |
| 12 | [Disputes and sandbox](contracts/disputes-and-sandbox.md) | Define provider dispute and evidence interfaces and tenant-scoped API and event contracts. |
| 13 | [Split payments](contracts/split-payments.md) | Use sub-merchant onboarding, settlement, and reporting. |

The operations gate covers repeatable settlement, a close without unexplained differences, reviewed monitoring alerts, measured fraud controls, and a hot-account load test. Set the pass thresholds before the test. Billing, cards, and savings remain deferred.

Use requirement IDs from the relevant contract in the [PR template](../.github/pull_request_template.md). Follow the [architecture decisions](adr/README.md) when choosing a design.
