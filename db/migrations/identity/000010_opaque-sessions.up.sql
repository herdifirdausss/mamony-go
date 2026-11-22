-- Identity v10: opaque session authority, fixed expiry and revocation.
-- This does not mount login or allow a client to authorize financial actions.
ALTER TABLE mamony_identity.subjects
    ADD COLUMN session_epoch bigint NOT NULL DEFAULT 1
    CHECK (session_epoch > 0);

-- Any status transition invalidates ALL tokens minted in an earlier epoch.
-- This runs in the status writer's transaction. Contact-reset/credential
-- rotation workflows MUST separately bump the epoch in their future ADR.
CREATE FUNCTION mamony_identity.bump_session_epoch_on_status()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, pg_temp
AS $$
BEGIN
    IF NEW.status IS DISTINCT FROM OLD.status THEN
        NEW.session_epoch := OLD.session_epoch + 1;
    END IF;
    RETURN NEW;
END;
$$;
REVOKE ALL ON FUNCTION mamony_identity.bump_session_epoch_on_status() FROM PUBLIC;
CREATE TRIGGER subjects_status_bumps_session_epoch
    BEFORE UPDATE OF status ON mamony_identity.subjects
    FOR EACH ROW EXECUTE FUNCTION mamony_identity.bump_session_epoch_on_status();

-- Database contains digests only; bearer secrets never enter SQL or logs.
CREATE TABLE mamony_identity.opaque_sessions (
    token_digest bytea PRIMARY KEY CHECK (octet_length(token_digest) = 32),
    subject_id text NOT NULL REFERENCES mamony_identity.subjects(subject_id),
    subject_epoch bigint NOT NULL CHECK (subject_epoch > 0),
    issued_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz NULL,
    CHECK (expires_at > issued_at),
    CHECK (revoked_at IS NULL OR revoked_at >= issued_at)
);
CREATE INDEX opaque_sessions_subject_expiry
    ON mamony_identity.opaque_sessions(subject_id, expires_at);
CREATE INDEX opaque_sessions_expiry_cleanup
    ON mamony_identity.opaque_sessions(expires_at);
REVOKE ALL ON mamony_identity.opaque_sessions FROM PUBLIC;

ALTER TABLE mamony_identity.security_outbox
    DROP CONSTRAINT security_outbox_event_type_check;
ALTER TABLE mamony_identity.security_outbox
    ADD CONSTRAINT security_outbox_event_type_check
    CHECK (event_type IN ('ENROLLMENT_PENDING', 'EMAIL_PROOF_ISSUED',
        'CONTACT_VERIFIED', 'SESSION_CREATED', 'SESSION_REVOKED'));

-- Only the separately provisioned session role may EXECUTE these carefully
-- scoped definer functions. It has ZERO direct SELECT/INSERT/UPDATE/DELETE
-- table rights; the session API is deliberately security-sensitive.
CREATE FUNCTION mamony_identity.issue_opaque_session(
    p_subject text, p_digest bytea, p_event text)
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, pg_temp
AS $$
DECLARE
    current_epoch bigint;
    stamp timestamptz;
BEGIN
    IF p_subject IS NULL OR p_subject !~ '^sub_[A-Za-z0-9_-]{22}$'
       OR p_digest IS NULL OR octet_length(p_digest) <> 32
       OR p_event IS NULL OR p_event !~ '^evt_[A-Za-z0-9_-]{22}$' THEN
        RETURN false;
    END IF;
    -- Lock both rows before deciding. A concurrent freeze/status change
    -- cannot commit between this recheck and the session INSERT commit.
    SELECT s.session_epoch INTO current_epoch
      FROM mamony_identity.subjects s
      JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
     WHERE s.subject_id=p_subject AND s.status='ACTIVE'
       AND a.verified_at IS NOT NULL AND a.contact_version=1
     FOR UPDATE OF s, a;
    IF NOT FOUND THEN
        RETURN false;
    END IF;
    stamp := clock_timestamp();
    INSERT INTO mamony_identity.opaque_sessions
        (token_digest, subject_id, subject_epoch, issued_at, expires_at)
    VALUES (p_digest, p_subject, current_epoch, stamp, stamp + interval '12 hours');
    INSERT INTO mamony_identity.security_outbox
        (event_id, subject_id, event_type, occurred_at)
    VALUES (p_event, p_subject, 'SESSION_CREATED', stamp);
    RETURN true;
