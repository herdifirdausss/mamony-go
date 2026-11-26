-- Stable disposable-only identity possession snapshot.
-- These records are read-only; output is a count tuple plus an MD5 comparison
-- checksum, never the actual credential/public-key or notice recipient.
WITH verified AS (
  SELECT credential_id, subject_id, possession_event_id, possession_challenge_id
  FROM mamony_identity.passkey_credentials
  WHERE state = 'POSSESSION_VERIFIED'
),
linked AS (
  SELECT v.credential_id
  FROM verified v
  JOIN mamony_identity.security_outbox o
    ON o.event_id = v.possession_event_id
   AND o.subject_id = v.subject_id
   AND o.event_type = 'PASSKEY_POSSESSION_VERIFIED'
  JOIN mamony_identity.passkey_activation_notices n
    ON n.event_id = v.possession_event_id
   AND n.subject_id = v.subject_id
   AND n.status = 'PENDING'
  JOIN mamony_identity.passkey_activation_challenges a
    ON a.subject_id = v.subject_id
   AND a.credential_id = v.credential_id
   AND a.challenge_id = v.possession_challenge_id
   AND a.consumed_at IS NOT NULL
),
records AS (
  SELECT 'C:' || to_jsonb(c)::text AS data
    FROM mamony_identity.passkey_credentials c
  UNION ALL
  SELECT 'H:' || to_jsonb(h)::text
    FROM mamony_identity.passkey_activation_challenges h
  UNION ALL
  SELECT 'N:' || to_jsonb(n)::text
    FROM mamony_identity.passkey_activation_notices n
  UNION ALL
  SELECT 'E:' || to_jsonb(e)::text
    FROM mamony_identity.security_outbox e
   WHERE e.event_type = 'PASSKEY_POSSESSION_VERIFIED'
)
SELECT (SELECT count(*) FROM verified)::text || '|' ||
       (SELECT count(*) FROM linked)::text || '|' ||
       (SELECT count(*) FROM mamony_identity.passkey_activation_notices)::text || '|' ||
       (SELECT count(*) FROM mamony_identity.security_outbox
         WHERE event_type = 'PASSKEY_POSSESSION_VERIFIED')::text || '|' ||
       (SELECT md5(coalesce(string_agg(data, E'\n' ORDER BY data), ''))
         FROM records);

