-- Identity v16: durable, bounded security notices, separate from the
-- identifier-only security outbox and encrypted email verification proofs.
-- No real-mail worker, public HTTP route or production secrets are enabled.
CREATE TABLE mamony_identity.password_change_notices (
    event_id text PRIMARY KEY REFERENCES mamony_identity.security_outbox(event_id),
    subject_id text NOT NULL REFERENCES mamony_identity.subjects(subject_id),
    recipient text NOT NULL CHECK (length(recipient) BETWEEN 3 AND 254
        AND recipient=lower(recipient)),
    occurred_at timestamptz NOT NULL,
    status text NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING','ACCEPTED','DEAD')),
    attempts smallint NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 5),
    next_attempt_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    lease_id text NULL CHECK (lease_id IS NULL OR lease_id ~ '^lse_[A-Za-z0-9_-]{22}$'),
    leased_until timestamptz NULL,
    accepted_at timestamptz NULL,
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CHECK ((lease_id IS NULL) = (leased_until IS NULL)),
    CHECK (accepted_at IS NULL OR status='ACCEPTED')
);
CREATE INDEX password_change_notices_due
    ON mamony_identity.password_change_notices(next_attempt_at,event_id)
    WHERE status='PENDING';
REVOKE ALL ON mamony_identity.password_change_notices FROM PUBLIC;

-- IMPORTANT: the verifier role can INSERT arbitrary security_outbox types
-- under pre-v16 privileges. Do NOT treat that alone as a trusted send order.
-- session_user remains the ORIGINAL authenticated DB role when the v15
-- security-definer password-change function inserts PASSWORD_CHANGED.
-- A forged event from registration/verifier/other roles fails atomically.
CREATE FUNCTION mamony_identity.enqueue_password_change_notice()
RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    recipient_email text;
BEGIN -- PL/pgSQL function block; not a migration transaction
    IF NEW.event_type <> 'PASSWORD_CHANGED' THEN RETURN NEW; END IF;
    IF session_user <> 'mamony_identity_password_change_runtime' THEN
        RAISE EXCEPTION 'password change notification event origin denied'
            USING ERRCODE='42501';
    END IF;
    SELECT a.canonical_email INTO recipient_email
      FROM mamony_identity.email_aliases a
      JOIN mamony_identity.subjects s ON s.subject_id=a.subject_id
     WHERE s.subject_id=NEW.subject_id AND s.status='ACTIVE'
       AND a.verified_at IS NOT NULL AND a.contact_version=1;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'password change event missing verified recipient'
            USING ERRCODE='23514';
    END IF;
    INSERT INTO mamony_identity.password_change_notices
        (event_id,subject_id,recipient,occurred_at)
    VALUES (NEW.event_id,NEW.subject_id,recipient_email,NEW.occurred_at);
    RETURN NEW;
END;
$$;
REVOKE ALL ON FUNCTION mamony_identity.enqueue_password_change_notice() FROM PUBLIC;
CREATE TRIGGER security_outbox_password_changed_notice
AFTER INSERT ON mamony_identity.security_outbox FOR EACH ROW
EXECUTE FUNCTION mamony_identity.enqueue_password_change_notice();

