# Seev reference — not a product specification

Inspected [Seev](https://github.com/herdifirdausss/seev) default branch on 2026-11-21 (tree `2e6e5ead17684daa92cb5f3f8fe8fdfffc0bfc53`).

Useful patterns:
- [Service-centric architecture ADR](https://github.com/herdifirdausss/seev/blob/main/docs/reference/decisions/0001-service-centric-repository-layout.md) clearly defines ownership.
- [Boundary tests](https://github.com/herdifirdausss/seev/blob/main/boundary_test.go) reject forbidden imports.
- [Code map](https://github.com/herdifirdausss/seev/blob/main/docs/development/code-map.md) gives contributor navigation.
- CI, security and runbook sections demonstrate evidence-driven process.

Do **not** copy nine independent services, Redis, RabbitMQ, gRPC, large Docker Compose or analytics into this Go scaffold without a Mamony requirement and independent ADR.
