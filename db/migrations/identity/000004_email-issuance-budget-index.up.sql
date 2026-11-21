-- Identity v4: historical issuance lookup for authoritative resend budgets.
-- Retain existing challenges until a separately approved retention policy.
-- No new email provider, HTTP endpoint, or privilege is granted here.
CREATE INDEX email_verify_budget_by_subject
    ON mamony_identity.email_verification_challenges(subject_id, requested_at DESC)
    WHERE purpose = 'EMAIL_VERIFY';

-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (4, '6e52ad8c350b51af851876b3fab688221b7e6d83d43666c24cca66539c6e1105');
