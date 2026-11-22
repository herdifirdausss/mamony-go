-- Identity v22: token-free PASSWORD_RECOVERED alert delivery and
-- independent count-only observability. No public route, production worker,
-- external mail, or recovery-assurance enrollment is enabled.

ALTER TABLE mamony_identity.password_recovery_notices
    ADD COLUMN provider_message_digest bytea NULL
        CHECK (provider_message_digest IS NULL OR octet_length(provider_message_digest)=32);
CREATE UNIQUE INDEX password_recovery_notice_provider_message
    ON mamony_identity.password_recovery_notices(provider_message_digest)
    WHERE provider_message_digest IS NOT NULL;

CREATE FUNCTION mamony_identity.claim_password_recovery_notice(p_lease text)
RETURNS TABLE(event_id text, recipient text, sendable boolean)
LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
BEGIN -- function block, not a migration transaction
    IF session_user<>'mamony_identity_recovery_notice_runtime' THEN
        RAISE EXCEPTION 'recovery notice origin denied' USING ERRCODE='42501';
    END IF;
    IF p_lease IS NULL OR p_lease !~ '^lse_[A-Za-z0-9_-]{22}$' THEN RETURN; END IF;
    RETURN QUERY
    WITH chosen AS MATERIALIZED (
        SELECT n.event_id,
               COALESCE(a.canonical_email=n.recipient
                   AND a.verified_at IS NOT NULL AND a.contact_version=1
                   AND s.status<>'DELETED'
                   AND n.occurred_at>clock_timestamp()-interval '24 hours',
                   false) AS eligible
          FROM mamony_identity.password_recovery_notices n
          LEFT JOIN mamony_identity.email_aliases a ON a.subject_id=n.subject_id
          LEFT JOIN mamony_identity.subjects s ON s.subject_id=n.subject_id
         WHERE n.status='PENDING' AND n.attempts<5
           AND n.next_attempt_at<=clock_timestamp()
           AND (n.leased_until IS NULL OR n.leased_until<clock_timestamp())
         ORDER BY n.next_attempt_at,n.event_id
         FOR UPDATE OF n SKIP LOCKED LIMIT 1
    ), advanced AS (
        UPDATE mamony_identity.password_recovery_notices n
           SET status=CASE WHEN c.eligible THEN 'PENDING' ELSE 'DEAD' END,
               lease_id=CASE WHEN c.eligible THEN p_lease ELSE NULL END,
               leased_until=CASE WHEN c.eligible THEN
                   clock_timestamp()+interval '45 seconds' ELSE NULL END,
               updated_at=clock_timestamp()
          FROM chosen c WHERE n.event_id=c.event_id
        RETURNING n.event_id,n.recipient,n.status
    )
    SELECT a.event_id,
           CASE WHEN a.status='PENDING' THEN a.recipient ELSE '' END,
           a.status='PENDING'
      FROM advanced a;
END;
$$;

