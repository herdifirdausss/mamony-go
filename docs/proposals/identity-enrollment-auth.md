# Identity Phase 1 — Secure enrollment, login and authorization

**Tracking issue:** [#20](https://github.com/herdifirdausss/mamony-go/issues/20). **Status:** The narrow first-party Profile A implementation direction and email-verification-before-activation were approved by the owner on 2026-11-21. PRs #22–#24 merged for internal synthetic use. Public enablement is NOT ACCEPTED. See the [Identity contract](../contracts/identity.md#enrollment-and-contact-verification) for the accepted requirements and the [release decisions](../open-decisions.md#identity-enrollment-release-decisions) for the outstanding operational scope. The proposal below preserves the original design review, including its candidate requirements. **Scope:** Mamony Go, one-process modular monolith; first-party browser customer flow with synthetic users. **Review date:** 2026-11-21.

## G0 — Problem, actors and goal

The current Go binary exposes only health endpoints. Its PostgreSQL migration runner and runtime connection-pool/RLS fixture are verified, but neither proves who an HTTP caller is or whether a user may act on another user's account. A client-controlled `tenant_id` passed to `WithTenantTx` is not authorization.

**Primary actors:** an unauthenticated visitor, a registered but unverified customer, a verified active customer, a suspended customer, and a separate operator (operator sign-in remains disabled until mandatory phishing-resistant/MFA policy is implemented). Threat actors include account enumerators, credential stuffers, stolen-session holders, arbitrary SQL writers and compromised application runtimes. The first release uses synthetic customer data only.

**Desired outcome:** a user can create a pending account, prove control over its registered email, authenticate under explicit policy, obtain and revoke a bounded server session, and read only resources explicitly authorized for the authenticated subject. No identity proofing, financial account eligibility or transfer permission follows automatically from email verification or login.

**Explicit dependency/roadmap change proposed:** begin Identity Phase 1 before ledger DEC-02 because it establishes authorization context for application-owned resources. This changes the `docs/roadmap.md` order and must be accepted by the repo owner. The ledger remains independently authorized by calling capability and does not trust a valid browser session as permission to post money. Do not silently treat the change as approved.

## G1 — Research synthesis, scope and alternatives

### Evidence source distinction

**User-supplied research baseline (design inputs, not verified software):** CIAM Stage 1 v2 (Profile A/B, attack-chain scenarios, identity vs legal proofing); Stage 2 v2 (`R-ID01..05`, `R-AU01..08`, `R-CR01..06`, `R-RE01..06`, `R-SE01..05`, `R-AZ01..`, `R-NT01..`, V01/V03–V13/V35–V38, P01–P10); Stage 3 v1 (D06 enrollment, D07 restricted recovery, D08 browser sessions and resource authorization, AD-01/04–08). The attached material also defines independent authorities and current-head anchoring for Profile B. This plan deliberately uses **Profile A as an implementation starting point** while preserving Profile B as a separately gated extension. It **does not** claim Stage 2 verification scenarios have passed.

**Independently checked primary resources (2026-11-21):**
- NIST SP 800-63B-4 passwords, authenticator and session requirements: https://pages.nist.gov/800-63-4/sp800-63b/authenticators/ and https://pages.nist.gov/800-63-4/sp800-63b/session/
- OWASP ASVS 5.0.0 V6/V7/V8: https://asvs.owasp.org/ (specific checks are adopted only when applicable).
- OWASP password storage, authentication, session, CSRF and forgot-password cheat sheets: https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html ; https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html ; https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html ; https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html ; https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html
- Candidate Go primitives: https://pkg.go.dev/golang.org/x/crypto/argon2 and https://github.com/go-webauthn/webauthn ; no crypto protocol will be hand-implemented.
- Mature self-service alternatives: https://www.ory.com/docs/oel/kratos/quickstart ; https://zitadel.com/docs/guides/integrate/onboarding/end-users ; https://www.keycloak.org/docs/latest/server_admin/ . Evaluate licenses, security update burden, integration and provider lock-in for any real deployment.

**Current repo facts:** `docs/contracts/identity.md` is an unimplemented owning contract; `internal/platform/postgres` only validates the runtime connection/transaction primitive using a disposable test table; `cmd/mamony` has no identity routes; DEC-03 and DEC-13 through DEC-18 remain unresolved. A separate PostgreSQL migration runner exists for the **platform** schema; no identity schema or owner/runtime grants exist yet. `docs/development-workflow.md` requires Critical G2/G3 owner acceptance before this implementation.

### Implementation alternatives

| Option | Strength | Cost/trust limitation | Candidate status |
|---|---|---|---|
| Integrate managed or self-hosted Ory Kratos / ZITADEL / Keycloak | Mature self-service registration/login/recovery/passkey features; avoid custom auth protocol | Operates an extra security-critical deployment, identity mapping and updates; details/licensing differ; opaque session and authorization bridge still need review | **Viable production alternative; do not claim rejected on security grounds** |
| Limited, repository-owned Go Identity capability | Best direct learning of DB state, transactions, admission, password and session behavior, using reviewed crypto/WebAuthn libraries | More maintenance and ATO/recovery attack surface; must not be labelled production-grade without independent assessment | **Proposed for synthetic Mamony learning Phase 1** |
| Build a new OAuth/OIDC issuer, invent passkey verification, or store browser JWT as first step | None justified for a single first-party browser | Larger protocol/security surface; JWT revocation and account changes are harder | **Rejected for Phase 1** |
| Deploy full independent Profile B current-head/approval/execution cells immediately | Stronger containment for selected DB-writer/business-app compromise threats **if** independent roots actually exist | Fundamentally new trust/admin/storage/credential boundary, recovery and availability requirements; a fake microservice split gives no such guarantee | **Deferred until an explicit Profile B threat/deployment decision** |

## G2 — Proposed product contract and falsifiable acceptance

**Phase 1 client:** one first-party browser over HTTPS; API keys, mobile refresh tokens, OIDC/social login, SMS/WhatsApp OTP, WebAuthn/passkeys and operator login are disabled until separately enabled/tested. Use standard `net/http` routing unless a framework-specific need is demonstrated. No unbounded external identity-provider callback route is exposed.

| Candidate AC | Behavior | Negative/observable acceptance |
|---|---|---|
| IDF-01 | Register one stable opaque subject ID and a distinct email alias/verification version in `UNVERIFIED`; email isn't subject ID or tenant permission | Races and duplicate-alias attempts never create second ownership; registry cannot assign `ACTIVE` from client payload |
| IDF-02 | Confirm email using a high-entropy, purpose/subject/value-version-bound, expiry-limited single-use challenge; deliver via a reviewed adapter | Old/replayed/wrong-subject/wrong-email/version challenges deny; concurrent consume has one winner; no contact proof implies legal identity/KYC |
| IDF-03 | Password enrollment follows P01: 15–128 Unicode code points, at most 512 UTF-8 bytes, **NFC-normalized input** (no trim/case-fold/silent truncation; the original no-NFC baseline is superseded for newly issued credential verifiers) and common/breached/contextual blocklist | Unicode composed/decomposed, whitespace, overlong, invalid encoding and listed passwords tested; all valid bytes used consistently |
| IDF-04 | Hash with Argon2id, unique random salt and versioned bounded params; private server-side admission/KDF concurrency and timing/error policy | DB dump has no plaintext; corrupted/too-expensive params are refused; load test enforces a measured finite resource budget; no password/verifier leaks |
| IDF-05 | Login resolves an existing **active, verified** subject, validates the credential and status, and only then issues a full session under server authority | Unknown/wrong/unverified/suspended accounts get generic failure without account enumeration (including status/body/timing checks); no MFA or role claims from client |
| IDF-06 | Use a 32-byte CSPRNG opaque bearer; persist only SHA-256 verifier/digest; send `__Host-...` Secure+HttpOnly+Path=/+SameSite=Lax first-party cookie | Stolen DB session verifier is not a bearer; no URL/localStorage tokens; session fixation and cookie scope tests |
| IDF-07 | Server enforces proposed Profile A: idle 30 min; absolute 12 h; 5 concurrent sessions max; rotate on full reauthentication and revoke current/all | Test fake clock, session issuance races/oldest eviction, logout, inactive/suspended subject and revoked session; no dependence on client time |
| IDF-08 | State-changing cookie-authenticated routes enforce origin and CSRF defenses; strict bounded JSON/schema, content type, method, request body and safe error handling | Forged cross-origin POST, duplicate JSON keys, bad media type, oversized inputs and spoofed proxy header deny |
| IDF-09 | A subject/action/resource/tenant decision is explicit, default deny, before repository's `WithTenantTx` and independently of session validity | BOLA: subject B cannot read/change A by path ID, alternate tenant ID, nested/export paths; permissions revoked/status frozen deny; UUID is not access control |
| IDF-10 | Commit security-state change with mandatory audit/event obligation; notification delivery is separately acknowledged, not faked by queued event | Failed obligation write aborts security mutation; provider outage doesn't silently discard or claim user read a notice; secrets absent from logs |
| IDF-11 | Self-service recovery and factor changes are **not available** until guarded restricted-state and revocation semantics are separately accepted | No password reset/support flag, factor downgrade or unverified email callback can mint full session; disabling MFA by editing profile has no active path |
| IDF-12 | Runtime/migrator credentials for identity-owned schema and required CI named integration tests follow existing gates | Runtime role cannot own/create/change auth schema or read migration history; SQL constraint/transaction/RLS/permissions and negative tests run on real PostgreSQL |

**Important:** these IDs live in this **proposal** until G2 approval, after which the accepted stable ACs belong in `docs/contracts/identity.md` rather than creating another authoritative register.

## G3 — Proposed technical design and threat review

### Module and trust boundaries

```text
Untrusted browser
  → public HTTP ingress (fixed trusted origin, request admission/CSRF)
  → identity handler (no SQL, secrets redacted)
  → identity usecase (status/verification/authorization decisions)
  → identity repository port (identity schema, tenant/session state)
  → dedicated identity runtime pgxpool and PostgreSQL
                    │
                    └→ durable event + mandatory notification obligation

Admin/CI provisioner → dedicated schema owner/migrator (never in HTTP app)
```

- Use `internal/modules/identity/` as owning capability with `internal/handler`, `internal/usecase`, and `internal/repository` only where needed. Public facade exposes typed operations and an authenticated principal/decision, not a writable user record. App wiring remains in `internal/app`.
- Identity owns tenant/subject/alias/contact/credential/session records, status and membership. Other modules own their resources and must verify permission **again** at resource use. A tenant string from HTTP is never an authority.
- Schema: one new `identity` capability schema with dedicated owner/migrator/runtime and narrow grants; a reviewed migration approach must extend the existing `platform` migration runner without silently sharing owner credentials. Candidate tables: subjects, aliases/verified contacts, password credentials, one-use challenges, sessions, memberships, audit/outbox obligations. **Do not create tables from this draft until G2/G3 acceptance.**
- Challenge token and session bearer verifiers store hashes; tokens use CSPRNG. Email verification ties exact email value/version/subject/purpose. Registration must be safe against races, duplicate emails and user enumeration; no auto-login before verification.
- A password-verifier substitution by arbitrary Identity DB writer **is not contained** in Profile A; row hashing, RLS and password pepper alone cannot prove authorized transition/currentness against that writer. Harden roles/audit/recovery but retain residual risk. Profile B would need an **independent trusted current-head/command root**, not a different table in the same admin domain.

### Flow outlines

1. **Register:** admission → alias lookup/reservation with unique constraint → subject `UNVERIFIED` and candidate password verifier under bounded KDF → contact challenge record + durable delivery obligation commit → generic response. Failure to persist required obligation prevents claiming registration completed. Don't return verification token publicly.
2. **Verify email:** retrieve digest-scoped challenge and current alias version under transaction → compare/consume once → set this exact contact version verified and subject `ACTIVE` only if all configured enrollment requirements are satisfied → rotate/revoke enrollment challenge state and emit event → commit; do not issue full session merely because email is verified.
3. **Login:** admission/throttle before bounded KDF → generic known/unknown password path → verify correct password and current active/verified status → apply operator MFA guard (operator login disabled initially) → atomically issue full session and evict oldest if 5-session limit → set cookie; use dummy KDF where appropriate and no distinguishable existence errors.
4. **Authenticated request:** read opaque cookie → verify hash against authoritative session, absolute and idle expiry, subject status and revocation generation → resolve action/resource/tenant from **trusted** backend state → explicitly authorize → only then call repository and tenant transaction. 
5. **Logout:** revoke authoritative session, invalidate cookie; logout-all revokes active sessions atomically. Sensitive contact/factor changes, password recovery and payment actions remain disabled until their dedicated guarded flows exist.

### Threat/verification priorities

| Attack | Countermeasure to verify | Source mapping |
|---|---|---|
| Register duplicate / email enumeration | Unique alias reservation + generic public responses + request throttling | Stage 2 R-ID01/R-ID05, V01/V03 |
| Password stuffing / Argon2 DoS / malicious KDF parameters | Bounded admission and KDF work, blocklists, trusted versioned params, dummy verification | R-AU02/03, R-CR01/02, R-AB01; V05/V13 |
| Steal/replay cookie or reset session state | Hashed random server sessions, rotation, idle/absolute, atomic revocation, CSRF | R-SE01–05, R-WB01, V10/V11/V35 |
| Use valid login for another account/tenant | Resource-owner authorization in owning module, NOT RLS context string alone | R-AZ01–04, V21/V22 |
| Change profile email then reset victim password | Version-bound current contact and recovery-restricted path; no automatic MFA downgrade | R-ID02, R-RE01–06, V38 |
| Rewrite password verifier/session mapping in authority DB | Profile A residual risk; no claim to withstand arbitrary authority DB writer | Stage 1 G10/11; Stage 2 R-SI08/09, V36/V37/V43 |
| Broken mail/notification worker | Durable security obligation, bounded retry and operator visibility, no claim of delivery from enqueue | R-NT01/04/05, V39 |
| Reuse older restored session after revoke | Restore invalidates or reconciles revocation/current session epoch before re-enabling traffic | R-OP03, V32/V40 |

## G4 — Deliberately small, reviewable implementation PRs after acceptance

1. **Identity storage and enrollment (no public login):** owned SQL migration/role fixture, stable subject and unique email-alias reservation, password policy and Argon2id hash with benchmark and admission bound, enrollment `UNVERIFIED`. Positive/negative real DB tests, including duplicate registration race and failed event-obligation rollback. No production mail provider required; the simulated outbox is **not** a delivered email.
2. **Contact verification:** scoped single-use challenges with value/version/subject/purpose binding; fake delivery adapter for integration fixtures, never an accessible public verification bypass; separate real provider choice before external activation. Test concurrent challenge consume and old-email replay.
3. **Login + sessions:** only after #1/#2 and accepted secure cookie/CSRF policy; bounded login admission, generic known/unknown error, 32-byte opaque sessions/hashed stored digest, TTL/rotation/revocation/5-session cap. Test actual HTTP, PostgreSQL, fake clock, stolen cookie and races.
4. **Resource authorization:** safe "/me" plus an owned resource authorization fixture/real capability only when product requirement exists; BOLA tests on HTTP and SQL, status/revocation checks, tenant context controlled by usecase.
5. **Advanced lifecycle:** recovery, passkeys/WebAuthn UV, operator MFA, security notification dispatch, approval/sensitive actions and federation **as separately accepted features**; no `login-with-token` shortcut or fake financial assurance.

**G4/G5 rules:** no test-only JWT or plaintext password storage, no `sqlmock` standing in for real PostgreSQL role/transaction evidence, no hidden dev-only endpoint in release binary. Keep `make check` database-independent; add named integration tests to the existing real PostgreSQL CI testgate before any route is considered complete. Record `PASS`/`FAIL`/`NOT RUN` per candidate AC and exact commit. Keep [Issue #14](https://github.com/herdifirdausss/mamony-go/issues/14) for platform/deployment grants and [Issue #18](https://github.com/herdifirdausss/mamony-go/issues/18) for later real usecase→repository tenant authorization, avoiding duplicate foundation work.

## Owner acceptance required at G2/G3

1. Approve **identity-before-ledger** deviation from the published roadmap for the first secure first-party browser profile.
2. Accept **Profile A trusted Identity DB** as a learning baseline; explicitly defer independent Profile B protected current heads, financial approvals and globally distributed authority, with no assertion of full DB-writer containment.
3. Select Go-owned capability versus the **mature IdP** alternative, including the ongoing security maintenance commitment.
4. Confirm registration requires email contact verification before an active login; select simulated delivery first versus a real email provider before external use.
5. Confirm P01 NFC-normalized password semantics (superseding the earlier no-NFC candidate), Argon2id candidate m=64 MiB/t=3/p=1 subject to actual load benchmark and safe fallback **not below accepted floor**, P03 30min/12h session ceilings, P10 max five sessions; reject implementation until the measured KDF resource budget is approved.
6. Approve first-party HTTPS cookie/CSRF controls and no JWT/mobile/OIDC/public operator login in Phase 1. Operator MFA and sensitive recovery are separate mandatory gates if those features become enabled.

**Until accepted:** leave `docs/contracts/identity.md`, `docs/roadmap.md` and runtime routes unchanged. This document is a design candidate, not working login or an authorization/security PASS.