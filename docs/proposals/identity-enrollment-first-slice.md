# Identity enrollment: first implementation slice

**Status:** DRAFT / code under review; no public HTTP registration, no Identity schema, no email verification, no login. Tracking: [Identity issue #20](https://github.com/herdifirdausss/mamony-go/issues/20).

This slice implements internal password policy + Argon2id credential primitives and an atomic enrollment **port**. It intentionally does not claim a working registration feature: the port is not connected to an actual PostgreSQL repository and the application still exposes health routes only.

## Accepted direction vs remaining decisions

Directionally agreed in chat: prioritize first-party Profile A Identity before ledger, modular-monolith Go implementation, email verification before active login. NIST SP 800-63B-4 and OWASP Password Storage guidance were checked on 2026-11-21; Project P01 currently chooses NFC before hashing rather than the prior proposal's no-NFC deviation. This requires explicit review and a recorded acceptance before persistent credentials are used.

The `64 MiB, t=3, p=1` Argon2id parameters are fixed in this first version to prevent stored-record cost inflation, **but they are only a candidate**. Run `go test -run ^$ -bench BenchmarkPasswordHashCandidate ./internal/modules/identity/internal/domain` on the supported machine and approve admission limits before exposing an endpoint. Concurrency is explicitly limited to 1..4 configured slots; this is **not** a distributed/rate-limit budget. A vetted common/breached/contextual password blocklist is required; `PasswordPolicy` intentionally fails closed if its rejector is absent.

The first synthetic identity profile uses ASCII email addresses with normalized application aliases, not EAI/IDN or provider-specific dot/plus mapping. It stores only a pending state and an *event identifier* at this interface; verification challenges, notification payload security, UI/HTTP admission and real store rollback are still pending.

## Next implementation gates

1. **Schema and atomic persistence:** separate Identity migrator/runtime credentials, SQL migration with fingerprint history, unique normalized alias, pending subject, verifier columns, durable event/outbox obligation. Prove actual PostgreSQL duplicate-race, obligation rollback, role denials and migrations. The `RegistrationStore` operation must be one transaction; fake-store unit tests do not prove this.
2. **Email proof:** current email+version+subject+purpose-bound one-time token with hashed verifier and protected delivery obligation; verify token consumption atomically; no token in public response, log, or unprotected DB event.
3. **Browser admission/login:** explicit Origin+CSRF controls, duplicate-key rejection, bounded rate/KDF budget, opaque sessions, revocation, no public operator access. Real HTTP+SQL negative tests required before enabling.
4. **Resource authorization:** default-deny subject/action/resource/tenant policy and BOLA tests. Profile A does not withstand arbitrary writes to the authoritative identity DB; Profile B independently trusted current heads remain deferred.

Run `make check` and `make race` under Go 1.27.4 and record exact evidence per requirement. Current expected identity DB integration state: **NOT RUN**. Do not mark G5 complete or merge as release-ready based solely on unit tests.
