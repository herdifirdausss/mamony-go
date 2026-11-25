-- Synthetic-only selected Identity rows, never raw credentials in output.
-- Stable comparison, not an independent or authenticated commit history.
BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SET LOCAL TIME ZONE 'UTC';
SET LOCAL bytea_output = 'hex';
SELECT 'subjects', count(*),
       encode(sha256(convert_to(coalesce(string_agg(to_jsonb(r)::text, E'\n'
           ORDER BY to_jsonb(r)::text COLLATE "C"), ''), 'UTF8')), 'hex')
FROM mamony_identity.subjects r;
SELECT 'email_aliases', count(*),
       encode(sha256(convert_to(coalesce(string_agg(to_jsonb(r)::text, E'\n'
           ORDER BY to_jsonb(r)::text COLLATE "C"), ''), 'UTF8')), 'hex')
FROM mamony_identity.email_aliases r;
SELECT 'password_verifiers', count(*),
       encode(sha256(convert_to(coalesce(string_agg(to_jsonb(r)::text, E'\n'
           ORDER BY to_jsonb(r)::text COLLATE "C"), ''), 'UTF8')), 'hex')
FROM mamony_identity.password_verifiers r;
SELECT 'recovery_code_factor_keys', count(*),
       encode(sha256(convert_to(coalesce(string_agg(to_jsonb(r)::text, E'\n'
           ORDER BY to_jsonb(r)::text COLLATE "C"), ''), 'UTF8')), 'hex')
FROM mamony_identity.recovery_code_factor_keys r;
SELECT 'password_recovery_offline_codes', count(*),
       encode(sha256(convert_to(coalesce(string_agg(to_jsonb(r)::text, E'\n'
           ORDER BY to_jsonb(r)::text COLLATE "C"), ''), 'UTF8')), 'hex')
FROM mamony_identity.password_recovery_offline_codes r;
SELECT 'passkey_credentials', count(*),
       encode(sha256(convert_to(coalesce(string_agg(to_jsonb(r)::text, E'\n'
           ORDER BY to_jsonb(r)::text COLLATE "C"), ''), 'UTF8')), 'hex')
FROM mamony_identity.passkey_credentials r;
SELECT 'security_outbox', count(*),
       encode(sha256(convert_to(coalesce(string_agg(to_jsonb(r)::text, E'\n'
           ORDER BY to_jsonb(r)::text COLLATE "C"), ''), 'UTF8')), 'hex')
FROM mamony_identity.security_outbox r;
SELECT 'password_change_notices', count(*),
       encode(sha256(convert_to(coalesce(string_agg(to_jsonb(r)::text, E'\n'
           ORDER BY to_jsonb(r)::text COLLATE "C"), ''), 'UTF8')), 'hex')
FROM mamony_identity.password_change_notices r;
SELECT 'password_recovery_notices', count(*),
       encode(sha256(convert_to(coalesce(string_agg(to_jsonb(r)::text, E'\n'
           ORDER BY to_jsonb(r)::text COLLATE "C"), ''), 'UTF8')), 'hex')
FROM mamony_identity.password_recovery_notices r;
SELECT 'password_recovery_code_notices', count(*),
       encode(sha256(convert_to(coalesce(string_agg(to_jsonb(r)::text, E'\n'
           ORDER BY to_jsonb(r)::text COLLATE "C"), ''), 'UTF8')), 'hex')
FROM mamony_identity.password_recovery_code_notices r;
SELECT 'passkey_registration_notices', count(*),
       encode(sha256(convert_to(coalesce(string_agg(to_jsonb(r)::text, E'\n'
           ORDER BY to_jsonb(r)::text COLLATE "C"), ''), 'UTF8')), 'hex')
FROM mamony_identity.passkey_registration_notices r;
SELECT 'opaque_sessions', count(*),
       encode(sha256(convert_to(coalesce(string_agg(to_jsonb(r)::text, E'\n'
           ORDER BY to_jsonb(r)::text COLLATE "C"), ''), 'UTF8')), 'hex')
FROM mamony_identity.opaque_sessions r;
SELECT 'password_recovery_challenges', count(*),
       encode(sha256(convert_to(coalesce(string_agg(to_jsonb(r)::text, E'\n'
           ORDER BY to_jsonb(r)::text COLLATE "C"), ''), 'UTF8')), 'hex')
FROM mamony_identity.password_recovery_challenges r;
SELECT 'password_recovery_assurance_grants', count(*),
       encode(sha256(convert_to(coalesce(string_agg(to_jsonb(r)::text, E'\n'
           ORDER BY to_jsonb(r)::text COLLATE "C"), ''), 'UTF8')), 'hex')
FROM mamony_identity.password_recovery_assurance_grants r;
SELECT 'password_recovery_code_enrollment_approvals', count(*),
       encode(sha256(convert_to(coalesce(string_agg(to_jsonb(r)::text, E'\n'
           ORDER BY to_jsonb(r)::text COLLATE "C"), ''), 'UTF8')), 'hex')
FROM mamony_identity.password_recovery_code_enrollment_approvals r;
SELECT 'recovery_code_factor_challenges', count(*),
       encode(sha256(convert_to(coalesce(string_agg(to_jsonb(r)::text, E'\n'
           ORDER BY to_jsonb(r)::text COLLATE "C"), ''), 'UTF8')), 'hex')
FROM mamony_identity.recovery_code_factor_challenges r;
SELECT 'passkey_activation_challenges', count(*),
       encode(sha256(convert_to(coalesce(string_agg(to_jsonb(r)::text, E'\n'
           ORDER BY to_jsonb(r)::text COLLATE "C"), ''), 'UTF8')), 'hex')
FROM mamony_identity.passkey_activation_challenges r;
SELECT 'migration_fingerprints', count(*),
       encode(sha256(convert_to(coalesce(string_agg(to_jsonb(r)::text, E'\n'
           ORDER BY to_jsonb(r)::text COLLATE "C"), ''), 'UTF8')), 'hex')
FROM mamony_identity.migration_fingerprints r;
COMMIT;
