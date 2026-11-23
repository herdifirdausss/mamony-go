-- Identity v9: independent pre-Argon2id login admission, not a login route.
-- Reuse the HMAC-keyed counter table and maintenance retention; do not store
-- raw email, IP address, password, subject, session or failure classification.
-- LOGIN uses a five-bucket static policy including a global per-minute cap.
ALTER TABLE mamony_identity.anonymous_admission_buckets
    DROP CONSTRAINT anonymous_admission_buckets_action_check;
ALTER TABLE mamony_identity.anonymous_admission_buckets
    ADD CONSTRAINT anonymous_admission_buckets_action_check
    CHECK (action IN ('REGISTER', 'RESEND', 'CONFIRM', 'LOGIN'));

ALTER TABLE mamony_identity.anonymous_admission_buckets
    DROP CONSTRAINT anonymous_admission_buckets_scope_check;
ALTER TABLE mamony_identity.anonymous_admission_buckets
    ADD CONSTRAINT anonymous_admission_buckets_scope_check
    CHECK (scope IN ('GLOBAL_MINUTE', 'CLIENT_MINUTE', 'CLIENT_HOUR', 'EMAIL_HOUR', 'EMAIL_DAY'));

-- Existing registration admission runtime continues to own bounded counter
-- INSERT + hits-only UPDATE. No verifier, authenticator, delivery or public
-- listener authority is enlarged by this migration.

-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (9, '475647715fdbba0a3365194834b87d52c03b6f18bbcd48ddf0ea0491e2606a7e');
