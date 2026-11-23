-- Identity v8: read-only password authentication authority, not login/session.
-- A separately provisioned authenticator role may inspect only the columns
-- needed to compare Argon2id verifiers and check ACTIVE+verified eligibility.
-- It CANNOT write status, issue proofs, read admission HMAC keys, or mint sessions.
GRANT USAGE ON SCHEMA mamony_identity TO mamony_identity_authenticator_runtime;
GRANT SELECT (subject_id, status)
    ON mamony_identity.subjects TO mamony_identity_authenticator_runtime;
GRANT SELECT (canonical_email, subject_id, verified_at)
    ON mamony_identity.email_aliases TO mamony_identity_authenticator_runtime;
GRANT SELECT (subject_id, kdf_version, memory_kib, iterations, parallelism, salt, digest)
    ON mamony_identity.password_verifiers TO mamony_identity_authenticator_runtime;
-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (8, 'e86d52ed6044c073492e5747583b13f874f9aec863aee294527f9be20124e557');
