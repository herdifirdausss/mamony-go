-- Identity v11: revoke subject-only mint; require one-time login proof grant.
-- A grant is a trusted Go authenticator assertion, NOT proof that SQL verified
-- Argon2id. Credential isolation limits a leaked session DB role, not full RCE.
CREATE TABLE mamony_identity.password_login_grants (
    grant_digest bytea PRIMARY KEY CHECK (octet_length(grant_digest) = 32),
    subject_id text NOT NULL REFERENCES mamony_identity.subjects(subject_id),
    subject_epoch bigint NOT NULL CHECK (subject_epoch > 0),
    purpose text NOT NULL DEFAULT 'PASSWORD_LOGIN_SESSION'
        CHECK (purpose = 'PASSWORD_LOGIN_SESSION'),
    issued_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz NULL,
    CHECK (expires_at > issued_at),
    CHECK (consumed_at IS NULL OR consumed_at >= issued_at)
);
CREATE INDEX password_login_grants_cleanup
    ON mamony_identity.password_login_grants(expires_at);
REVOKE ALL ON mamony_identity.password_login_grants FROM PUBLIC;

-- This function is itself a privileged assertion oracle. Its separate role
-- must be accessible ONLY after successful trusted Go password verification.
CREATE FUNCTION mamony_identity.stage_password_auth_grant(
    p_subject text, p_grant_digest bytea)
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, pg_temp
AS $$
DECLARE
    current_epoch bigint;
    stamp timestamptz;
BEGIN -- PL/pgSQL block, not a migration transaction
    IF p_subject IS NULL OR p_subject !~ '^sub_[A-Za-z0-9_-]{22}$'
       OR p_grant_digest IS NULL OR octet_length(p_grant_digest) <> 32 THEN
        RETURN false;
    END IF;
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
    INSERT INTO mamony_identity.password_login_grants
        (grant_digest, subject_id, subject_epoch, issued_at, expires_at)
    VALUES (p_grant_digest, p_subject, current_epoch, stamp,
        stamp + interval '30 seconds');
    RETURN true;
END;
$$;

-- Subject is derived EXCLUSIVELY from the grant; caller cannot choose it.
-- Consume + session digest + identifier-only event are one atomic transaction.
CREATE FUNCTION mamony_identity.issue_opaque_session_with_grant(
    p_grant_digest bytea, p_token_digest bytea, p_event text)
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, pg_temp
AS $$
DECLARE
    grant_subject text;
    current_epoch bigint;
    stamp timestamptz;
BEGIN -- PL/pgSQL block, not a migration transaction
    IF p_grant_digest IS NULL OR octet_length(p_grant_digest) <> 32
       OR p_token_digest IS NULL OR octet_length(p_token_digest) <> 32
       OR p_event IS NULL OR p_event !~ '^evt_[A-Za-z0-9_-]{22}$' THEN
        RETURN false;
    END IF;
    SELECT g.subject_id, s.session_epoch INTO grant_subject, current_epoch
      FROM mamony_identity.password_login_grants g
      JOIN mamony_identity.subjects s ON s.subject_id=g.subject_id
      JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
     WHERE g.grant_digest=p_grant_digest
       AND g.purpose='PASSWORD_LOGIN_SESSION'
       AND g.consumed_at IS NULL AND g.expires_at > clock_timestamp()
       AND g.subject_epoch=s.session_epoch
       AND s.status='ACTIVE'
       AND a.verified_at IS NOT NULL AND a.contact_version=1
     FOR UPDATE OF g, s, a;
    IF NOT FOUND THEN
        RETURN false;
    END IF;
    stamp := clock_timestamp();
    -- Recheck expiry after potentially waiting for row locks.
    UPDATE mamony_identity.password_login_grants
       SET consumed_at=stamp
     WHERE grant_digest=p_grant_digest AND consumed_at IS NULL
       AND expires_at > stamp;
    IF NOT FOUND THEN
        RETURN false;
    END IF;
    INSERT INTO mamony_identity.opaque_sessions
        (token_digest, subject_id, subject_epoch, issued_at, expires_at)
    VALUES (p_token_digest, grant_subject, current_epoch, stamp,
        stamp + interval '12 hours');
    INSERT INTO mamony_identity.security_outbox
        (event_id, subject_id, event_type, occurred_at)
    VALUES (p_event, grant_subject, 'SESSION_CREATED', stamp);
    RETURN true;
END;
$$;

-- Bounded retention of consumed/expired grants; never delete unexpired grants.
CREATE FUNCTION mamony_identity.purge_expired_password_login_grants()
RETURNS integer
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, pg_temp
AS $$
    WITH targets AS MATERIALIZED (
        SELECT grant_digest FROM mamony_identity.password_login_grants
        WHERE expires_at < clock_timestamp() - interval '24 hours'
        ORDER BY expires_at, grant_digest
        FOR UPDATE SKIP LOCKED LIMIT 100
    ), erased AS (
        DELETE FROM mamony_identity.password_login_grants g
        USING targets t WHERE g.grant_digest=t.grant_digest
        RETURNING g.grant_digest
    )
    SELECT count(*)::integer FROM erased;
$$;

-- This atomic forward migration removes the old arbitrary-subject mint API.
DROP FUNCTION mamony_identity.issue_opaque_session(text,bytea,text);
REVOKE ALL ON FUNCTION mamony_identity.stage_password_auth_grant(text,bytea),
                       mamony_identity.issue_opaque_session_with_grant(bytea,bytea,text),
                       mamony_identity.purge_expired_password_login_grants()
    FROM PUBLIC;
GRANT USAGE ON SCHEMA mamony_identity TO mamony_identity_auth_grant_runtime;
GRANT EXECUTE ON FUNCTION mamony_identity.stage_password_auth_grant(text,bytea)
    TO mamony_identity_auth_grant_runtime;
GRANT EXECUTE ON FUNCTION mamony_identity.issue_opaque_session_with_grant(bytea,bytea,text)
    TO mamony_identity_session_runtime;
GRANT EXECUTE ON FUNCTION mamony_identity.purge_expired_password_login_grants()
    TO mamony_identity_maintenance_runtime;
-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (11, '5ac7ab25bed5d6c10ea7a881b16cb13ece50d076802e5478100f8f76d5aa8157');
