-- Identity v32: purpose-specific fresh-password proof for an existing
-- (v30) passkey challenge. This is registration ADMISSION ONLY; never
-- independent MFA, recovery-factor promotion or credential persistence.
-- v30 challenge may be overwritten when expired. Admissions are stored in a
-- separate one-per-subject table and MUST match the CURRENT intent ID.
ALTER TABLE mamony_identity.password_login_grants
    DROP CONSTRAINT password_login_grants_purpose_check;
ALTER TABLE mamony_identity.password_login_grants
    ADD CONSTRAINT password_login_grants_purpose_check
    CHECK (purpose IN ('PASSWORD_LOGIN_SESSION','PASSWORD_CHANGE',
        'RECOVERY_CODE_ROTATION','PASSKEY_REGISTRATION_REAUTH'));

CREATE TABLE mamony_identity.passkey_registration_admissions (
    subject_id text PRIMARY KEY REFERENCES mamony_identity.subjects(subject_id),
    intent_id text NOT NULL CHECK (intent_id ~ '^wrg_[A-Za-z0-9_-]{22}$'),
    session_digest bytea NOT NULL CHECK (octet_length(session_digest)=32),
    subject_epoch bigint NOT NULL CHECK (subject_epoch>0),
    contact_version bigint NOT NULL CHECK (contact_version=1),
    provenance text NOT NULL CHECK (provenance='PASSWORD_REAUTH_ONLY'),
    approved_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz NULL,
    CHECK (expires_at>approved_at AND expires_at<=approved_at+interval '3 minutes'),
    CHECK (consumed_at IS NULL OR consumed_at>=approved_at)
);
REVOKE ALL ON mamony_identity.passkey_registration_admissions FROM PUBLIC;

