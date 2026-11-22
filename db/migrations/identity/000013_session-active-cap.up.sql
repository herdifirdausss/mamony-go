-- Identity v13: candidate cap of five simultaneously usable password sessions per subject.
-- All operations share the subject row lock. Never trust cookie counts, client IP,
-- bearer timestamps, or process-local bookkeeping to enforce this cap.
CREATE INDEX opaque_sessions_subject_oldest_live
  ON mamony_identity.opaque_sessions(subject_id, issued_at, token_digest)
  WHERE revoked_at IS NULL;

-- The issuer still consumes ONLY one unexpired one-use v11 grant.
-- Subject status/contact/epoch recheck, oldest-session eviction and creation
-- are committed in the SAME transaction with identifier-only outbox records.
-- The locked subject serializes distinct login grants for the same account.
CREATE OR REPLACE FUNCTION mamony_identity.issue_opaque_session_with_grant(
    p_grant_digest bytea, p_token_digest bytea, p_event text)
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, pg_temp
AS $$
DECLARE
    grant_subject text;
    grant_epoch bigint;
    current_epoch bigint;
    stamp timestamptz;
    live_count integer;
    oldest_digest bytea;
    evicted_subject text;
    eviction_event text;
BEGIN
    IF p_grant_digest IS NULL OR octet_length(p_grant_digest) <> 32
       OR p_token_digest IS NULL OR octet_length(p_token_digest) <> 32
       OR p_event IS NULL OR p_event !~ '^evt_[A-Za-z0-9_-]{22}$' THEN
        RETURN false;
    END IF;
    -- Only the grant row is locked at first. All three session paths use
    -- subject -> session ordering, avoiding issue/resolve/logout inversion.
    SELECT g.subject_id, g.subject_epoch INTO grant_subject, grant_epoch
      FROM mamony_identity.password_login_grants g
     WHERE g.grant_digest=p_grant_digest
       AND g.purpose='PASSWORD_LOGIN_SESSION'
       AND g.consumed_at IS NULL
     FOR UPDATE OF g;
    IF NOT FOUND THEN
        RETURN false;
    END IF;
    SELECT s.session_epoch INTO current_epoch
      FROM mamony_identity.subjects s
      JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
     WHERE s.subject_id=grant_subject AND s.status='ACTIVE'
       AND a.verified_at IS NOT NULL AND a.contact_version=1
     FOR UPDATE OF s, a;
    IF NOT FOUND OR current_epoch <> grant_epoch THEN
        RETURN false;
    END IF;
    -- Sample DB time AFTER locks; a queued login grant cannot be revived.
    stamp := clock_timestamp();
    UPDATE mamony_identity.password_login_grants
       SET consumed_at=stamp
     WHERE grant_digest=p_grant_digest AND consumed_at IS NULL
       AND expires_at > stamp;
    IF NOT FOUND THEN
        RETURN false;
    END IF;

    SELECT count(*)::integer INTO live_count
      FROM mamony_identity.opaque_sessions ss
     WHERE ss.subject_id=grant_subject
       AND ss.subject_epoch=current_epoch
       AND ss.revoked_at IS NULL AND ss.expires_at > stamp
       AND ss.last_seen_at <= stamp
       AND ss.last_seen_at + interval '30 minutes' > stamp;
    -- Defensive fail-closed rule for pre-v13 over-cap legacy sessions.
    -- No extra mint is allowed until old sessions expire/revoke.
    IF live_count > 5 THEN
        RETURN false;
    END IF;
    IF live_count = 5 THEN
        SELECT ss.token_digest INTO oldest_digest
          FROM mamony_identity.opaque_sessions ss
         WHERE ss.subject_id=grant_subject
           AND ss.subject_epoch=current_epoch
           AND ss.revoked_at IS NULL AND ss.expires_at > stamp
           AND ss.last_seen_at <= stamp
           AND ss.last_seen_at + interval '30 minutes' > stamp
         ORDER BY ss.issued_at, ss.token_digest
         LIMIT 1 FOR UPDATE OF ss;
        IF NOT FOUND THEN
            RETURN false;
        END IF;
        UPDATE mamony_identity.opaque_sessions
           SET revoked_at=stamp
         WHERE token_digest=oldest_digest AND revoked_at IS NULL
        RETURNING subject_id INTO evicted_subject;
        IF NOT FOUND OR evicted_subject <> grant_subject THEN
            RETURN false;
        END IF;
        -- Core PostgreSQL gen_random_uuid, not pgcrypto extension.
        -- Event IDs are not secrets; bearer/digest never enters outbox.
        eviction_event := 'evt_' ||
            translate(rtrim(encode(uuid_send(gen_random_uuid()), 'base64'), '='),
                      '+/', '-_');
        INSERT INTO mamony_identity.security_outbox
            (event_id, subject_id, event_type, occurred_at)
        VALUES (eviction_event, grant_subject, 'SESSION_REVOKED', stamp);
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

