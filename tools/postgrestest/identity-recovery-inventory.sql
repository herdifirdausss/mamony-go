-- Read-only catalog inventory for the synthetic Identity recovery design.
-- Classification is NOT an invalidation/repair command or release decision.
-- Compare against live schema, not just migration file names.
WITH expected(name, recovery_class) AS (
  VALUES
    ('migration_fingerprints', 'schema_integrity'),
    ('subjects', 'binding_reconcile'),
    ('email_aliases', 'binding_reconcile'),
    ('password_verifiers', 'binding_reconcile'),
    ('password_recovery_offline_codes', 'binding_reconcile'),
    ('recovery_code_factor_keys', 'binding_reconcile'),
    ('passkey_credentials', 'binding_reconcile'),
    ('email_verification_challenges', 'temporary_authority'),
    ('opaque_sessions', 'temporary_authority'),
    ('password_login_grants', 'temporary_authority'),
    ('password_recovery_challenges', 'temporary_authority'),
    ('password_recovery_assurance_grants', 'temporary_authority'),
    ('password_recovery_code_enrollment_approvals', 'temporary_authority'),
    ('recovery_code_factor_challenges', 'temporary_authority'),
    ('passkey_registration_intents', 'temporary_authority'),
    ('passkey_registration_admissions', 'temporary_authority'),
    ('passkey_activation_challenges', 'temporary_authority'),
    ('email_proof_deliveries', 'proof_delivery'),
    ('password_recovery_deliveries', 'proof_delivery'),
    ('password_change_notices', 'notice_obligation'),
    ('password_recovery_notices', 'notice_obligation'),
    ('password_recovery_code_notices', 'notice_obligation'),
    ('passkey_registration_notices', 'notice_obligation'),
    ('passkey_activation_notices', 'notice_obligation'),
    ('security_notice_mailgun_receipts', 'provider_receipt'),
    ('password_recovery_notice_mailgun_receipts', 'provider_receipt'),
    ('recovery_code_notice_mailgun_receipts', 'provider_receipt'),
    ('security_outbox', 'security_event'),
    ('anonymous_admission_buckets', 'abuse_budget')
), actual(name) AS (
  SELECT c.relname::text
  FROM pg_catalog.pg_class c
  JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
  WHERE n.nspname = 'mamony_identity' AND c.relkind IN ('r', 'p')
), missing AS (
  SELECT name FROM expected EXCEPT SELECT name FROM actual
), unexpected AS (
  SELECT name FROM actual EXCEPT SELECT name FROM expected
)
SELECT (SELECT count(*) FROM expected),
       (SELECT count(*) FROM actual),
       (SELECT count(*) FROM missing),
       (SELECT count(*) FROM unexpected),
       (SELECT count(*) FROM expected) - (SELECT count(DISTINCT name) FROM expected),
       coalesce((SELECT string_agg(name, ',' ORDER BY name) FROM missing), '-'),
       coalesce((SELECT string_agg(name, ',' ORDER BY name) FROM unexpected), '-');
