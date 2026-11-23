-- Identity v6: bounded, separately authorized retention maintenance.
-- Erase the encrypted bearer after expiry + 24 hours, preserving delivery
-- metadata and security events. This is logical erasure, NOT backup/WAL purge.
ALTER TABLE mamony_identity.email_proof_deliveries
    ADD COLUMN purged_at timestamptz NULL;
ALTER TABLE mamony_identity.email_proof_deliveries
    ALTER COLUMN sealed_token DROP NOT NULL;
ALTER TABLE mamony_identity.email_proof_deliveries
    ADD CONSTRAINT delivery_envelope_erasure_check
    CHECK ((sealed_token IS NULL) = (purged_at IS NOT NULL));

-- Fixed retention rules, no caller-provided identifier or interval.
-- Each invocation locks only 100 due rows, avoiding long table sweeps.
CREATE FUNCTION mamony_identity.purge_expired_email_envelopes()
RETURNS integer
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, pg_temp
AS $$
    WITH eligible AS MATERIALIZED (
        SELECT d.challenge_id
        FROM mamony_identity.email_proof_deliveries d
        JOIN mamony_identity.email_verification_challenges c
          ON c.challenge_id = d.challenge_id
        WHERE d.sealed_token IS NOT NULL
          AND c.expires_at < clock_timestamp() - interval '24 hours'
          AND (d.leased_until IS NULL OR d.leased_until < clock_timestamp())
        ORDER BY c.expires_at, d.challenge_id
        FOR UPDATE OF d SKIP LOCKED
        LIMIT 100
    ), purged AS (
        UPDATE mamony_identity.email_proof_deliveries d
           SET sealed_token = NULL, purged_at = clock_timestamp()
          FROM eligible
         WHERE d.challenge_id = eligible.challenge_id
        RETURNING d.challenge_id
    )
    SELECT count(*)::integer FROM purged;
$$;

CREATE FUNCTION mamony_identity.purge_expired_anonymous_buckets()
RETURNS integer
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, pg_temp
AS $$
    WITH eligible AS MATERIALIZED (
        SELECT b.action, b.scope, b.key_digest, b.window_start
          FROM mamony_identity.anonymous_admission_buckets b
         WHERE b.window_start < clock_timestamp() - interval '48 hours'
         ORDER BY b.window_start, b.action, b.scope
         FOR UPDATE OF b SKIP LOCKED
         LIMIT 100
    ), purged AS (
        DELETE FROM mamony_identity.anonymous_admission_buckets b
        USING eligible
        WHERE b.action = eligible.action
          AND b.scope = eligible.scope
          AND b.key_digest = eligible.key_digest
          AND b.window_start = eligible.window_start
        RETURNING b.window_start
    )
    SELECT count(*)::integer FROM purged;
$$;

-- PostgreSQL grants EXECUTE to PUBLIC by default; revoke explicitly.
REVOKE ALL ON FUNCTION mamony_identity.purge_expired_email_envelopes() FROM PUBLIC;
REVOKE ALL ON FUNCTION mamony_identity.purge_expired_anonymous_buckets() FROM PUBLIC;
GRANT USAGE ON SCHEMA mamony_identity TO mamony_identity_maintenance_runtime;
GRANT EXECUTE ON FUNCTION mamony_identity.purge_expired_email_envelopes(),
                          mamony_identity.purge_expired_anonymous_buckets()
    TO mamony_identity_maintenance_runtime;

-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (6, '899335a82ccc0e0626d11ed7652abbb41f54915cc00c42cee904ca802de278cf');
