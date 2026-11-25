-- Read-only lifecycle observations for the isolated RH-03 fixture.
-- Counts are neither eligibility predicates nor authorization to release.
-- In particular, NULL consumed_at alone does not mean an authority is valid.
BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT 'code_consumed', count(*) FROM mamony_identity.password_recovery_offline_codes WHERE consumed_at IS NOT NULL;
SELECT 'code_unconsumed', count(*) FROM mamony_identity.password_recovery_offline_codes WHERE consumed_at IS NULL;
SELECT 'recovery_challenge_consumed', count(*) FROM mamony_identity.password_recovery_challenges WHERE consumed_at IS NOT NULL;
SELECT 'recovery_challenge_revoked', count(*) FROM mamony_identity.password_recovery_challenges WHERE revoked_at IS NOT NULL;
SELECT 'recovery_assurance_consumed', count(*) FROM mamony_identity.password_recovery_assurance_grants WHERE consumed_at IS NOT NULL;
SELECT 'factor_revoked', count(*) FROM mamony_identity.recovery_code_factor_keys WHERE revoked_at IS NOT NULL;
SELECT 'factor_approval_consumed', count(*) FROM mamony_identity.password_recovery_code_enrollment_approvals WHERE consumed_at IS NOT NULL;
SELECT 'factor_challenge_consumed', count(*) FROM mamony_identity.recovery_code_factor_challenges WHERE consumed_at IS NOT NULL;
COMMIT;