-- Worker is EXECUTE-only: it never receives unrestricted SELECT on Identity
-- contacts, sessions, password hashes or generic security event history.
CREATE FUNCTION mamony_identity.claim_password_change_notice(p_lease text)
RETURNS TABLE(event_id text,recipient text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN -- PL/pgSQL function block; not a migration transaction
    IF p_lease IS NULL OR p_lease !~ '^lse_[A-Za-z0-9_-]{22}$' THEN
        RETURN;
    END IF;
    RETURN QUERY
    WITH chosen AS MATERIALIZED (
        SELECT n.event_id,
               COALESCE(a.canonical_email=n.recipient
                   AND a.verified_at IS NOT NULL AND a.contact_version=1
                   AND s.status<>'DELETED'
                   AND n.occurred_at>clock_timestamp()-interval '24 hours',
                   false) AS eligible
          FROM mamony_identity.password_change_notices n
          LEFT JOIN mamony_identity.email_aliases a ON a.subject_id=n.subject_id
          LEFT JOIN mamony_identity.subjects s ON s.subject_id=n.subject_id
         WHERE n.status='PENDING' AND n.attempts<5
           AND n.next_attempt_at<=clock_timestamp()
           AND (n.leased_until IS NULL OR n.leased_until<clock_timestamp())
         ORDER BY n.next_attempt_at,n.event_id
         FOR UPDATE OF n SKIP LOCKED LIMIT 1
    ), updated AS (
        UPDATE mamony_identity.password_change_notices n
           SET status=CASE WHEN c.eligible THEN 'PENDING' ELSE 'DEAD' END,
               lease_id=CASE WHEN c.eligible THEN p_lease ELSE NULL END,
               leased_until=CASE WHEN c.eligible THEN
                   clock_timestamp()+interval '45 seconds' ELSE NULL END,
               updated_at=clock_timestamp()
          FROM chosen c WHERE n.event_id=c.event_id
        RETURNING n.event_id,n.recipient,n.status
    )
    SELECT u.event_id,u.recipient FROM updated u WHERE u.status='PENDING';
END;
$$;

CREATE FUNCTION mamony_identity.finalize_password_change_notice(
    p_event text,p_lease text,p_outcome text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
BEGIN -- PL/pgSQL function block; not a migration transaction
    IF p_event IS NULL OR p_event !~ '^evt_[A-Za-z0-9_-]{22}$'
       OR p_lease IS NULL OR p_lease !~ '^lse_[A-Za-z0-9_-]{22}$'
       OR p_outcome IS NULL OR p_outcome NOT IN
           ('ACCEPTED','TEMPORARY','PERMANENT') THEN RETURN false; END IF;
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
           lease_id=NULL,leased_until=NULL,updated_at=clock_timestamp()
     WHERE n.event_id=p_event AND n.lease_id=p_lease
       AND n.leased_until>clock_timestamp()
       AND n.status='PENDING' AND n.attempts<5;
    RETURN FOUND;
END;
$$;

-- Bounded PII retention; the identifier-only security_outbox event is kept.
-- Pending notices older than seven days have already passed their 24h TTL.
CREATE FUNCTION mamony_identity.purge_old_password_change_notices()
RETURNS integer LANGUAGE sql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
    WITH targets AS MATERIALIZED (
        SELECT event_id FROM mamony_identity.password_change_notices
         WHERE occurred_at<clock_timestamp()-interval '7 days'
           AND (leased_until IS NULL OR leased_until<clock_timestamp())
         ORDER BY occurred_at,event_id
         FOR UPDATE SKIP LOCKED LIMIT 100
    ), erased AS (
        DELETE FROM mamony_identity.password_change_notices n
         USING targets t WHERE n.event_id=t.event_id
         RETURNING n.event_id
    )
    SELECT count(*)::integer FROM erased;
$$;
REVOKE ALL ON FUNCTION
    mamony_identity.claim_password_change_notice(text),
    mamony_identity.finalize_password_change_notice(text,text,text),
    mamony_identity.purge_old_password_change_notices()
FROM PUBLIC;
GRANT USAGE ON SCHEMA mamony_identity TO mamony_identity_security_notice_runtime;
GRANT EXECUTE ON FUNCTION
    mamony_identity.claim_password_change_notice(text),
    mamony_identity.finalize_password_change_notice(text,text,text)
TO mamony_identity_security_notice_runtime;
GRANT EXECUTE ON FUNCTION
    mamony_identity.purge_old_password_change_notices()
TO mamony_identity_maintenance_runtime;
-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (16, '6b0281c1ee8b4a0d52205aab975629292a9b648bea906d3f7d10edf9eb73a86e');
