# Provider connector

The provider connector owns provider requests, normalized results, and inbound messages. A simulator implements the same ports as a provider adapter. The provider connector has no dependency on the ledger. It sends no real request from the pilot environment.

## Operation lifecycle

| ID | Requirement | Acceptance check |
|---|---|---|
| PRV-01 | Record the operation and stable provider reference before dispatch. Include provider, corridor, kind, and a fingerprint of the financial input. Reject different input for the same reference. | Stop the worker before it sends, after it sends, and after it records a result. Repeat each case. Check durable identity and one effect. |
| PRV-02 | Normalize outcomes to not sent, accepted, succeeded, failed, or unknown. Record not sent only when the connector knows that dispatch did not occur. Keep a deadline and an inquiry path for accepted or unknown work. | Simulate cutoff, rejection, pre-send failure, timeout after send, and lost response. Check that the last two never become definitive failures. |
| PRV-03 | Query the same provider and reference for an unresolved operation. Retry a money operation only if its provider contract proves the repeat safe. | Exercise a duplicate worker and an unknown response. Check that no other provider receives the same operation. |
| PRV-04 | Set connection and response deadlines by provider and operation. Keep provider calls outside database transactions. Apply cutoff and breaker policies before sending. | Use a slow simulator and a closed cutoff. Check the normalized result and retained request record. |

Represent provider capabilities as explicit ports where callers need them. Charge, pay-out, refund, status inquiry, recipient verification, and dispute evidence have different inputs. Record the missing ports before starting pay-out or disputes.

## Inbound messages

1. Apply request size and time limits. Check the signature against the raw message and check the timestamp before parsing.
2. Store a valid message and acknowledge it before running its business handler.
3. Deduplicate by provider and message identity. Record one normalized transition and its event in one transaction.
4. Protect a final result from late or conflicting messages. Record a conflict for reconciliation.

**PRV-05.** Test wrong signatures, invalid timestamps, duplicate delivery, out-of-order messages, a refused update to a final status, and a worker that stops after acknowledging the message.

The simulator must support per-reference success, refusal, pending result, timeout after acceptance, and delayed or duplicate result messages. Use these fixtures in the [money flows](payments-and-payouts.md) and [reconciliation](reconciliation-and-events.md).

## Provider trust and outbound network boundary

| ID | Requirement | Acceptance check |
|---|---|---|
| PRV-06 | Verify each inbound provider message according to **that provider's** signature algorithm, key scope and signed raw bytes. Apply bounded timestamp/replay policy when supported. Authenticate the provider identity independently of user-controlled fields. | Reject forged or altered bytes, wrong key/provider, stale timestamp, repeated delivery ID and signature parsing ambiguity; test the selected provider specification, not a generic signing helper. |
| PRV-07 | Only call approved provider destinations and operations from configuration. Deny arbitrary user URLs, unexpected schemes, internal/metadata addresses and unsafe redirects. Enforce DNS/connect-time destination policy and egress network controls where the deployment supports them. | Exercise SSRF to loopback/metadata/private IP, DNS change, redirects, alternate scheme/port and timeout; confirm no request is dispatched. |
| PRV-08 | Preserve the original request identity and canonical financial fingerprint across retry or inquiry. Never infer safe resend from an HTTP timeout alone, and never accept a callback as authority to mutate identity, beneficiary or ledger without the owning workflow. | Simulate timeout after external acceptance, duplicate worker, callback-before-response, conflicting late callback and a changed request under the same reference. |

Select message authentication, timestamp tolerance and destination rules per provider contract. A shared helper can implement cryptographic primitives but must not impose one unverified provider format. Keep real credentials and real provider dispatch disabled in the synthetic pilot.

