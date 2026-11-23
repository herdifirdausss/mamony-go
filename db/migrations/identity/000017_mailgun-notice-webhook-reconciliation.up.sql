-- Identity v17: provider response correlation, signed-webhook receipt dedup,
-- and bounded operator health aggregates. No HTTP route is mounted.
-- The new tables store SHA-256 digests only: no webhook body, recipient,
-- bearer secret, raw provider message ID, signing token or signature.
ALTER TABLE mamony_identity.password_change_notices
    ADD COLUMN provider_message_digest bytea NULL
        CHECK (provider_message_digest IS NULL OR octet_length(provider_message_digest)=32),
    ADD COLUMN provider_state text NOT NULL DEFAULT 'UNCONFIRMED'
        CHECK (provider_state IN ('UNCONFIRMED','SUBMITTED','TEMPORARY_FAILURE',
             'DELIVERED','PERMANENT_FAILURE','COMPLAINED','UNSUBSCRIBED'));
CREATE UNIQUE INDEX password_change_notice_provider_message
    ON mamony_identity.password_change_notices(provider_message_digest)
    WHERE provider_message_digest IS NOT NULL;

CREATE TABLE mamony_identity.security_notice_mailgun_receipts (
    token_digest bytea PRIMARY KEY CHECK (octet_length(token_digest)=32),
    event_digest bytea NOT NULL UNIQUE CHECK (octet_length(event_digest)=32),
    message_digest bytea NOT NULL CHECK (octet_length(message_digest)=32),
    delivery_state text NOT NULL CHECK (delivery_state IN
        ('TEMPORARY_FAILURE','DELIVERED','PERMANENT_FAILURE','COMPLAINED','UNSUBSCRIBED')),
    event_at timestamptz NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX security_notice_receipts_message
    ON mamony_identity.security_notice_mailgun_receipts(message_digest);
CREATE INDEX security_notice_receipts_retention
    ON mamony_identity.security_notice_mailgun_receipts(recorded_at);
REVOKE ALL ON mamony_identity.security_notice_mailgun_receipts FROM PUBLIC;

-- Ranks ensure that late provider acknowledgements or delayed delivered
-- callbacks never overwrite a serious non-delivery or complaint result.
-- The function reads only fixed server-owned state and has no runtime grants.
CREATE FUNCTION mamony_identity.security_notice_state_rank(p_state text)
RETURNS integer LANGUAGE sql IMMUTABLE SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
SELECT CASE p_state
    WHEN 'UNCONFIRMED' THEN 0
    WHEN 'SUBMITTED' THEN 1
    WHEN 'TEMPORARY_FAILURE' THEN 2
    WHEN 'DELIVERED' THEN 3
    WHEN 'PERMANENT_FAILURE' THEN 4
    WHEN 'COMPLAINED' THEN 5
    WHEN 'UNSUBSCRIBED' THEN 6
    ELSE -1 END;
$$;
REVOKE ALL ON FUNCTION mamony_identity.security_notice_state_rank(text) FROM PUBLIC;

-- v16's three-arg finalize no longer has worker EXECUTE: accepting an email
-- without persisting its Mailgun message digest would make webhook events
-- impossible to correlate. Existing queued notices remain compatible.
REVOKE EXECUTE ON FUNCTION mamony_identity.finalize_password_change_notice(text,text,text)
    FROM mamony_identity_security_notice_runtime;

CREATE FUNCTION mamony_identity.finalize_password_change_notice_v2(
    p_event text,p_lease text,p_outcome text,p_provider_digest bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
BEGIN -- PL/pgSQL function block; not a migration transaction
    IF p_event IS NULL OR p_event !~ '^evt_[A-Za-z0-9_-]{22}$'
       OR p_lease IS NULL OR p_lease !~ '^lse_[A-Za-z0-9_-]{22}$'
       OR p_outcome IS NULL OR p_outcome NOT IN ('ACCEPTED','TEMPORARY','PERMANENT')
       OR (p_outcome='ACCEPTED')<>(p_provider_digest IS NOT NULL)
       OR (p_provider_digest IS NOT NULL AND octet_length(p_provider_digest)<>32)
    THEN RETURN false; END IF;

    UPDATE mamony_identity.password_change_notices n
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
                    FROM mamony_identity.security_notice_mailgun_receipts r
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
REVOKE ALL ON FUNCTION mamony_identity.finalize_password_change_notice_v2(text,text,text,bytea)
    FROM PUBLIC;
GRANT EXECUTE ON FUNCTION mamony_identity.finalize_password_change_notice_v2(text,text,text,bytea)
    TO mamony_identity_security_notice_runtime;

-- This EXECUTE-only receiver is deliberately isolated from the Mailgun
-- sending key/role. Its caller must first verify Mailgun's webhook signature
-- with a distinct key; SQL receives only synthetic collision-resistant digests
-- and a bounded normalized outcome. This DB privilege alone is NOT proof of
-- Mailgun origin if the webhook workload credential is compromised.
CREATE FUNCTION mamony_identity.record_mailgun_security_notice_receipt(
    p_token bytea,p_event bytea,p_message bytea,p_state text,p_event_at timestamptz)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    inserted integer;
BEGIN -- PL/pgSQL function block; not a migration transaction
    IF p_token IS NULL OR octet_length(p_token)<>32
       OR p_event IS NULL OR octet_length(p_event)<>32
       OR p_message IS NULL OR octet_length(p_message)<>32
       OR p_state IS NULL OR mamony_identity.security_notice_state_rank(p_state)<2
       OR p_event_at IS NULL OR p_event_at<clock_timestamp()-interval '10 hours'
       OR p_event_at>clock_timestamp()+interval '15 minutes'
    THEN RETURN false; END IF;
    INSERT INTO mamony_identity.security_notice_mailgun_receipts
        (token_digest,event_digest,message_digest,delivery_state,event_at)
    VALUES (p_token,p_event,p_message,p_state,p_event_at)
    ON CONFLICT DO NOTHING;
    GET DIAGNOSTICS inserted = ROW_COUNT;
    IF inserted=0 THEN RETURN true; END IF;
    UPDATE mamony_identity.password_change_notices n
       SET provider_state=p_state,updated_at=clock_timestamp()
     WHERE n.provider_message_digest=p_message
       AND n.status='ACCEPTED'
       AND mamony_identity.security_notice_state_rank(p_state)>
           mamony_identity.security_notice_state_rank(n.provider_state);
    RETURN true;
END;
$$;
REVOKE ALL ON FUNCTION
    mamony_identity.record_mailgun_security_notice_receipt(bytea,bytea,bytea,text,timestamptz)
FROM PUBLIC;
GRANT USAGE ON SCHEMA mamony_identity TO mamony_identity_notice_webhook_runtime;
GRANT EXECUTE ON FUNCTION
    mamony_identity.record_mailgun_security_notice_receipt(bytea,bytea,bytea,text,timestamptz)
TO mamony_identity_notice_webhook_runtime;

-- Only aggregate numbers cross the monitoring boundary; no IDs, addresses,
-- provider metadata, arbitrary caller-supplied SQL or event bodies.
CREATE FUNCTION mamony_identity.security_notice_health()
RETURNS TABLE(pending_due bigint,dead bigint,submitted_unknown bigint,
              delivered bigint,provider_failures bigint,unmatched_events bigint)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
SELECT
    (SELECT count(*) FROM mamony_identity.password_change_notices n
      WHERE n.status='PENDING' AND n.next_attempt_at<=clock_timestamp()),
    (SELECT count(*) FROM mamony_identity.password_change_notices n
      WHERE n.status='DEAD'),
    (SELECT count(*) FROM mamony_identity.password_change_notices n
      WHERE n.status='ACCEPTED' AND n.provider_state IN ('SUBMITTED','TEMPORARY_FAILURE')),
    (SELECT count(*) FROM mamony_identity.password_change_notices n
      WHERE n.provider_state='DELIVERED'),
    (SELECT count(*) FROM mamony_identity.password_change_notices n
      WHERE n.provider_state IN ('PERMANENT_FAILURE','COMPLAINED','UNSUBSCRIBED')),
    (SELECT count(*) FROM mamony_identity.security_notice_mailgun_receipts r
      WHERE NOT EXISTS (
          SELECT 1 FROM mamony_identity.password_change_notices n
           WHERE n.provider_message_digest=r.message_digest));
$$;
REVOKE ALL ON FUNCTION mamony_identity.security_notice_health() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION mamony_identity.security_notice_health()
    TO mamony_identity_maintenance_runtime;

-- Seven-day bounded receipt erasure; the event-only security audit survives.
CREATE FUNCTION mamony_identity.purge_old_mailgun_security_receipts()
RETURNS integer LANGUAGE sql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
WITH selected AS MATERIALIZED (
    SELECT token_digest FROM mamony_identity.security_notice_mailgun_receipts
    WHERE recorded_at<clock_timestamp()-interval '7 days'
    ORDER BY recorded_at,token_digest FOR UPDATE SKIP LOCKED LIMIT 100
), purged AS (
    DELETE FROM mamony_identity.security_notice_mailgun_receipts r
    USING selected s WHERE r.token_digest=s.token_digest
    RETURNING r.token_digest
)
SELECT count(*)::integer FROM purged;
$$;
REVOKE ALL ON FUNCTION mamony_identity.purge_old_mailgun_security_receipts() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION mamony_identity.purge_old_mailgun_security_receipts()
    TO mamony_identity_maintenance_runtime;
-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (17, 'bdb0cc7fed49c0cce236ab1eed0a9cc391c620d3df7b599193c4d09de4632d7f');
