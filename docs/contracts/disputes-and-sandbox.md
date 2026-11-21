# Disputes and sandbox

The disputes module owns disputes and refund policy. The payments module performs a permitted refund. The developer sandbox owns the developer experience. The event relay owns merchant event delivery and notices.

## Disputes and refunds

| ID | Requirement | Acceptance check |
|---|---|---|
| DSP-01 | Ingest a normalized dispute message once. Record payment, amount, currency, status, and provider deadline. | Deliver duplicate, delayed, and conflicting messages. Check one dispute and one active deadline. |
| DSP-02 | Hold disputed funds and fees through the ledger. Record any uncovered amount under the approved shortfall policy. Settle the hold with a separate journal when the provider decides. | Test insufficient available balance, a win, a loss, and a late change of decision. Check no duplicate release or debit. |
| DSP-03 | Accept evidence only for an eligible dispute. Send it with a stable operation reference. Prompt authorized reviewers before the deadline. | Submit twice and after expiry. Check one submission and auditable disposition. |
| DSP-04 | Block a refund on a payment with an open dispute. Apply versioned partial or full refund rules. Route authorized refund execution to the payments module. | Race dispute opening and refund acceptance. Check the combined effect cannot exceed the applicable balance and refund rules. |

Define the dispute event and evidence-submission port in [the provider connector](provider.md) before wiring the flow. Define how final changes and shortfalls affect ledger accounts. Keep evidence access separate from public status fields.

## Merchant webhooks and notices

- Sign each event with the endpoint secret. Use the same event ID on retries. Define a rotation overlap.
- Apply a destination policy on every delivery. Use HTTPS. Reject disallowed destinations and redirects. Connect only to a checked address.
- Bound retries and record each attempt. Disable persistently failing endpoints according to policy. Let an authorized tenant enable its endpoint.
- Document at-least-once delivery without a public ordering guarantee. Let the merchant read the current state through the API.
- Send notices through configured test channels. Use templates and record delivery status. Keep contact details out of event payloads.

**DVL-01.** Test delayed and duplicate delivery, key rotation, a disallowed destination, and an endpoint that fails. Verify tenant isolation of delivery records.

**DVL-05.** Merchant webhook destinations are untrusted configuration, not privileged network destinations. Enforce HTTPS and a documented destination policy at URL validation, DNS resolution and connection time; forbid metadata, internal and other disallowed IP ranges, schemes and ports. Do not follow an unchecked redirect. Bound attempts, timeouts, payload size and endpoint-secret access. Sign each event using the selected versioned delivery contract and rotate keys with an explicit overlap.

**Acceptance:** Reject loopback/private/metadata destinations, DNS rebinding, unsafe redirects, changed host/port after validation and a revoked endpoint key. Exercise duplicate/retried delivery, endpoint refusal and a compromised tenant attempting to reach another tenant's delivery records. Use network egress restrictions as an additional boundary where the deployment supports them.

## Developer sandbox

| ID | Requirement | Acceptance check |
|---|---|---|
| DVL-02 | Generate API docs from the local OpenAPI contract. Check the route inventory and compatibility. Provide a changelog and a deprecation policy. | Follow the guide from a clean fixture. Make a first synthetic payment with documented inputs. |
| DVL-03 | Give each sandbox isolated data and test keys. Support explicit outcome fixtures and a reset that revokes old keys. | Recover from a reset failure. Check that old credentials cannot reach a new sandbox and that one sandbox cannot reach another. |
| DVL-04 | Show authorized developers request metadata for their keys. Exclude secrets, raw personal fields, and other tenants' data. | Test request logs with rotated keys, an unauthorized caller, and a payload that contains data to redact. |

Choose the documentation generator and sandbox isolation mechanism when sandbox work starts. Use the [HTTP contract](../adr/0003-public-http-contract.md) for the public surface.
