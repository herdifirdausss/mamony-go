# 09 — Fraud and activity monitoring

C13 scores a transaction before the affected action. C19 monitoring reviews activity over time. [Pilot screening](03-screening-onboarding.md) still applies before any cross-border transaction.

## C13 fraud decisions

| ID | Requirement | Acceptance check |
|---|---|---|
| RSK-01 | Return allow, review, or block with reasons and the rule version. Apply rules in a fixed order. The same transaction reference returns the stored decision. | Test overlapping lists, rule bands, expiry, and replay after a rule change. |
| RSK-02 | Record the facts used by each decision. Include the current transaction in velocity limits. Use a versioned failure policy for each transaction kind. | Replay a decision after activity changes. Test a stale fact feed and timeout at each configured failure posture. |
| RSK-03 | Run a candidate rule in shadow mode before rollout. Compare results on labeled synthetic cases. Record estimated precision, recall, and good-transaction impact. | Check that shadow mode produces observations while the active decision stays unchanged. Evaluate the candidate against baseline cases. |
| RSK-04 | Hold the operation during review. Require an authorized reviewer action. Report overdue cases without automatically approving them. | Test an overdue case, unauthorized approval, and a blocked ledger posting. |
| RSK-05 | Apply a new rule, failure posture, or score band only after independent approval. Keep the decision enough to explain the outcome later. | Test an unapproved rule change and a kill switch. Check traceable rule and fact versions. |

Consume stable events through C12. Record outcome labels with their provenance. Keep the facts needed for an explanation. Limit access to review data and audit each read.

## C19 activity monitoring

| ID | Requirement | Acceptance check |
|---|---|---|
| AML-01 | Build a deduplicated activity record from supported money events. Match event versions to amount, currency, corridor, and subject identifiers. | Replay a batch. Check that one payment produces one fact. Reject or report an event version that lacks required facts. |
| AML-02 | Evaluate versioned rules with activity and the expected subject profile. Record each alert's rule, evidence, priority, and time. | Test synthetic cumulative-amount, rapid-movement, and risk-corridor patterns. Check that repeat input does not duplicate an alert. |
| AML-03 | Link alerts to restricted cases. Require separate approval for designated high-priority closures and synthetic report filings. Escalate overdue cases. | Check that a merchant cannot read alert or case existence. Test duplicate filing and unauthorized closure. |
| AML-04 | Emit limited identifiers and codes to authorized consumers. Record rule changes, review actions, and retention decisions. | Verify restricted information stays out of merchant events and notifications. Test the purge policy against held records. |

This is synthetic monitoring. Generate simulated reports only. A Go design must define the event versions it consumes, the monitoring windows, and how gaps are backfilled.
