---
name: mamony-ledger-review
description: Use for C01 ledger posting, reversal, balances, money types, outbox, or replay work in Mamony Go. Required for finance-affecting code, not for unrelated modules.
---
# Mamony ledger invariant review

1. Resolve currently accepted PRD-01, TD-01, ADR-0005/0009/0010/0017 and relevant Gate A evidence, using exact headings and requirement IDs from the repo. Do not infer decisions from old Rust test status.
2. Establish expected journal/entry balance, single currency, integer minor units, checked arithmetic, immutable journal, balance floor, idempotency scope and outcome, reversal exactly once, transactional outbox, per-account ordering and verifier invariants.
3. Build a compact AC-to-test table. Include normal post, concurrent same-key, different payload, balance-floor race, overflow, invalid currency, rollback, response lost after commit, reversal race, consumer replay, outbox absence on rollback and restoration/verifier.
4. For concurrency and atomicity require tests against real PostgreSQL with multiple connections and deterministic barriers. The Go race detector addresses a different class of race; do not treat it as proof of DB serialization.
5. If nominal wire format, permission gate, or authoritative decision is unresolved, flag it rather than choosing silently.
6. Only say PASS for commands actually executed and verified. Record unmet scenarios and evidence paths.
