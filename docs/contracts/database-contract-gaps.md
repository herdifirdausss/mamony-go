# Database implementation tasks — unresolved in Go

This list is adapted from the user-attached Mamony Engineering Harness v7. No PostgreSQL implementation or database Gate A evidence exists in the Go bootstrap.

1. Read the **approved** Mamony schema and role/grant design before writing Go migrations. Do not invent tenant authorization or new ownership rules.
2. Choose/pin `pgx`, migration tooling and `sqlc` only after a compatibility spike, contract review and recorded Go ADR where needed. The v7 target was PostgreSQL 18; it remains a **proposal**, not a tested Go dependency.
3. Implement transaction-scoped tenant context and test with the real runtime DB role, including reuse of pooled connections. Missing/invalid tenant context must fail closed.
4. Test migrations from empty and supported previous schema states. Record locking expectations, deployment order, recovery and safe rollback/roll-forward behavior.
5. Prove C01 posting and transactional outbox atomicity with concurrent replay, insufficient-balance contention, rollback and no-event-on-rollback scenarios.
6. Exercise an isolated backup/restore with a posted journal and ledger verifier before claiming Gate A.

No `DATABASE_URL` is needed for `make check`; basic unit checks should remain offline and hermetic. For the accepted constraints, see [Gate A trace](ledger-gate-a.md) and original Mamony ADR-0010/0021. **All database execution above is NOT RUN.**
