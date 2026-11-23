# ADR 0032 — Enforce live factor binding when enrolling first recovery codes (Identity v29)

Status: **PROPOSED / stacked Draft PR #69**, based on Draft #68. No public route, runtime factor provisioner, deployment or MFA assurance claim.

## Problem

In v28, a valid signed Ed25519 challenge commits a five-minute one-use v25 enrollment approval. The v25 code-enrollment transaction validates subject, password, session, grant, contact, epoch and the approval digest, but has no key membership check. If the signer is revoked or rebound **after approval and before enrollment**, a stale approval could still create the initial five saved offline recovery codes.

## Decision

- Add nullable `factor_public_key` to historical enrollment approvals. Existing rows have NULL and **fail closed**; do not backfill or silently treat older password/session-only approvals as authentic.
- Record the public key whose signature was verified in the same owner-controlled transaction that consumes the v28 challenge and issues the short-lived approval. This contains **public** material, not private keys or signatures.
- Before consuming a bootstrap approval, the v25 code-manager transaction locks the subject's independently prebound current factor row and checks active, unrevoked, matching subject/epoch/contact and **exact public-key equality** with that approval. A revocation/rebind which commits first denies the stale approval; if enrollment commits first, a later revocation cannot undo an already committed code set.
- Keep v25 rotation by an existing unused saved code unchanged. Keep original Go+SQL entrypoint signatures, original-runtime-login fences, default-deny key provisioning, security outbox/notice atomicity, and v28 issuer/verifier isolation. No new public permissions are granted.
- Treat the ADMIN test fixture as a synthetic authority only. Compromising the DB owner or future provisioning authority remains an explicit Profile-A risk; SQL row locks are not a defense against a malicious root.

## Verification and limitations

Required PostgreSQL integration proves **sign -> approval -> revoke key -> enrollment denied with zero saved codes and no enrollment audit**, plus nonrevoked successful Ed25519 sign/enroll, rotation with older recovery code, and a synthetic prebound-key v25 fixture. Migrating historical approvals yields NULL key and safe denial, not a credential-recovery bypass.

Revocation of a key **after five codes are already enrolled does not revoke those codes**. Key-compromise incident policy must independently decide whether to revoke the existing code set, sessions and recovery grants. Authentic trusted key registration/revocation ceremonies, phishing-resistant origin binding, user verification, lost-all-factors recovery, browser/CSRF/abuse policy and production runtime separation remain unimplemented. This changes no externally available feature. Keep Draft until security review and exact-head 5/5 CI.
