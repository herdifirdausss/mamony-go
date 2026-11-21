-- Identity v7: independently budget email confirmation by client network.
-- Confirmation is a distinct anonymous action, never an authenticated subject.
-- The existing HMAC-keyed bucket table and bounded retention owner are reused.
-- No verifier, delivery, or public HTTP permission is granted here.
ALTER TABLE mamony_identity.anonymous_admission_buckets
    DROP CONSTRAINT anonymous_admission_buckets_action_check;

ALTER TABLE mamony_identity.anonymous_admission_buckets
    ADD CONSTRAINT anonymous_admission_buckets_action_check
    CHECK (action IN ('REGISTER', 'RESEND', 'CONFIRM'));

-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (7, 'cb87d3815b249a19892824dc538b892cb4f3586a6dc0d7eb2c911f676e1af271');
