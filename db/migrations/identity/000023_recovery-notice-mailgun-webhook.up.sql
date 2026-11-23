-- Identity v23: separate signed recovery notice Mailgun webhook reconciliation.
-- The HTTP verifier is unmounted and performs HMAC verification before SQL.
-- This role cannot see or mutate PASSWORD_CHANGED notice receipts, account
-- verifiers, recovery proofs, sessions, assurance or recipient-bearing rows.
ALTER TABLE mamony_identity.password_recovery_notices
    ADD COLUMN provider_state text NOT NULL DEFAULT 'UNCONFIRMED'
        CHECK (provider_state IN ('UNCONFIRMED','SUBMITTED',
            'TEMPORARY_FAILURE','DELIVERED','PERMANENT_FAILURE',
            'COMPLAINED','UNSUBSCRIBED'));

CREATE TABLE mamony_identity.password_recovery_notice_mailgun_receipts (
    token_digest bytea PRIMARY KEY CHECK (octet_length(token_digest)=32),
    event_digest bytea NOT NULL UNIQUE CHECK (octet_length(event_digest)=32),
    message_digest bytea NOT NULL CHECK (octet_length(message_digest)=32),
    delivery_state text NOT NULL CHECK (delivery_state IN
        ('TEMPORARY_FAILURE','DELIVERED','PERMANENT_FAILURE',
         'COMPLAINED','UNSUBSCRIBED')),
    event_at timestamptz NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX password_recovery_receipts_message
    ON mamony_identity.password_recovery_notice_mailgun_receipts(message_digest);
CREATE INDEX password_recovery_receipts_retention
    ON mamony_identity.password_recovery_notice_mailgun_receipts(recorded_at);
REVOKE ALL ON mamony_identity.password_recovery_notice_mailgun_receipts FROM PUBLIC;

-- v22 role still controls ONLY lease-fenced finalize. v23 adds recovery-
-- specific receipt-before-ack reconciliation. No extra table privileges.
CREATE OR REPLACE FUNCTION mamony_identity.finalize_password_recovery_notice(
    p_event text,p_lease text,p_outcome text,p_provider_digest bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
BEGIN -- PL/pgSQL function block, not migration transaction
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
           provider_state=CASE WHEN p_outcome='ACCEPTED' THEN
               COALESCE((
                   SELECT r.delivery_state
                     FROM mamony_identity.password_recovery_notice_mailgun_receipts r
                    WHERE r.message_digest=p_provider_digest
                    ORDER BY mamony_identity.security_notice_state_rank(r.delivery_state) DESC,
                             r.event_at DESC LIMIT 1
               ),'SUBMITTED') ELSE 'UNCONFIRMED' END,
           lease_id=NULL,leased_until=NULL,updated_at=clock_timestamp()
     WHERE n.event_id=p_event AND n.lease_id=p_lease
       AND n.leased_until>clock_timestamp()
       AND n.status='PENDING' AND n.attempts<5;
    RETURN FOUND;
END;
$$;

-- Receipt recorder is signer-verified by the unmounted Go HTTP adapter.
-- Origin check enforces that the recovery webhook SQL credential cannot
-- be impersonated via SET ROLE or another Identity runtime.
CREATE FUNCTION mamony_identity.record_mailgun_recovery_notice_receipt(
    p_token bytea,p_event bytea,p_message bytea,p_state text,p_event_at timestamptz)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE inserted integer;
BEGIN -- PL/pgSQL function block, not migration transaction
    IF session_user<>'mamony_identity_recovery_notice_webhook_runtime' THEN
        RAISE EXCEPTION 'recovery receipt origin denied' USING ERRCODE='42501';
    END IF;
    IF p_token IS NULL OR octet_length(p_token)<>32
       OR p_event IS NULL OR octet_length(p_event)<>32
       OR p_message IS NULL OR octet_length(p_message)<>32
       OR p_state IS NULL OR mamony_identity.security_notice_state_rank(p_state)<2
       OR p_event_at IS NULL
       OR p_event_at<clock_timestamp()-interval '10 hours'
       OR p_event_at>clock_timestamp()+interval '15 minutes'
    THEN RETURN false; END IF;
    INSERT INTO mamony_identity.password_recovery_notice_mailgun_receipts
        (token_digest,event_digest,message_digest,delivery_state,event_at)
    VALUES (p_token,p_event,p_message,p_state,p_event_at)
    ON CONFLICT DO NOTHING;
    GET DIAGNOSTICS inserted=ROW_COUNT;
    IF inserted=0 THEN RETURN true; END IF;
    UPDATE mamony_identity.password_recovery_notices n
       SET provider_state=p_state,updated_at=clock_timestamp()
     WHERE n.provider_message_digest=p_message
       AND n.status='ACCEPTED'
       AND mamony_identity.security_notice_state_rank(p_state)>
           mamony_identity.security_notice_state_rank(n.provider_state);
    RETURN true;
END;
$$;
REVOKE ALL ON FUNCTION
    mamony_identity.record_mailgun_recovery_notice_receipt(bytea,bytea,bytea,text,timestamptz)
FROM PUBLIC;
GRANT USAGE ON SCHEMA mamony_identity
    TO mamony_identity_recovery_notice_webhook_runtime;
GRANT EXECUTE ON FUNCTION
    mamony_identity.record_mailgun_recovery_notice_receipt(bytea,bytea,bytea,text,timestamptz)
TO mamony_identity_recovery_notice_webhook_runtime;

-- The original five-field v22 health function retains its ABI. Its
-- submitted-unconfirmed metric is corrected to exclude DELIVERED, bounce,
-- complaint and unsubscribe states, including v23 callback updates.
CREATE OR REPLACE FUNCTION mamony_identity.password_recovery_notice_health()
RETURNS TABLE(pending_due bigint,dead bigint,submitted_unconfirmed bigint,
              retrying bigint,expired_pending bigint)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN -- PL/pgSQL function block; not a migration transaction
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
         WHERE n.status='ACCEPTED' AND n.provider_state IN
             ('SUBMITTED','TEMPORARY_FAILURE')),
        (SELECT count(*) FROM mamony_identity.password_recovery_notices n
         WHERE n.status='PENDING' AND n.attempts>0),
        (SELECT count(*) FROM mamony_identity.password_recovery_notices n
         WHERE n.status='PENDING'
           AND n.occurred_at<=clock_timestamp()-interval '24 hours');
END;
$$;

-- Extra count-only metrics live in a separate function to preserve the
-- v22 five-field contract for already-existing consumers.
CREATE FUNCTION mamony_identity.password_recovery_notice_delivery_health()
RETURNS TABLE(delivered bigint,provider_failures bigint,unmatched_events bigint)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN -- PL/pgSQL function block; not a migration transaction
    IF session_user<>'mamony_identity_recovery_notice_monitor_runtime' THEN
        RAISE EXCEPTION 'recovery delivery monitoring origin denied'
            USING ERRCODE='42501';
    END IF;
    RETURN QUERY SELECT
        (SELECT count(*) FROM mamony_identity.password_recovery_notices n
         WHERE n.provider_state='DELIVERED'),
        (SELECT count(*) FROM mamony_identity.password_recovery_notices n
         WHERE n.provider_state IN
             ('PERMANENT_FAILURE','COMPLAINED','UNSUBSCRIBED')),
        (SELECT count(*) FROM mamony_identity.password_recovery_notice_mailgun_receipts r
         WHERE NOT EXISTS (
             SELECT 1 FROM mamony_identity.password_recovery_notices n
              WHERE n.provider_message_digest=r.message_digest));
END;
$$;
REVOKE ALL ON FUNCTION
    mamony_identity.password_recovery_notice_delivery_health() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION
    mamony_identity.password_recovery_notice_delivery_health()
TO mamony_identity_recovery_notice_monitor_runtime;

-- Erase only receipt digests older than seven days in bounded batches.
-- No deletion of immutable security audit, recovery grants or notice data.
CREATE FUNCTION mamony_identity.purge_old_recovery_notice_mailgun_receipts()
RETURNS integer LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE purged_count integer;
BEGIN -- PL/pgSQL function block; not a migration transaction
    IF session_user<>'mamony_identity_maintenance_runtime' THEN
        RAISE EXCEPTION 'recovery receipt purge origin denied'
            USING ERRCODE='42501';
    END IF;
    WITH selected AS MATERIALIZED (
        SELECT token_digest
          FROM mamony_identity.password_recovery_notice_mailgun_receipts
         WHERE recorded_at<clock_timestamp()-interval '7 days'
         ORDER BY recorded_at,token_digest
         FOR UPDATE SKIP LOCKED LIMIT 100
    ), purged AS (
        DELETE FROM mamony_identity.password_recovery_notice_mailgun_receipts r
         USING selected s WHERE r.token_digest=s.token_digest
        RETURNING r.token_digest
    )
    SELECT count(*)::integer INTO purged_count FROM purged;
    RETURN purged_count;
END;
$$;
REVOKE ALL ON FUNCTION
    mamony_identity.purge_old_recovery_notice_mailgun_receipts() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION
    mamony_identity.purge_old_recovery_notice_mailgun_receipts()
TO mamony_identity_maintenance_runtime;

-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (23, 'a75100ea0624d6d749bcd56a5a6431e791aad5d93a8746ec4ac0753eff3a2035');
