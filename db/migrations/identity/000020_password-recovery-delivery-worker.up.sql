-- Identity v20: a strictly fenced recovery delivery worker credential.
-- Delivers only current, eligible, encrypted recovery challenges, with no
-- direct table SELECT/UPDATE and no production process or public HTTP route.
CREATE FUNCTION mamony_identity.claim_password_recovery_delivery(p_lease text)
RETURNS TABLE(challenge_id text, recipient text, key_id text,
              sealed_token bytea, expires_at timestamptz, sendable boolean)
LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
BEGIN -- PL/pgSQL block, not a migration transaction
    IF session_user <> 'mamony_identity_recovery_delivery_runtime' THEN
        RAISE EXCEPTION 'recovery delivery role denied'
            USING ERRCODE='42501';
    END IF;
    IF p_lease IS NULL OR p_lease !~ '^lse_[A-Za-z0-9_-]{22}$' THEN
        RETURN;
    END IF;
    RETURN QUERY
    WITH candidate AS MATERIALIZED (
        SELECT d.challenge_id, c.canonical_email, c.expires_at,
               (s.status='ACTIVE' AND s.session_epoch=c.subject_epoch
                AND a.canonical_email=c.canonical_email
                AND a.contact_version=c.contact_version
                AND a.verified_at IS NOT NULL
                AND c.consumed_at IS NULL AND c.revoked_at IS NULL
                AND c.expires_at>clock_timestamp()+interval '15 seconds'
               ) AS eligible
          FROM mamony_identity.password_recovery_deliveries d
          JOIN mamony_identity.password_recovery_challenges c
            ON c.challenge_id=d.challenge_id
          JOIN mamony_identity.subjects s ON s.subject_id=c.subject_id
          JOIN mamony_identity.email_aliases a ON a.subject_id=c.subject_id
         WHERE d.status='PENDING' AND d.attempts<5
           AND d.next_attempt_at<=clock_timestamp()
           AND (d.leased_until IS NULL OR d.leased_until<clock_timestamp())
         ORDER BY d.next_attempt_at,d.challenge_id
         FOR UPDATE OF d SKIP LOCKED LIMIT 1
    ), advanced AS (
        UPDATE mamony_identity.password_recovery_deliveries d
           SET status=CASE WHEN candidate.eligible THEN 'PENDING' ELSE 'DEAD' END,
               lease_id=CASE WHEN candidate.eligible THEN p_lease ELSE NULL END,
               leased_until=CASE WHEN candidate.eligible THEN
                   clock_timestamp()+interval '45 seconds' ELSE NULL END,
               updated_at=clock_timestamp()
          FROM candidate WHERE d.challenge_id=candidate.challenge_id
        RETURNING d.challenge_id,d.key_id,d.sealed_token,d.status
    )
    SELECT a.challenge_id,
           CASE WHEN a.status='PENDING' THEN c.canonical_email ELSE '' END,
           CASE WHEN a.status='PENDING' THEN a.key_id ELSE '' END,
           CASE WHEN a.status='PENDING' THEN a.sealed_token ELSE ''::bytea END,
           c.expires_at, a.status='PENDING'
      FROM advanced a JOIN candidate c ON c.challenge_id=a.challenge_id;
END;
$$;

CREATE FUNCTION mamony_identity.finalize_password_recovery_delivery(
    p_challenge text,p_lease text,p_outcome text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
BEGIN -- PL/pgSQL block, not a migration transaction
    IF session_user <> 'mamony_identity_recovery_delivery_runtime' THEN
        RAISE EXCEPTION 'recovery delivery role denied'
            USING ERRCODE='42501';
    END IF;
    IF p_challenge IS NULL OR p_challenge !~ '^rcv_[A-Za-z0-9_-]{22}$'
       OR p_lease IS NULL OR p_lease !~ '^lse_[A-Za-z0-9_-]{22}$'
       OR p_outcome IS NULL OR p_outcome NOT IN
           ('ACCEPTED','TEMPORARY','PERMANENT') THEN RETURN false; END IF;
    UPDATE mamony_identity.password_recovery_deliveries d
       SET status=CASE WHEN p_outcome='ACCEPTED' THEN 'ACCEPTED'
                      WHEN p_outcome='PERMANENT' OR d.attempts>=4 THEN 'DEAD'
                      ELSE 'PENDING' END,
           attempts=d.attempts+1,
           next_attempt_at=CASE WHEN p_outcome='TEMPORARY' THEN
               clock_timestamp()+CASE d.attempts
                   WHEN 0 THEN interval '1 minute'
                   WHEN 1 THEN interval '2 minutes'
                   WHEN 2 THEN interval '4 minutes'
                   ELSE interval '8 minutes' END
               ELSE d.next_attempt_at END,
           accepted_at=CASE WHEN p_outcome='ACCEPTED' THEN
               clock_timestamp() ELSE NULL END,
           lease_id=NULL,leased_until=NULL,updated_at=clock_timestamp()
     WHERE d.challenge_id=p_challenge AND d.lease_id=p_lease
       AND d.leased_until>clock_timestamp() AND d.status='PENDING'
       AND d.attempts<5;
    RETURN FOUND;
END;
$$;

REVOKE ALL ON FUNCTION mamony_identity.claim_password_recovery_delivery(text),
    mamony_identity.finalize_password_recovery_delivery(text,text,text)
    FROM PUBLIC;
GRANT USAGE ON SCHEMA mamony_identity
    TO mamony_identity_recovery_delivery_runtime;
GRANT EXECUTE ON FUNCTION
    mamony_identity.claim_password_recovery_delivery(text),
    mamony_identity.finalize_password_recovery_delivery(text,text,text)
    TO mamony_identity_recovery_delivery_runtime;

-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (20, '__REPLACE_SHA256__');