CREATE FUNCTION mamony_identity.finalize_password_recovery_notice(
    p_event text,p_lease text,p_outcome text,p_provider_digest bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
BEGIN -- function block, not migration transaction
    IF session_user<>'mamony_identity_recovery_notice_runtime' THEN
        RAISE EXCEPTION 'recovery notice origin denied' USING ERRCODE='42501';
    END IF;
    IF p_event IS NULL OR p_event !~ '^evt_[A-Za-z0-9_-]{22}$'
       OR p_lease IS NULL OR p_lease !~ '^lse_[A-Za-z0-9_-]{22}$'
       OR p_outcome IS NULL OR p_outcome NOT IN ('ACCEPTED','TEMPORARY','PERMANENT')
       OR (p_outcome='ACCEPTED')<>(p_provider_digest IS NOT NULL)
       OR (p_provider_digest IS NOT NULL AND octet_length(p_provider_digest)<>32)
    THEN RETURN false; END IF;
    UPDATE mamony_identity.password_recovery_notices n
       SET status=CASE WHEN p_outcome='ACCEPTED' THEN 'ACCEPTED'
                  WHEN p_outcome='PERMANENT' OR n.attempts>=4 THEN 'DEAD'
                  ELSE 'PENDING' END,
           attempts=n.attempts+1,
           next_attempt_at=CASE WHEN p_outcome='TEMPORARY' THEN
               clock_timestamp()+CASE n.attempts
                   WHEN 0 THEN interval '1 minute'
                   WHEN 1 THEN interval '2 minutes'
                   WHEN 2 THEN interval '4 minutes'
                   ELSE interval '8 minutes' END
               ELSE n.next_attempt_at END,
           accepted_at=CASE WHEN p_outcome='ACCEPTED' THEN
               clock_timestamp() ELSE NULL END,
           provider_message_digest=CASE WHEN p_outcome='ACCEPTED' THEN
               p_provider_digest ELSE NULL END,
           lease_id=NULL,leased_until=NULL,updated_at=clock_timestamp()
     WHERE n.event_id=p_event AND n.lease_id=p_lease
       AND n.leased_until>clock_timestamp()
       AND n.status='PENDING' AND n.attempts<5;
    RETURN FOUND;
END;
$$;

-- Count-only operator boundary. Provider acceptance is NOT inbox delivery;
-- no success message, email address, event ID or provider receipt leaks here.
CREATE FUNCTION mamony_identity.password_recovery_notice_health()
RETURNS TABLE(pending_due bigint,dead bigint,submitted_unconfirmed bigint,
              retrying bigint,expired_pending bigint)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN -- function block, not migration transaction
    IF session_user<>'mamony_identity_recovery_notice_monitor_runtime' THEN
        RAISE EXCEPTION 'recovery notice monitoring origin denied'
            USING ERRCODE='42501';
    END IF;
    RETURN QUERY SELECT
        (SELECT count(*) FROM mamony_identity.password_recovery_notices n
         WHERE n.status='PENDING' AND n.next_attempt_at<=clock_timestamp()),
        (SELECT count(*) FROM mamony_identity.password_recovery_notices n
         WHERE n.status='DEAD'),
        (SELECT count(*) FROM mamony_identity.password_recovery_notices n
         WHERE n.status='ACCEPTED'),
        (SELECT count(*) FROM mamony_identity.password_recovery_notices n
         WHERE n.status='PENDING' AND n.attempts>0),
        (SELECT count(*) FROM mamony_identity.password_recovery_notices n
         WHERE n.status='PENDING' AND n.occurred_at<=clock_timestamp()-interval '24 hours');
END;
$$;

-- Bounded recipient PII erasure. Existing identifier-only audit survives.
CREATE FUNCTION mamony_identity.purge_old_password_recovery_notices()
RETURNS integer LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE removed integer;
BEGIN -- function block, not migration transaction
    IF session_user<>'mamony_identity_maintenance_runtime' THEN
        RAISE EXCEPTION 'recovery notice purge origin denied' USING ERRCODE='42501';
    END IF;
    WITH chosen AS MATERIALIZED (
        SELECT n.event_id FROM mamony_identity.password_recovery_notices n
         WHERE n.occurred_at<clock_timestamp()-interval '7 days'
           AND (n.leased_until IS NULL OR n.leased_until<clock_timestamp())
         ORDER BY n.occurred_at,n.event_id
         FOR UPDATE OF n SKIP LOCKED LIMIT 100
    ), purged AS (
        DELETE FROM mamony_identity.password_recovery_notices n
        USING chosen c WHERE n.event_id=c.event_id RETURNING n.event_id
    )
    SELECT count(*)::integer INTO removed FROM purged;
    RETURN removed;
END;
$$;

REVOKE ALL ON FUNCTION
    mamony_identity.claim_password_recovery_notice(text),
    mamony_identity.finalize_password_recovery_notice(text,text,text,bytea),
    mamony_identity.password_recovery_notice_health(),
    mamony_identity.purge_old_password_recovery_notices()
    FROM PUBLIC;
GRANT USAGE ON SCHEMA mamony_identity
    TO mamony_identity_recovery_notice_runtime,
       mamony_identity_recovery_notice_monitor_runtime;
GRANT EXECUTE ON FUNCTION
    mamony_identity.claim_password_recovery_notice(text),
    mamony_identity.finalize_password_recovery_notice(text,text,text,bytea)
    TO mamony_identity_recovery_notice_runtime;
GRANT EXECUTE ON FUNCTION mamony_identity.password_recovery_notice_health()
    TO mamony_identity_recovery_notice_monitor_runtime;
GRANT EXECUTE ON FUNCTION mamony_identity.purge_old_password_recovery_notices()
    TO mamony_identity_maintenance_runtime;

-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (22, '1ae5690ebf565a8a5dc7a3d54cd7a7ff998c21423adf60ab47e34fb2ebfadbb6');