-- A read begins by locating a candidate subject WITHOUT changing state,
-- then locks the subject before the session row. Fresh checks after acquiring
-- both locks preserve v12 expiry, idle, epoch, revocation and contact policy.
CREATE OR REPLACE FUNCTION mamony_identity.resolve_opaque_session(p_digest bytea)
RETURNS text
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, pg_temp
AS $$
DECLARE
    resolved_subject text;
    current_epoch bigint;
    seen_at timestamptz;
    limit_at timestamptz;
    stamp timestamptz;
BEGIN
    IF p_digest IS NULL OR octet_length(p_digest) <> 32 THEN
        RETURN NULL;
    END IF;
    SELECT ss.subject_id INTO resolved_subject
      FROM mamony_identity.opaque_sessions ss WHERE ss.token_digest=p_digest;
    IF NOT FOUND THEN
        RETURN NULL;
    END IF;
    SELECT s.session_epoch INTO current_epoch
      FROM mamony_identity.subjects s
      JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
     WHERE s.subject_id=resolved_subject AND s.status='ACTIVE'
       AND a.verified_at IS NOT NULL AND a.contact_version=1
     FOR UPDATE OF s, a;
    IF NOT FOUND THEN
        RETURN NULL;
    END IF;
    SELECT ss.last_seen_at, ss.expires_at INTO seen_at, limit_at
      FROM mamony_identity.opaque_sessions ss
     WHERE ss.token_digest=p_digest AND ss.subject_id=resolved_subject
       AND ss.subject_epoch=current_epoch AND ss.revoked_at IS NULL
     FOR UPDATE OF ss;
    IF NOT FOUND THEN
        RETURN NULL;
    END IF;
    stamp := clock_timestamp();
    IF stamp >= limit_at OR stamp >= seen_at + interval '30 minutes'
       OR stamp < seen_at THEN
        RETURN NULL;
    END IF;
    UPDATE mamony_identity.opaque_sessions
       SET last_seen_at=stamp WHERE token_digest=p_digest;
    RETURN resolved_subject;
END;
$$;

-- Logout follows the same subject -> session lock order. It still revokes
-- even if a session has expired, been frozen or previously invalidated.
CREATE OR REPLACE FUNCTION mamony_identity.revoke_opaque_session(
    p_digest bytea, p_event text)
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, pg_temp
AS $$
DECLARE
    revoked_subject text;
    candidate_subject text;
    stamp timestamptz;
BEGIN
    IF p_digest IS NULL OR octet_length(p_digest) <> 32
       OR p_event IS NULL OR p_event !~ '^evt_[A-Za-z0-9_-]{22}$' THEN
        RETURN false;
    END IF;
    SELECT ss.subject_id INTO candidate_subject
      FROM mamony_identity.opaque_sessions ss WHERE ss.token_digest=p_digest;
    IF NOT FOUND THEN
        RETURN true;
    END IF;
    PERFORM 1 FROM mamony_identity.subjects s
      WHERE s.subject_id=candidate_subject FOR UPDATE OF s;
    IF NOT FOUND THEN
        RETURN true;
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
    RETURN true;
END;
$$;
-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (13, '9cc3c08aa31f07ea713acdb981204567210b80a88e8fba268ba5cec0a6184bb0');
