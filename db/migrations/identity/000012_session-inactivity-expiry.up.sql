-- Identity v12: enforce a DB-clock, server-side 30 minute inactivity cutoff.
-- The fixed 12 hour absolute expiry is NOT extended by session activity.
-- This forward migration backfills last_seen_at=issued_at: any old session
-- unused for >=30min becomes immediately invalid, rather than inheriting a
-- fresh activity timestamp merely because a migration was applied.
ALTER TABLE mamony_identity.opaque_sessions
    ADD COLUMN last_seen_at timestamptz;
UPDATE mamony_identity.opaque_sessions
   SET last_seen_at=issued_at;
ALTER TABLE mamony_identity.opaque_sessions
    ALTER COLUMN last_seen_at SET NOT NULL,
    ALTER COLUMN last_seen_at SET DEFAULT clock_timestamp(),
    ADD CONSTRAINT opaque_sessions_activity_window
        CHECK (last_seen_at >= issued_at AND last_seen_at < expires_at);

-- A single short transaction validates and records activity under the same
-- locks. Re-sample DB clock AFTER acquiring locks: a long lock wait cannot
-- move an expired session back into a valid window.
-- FOR UPDATE serializes same-session requests, so throughput/load calibration
-- remains a release blocker; no SQL clock or bearer travels to the client.
CREATE OR REPLACE FUNCTION mamony_identity.resolve_opaque_session(p_digest bytea)
RETURNS text
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, pg_temp
AS $$
DECLARE
    resolved_subject text;
    seen_at timestamptz;
    limit_at timestamptz;
    stamp timestamptz;
BEGIN -- PL/pgSQL function block; not a migration transaction
    IF p_digest IS NULL OR octet_length(p_digest) <> 32 THEN
        RETURN NULL;
    END IF;
    SELECT ss.subject_id, ss.last_seen_at, ss.expires_at
      INTO resolved_subject, seen_at, limit_at
      FROM mamony_identity.opaque_sessions ss
      JOIN mamony_identity.subjects s ON s.subject_id=ss.subject_id
      JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
     WHERE ss.token_digest=p_digest AND ss.revoked_at IS NULL
       AND ss.subject_epoch=s.session_epoch
       AND s.status='ACTIVE' AND a.verified_at IS NOT NULL
       AND a.contact_version=1
     FOR UPDATE OF ss, s, a;
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
-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (12, 'acedf07e7e5b00a9a76cb543e909504719e0bd736dbf7d86e9b598de75688ba9');
