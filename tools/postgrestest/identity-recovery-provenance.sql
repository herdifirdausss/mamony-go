-- Read-only targeted recovery provenance guard, not a complete FK inventory.
-- Required links must remain validated and must not silently cascade deletion.
WITH expected(child_table, child_column, parent_table, parent_column) AS (
  VALUES
    ('password_change_notices', 'event_id', 'security_outbox', 'event_id'),
    ('password_change_notices', 'subject_id', 'subjects', 'subject_id'),
    ('password_recovery_notices', 'event_id', 'security_outbox', 'event_id'),
    ('password_recovery_notices', 'subject_id', 'subjects', 'subject_id'),
    ('password_recovery_code_notices', 'event_id', 'security_outbox', 'event_id'),
    ('password_recovery_code_notices', 'subject_id', 'subjects', 'subject_id'),
    ('passkey_registration_notices', 'event_id', 'security_outbox', 'event_id'),
    ('passkey_registration_notices', 'subject_id', 'subjects', 'subject_id'),
    ('passkey_activation_notices', 'event_id', 'security_outbox', 'event_id'),
    ('passkey_activation_notices', 'subject_id', 'subjects', 'subject_id'),
    ('email_proof_deliveries', 'challenge_id', 'email_verification_challenges', 'challenge_id'),
    ('password_recovery_deliveries', 'challenge_id', 'password_recovery_challenges', 'challenge_id'),
    ('passkey_credentials', 'event_id', 'security_outbox', 'event_id'),
    ('passkey_credentials', 'possession_event_id', 'security_outbox', 'event_id'),
    ('passkey_credentials', 'subject_id', 'subjects', 'subject_id'),
    ('security_outbox', 'subject_id', 'subjects', 'subject_id'),
    ('passkey_activation_challenges', 'credential_id', 'passkey_credentials', 'credential_id')
), actual AS (
  SELECT child.relname::text AS child_table, ca.attname::text AS child_column,
         parent.relname::text AS parent_table, pa.attname::text AS parent_column,
         c.convalidated, c.confdeltype
  FROM pg_catalog.pg_constraint c
  JOIN pg_catalog.pg_class child ON child.oid=c.conrelid
  JOIN pg_catalog.pg_namespace cn ON cn.oid=child.relnamespace
  JOIN pg_catalog.pg_class parent ON parent.oid=c.confrelid
  JOIN pg_catalog.pg_namespace pn ON pn.oid=parent.relnamespace
  JOIN pg_catalog.pg_attribute ca ON ca.attrelid=child.oid AND ca.attnum=c.conkey[1]
  JOIN pg_catalog.pg_attribute pa ON pa.attrelid=parent.oid AND pa.attnum=c.confkey[1]
  WHERE c.contype='f' AND cn.nspname='mamony_identity' AND pn.nspname='mamony_identity'
    AND cardinality(c.conkey)=1 AND cardinality(c.confkey)=1
), missing AS (
  SELECT * FROM expected
  EXCEPT
  SELECT child_table,child_column,parent_table,parent_column FROM actual
  WHERE convalidated AND confdeltype IN ('a','r')
)
SELECT (SELECT count(*) FROM expected),
       (SELECT count(*) FROM missing),
       coalesce((SELECT string_agg(child_table||'.'||child_column||'->'||parent_table||'.'||parent_column, ',' ORDER BY child_table,child_column) FROM missing), '-');
