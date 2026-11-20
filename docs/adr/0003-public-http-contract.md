# ADR-0003 — Public HTTP contract

**Status:** Contract baseline. The amount format is open.

## Request and response

Use versioned REST endpoints with JSON. Add each released route to the OpenAPI document in this repository. Test requests, responses, and permissions against that contract.

Terminate TLS at the public edge. Authorize the action and resource in the caller's tenant. Use typed request and response fields for each route. Reject unknown input fields. Expose only the response fields that the caller can read.

Use `application/problem+json` with a stable error code. Keep secrets, query text, and stack traces out of the response. Set body size, time, page size, and rate limits for each route before release. Do not expose the private listener to public traffic.

## HTTP idempotency

Require an `Idempotency-Key` for requests that change money or state. Scope the key to the authorized caller. Store a fingerprint of the canonical request in the owning capability's database.

| Request | Result |
|---|---|
| Missing key on a mutating route | Return `400`. |
| Same key and same payload after completion | Return the stored outcome after authorization. |
| Same key and different payload | Return `422` without a write. |
| Same key while the first request is running | Return `409`. |

Set the key lifetime in the route contract. A request after expiry is new. The internal [C01 journal key](../contracts/ledger-gate-a.md#posting-and-replay) has its own lifetime.

## Open decision on wire amounts

Send exact minor units with a currency code. Two wire formats are under consideration.

| Option | Range and effect |
|---|---|
| JSON integer | Use the interoperable safe integer range, up to `2^53 - 1` in magnitude. |
| JSON string of digits | Can carry the full positive signed 64-bit range when the route validates it. Define whether the route accepts a sign or zero. |

Choose the JSON type and range before publishing the first amount-bearing route. Record the choice here and in OpenAPI. Test the wire range and the [internal ledger range](../contracts/ledger-gate-a.md#posting-and-replay) independently.