END;
$$;

CREATE FUNCTION mamony_identity.resolve_opaque_session(p_digest bytea)
RETURNS text
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, pg_temp
AS $$
DECLARE
    resolved_subject text;
BEGIN
    IF p_digest IS NULL OR octet_length(p_digest) <> 32 THEN
        RETURN NULL;
    END IF;
    SELECT ss.subject_id INTO resolved_subject
      FROM mamony_identity.opaque_sessions ss
      JOIN mamony_identity.subjects s ON s.subject_id=ss.subject_id
      JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
     WHERE ss.token_digest=p_digest AND ss.revoked_at IS NULL
       AND ss.expires_at > clock_timestamp()
       AND ss.subject_epoch=s.session_epoch
       AND s.status='ACTIVE' AND a.verified_at IS NOT NULL
       AND a.contact_version=1
     FOR SHARE OF ss, s, a;
    RETURN resolved_subject;
END;
$$;

CREATE FUNCTION mamony_identity.revoke_opaque_session(p_digest bytea, p_event text)
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, pg_temp
AS $$
DECLARE
    revoked_subject text;
    stamp timestamptz;
BEGIN
    IF p_digest IS NULL OR octet_length(p_digest) <> 32
       OR p_event IS NULL OR p_event !~ '^evt_[A-Za-z0-9_-]{22}$' THEN
        RETURN false;
    END IF;
    stamp := clock_timestamp();
    UPDATE mamony_identity.opaque_sessions
       SET revoked_at=stamp
     WHERE token_digest=p_digest AND revoked_at IS NULL
    RETURNING subject_id INTO revoked_subject;
    IF FOUND THEN
        INSERT INTO mamony_identity.security_outbox
            (event_id, subject_id, event_type, occurred_at)
        VALUES (p_event, revoked_subject, 'SESSION_REVOKED', stamp);
    END IF;
    -- Unknown and already revoked tokens have the same safe response.
    RETURN true;
END;
$$;

-- Bounded retention: 100 expired session digests per maintenance call.
-- Never delete active sessions or mandatory identifier-only audit evidence.
CREATE FUNCTION mamony_identity.purge_expired_opaque_sessions()
RETURNS integer
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, pg_temp
AS $$
    WITH targets AS MATERIALIZED (
        SELECT token_digest FROM mamony_identity.opaque_sessions
        WHERE expires_at < clock_timestamp() - interval '24 hours'
        ORDER BY expires_at, token_digest
        FOR UPDATE SKIP LOCKED LIMIT 100
    ), erased AS (
        DELETE FROM mamony_identity.opaque_sessions ss
        USING targets t
        WHERE ss.token_digest=t.token_digest
        RETURNING ss.token_digest
    )
    SELECT count(*)::integer FROM erased;
$$;

REVOKE ALL ON FUNCTION mamony_identity.issue_opaque_session(text,bytea,text),
                       mamony_identity.resolve_opaque_session(bytea),
                       mamony_identity.revoke_opaque_session(bytea,text),
                       mamony_identity.purge_expired_opaque_sessions()
    FROM PUBLIC;
GRANT USAGE ON SCHEMA mamony_identity TO mamony_identity_session_runtime;
GRANT EXECUTE ON FUNCTION mamony_identity.issue_opaque_session(text,bytea,text),
                          mamony_identity.resolve_opaque_session(bytea),
                          mamony_identity.revoke_opaque_session(bytea,text)
    TO mamony_identity_session_runtime;
GRANT EXECUTE ON FUNCTION mamony_identity.purge_expired_opaque_sessions()
    TO mamony_identity_maintenance_runtime;
-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (10, 'a15eba43dc2500c1bb4a5de0acba338c3a6991152eb46efcd683fb29a28b311e');
