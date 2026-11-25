-- RH-03a3: aggregate positive witnesses from the synthetic integration fixture.
-- No subject IDs, secret material, credential values or bearer tokens are output.
-- These are relational observations, not application enforcement or release authority.
BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT 'password_change', count(*)
FROM mamony_identity.security_outbox o
JOIN mamony_identity.password_change_notices n
  ON n.event_id=o.event_id AND n.subject_id=o.subject_id
JOIN mamony_identity.subjects s ON s.subject_id=o.subject_id
JOIN mamony_identity.password_verifiers p ON p.subject_id=s.subject_id
JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
WHERE o.event_type='PASSWORD_CHANGED' AND n.recipient=a.canonical_email
  AND a.verified_at IS NOT NULL
  AND EXISTS (SELECT 1 FROM mamony_identity.opaque_sessions x
              WHERE x.subject_id=s.subject_id AND x.subject_epoch<s.session_epoch)
  AND EXISTS (SELECT 1 FROM mamony_identity.opaque_sessions x
              WHERE x.subject_id=s.subject_id AND x.subject_epoch=s.session_epoch);
SELECT 'password_recovery', count(*)
FROM mamony_identity.security_outbox o
JOIN mamony_identity.password_recovery_notices n
  ON n.event_id=o.event_id AND n.subject_id=o.subject_id
JOIN mamony_identity.subjects s ON s.subject_id=o.subject_id
JOIN mamony_identity.password_verifiers p ON p.subject_id=s.subject_id
JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
WHERE o.event_type='PASSWORD_RECOVERED' AND n.recipient=a.canonical_email
  AND a.verified_at IS NOT NULL
  AND EXISTS (SELECT 1 FROM mamony_identity.password_recovery_challenges c
              WHERE c.subject_id=s.subject_id AND c.consumed_at IS NOT NULL)
  AND EXISTS (SELECT 1 FROM mamony_identity.password_recovery_assurance_grants g
              WHERE g.subject_id=s.subject_id AND g.consumed_at IS NOT NULL)
  AND EXISTS (SELECT 1 FROM mamony_identity.opaque_sessions x
              WHERE x.subject_id=s.subject_id AND x.subject_epoch<s.session_epoch);
SELECT 'code_rotation', count(*)
FROM mamony_identity.security_outbox o
JOIN mamony_identity.password_recovery_code_notices n
  ON n.event_id=o.event_id AND n.subject_id=o.subject_id
JOIN mamony_identity.subjects s ON s.subject_id=o.subject_id
JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
WHERE o.event_type='RECOVERY_CODES_ROTATED' AND n.recipient=a.canonical_email
  AND a.verified_at IS NOT NULL
  AND (SELECT count(*) FROM mamony_identity.password_recovery_offline_codes c
       WHERE c.subject_id=s.subject_id)=5
  AND EXISTS (SELECT 1 FROM mamony_identity.password_login_grants g
              WHERE g.subject_id=s.subject_id AND g.consumed_at IS NOT NULL);
COMMIT;