-- This privileged assertion is emitted ONLY by trusted Go AFTER
-- budget-limited Argon2id verification of the current password for the
-- same session subject; SQL does not independently verify Argon2id.
-- Do not give this function to the passkey issue/read/admission principals.
CREATE FUNCTION mamony_identity.stage_passkey_registration_reauth_grant(
    p_subject text,p_epoch bigint,p_digest bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE current_epoch bigint; stamp timestamptz;
BEGIN -- PL/pgSQL function block, not migration transaction
    IF session_user<>'mamony_identity_auth_grant_runtime' THEN
        RAISE EXCEPTION 'passkey reauth grant origin denied' USING ERRCODE='42501';
    END IF;
    IF p_subject IS NULL OR p_subject !~ '^sub_[A-Za-z0-9_-]{22}$'
       OR p_epoch IS NULL OR p_epoch<=0
       OR p_digest IS NULL OR octet_length(p_digest)<>32
    THEN RETURN false; END IF;
    SELECT s.session_epoch INTO current_epoch
      FROM mamony_identity.subjects s
      JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
     WHERE s.subject_id=p_subject AND s.status='ACTIVE'
       AND s.session_epoch=p_epoch AND a.verified_at IS NOT NULL
       AND a.contact_version=1
     FOR UPDATE OF s,a;
    IF NOT FOUND THEN RETURN false; END IF;
    stamp:=clock_timestamp();
    INSERT INTO mamony_identity.password_login_grants
        (grant_digest,subject_id,subject_epoch,purpose,issued_at,expires_at)
    VALUES(p_digest,p_subject,current_epoch,'PASSKEY_REGISTRATION_REAUTH',
           stamp,stamp+interval '30 seconds');
    RETURN true;
END;
$$;

-- Separate EXECUTE-only consumer, unable to stage/reuse grants. Lock order
-- grant -> subject+verified alias -> session -> current v30 intent -> admission.
-- A grant for another purpose, subject, epoch, session or intent DENIES.
-- No admission grants permission to insert WebAuthn credentials. Its fixed
-- provenance forbids future recovery factor bootstrap without distinct proof.
CREATE FUNCTION mamony_identity.admit_passkey_registration_intent(
    p_session_digest bytea,p_intent_id text,p_grant_digest bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    grant_subject text;
    grant_epoch bigint;
    current_epoch bigint;
    current_contact_version bigint;
    candidate_subject text;
    session_epoch bigint;
    last_seen timestamptz;
    absolute_expiry timestamptz;
    issued timestamptz;
    expiry timestamptz;
    stamp timestamptz;
    affected integer;
BEGIN -- PL/pgSQL function block, not migration transaction
    IF session_user<>'mamony_identity_passkey_registration_admission_runtime' THEN
        RAISE EXCEPTION 'passkey admission origin denied' USING ERRCODE='42501';
    END IF;
    IF p_session_digest IS NULL OR octet_length(p_session_digest)<>32
       OR p_grant_digest IS NULL OR octet_length(p_grant_digest)<>32
       OR p_intent_id IS NULL OR p_intent_id !~ '^wrg_[A-Za-z0-9_-]{22}$'
    THEN RETURN false; END IF;
    SELECT g.subject_id,g.subject_epoch INTO grant_subject,grant_epoch
      FROM mamony_identity.password_login_grants g
     WHERE g.grant_digest=p_grant_digest
       AND g.purpose='PASSKEY_REGISTRATION_REAUTH'
       AND g.consumed_at IS NULL
     FOR UPDATE OF g;
    IF NOT FOUND THEN RETURN false; END IF;

    SELECT ss.subject_id INTO candidate_subject
      FROM mamony_identity.opaque_sessions ss
     WHERE ss.token_digest=p_session_digest;
    IF NOT FOUND OR candidate_subject IS DISTINCT FROM grant_subject THEN
        RETURN false;
    END IF;

    SELECT s.session_epoch,a.contact_version
      INTO current_epoch,current_contact_version
      FROM mamony_identity.subjects s
      JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
     WHERE s.subject_id=grant_subject AND s.status='ACTIVE'
       AND s.session_epoch=grant_epoch
       AND a.verified_at IS NOT NULL AND a.contact_version=1
     FOR UPDATE OF s,a;
    IF NOT FOUND THEN RETURN false; END IF;

    SELECT ss.subject_epoch,ss.last_seen_at,ss.expires_at
      INTO session_epoch,last_seen,absolute_expiry
      FROM mamony_identity.opaque_sessions ss
     WHERE ss.token_digest=p_session_digest AND ss.subject_id=grant_subject
       AND ss.subject_epoch=current_epoch AND ss.revoked_at IS NULL
     FOR UPDATE OF ss;
    IF NOT FOUND OR session_epoch<>current_epoch THEN RETURN false; END IF;

    SELECT i.issued_at,i.expires_at INTO issued,expiry
      FROM mamony_identity.passkey_registration_intents i
     WHERE i.subject_id=grant_subject AND i.intent_id=p_intent_id
       AND i.session_digest=p_session_digest
       AND i.subject_epoch=current_epoch
       AND i.contact_version=current_contact_version
       AND i.purpose='PASSKEY_REGISTRATION'
       AND i.policy_generation=1
       AND i.origin='https://' || i.rp_id
       AND i.consumed_at IS NULL
     FOR UPDATE OF i;
    IF NOT FOUND THEN RETURN false; END IF;

    stamp:=clock_timestamp();
    IF stamp>=absolute_expiry OR stamp>=last_seen+interval '30 minutes'
       OR stamp<last_seen OR stamp<issued OR stamp>=expiry
       OR NOT EXISTS(
           SELECT 1 FROM mamony_identity.password_login_grants g
            WHERE g.grant_digest=p_grant_digest
              AND g.purpose='PASSKEY_REGISTRATION_REAUTH'
              AND g.subject_epoch=current_epoch
              AND g.consumed_at IS NULL
              AND g.expires_at>stamp)
    THEN RETURN false; END IF;

    -- One admission per subject. A same-intent live admission is immutable.
    -- A superseded intent may replace the previous admission ONLY by
    -- consuming a separate new purpose-specific Argon2id grant. Never
    -- carry an old approval when v30 upserts an expired challenge.
    INSERT INTO mamony_identity.passkey_registration_admissions (
        subject_id,intent_id,session_digest,subject_epoch,contact_version,
        provenance,approved_at,expires_at)
    VALUES(grant_subject,p_intent_id,p_session_digest,current_epoch,
           current_contact_version,'PASSWORD_REAUTH_ONLY',stamp,expiry)
    ON CONFLICT(subject_id) DO UPDATE SET
        intent_id=EXCLUDED.intent_id,
        session_digest=EXCLUDED.session_digest,
        subject_epoch=EXCLUDED.subject_epoch,
        contact_version=EXCLUDED.contact_version,
        provenance=EXCLUDED.provenance,
        approved_at=EXCLUDED.approved_at,
        expires_at=EXCLUDED.expires_at,
        consumed_at=NULL
    WHERE mamony_identity.passkey_registration_admissions.expires_at<=stamp
       OR mamony_identity.passkey_registration_admissions.consumed_at IS NOT NULL
       OR mamony_identity.passkey_registration_admissions.intent_id<>EXCLUDED.intent_id;
    GET DIAGNOSTICS affected=ROW_COUNT;
    IF affected<>1 THEN RETURN false; END IF;
    UPDATE mamony_identity.password_login_grants SET consumed_at=stamp
     WHERE grant_digest=p_grant_digest
       AND purpose='PASSKEY_REGISTRATION_REAUTH'
       AND consumed_at IS NULL AND expires_at>stamp;
    IF NOT FOUND THEN
       -- Never leave a committed admission when its proof was not consumed.
       RAISE EXCEPTION 'passkey registration grant commit fence'
           USING ERRCODE='40001';
    END IF;
    RETURN true;
END;
$$;

REVOKE ALL ON FUNCTION mamony_identity.stage_passkey_registration_reauth_grant(
    text,bigint,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION mamony_identity.stage_passkey_registration_reauth_grant(
    text,bigint,bytea) TO mamony_identity_auth_grant_runtime;
REVOKE ALL ON FUNCTION mamony_identity.admit_passkey_registration_intent(
    bytea,text,bytea) FROM PUBLIC;
GRANT USAGE ON SCHEMA mamony_identity
    TO mamony_identity_passkey_registration_admission_runtime;
GRANT EXECUTE ON FUNCTION mamony_identity.admit_passkey_registration_intent(
    bytea,text,bytea) TO mamony_identity_passkey_registration_admission_runtime;

-- No credential table or role, no finish/consume, no self-upgrade to
-- independent recovery assurance, no endpoint. Security notice required
-- only when future approved atomic credential binding is implemented.

-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (32, 'a47345ea029ed44cf052502d9ec7531d48f7ea29bdad2bb8b2a8219e79fa0673');
