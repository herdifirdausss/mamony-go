# ADR 0012 — Auth material epoch invalidation and stale-password grant fence

Status: **PROPOSED security candidate; not an approved password-reset or email-change feature**.
Stack: Draft #49 → #48 → #47 → #46 → #45 → #44 → #43 → #42 → #41 → #40. The application still mounts **no public Identity browser routes**.

## The two independent bugs

1. **Surviving bearer:** v10–v13 increments `subjects.session_epoch` only when `subjects.status` changes. An authorized future UPDATE of the Argon2id verifier or current login contact can otherwise leave old bearer digests valid. Client-side cookie removal is not a revocation authority.
2. **Stale Argon2id verdict:** the Go password reader fetches the verifier, then performs a deliberately expensive KDF comparison outside any database lock. A credential/contact change can commit while the KDF runs. Previous `stage_password_auth_grant(subject,digest)` re-read the *new* epoch but accepted the *old* authentication verdict, enabling an after-change session even if old sessions were invalidated.

OWASP Password Recovery and Session Management guidance recommends invalidating sessions after successful recovery and treating credential changes as reauthentication-sensitive events. These recommendations informed the scope; our current implementation remains an unapproved Profile A candidate: https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html and https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html.

## v14 SQL solution

The immutable, fingerprinted forward-only v14 migration adds a restricted `SECURITY DEFINER` trigger function, with a trusted catalog-only search path, to advance the existing `subjects.session_epoch` in **the same transaction** after:

- A real update to stored password-verifier material (Argon2id algorithm parameters, salt, digest), or deletion of that row.
- A real change to the current email alias (canonical email, verification timestamp, contact version), or deletion of the alias. An alias/verifier ownership reassignment is rejected rather than silently migrating identity to another subject.
- A no-op `SET digest=digest` is not a credential change; the `WHEN` trigger predicate avoids an unnecessary session revocation.

Initial registration INSERT does not trigger a password-change revocation. The current verifier already locks subject before alias when applying initial email verification; initial activation may increment the epoch twice (contact verification then status ACTIVE), which is safe because no session existed. No new SQL table-write privileges are granted to HTTP roles. Future mutation services must lock the subject row **before** changing verifier/alias rows; an ad hoc row-first mutation could deadlock with the issuer's subject-before-alias lock. PostgreSQL transaction abort is fail closed, but not operationally acceptable at scale.

**A changed verifier digest is not the same as an authorized password change.** This migration does not implement reauthentication, password-strength/reuse checks, MFA, email confirmation or an audit event for changes. A future privileged mutation service must own those checks and write mandatory identifier-only security evidence in the same transaction. Existing auditor/accounting events should not be invented by raw SQL mutation fixtures.

## Bind the KDF verdict to the same epoch

- The restricted read-only authenticator's SQL lookup returns `s.session_epoch` in the *same SELECT snapshot* as the password verifier, subject, and contact status (one extra SELECT column grant only). The result is a trusted internal `AuthenticatedSubject{SubjectID,SessionEpoch}`, not an HTTP session or financial authorization.
- The pre-KDF admission requirement remains mandatory. The epoch travels through `PasswordAuthentication.AuthenticateWithEpoch`, `PasswordLoginFlow.AuthenticateWithEpoch`, and `SessionLogin` to `AuthenticationGrantAuthority.Issue(subject,expectedEpoch)`.
- **Drop** the unsafe unversioned SQL `stage_password_auth_grant(text,bytea)` forward-only. Its replacement `stage_password_auth_grant(text,bigint,bytea)` row-locks ACTIVE subject/current verified alias and allows grant staging **only if the current DB epoch equals the one captured before KDF**. On mismatch, it denies without writing a grant. Consumed grants remain epoch-checked by the v13 issuer.
- A change winning before grant stage makes the epoch mismatch. A stage winning first stores the old epoch, and a later change makes v13 consumption reject it. A change during/after session issuance invalidates that earlier session at Resolve. No long Argon2id lock is introduced.

The separate grant role is still a **privileged successful-authentication assertion oracle** if compromised. Supplying a guessed correct epoch to the grant role does not cryptographically prove a password; this Profile A control protects only the *trusted* Go login flow from accidental stale verifier conclusions, not from RCE or malicious SQL role holders.

## Required evidence and limitations

Real disposable PostgreSQL `TestIdentityZAuthMaterialEpochFence` must demonstrate: no-op updates preserve existing session; password modification commits epoch+1 and invalidates old session; unconsumed grant from old epoch cannot mint; a successful old-epoch Argon2id verdict cannot stage; rollback preserves the old epoch and bearer; contact verification removal/reinstatement and email change do not resurrect an old session; deletion of verifier/contact bumps epoch; and **a staged login blocked behind a password mutation's subject lock is denied after commit**.

Existing integration suites must adapt synthetic grant staging to pass the actual subject epoch, not a hardcoded constant. Old two-argument SQL staging entrypoint must be absent; limited roles cannot bypass the new three-argument stage function. Also require all v13 cap, v12 idle, v11 grant, freeze/issue, mandatory testgate and actual Chromium E2E workflows PASS at the exact final SHA.

**Not solved:** password reset/recovery token lifecycle, old-password verification and MFA for authenticated changes, email ownership proof, outbox notification/audit for sensitive changes, permission grants for a future mutation role, account/device management UX, production workload isolation, load/deadlock/lock-timeout performance and financial action/tenant/resource authorization. An administrator with underlying DB owner/migrator privileges remains outside Profile A containment. A user may lose all sessions after a credential change; the next user-facing flow must explicitly handle this as a re-login, not silent session renewal.

Keep all stacked PRs Draft. No merge or public route activation.
