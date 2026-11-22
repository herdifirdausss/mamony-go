-- Identity v25: unmounted, strongly reauthenticated offline recovery-code
-- enrollment/rotation. Bootstrap additionally requires a distinct independent
-- approval. No production role can issue that approval: initial enrollment is
-- DEFAULT DENY until an independent authenticator is designed and approved.
-- Rotation requires ONE previously enrolled, unconsumed offline code.
-- There is NO public endpoint or production email/notice worker.

ALTER TABLE mamony_identity.password_login_grants
    DROP CONSTRAINT password_login_grants_purpose_check;
ALTER TABLE mamony_identity.password_login_grants
    ADD CONSTRAINT password_login_grants_purpose_check
    CHECK (purpose IN ('PASSWORD_LOGIN_SESSION','PASSWORD_CHANGE',
        'RECOVERY_CODE_ROTATION'));

-- The existing authenticated password verifier stages this purpose only
-- after independent subject-equal Argon2id reauthentication. A grant cannot
-- mint sessions or change a password because consumers require exact purpose.
CREATE FUNCTION mamony_identity.stage_recovery_code_rotation_grant(
    p_subject text,p_expected_epoch bigint,p_digest bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE current_epoch bigint; stamp timestamptz;
BEGIN -- PL/pgSQL function block, not a migration transaction
    IF session_user<>'mamony_identity_auth_grant_runtime' THEN
        RAISE EXCEPTION 'recovery code grant origin denied' USING ERRCODE='42501';
    END IF;
    IF p_subject IS NULL OR p_subject !~ '^sub_[A-Za-z0-9_-]{22}$'
       OR p_expected_epoch IS NULL OR p_expected_epoch<=0
       OR p_digest IS NULL OR octet_length(p_digest)<>32 THEN RETURN false; END IF;
    SELECT s.session_epoch INTO current_epoch
      FROM mamony_identity.subjects s
      JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
     WHERE s.subject_id=p_subject AND s.status='ACTIVE'
       AND s.session_epoch=p_expected_epoch
       AND a.verified_at IS NOT NULL AND a.contact_version=1
     FOR UPDATE OF s,a;
    IF NOT FOUND THEN RETURN false; END IF;
    stamp:=clock_timestamp();
    INSERT INTO mamony_identity.password_login_grants
        (grant_digest,subject_id,subject_epoch,purpose,issued_at,expires_at)
    VALUES (p_digest,p_subject,current_epoch,'RECOVERY_CODE_ROTATION',
            stamp,stamp+interval '30 seconds');
    RETURN true;
END;
$$;
REVOKE ALL ON FUNCTION
    mamony_identity.stage_recovery_code_rotation_grant(text,bigint,bytea)
FROM PUBLIC;
GRANT EXECUTE ON FUNCTION
    mamony_identity.stage_recovery_code_rotation_grant(text,bigint,bytea)
TO mamony_identity_auth_grant_runtime;

-- Initial setup additionally requires an independent, single-use
-- approval from a trusted future step-up authority. This PR deliberately
-- provides NO issuer, writer role, INSERT grant or HTTP enrollment.
CREATE TABLE mamony_identity.password_recovery_code_enrollment_approvals (
    subject_id text PRIMARY KEY REFERENCES mamony_identity.subjects(subject_id),
    subject_epoch bigint NOT NULL CHECK (subject_epoch>0),
    proof_digest bytea NOT NULL UNIQUE CHECK (octet_length(proof_digest)=32),
    approved_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz NULL,
    CHECK (expires_at>approved_at
       AND expires_at<=approved_at+interval '5 minutes'),
    CHECK (consumed_at IS NULL OR consumed_at>=approved_at)
);
REVOKE ALL ON mamony_identity.password_recovery_code_enrollment_approvals
FROM PUBLIC;

-- Token-free notification obligation, purpose separate from PASSWORD_CHANGED
-- or PASSWORD_RECOVERED. No runtime can claim/send this queue yet.
CREATE TABLE mamony_identity.password_recovery_code_notices (
    event_id text PRIMARY KEY REFERENCES mamony_identity.security_outbox(event_id),
    subject_id text NOT NULL REFERENCES mamony_identity.subjects(subject_id),
    recipient text NOT NULL CHECK (length(recipient) BETWEEN 3 AND 254
        AND recipient=lower(recipient)),
    occurred_at timestamptz NOT NULL,
    status text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','DEAD')),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX password_recovery_code_notices_due
    ON mamony_identity.password_recovery_code_notices(occurred_at,event_id)
    WHERE status='PENDING';
REVOKE ALL ON mamony_identity.password_recovery_code_notices FROM PUBLIC;

ALTER TABLE mamony_identity.security_outbox
    DROP CONSTRAINT security_outbox_event_type_check;
ALTER TABLE mamony_identity.security_outbox
    ADD CONSTRAINT security_outbox_event_type_check
    CHECK (event_type IN ('ENROLLMENT_PENDING','EMAIL_PROOF_ISSUED',
        'CONTACT_VERIFIED','SESSION_CREATED','SESSION_REVOKED',
        'PASSWORD_CHANGED','PASSWORD_RECOVERED',
        'RECOVERY_ASSURANCE_APPROVED','RECOVERY_CODES_ENROLLED',
        'RECOVERY_CODES_ROTATED'));

CREATE FUNCTION mamony_identity.enqueue_recovery_code_change_notice()
RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE current_contact text;
BEGIN -- PL/pgSQL function block, not a migration transaction
    IF NEW.event_type NOT IN
        ('RECOVERY_CODES_ENROLLED','RECOVERY_CODES_ROTATED') THEN
        RETURN NEW;
    END IF;
    IF session_user<>'mamony_identity_recovery_code_manager_runtime' THEN
        RAISE EXCEPTION 'recovery code notice origin denied' USING ERRCODE='42501';
    END IF;
    SELECT a.canonical_email INTO current_contact
      FROM mamony_identity.email_aliases a
      JOIN mamony_identity.subjects s ON s.subject_id=a.subject_id
     WHERE s.subject_id=NEW.subject_id AND s.status='ACTIVE'
       AND a.verified_at IS NOT NULL AND a.contact_version=1;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'recovery code change missing verified contact'
            USING ERRCODE='23514';
    END IF;
    INSERT INTO mamony_identity.password_recovery_code_notices
        (event_id,subject_id,recipient,occurred_at)
    VALUES (NEW.event_id,NEW.subject_id,current_contact,NEW.occurred_at);
    RETURN NEW;
END;
$$;
REVOKE ALL ON FUNCTION
    mamony_identity.enqueue_recovery_code_change_notice() FROM PUBLIC;
CREATE TRIGGER security_outbox_recovery_code_change_notice
AFTER INSERT ON mamony_identity.security_outbox FOR EACH ROW
EXECUTE FUNCTION mamony_identity.enqueue_recovery_code_change_notice();

-- Exactly five freshly generated 256-bit digest-only codes per set. SQL
-- never receives raw code material. Both paths require a valid user session
-- and a 30-second, subject/epoch-bound RECOVERY_CODE_ROTATION grant.
-- Bootstrap needs a separate pre-approved independent proof digest; rotate
-- requires a previously saved unconsumed offline recovery-code digest.
CREATE FUNCTION mamony_identity.replace_password_recovery_offline_codes(
    p_session_digest bytea,p_grant_digest bytea,p_independent_digest bytea,
    p_new_code_digests bytea[],p_event text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    authorized_subject text;
    expected_epoch bigint;
    session_subject text;
    current_epoch bigint;
    contact_version bigint;
    last_seen timestamptz;
    absolute_expiry timestamptz;
    previously_enrolled boolean;
    stamp timestamptz;
    code bytea;
    event_type text;
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
      INTO current_epoch,contact_version
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
           AND c.contact_version=contact_version
           AND c.consumed_at IS NULL
         FOR UPDATE OF c;
        IF NOT FOUND THEN RETURN false; END IF;
        event_type:='RECOVERY_CODES_ROTATED';
    ELSE
        PERFORM 1 FROM mamony_identity.password_recovery_code_enrollment_approvals a
         WHERE a.subject_id=authorized_subject AND a.subject_epoch=current_epoch
           AND a.proof_digest=p_independent_digest
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
           contact_version,stamp
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
REVOKE ALL ON FUNCTION
    mamony_identity.replace_password_recovery_offline_codes(
        bytea,bytea,bytea,bytea[],text)
FROM PUBLIC;
GRANT USAGE ON SCHEMA mamony_identity
    TO mamony_identity_recovery_code_manager_runtime;
GRANT EXECUTE ON FUNCTION
    mamony_identity.replace_password_recovery_offline_codes(
        bytea,bytea,bytea,bytea[],text)
TO mamony_identity_recovery_code_manager_runtime;

-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (25, 'a883d1b84d9a4b72bbe255dadd93f0baf93f7550c8e3d138269d12738cf06039');
