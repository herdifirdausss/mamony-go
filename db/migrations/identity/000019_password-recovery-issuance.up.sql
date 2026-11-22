-- Identity v19: internal-only password-recovery issuance. A new issuer
-- role can EXECUTE one eligibility-fenced function, never read tables or
-- password hashes. No public HTTP, provider worker or redemption route.
ALTER TABLE mamony_identity.anonymous_admission_buckets
    DROP CONSTRAINT anonymous_admission_buckets_action_check;
ALTER TABLE mamony_identity.anonymous_admission_buckets
    ADD CONSTRAINT anonymous_admission_buckets_action_check
    CHECK (action IN ('REGISTER','RESEND','CONFIRM','LOGIN','RECOVERY'));

CREATE TABLE mamony_identity.password_recovery_challenges (
    challenge_id text PRIMARY KEY CHECK (challenge_id ~ '^rcv_[A-Za-z0-9_-]{22}$'),
    subject_id text NOT NULL REFERENCES mamony_identity.subjects(subject_id),
    canonical_email text NOT NULL REFERENCES mamony_identity.email_aliases(canonical_email),
    contact_version bigint NOT NULL CHECK (contact_version >= 1),
    subject_epoch bigint NOT NULL CHECK (subject_epoch > 0),
    token_digest bytea NOT NULL UNIQUE CHECK (octet_length(token_digest)=32),
    requested_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz NULL,
    revoked_at timestamptz NULL,
    CHECK (expires_at > requested_at),
    CHECK (consumed_at IS NULL OR revoked_at IS NULL)
);
CREATE UNIQUE INDEX password_recovery_one_live_per_subject
    ON mamony_identity.password_recovery_challenges(subject_id)
    WHERE consumed_at IS NULL AND revoked_at IS NULL;
CREATE INDEX password_recovery_issuance_budget
    ON mamony_identity.password_recovery_challenges(subject_id, requested_at DESC);

-- Delivery encryption is mandatory and opaque to the DB. No SQL role may
-- claim/decrypt this queue in this PR; the future worker needs a new grant.
CREATE TABLE mamony_identity.password_recovery_deliveries (
    challenge_id text PRIMARY KEY
        REFERENCES mamony_identity.password_recovery_challenges(challenge_id),
    key_id text NOT NULL CHECK (key_id ~ '^[A-Za-z0-9_-]{1,32}$'),
    sealed_token bytea NOT NULL CHECK (octet_length(sealed_token) BETWEEN 60 AND 256),
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
CREATE INDEX password_recovery_deliveries_due
    ON mamony_identity.password_recovery_deliveries(next_attempt_at, challenge_id)
    WHERE status='PENDING';
REVOKE ALL ON mamony_identity.password_recovery_challenges,
    mamony_identity.password_recovery_deliveries FROM PUBLIC;

-- Use a single SECURITY DEFINER transaction. No caller-provided subject,
-- verifier, event type or recovery-channel destination is accepted.
CREATE FUNCTION mamony_identity.issue_password_recovery(
    p_email text, p_challenge text, p_digest bytea,
    p_key text, p_sealed bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    authorized_subject text;
    current_version bigint;
    current_epoch bigint;
    stamp timestamptz;
    minute_count integer;
    hour_count integer;
    day_count integer;
BEGIN -- PL/pgSQL function block; not a migration transaction
    IF session_user <> 'mamony_identity_recovery_issuer_runtime' THEN
        RAISE EXCEPTION 'password recovery issuer origin denied'
            USING ERRCODE='42501';
    END IF;
    IF p_email IS NULL OR length(p_email)<3 OR length(p_email)>254
       OR p_email<>lower(p_email)
       OR p_challenge IS NULL OR p_challenge !~ '^rcv_[A-Za-z0-9_-]{22}$'
       OR p_digest IS NULL OR octet_length(p_digest)<>32
       OR p_key IS NULL OR p_key !~ '^[A-Za-z0-9_-]{1,32}$'
       OR p_sealed IS NULL OR octet_length(p_sealed) NOT BETWEEN 60 AND 256
    THEN RETURN false; END IF;

    SELECT s.subject_id,a.contact_version,s.session_epoch
      INTO authorized_subject,current_version,current_epoch
      FROM mamony_identity.email_aliases a
      JOIN mamony_identity.subjects s ON s.subject_id=a.subject_id
     WHERE a.canonical_email=p_email
       AND s.status='ACTIVE' AND a.verified_at IS NOT NULL
       AND a.contact_version=1 AND s.session_epoch>0
     FOR UPDATE OF s,a;
    IF NOT FOUND THEN RETURN false; END IF;

    stamp:=clock_timestamp();
    SELECT
       COUNT(*) FILTER (WHERE c.requested_at>stamp-interval '60 seconds'),
       COUNT(*) FILTER (WHERE c.requested_at>stamp-interval '1 hour'),
       COUNT(*)
      INTO minute_count,hour_count,day_count
      FROM mamony_identity.password_recovery_challenges c
     WHERE c.subject_id=authorized_subject
       AND c.requested_at>stamp-interval '24 hours';
    IF minute_count>=1 OR hour_count>=3 OR day_count>=5 THEN
        RETURN false;
    END IF;

    -- One active proof per subject; failed issuance must roll back this
    -- revocation, the fresh digest and the encrypted outbox together.
    UPDATE mamony_identity.password_recovery_challenges
       SET revoked_at=stamp
     WHERE subject_id=authorized_subject
       AND consumed_at IS NULL AND revoked_at IS NULL;
    INSERT INTO mamony_identity.password_recovery_challenges
        (challenge_id,subject_id,canonical_email,contact_version,
         subject_epoch,token_digest,requested_at,expires_at)
    VALUES (p_challenge,authorized_subject,p_email,current_version,
            current_epoch,p_digest,stamp,stamp+interval '15 minutes');
    INSERT INTO mamony_identity.password_recovery_deliveries
        (challenge_id,key_id,sealed_token)
    VALUES (p_challenge,p_key,p_sealed);
    RETURN true;
END;
$$;
REVOKE ALL ON FUNCTION mamony_identity.issue_password_recovery(
    text,text,bytea,text,bytea) FROM PUBLIC;
GRANT USAGE ON SCHEMA mamony_identity
    TO mamony_identity_recovery_issuer_runtime;
GRANT EXECUTE ON FUNCTION mamony_identity.issue_password_recovery(
    text,text,bytea,text,bytea)
    TO mamony_identity_recovery_issuer_runtime;

-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (19, '__REPLACE_SHA256__');
