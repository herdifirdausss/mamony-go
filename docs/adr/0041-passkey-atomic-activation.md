# ADR-0041 — Proposed atomic possession verification of a staged passkey

**Status: PROPOSED / UNACCEPTED.** Related [Critical issue #78](https://github.com/herdifirdausss/mamony-go/issues/78) and [G0–G4 proposal](../proposals/identity-passkey-atomic-activation.md). No implementation, migration, public route, session issuer, production role or security sender has been authorized.

## Context

V33 creates a globally unique `PENDING_ASSERTION` credential after original password-backed registration/nonce/admission. V34 binds a fresh three-minute assertion nonce to the same original live session, RP, subject epoch and pending credential, and provides a restricted read-only evidence reader. V35 uses a pinned maintained WebAuthn library to validate genuine `webauthn.get` signature and UP/UV/counter/BE/BS, but `VerifyOnly` is repeatable; the nonce is not consumed. Persisting `ACTIVE` based merely on signed assertion would mix technical possession proof with policy authority to log in or bootstrap independent recovery.

## Proposed decision (requires independent technical acceptance)

Use one **separately privileged, internally invoked SQL finish** as a trusted-Profile-A verifier oracle. It accepts only original server-owned v34 evidence paired with already signature-verified facts; it takes fixed locks subject+alias → original session → pending credential → exact challenge and rechecks DB-clock expiry, current ownership/epoch/contact, original v30/v32 consumed stage proofs, source COSE byte equality, RP, state, BE and counter freshness. Commit the one-use challenge consumption, transition `PENDING_ASSERTION → POSSESSION_VERIFIED`, current sign count and backup state, identifier-only event and mandatory contact-verified queued notice atomically. Any failure rolls back all. No caller-selected subject or arbitrary SQL role flag.

The existing v33 `PENDING_ASSERTION` CHECK must be enlarged by a **new, forward-only Identity SQL migration 000035**, retaining historical v33/v34 fingerprints. Capability is called v36 because v35 was Go-only signature verification. The independent activation-finisher role is EXECUTE-only, `session_user`-gated, and may not read/write tables or issue sessions, login grants or recovery approvals.

## Alternatives and trade-offs

- **Direct ACTIVE:** fewer states, but accidental entitlement escalation by future readers and no accepted login/assurance policy. Rejected pending decision.
- **Nonce-only consume:** replay protection without explicit durable possession state or notification. Does not satisfy traceable credential lifecycle.
- **Application-side read then separate update:** vulnerable to TOCTOU, replay, lost audits and partial side effects. Rejected.
- **Proposed SQL one-transaction finish:** serializable authorization at Profile A root, but DB cannot independently verify WebAuthn signature; a compromised finisher credential can lie about signed facts, so it must never be treated as independent factor authority or stronger Profile B.

## Acceptance and negative tests

Require real signed WebAuthn assertion fixture coupled with real PostgreSQL tests for 16 concurrent finishes with one winner; replay, replaced nonce, another valid same-subject session, stale epoch/revoked session, changed RP/COSE, failed/stale counter, invalid BE/BS, expired challenge *after lock wait*, late duplicate event/notice rollback and clean corrected retry, role grants denial and original `session_user` provenance. Include no-mounted-route inventory, no login/session/recovery eligibility, and sanitized uncertain COMMIT tests.

## Release constraints

Remain strictly unmounted; `POSSESSION_VERIFIED` must NOT authorize password-independent account recovery, login, higher assurance or financial approval. The separate notification queue has no worker and `PENDING` is not evidence of delivery. Complete browser options, authorization, ingress/proxy, key custody, role isolation, device loss, backup/restore and alerting decisions before any public release.

**Decision owner approval:** PENDING. **Independent technical acceptance:** PENDING. **Code/CI evidence:** NOT RUN.

## Concurrency and code-review findings (2026-11-23)

The v34 issuer requires subject+alias → session → pending credential → challenge; the v34 evidence reader first acquires shared subject/alias and session locks, then performs a joined `FOR SHARE OF c,a` for credential and challenge. For the future finisher, acquire subject+alias and session **before** an exclusive credential lock and **then** challenge lock. Recheck the original v30 intent/v32 admission and actual server clock after conflicting row locks. Because joined locking and other writers can interact, add an explicit three-way **reader vs issuer vs finisher** real-PG deadlock regression; never infer lock acquisition order from SQL JOIN syntax. [PostgreSQL 18 explicit locking](https://www.postgresql.org/docs/18/explicit-locking.html).

The finisher must be given **the exact v35 evidence snapshot**, including byte-identical COSE public key and prior counter; it must not refetch a new key and treat an earlier signature as verification of it. The SQL role is a privileged verifier oracle only under Profile A. Even after a successful proof, `POSSESSION_VERIFIED` carries **zero** login, independently trusted recovery or financial permissions.

V34 can overwrite a consumed nonce row. Review whether the credential needs a non-secret `possession_challenge_id`/audit relation to preserve original challenge lineage after overwrite; storing the random nonce or any bearer in audit is out of scope.

The detailed proposed **28-case** acceptance/test matrix and schema/role changes reside in [the Critical proposal](../proposals/identity-passkey-atomic-activation.md#g4-supplement--executable-test-matrix-for-the-future-accepted-slice). All cases are **NOT RUN** pending explicit G2/G3 acceptance.
