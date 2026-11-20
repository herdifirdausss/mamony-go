# Mamony Go

Go modular-monolith implementation of the [Mamony](https://github.com/herdifirdausss/mamony) **synthetic** cross-border pay-in/pay-out learning project.

Status: bootstrap only. No real money, credentials, providers, or identities. The repository is not production-ready.

Authoritative references:
- Product requirements and Accepted ADRs: `herdifirdausss/mamony` (read only).
- Go engineering reference: `herdifirdausss/seev` (read only).

Source-derived constraints: C01 alone writes balances; immutable, balanced journals; idempotent posting; one runtime first; isolated PostgreSQL capability schemas; fail-closed tenant isolation. These constraints are not yet implemented by this foundation.

The initial engineering harness will be proposed on a separate branch for review.
