# Architecture

## Running packages

Only `cmd/mamony` is started by the default HTTP run. Other commands and internal candidates are separately invoked or unmounted; their presence is not deployment evidence. See [current state](current-state.md).

| Package | Responsibility |
|---|---|
| `cmd/mamony` | Read listener addresses and handle process signals. |
| `cmd/mamony-identity-mail-worker` | Independently launched, double-opt-in Mailgun delivery worker; strict private-file secrets, no public routes. |
| `cmd/mamony-identity-maintain` | Separately opt-in, single-batch retention erasure; not deployed or scheduled. |
| `cmd/mamony-identity-reconcile` | Separately invoked Identity reconciliation operations. |
| `cmd/mamony-identity-notice-health` | Separate Identity notice health inspection. |
| `cmd/mamony-identity-security-notice-worker` | Separate opt-in security-notice worker; no claim of deployment or real delivery. |
| `cmd/mamony-migrate`, `cmd/mamony-identity-migrate` | Separate migration entrypoints, not invoked by HTTP startup. |
| `internal/app` | Start and stop the two HTTP listeners; contains an unmounted Ledger reference composition helper, not called by `Run`. |
| `internal/platform/httpedge` | Serve liveness and readiness. |
| `internal/platform/migration` | Standalone versioned platform and Identity migration verification, with separate schema roles and migration histories; never invoked by HTTP startup. |
| `internal/platform/postgres` | Bounded runtime-only pgxpool plus transaction primitives; Identity uses a dedicated least-privileged pool, not wired to HTTP. |
| `internal/platform/reference` | Curated immutable synthetic currency/country reference catalog; not mounted. |
| `internal/modules/identity` | Internal enrollment, authentication, sessions, recovery, security notices and WebAuthn verification through v35; not mounted in the default HTTP process. |
| `internal/modules/ledger` | Isolated monetary domain and USD-only reference admission; no posting persistence or runtime wiring. |

| Listener | Default | Routes |
|---|---|---|
| Public | `127.0.0.1:8080` | `GET /livez` |
| Private | `127.0.0.1:8081` | `GET /livez`, `GET /readyz` |

Set `MAMONY_PUBLIC_ADDR` and `MAMONY_PRIVATE_ADDR` to override the addresses. The private listener accepts `127.0.0.1` and `::1`. It maps `localhost` to `127.0.0.1`. Readiness reports only HTTP process readiness.

## Capability packages

[ADR-0001](adr/0001-go-modular-monolith.md) describes the planned package structure. Add each capability as work reaches it.

- Place the capability facade at `internal/modules/<capability>`.
- Keep the domain, use cases, repositories, and handlers private under the capability's `internal/` directory.
- Place cross-capability adapters in `internal/app`. Keep domain rules independent of HTTP and SQL. Give use cases their own ports.
- Add a feature subpackage when it has a distinct responsibility. Run `make architecture` after an import change.

See the [data ownership decision](adr/0002-ledger-and-data-ownership.md), [public HTTP contract](adr/0003-public-http-contract.md), and [roadmap](roadmap.md) for planned work.

## Security authority and trust boundaries

**Current HTTP implementation:** only the health listeners run. Internal Identity enrollment, sessions, recovery, notice and WebAuthn candidates through v35 are merged but not publicly mounted. No deployed Mailgun worker, end-to-end tenant/resource authorization, passkey login, or financial approval/posting enforcement is established. The wider security boundaries below remain design contracts, not deployed controls. Mamony uses synthetic identities, balances and providers; these requirements do not imply regulatory compliance or production readiness.

The initial modular-monolith security model assumes the application runtime and its authoritative database remain trusted (**Profile A**). It does **not** claim to resist an arbitrary writer to that authority database or complete application compromise. Before claiming stronger protection (**Profile B**), choose and verify independently controlled identity, approval, and execution authority, including their persistence, deployment administration, key-operation permissions, and recovery roots. Signed rows, checksums, and KMS alone cannot establish fresh, authorized current state when the signing caller or current-state authority is compromised. Multi-region fencing (**Profile G**) is deferred.

| Boundary | Authority and obligation | Never trust solely |
|---|---|---|
| Public HTTP edge | Parse bounded inputs; authenticate the channel; pass a verified caller context | Client tenant, role, account-owner, verification flag, proxy headers |
| Identity capability | Authoritative subject, session, credential, recovery and revocation state | Business read projection or an old signed credential record |
| Owning capability | Authorize **subject + action + resource + tenant + context** on every operation | Successful login, possession of an object ID, or RLS alone |
| Ledger | Sole authority for immutable journals, balance updates, posting replay and monetary constraints | Provider callback, business balance projection, or caller-supplied approval flag |
| Financial workflow | Immutable intent, applicable policy and approval, recipient and outcome transitions | A generic OTP success or a timed-out provider request |
| Provider connector | Per-provider signed-message verification, durable operation identity and bounded egress | Arbitrary callback data or user-selected outbound URL |
| PostgreSQL | Limited runtime grants, capability schema boundaries and tenant-table RLS where applicable | A global superuser, owner role, or client-supplied tenant context |
| Security audit / notification | Durable evidence and required security-event obligations; enforce access separately | Mutable marketing preferences or best-effort application logs |
| CI, deployment and keys | Least-privilege build, runtime, migration and key-purpose capabilities | The assumption that a scanner or KMS blocks an already-privileged caller |

Threat-driven checks are owned by the affected contract rather than by a generic security module:

- **Cross-tenant access, BOLA/BFLA, mass assignment:** [identity](contracts/identity.md#authorization-and-approvals) and [PostgreSQL](contracts/postgresql.md#tenant-isolation)
- **Credential substitution, stale sessions, recovery bypass:** [identity lifecycle](contracts/identity.md#authentication-and-lifecycle)
- **Replay, balance races and altered financial intent:** [ledger](contracts/ledger.md#posting-and-replay) and [payments](contracts/payments-and-payouts.md)
- **Forged callbacks, ambiguous dispatch and SSRF:** [provider](contracts/provider.md)
- **Secret misuse, audit suppression, restore rollback:** [platform](contracts/platform.md), [operations](contracts/reconciliation-and-events.md#operator-workflow-and-audit) and [recovery procedures](runbooks/recovery.md)

Document the attacker foothold, surviving trusted authority, expected refusal, and concrete test for every new security claim. The [verification rules](engineering.md#security-verification) require evidence for implemented capabilities and `NOT RUN` for unimplemented controls. Record unresolved choices in the [decision register](open-decisions.md#security-architecture-decisions).

### Identity HTTP enrollment candidate (Draft PR #31)

PR #31 is merged; the historical heading is retained for deep links. `internal/modules/identity/internal/handler/enrollment_http.go` is a nonmounted, manually constructed `http.Handler` for two proposed JSON endpoints. It requires direct TLS (no trusted `X-Forwarded-Proto`), exact operator-configured Origin/Host, strict 2KB JSON without duplicate/unknown keys, no cookies or Authorization, and already implemented anonymous HMAC admission. Internal `EnrollmentFlow` connects new pending subjects to the encrypted proof issuer and restricts resend to server-resolved UNVERIFIED subjects. No public listener routes changed. The optional browser-facing workflow is **not safe to deploy** until a reviewed TLS proxy boundary, blocklist, latency obfuscation, async reconciliation/monitoring, real provider quota and frontend proof-consumption flow exist.
