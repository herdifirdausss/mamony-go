# Screening and onboarding

Build screening before onboarding checks account eligibility. Keep this step inside the synthetic pilot. The [monitoring contract](fraud-and-monitoring.md) covers later activity review.

## Screening

| ID | Requirement | Acceptance check |
|---|---|---|
| SCR-01 | Import list versions with source, time, counts, and a fingerprint. Keep synthetic test entries separate. Keep the previous valid version if an import fails. | Reject a corrupt import. Check that accepted results identify the list version used. |
| SCR-02 | Screen the entity in its context. Compare names and aliases. Compare dates, countries, or identifiers when both records contain them. Record candidates and reasons. | Test spelling variants, weak aliases, close matches, exact identifiers, and the synthetic sanctions fixtures. |
| SCR-03 | Return pass, hold, or reject. A potential match or PEP match enters review. Reject a confirmed match or an exact listed identifier. Missing, stale, or unavailable sources cause a hold. | Test all three outcomes. Assert that a stale list or unavailable service cannot produce a pass. |
| SCR-04 | Record the input fingerprint, list versions, decision, and review history. Replay the decision for the same request reference. A new reference allows a new decision. | Replay after the list changes. Check that a fresh request uses the new version. |

List age, matching thresholds, normalization rules, and review deadlines are implementation decisions. Validate candidate recall with synthetic examples before relying on a database search extension.

## Onboarding

| ID | Requirement | Acceptance check |
|---|---|---|
| ONB-01 | Select checks by subject type, country, and tier. Record the versions and outcomes. Keep only the synthetic data that the check needs. | Exercise a person and an organization. Reject an incomplete submission without changing its tier. |
| ONB-02 | Send unresolved checks and screening holds to review. Require a second approver for designated sensitive decisions. Keep the decision and its reason. | Test a hold, approval, rejection, and an expired case. Test the independent approver. |
| ONB-03 | Issue an eligibility status and tier. Allow account activation only when the status is eligible and screening passes. Publish state and tier changes without identity details. | Activate an eligible fixture. Deny a held or restricted subject. Apply a restriction to an existing account through the event. |

The onboarding module provides status and tier. The policy module uses them when it decides limits. The accounts module uses them when it opens or suspends an account. Only the capability that owns the data can change it.
