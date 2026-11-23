-- Identity v29: bind pending initial recovery-code approvals to their
-- verified, still-current asymmetric factor at code enrollment COMMIT.
-- Revoking or rebinding a public key between v28 signature approval and
-- v25 code enrollment must fail closed. Historical approvals without a
-- factor key are deliberately invalid; do not repair them with password.
ALTER TABLE mamony_identity.password_recovery_code_enrollment_approvals
    ADD COLUMN factor_public_key bytea NULL
    CHECK (factor_public_key IS NULL OR octet_length(factor_public_key)=32);

-- Preserve original function ABI, privileges and Go issuer/verifier split.
-- This setter can only be called by the existing original-login fenced
-- verifier role. No new public endpoint, SQL privilege or key binder.
CREATE OR REPLACE FUNCTION mamony_identity.complete_recovery_code_factor_challenge(
    p_id text,p_public_key bytea,p_approval_digest bytea,p_event text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE account_subject text; current_epoch bigint; contact_version bigint;
        stamp timestamptz; updated integer;
BEGIN -- PL/pgSQL function block, not a migration transaction
    IF session_user<>'mamony_identity_recovery_factor_verifier_runtime' THEN
        RAISE EXCEPTION 'factor approval verifier origin denied' USING ERRCODE='42501';
    END IF;
    IF p_id IS NULL OR p_id !~ '^fac_[A-Za-z0-9_-]{22}$'
       OR p_public_key IS NULL OR octet_length(p_public_key)<>32
       OR p_approval_digest IS NULL OR octet_length(p_approval_digest)<>32
       OR p_event IS NULL OR p_event !~ '^evt_[A-Za-z0-9_-]{22}$'
    THEN RETURN false; END IF;
    SELECT c.subject_id INTO account_subject
      FROM mamony_identity.recovery_code_factor_challenges c
     WHERE c.challenge_id=p_id;
    IF NOT FOUND THEN RETURN false; END IF;
    SELECT s.session_epoch,a.contact_version INTO current_epoch,contact_version
      FROM mamony_identity.subjects s
      JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
     WHERE s.subject_id=account_subject AND s.status='ACTIVE'
       AND a.verified_at IS NOT NULL AND a.contact_version=1
     FOR UPDATE OF s,a;
    IF NOT FOUND THEN RETURN false; END IF;
    PERFORM 1 FROM mamony_identity.recovery_code_factor_keys k
     WHERE k.subject_id=account_subject AND k.public_key=p_public_key
       AND k.bound_epoch=current_epoch
       AND k.bound_contact_version=contact_version
       AND k.revoked_at IS NULL FOR UPDATE OF k;
    IF NOT FOUND THEN RETURN false; END IF;
    stamp:=clock_timestamp();
    -- Denial must not burn a valid challenge or replace an active approval.
    IF EXISTS(SELECT 1 FROM mamony_identity.password_recovery_offline_codes c
        WHERE c.subject_id=account_subject) THEN RETURN false; END IF;
    IF EXISTS(SELECT 1 FROM mamony_identity.password_recovery_code_enrollment_approvals a
        WHERE a.subject_id=account_subject AND a.consumed_at IS NULL
          AND a.expires_at>stamp) THEN RETURN false; END IF;
    UPDATE mamony_identity.recovery_code_factor_challenges c
       SET consumed_at=stamp
     WHERE c.challenge_id=p_id AND c.subject_id=account_subject
       AND c.subject_epoch=current_epoch AND c.expires_at>stamp
       AND c.consumed_at IS NULL AND c.failed_attempts<5;
    GET DIAGNOSTICS updated=ROW_COUNT;
    IF updated<>1 THEN RETURN false; END IF;
    INSERT INTO mamony_identity.password_recovery_code_enrollment_approvals
        (subject_id,subject_epoch,proof_digest,approved_at,expires_at,factor_public_key)
    VALUES(account_subject,current_epoch,p_approval_digest,stamp,stamp+interval '5 minutes',p_public_key)
    ON CONFLICT(subject_id) DO UPDATE SET
        subject_epoch=EXCLUDED.subject_epoch,
        proof_digest=EXCLUDED.proof_digest,
        approved_at=EXCLUDED.approved_at,
        expires_at=EXCLUDED.expires_at,
        consumed_at=NULL,
        factor_public_key=EXCLUDED.factor_public_key
    WHERE mamony_identity.password_recovery_code_enrollment_approvals.consumed_at IS NOT NULL
       OR mamony_identity.password_recovery_code_enrollment_approvals.expires_at<=stamp;
    GET DIAGNOSTICS updated=ROW_COUNT;
    IF updated<>1 THEN
        RAISE EXCEPTION 'factor approval conflict' USING ERRCODE='23505';
    END IF;
    INSERT INTO mamony_identity.security_outbox
        (event_id,subject_id,event_type,occurred_at)
    VALUES(p_event,account_subject,'RECOVERY_FACTOR_APPROVED',stamp);
    RETURN true;
END;
$$;

-- Preserve the manager ABI, one-use grants, full lock order, rotation
-- semantics, and atomic recovery-code notice + security-outbox commit.
-- The initial-enrollment branch now revalidates current prebound key
-- under a row lock BEFORE consuming its approval and issuing any codes.
CREATE OR REPLACE FUNCTION mamony_identity.replace_password_recovery_offline_codes(
    p_session_digest bytea,p_grant_digest bytea,p_independent_digest bytea,
    p_new_code_digests bytea[],p_event text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    authorized_subject text;
    expected_epoch bigint;
    session_subject text;
    current_epoch bigint;
    locked_contact_version bigint;
    last_seen timestamptz;
    absolute_expiry timestamptz;
    previously_enrolled boolean;
    stamp timestamptz;
    code bytea;
    event_type text;
    current_factor_key bytea;
BEGIN -- PL/pgSQL function block, not a migration transaction
    IF session_user<>'mamony_identity_recovery_code_manager_runtime' THEN
        RAISE EXCEPTION 'recovery code manager origin denied'
            USING ERRCODE='42501';
    END IF;
    IF p_session_digest IS NULL OR octet_length(p_session_digest)<>32
       OR p_grant_digest IS NULL OR octet_length(p_grant_digest)<>32
       OR p_independent_digest IS NULL OR octet_length(p_independent_digest)<>32
       OR p_new_code_digests IS NULL
       OR cardinality(p_new_code_digests)<>5
       OR p_event IS NULL OR p_event !~ '^evt_[A-Za-z0-9_-]{22}$'
    THEN RETURN false; END IF;
    FOREACH code IN ARRAY p_new_code_digests LOOP
        IF code IS NULL OR octet_length(code)<>32
           OR code=p_independent_digest THEN RETURN false; END IF;
    END LOOP;
    IF (SELECT count(DISTINCT code_digest) FROM
           unnest(p_new_code_digests) AS codes(code_digest))<>5 THEN
        RETURN false;
    END IF;

    -- Match v15 password-change locking: grant -> subject+alias ->
    -- session. Authentication proof is staged ONLY by the trusted Go
    -- verifier after durable pre-Argon2id admission and current-password
    -- reauthentication on this same account.
    SELECT g.subject_id,g.subject_epoch
      INTO authorized_subject,expected_epoch
      FROM mamony_identity.password_login_grants g
     WHERE g.grant_digest=p_grant_digest
       AND g.purpose='RECOVERY_CODE_ROTATION'
       AND g.consumed_at IS NULL FOR UPDATE OF g;
    IF NOT FOUND THEN RETURN false; END IF;
    SELECT ss.subject_id INTO session_subject
      FROM mamony_identity.opaque_sessions ss
     WHERE ss.token_digest=p_session_digest;
    IF NOT FOUND OR session_subject IS DISTINCT FROM authorized_subject THEN
        RETURN false;
    END IF;

    SELECT s.session_epoch,a.contact_version
      INTO current_epoch,locked_contact_version
      FROM mamony_identity.subjects s
      JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
     WHERE s.subject_id=authorized_subject AND s.status='ACTIVE'
       AND s.session_epoch=expected_epoch
       AND a.verified_at IS NOT NULL AND a.contact_version=1
     FOR UPDATE OF s,a;
    IF NOT FOUND THEN RETURN false; END IF;

    SELECT ss.last_seen_at,ss.expires_at INTO last_seen,absolute_expiry
      FROM mamony_identity.opaque_sessions ss
     WHERE ss.token_digest=p_session_digest AND ss.subject_id=authorized_subject
       AND ss.subject_epoch=current_epoch AND ss.revoked_at IS NULL
     FOR UPDATE OF ss;
    IF NOT FOUND THEN RETURN false; END IF;
    stamp:=clock_timestamp();
    IF stamp>=absolute_expiry OR stamp>=last_seen+interval '30 minutes'
       OR stamp<last_seen THEN RETURN false; END IF;

    PERFORM 1 FROM mamony_identity.password_login_grants g
     WHERE g.grant_digest=p_grant_digest AND g.subject_epoch=current_epoch
       AND g.consumed_at IS NULL AND g.expires_at>stamp FOR UPDATE OF g;
    IF NOT FOUND THEN RETURN false; END IF;

    -- An account that had any code (even all already consumed) is NOT
    -- eligible for bootstrap. Code rotations must prove possession of
    -- an existing valid code; a plain password+session cannot take over.
    SELECT EXISTS (
        SELECT 1 FROM mamony_identity.password_recovery_offline_codes c
         WHERE c.subject_id=authorized_subject
    ) INTO previously_enrolled;
    IF previously_enrolled THEN
        PERFORM 1 FROM mamony_identity.password_recovery_offline_codes c
         WHERE c.code_digest=p_independent_digest
           AND c.subject_id=authorized_subject
           AND c.enrolled_epoch=current_epoch
           AND c.contact_version=locked_contact_version
           AND c.consumed_at IS NULL
         FOR UPDATE OF c;
        IF NOT FOUND THEN RETURN false; END IF;
        event_type:='RECOVERY_CODES_ROTATED';
    ELSE
        -- A previously approved but now revoked/rebound factor cannot bootstrap
        -- recovery codes. Lock key BEFORE approval in the shared subject order.
        SELECT k.public_key INTO current_factor_key
          FROM mamony_identity.recovery_code_factor_keys k
         WHERE k.subject_id=authorized_subject
           AND k.bound_epoch=current_epoch
           AND k.bound_contact_version=locked_contact_version
           AND k.revoked_at IS NULL
         FOR UPDATE OF k;
        IF NOT FOUND THEN RETURN false; END IF;
        PERFORM 1 FROM mamony_identity.password_recovery_code_enrollment_approvals a
         WHERE a.subject_id=authorized_subject AND a.subject_epoch=current_epoch
           AND a.proof_digest=p_independent_digest
           AND a.factor_public_key=current_factor_key
           AND a.consumed_at IS NULL AND a.expires_at>stamp
         FOR UPDATE OF a;
        IF NOT FOUND THEN RETURN false; END IF;
        event_type:='RECOVERY_CODES_ENROLLED';
    END IF;

    UPDATE mamony_identity.password_login_grants SET consumed_at=stamp
     WHERE grant_digest=p_grant_digest AND consumed_at IS NULL
       AND purpose='RECOVERY_CODE_ROTATION' AND expires_at>stamp;
    IF NOT FOUND THEN RETURN false; END IF;
    IF NOT previously_enrolled THEN
        UPDATE mamony_identity.password_recovery_code_enrollment_approvals
           SET consumed_at=stamp
         WHERE subject_id=authorized_subject AND proof_digest=p_independent_digest
           AND consumed_at IS NULL AND expires_at>stamp;
        IF NOT FOUND THEN RETURN false; END IF;
    END IF;

    DELETE FROM mamony_identity.password_recovery_offline_codes
     WHERE subject_id=authorized_subject;
    INSERT INTO mamony_identity.password_recovery_offline_codes
        (code_digest,subject_id,enrolled_epoch,contact_version,enrolled_at)
    SELECT codes.code_digest,authorized_subject,current_epoch,
           locked_contact_version,stamp
      FROM unnest(p_new_code_digests) AS codes(code_digest);

    -- A code-set change revokes all outstanding email recovery attempts
    -- and approval grants. Otherwise a previously approved compromised
    -- email challenge could still reset credentials after code rotation.
    UPDATE mamony_identity.password_recovery_challenges
       SET revoked_at=stamp
     WHERE subject_id=authorized_subject AND revoked_at IS NULL
       AND consumed_at IS NULL;
    UPDATE mamony_identity.password_recovery_deliveries d
       SET status='DEAD',lease_id=NULL,leased_until=NULL,updated_at=stamp
     WHERE d.status='PENDING' AND EXISTS (
        SELECT 1 FROM mamony_identity.password_recovery_challenges c
         WHERE c.challenge_id=d.challenge_id
           AND c.subject_id=authorized_subject
     );
    UPDATE mamony_identity.password_recovery_assurance_grants
       SET consumed_at=stamp
     WHERE subject_id=authorized_subject AND consumed_at IS NULL;

    -- A failed notice insertion or audit ID collision MUST roll back
    -- ALL code changes, consumed proof, challenge revocation and approval.
    INSERT INTO mamony_identity.security_outbox
        (event_id,subject_id,event_type,occurred_at)
    VALUES (p_event,authorized_subject,event_type,stamp);
    RETURN true;
END;
$$;

-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (29, 'b0e841dca636291ea2d71c5328c248177eb9f844553eefb07aa25c40ffb6ad91');
