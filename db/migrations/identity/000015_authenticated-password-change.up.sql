-- Identity v15: first-party password changes require a valid session and a
-- separate 30s one-use, purpose-bound successful-old-password assertion.
-- NO public route, independent MFA or production workload isolation is added.
ALTER TABLE mamony_identity.password_login_grants
    DROP CONSTRAINT password_login_grants_purpose_check;
ALTER TABLE mamony_identity.password_login_grants
    ADD CONSTRAINT password_login_grants_purpose_check
    CHECK (purpose IN ('PASSWORD_LOGIN_SESSION','PASSWORD_CHANGE'));
ALTER TABLE mamony_identity.security_outbox
    DROP CONSTRAINT security_outbox_event_type_check;
ALTER TABLE mamony_identity.security_outbox
    ADD CONSTRAINT security_outbox_event_type_check
    CHECK (event_type IN ('ENROLLMENT_PENDING','EMAIL_PROOF_ISSUED',
        'CONTACT_VERIFIED','SESSION_CREATED','SESSION_REVOKED','PASSWORD_CHANGED'));

CREATE FUNCTION mamony_identity.stage_password_change_grant(
    p_subject text, p_expected_epoch bigint, p_digest bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    current_epoch bigint;
    stamp timestamptz;
BEGIN -- PL/pgSQL function block; not a migration transaction
    IF p_subject IS NULL OR p_subject !~ '^sub_[A-Za-z0-9_-]{22}$'
       OR p_expected_epoch IS NULL OR p_expected_epoch <= 0
       OR p_digest IS NULL OR octet_length(p_digest) <> 32 THEN RETURN false; END IF;
    SELECT s.session_epoch INTO current_epoch
      FROM mamony_identity.subjects s
      JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
     WHERE s.subject_id=p_subject AND s.status='ACTIVE'
       AND a.verified_at IS NOT NULL AND a.contact_version=1
       AND s.session_epoch=p_expected_epoch FOR UPDATE OF s,a;
    IF NOT FOUND THEN RETURN false; END IF;
    stamp:=clock_timestamp();
    INSERT INTO mamony_identity.password_login_grants
        (grant_digest,subject_id,subject_epoch,purpose,issued_at,expires_at)
    VALUES (p_digest,p_subject,current_epoch,'PASSWORD_CHANGE',stamp,
            stamp+interval '30 seconds');
    RETURN true;
END;
$$;

-- Only the restricted password-change runtime can execute. The trusted Go
-- usecase must verify the current password via admission + real Argon2id,
-- enforce the new-password policy and stage the purpose-bound grant.
CREATE FUNCTION mamony_identity.rotate_password_verifier(
    p_session_digest bytea, p_grant_digest bytea,
    p_new_salt bytea, p_new_digest bytea, p_event text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    proof_subject text;
    proof_epoch bigint;
    candidate_subject text;
    current_epoch bigint;
    seen timestamptz;
    absolute_expiry timestamptz;
    stamp timestamptz;
    changed_subject text;
BEGIN -- PL/pgSQL function block; not a migration transaction
    IF p_session_digest IS NULL OR octet_length(p_session_digest) <> 32
       OR p_grant_digest IS NULL OR octet_length(p_grant_digest) <> 32
       OR p_new_salt IS NULL OR octet_length(p_new_salt) <> 16
       OR p_new_digest IS NULL OR octet_length(p_new_digest) <> 32
       OR p_event IS NULL OR p_event !~ '^evt_[A-Za-z0-9_-]{22}$' THEN
        RETURN false;
    END IF;
    SELECT g.subject_id,g.subject_epoch INTO proof_subject,proof_epoch
      FROM mamony_identity.password_login_grants g
     WHERE g.grant_digest=p_grant_digest AND g.purpose='PASSWORD_CHANGE'
       AND g.consumed_at IS NULL FOR UPDATE OF g;
    IF NOT FOUND THEN RETURN false; END IF;
    SELECT ss.subject_id INTO candidate_subject
      FROM mamony_identity.opaque_sessions ss
     WHERE ss.token_digest=p_session_digest;
    IF NOT FOUND OR candidate_subject IS DISTINCT FROM proof_subject THEN
        RETURN false;
    END IF;
    -- Global order: grant -> subject/alias -> session -> password verifier.
    SELECT s.session_epoch INTO current_epoch
      FROM mamony_identity.subjects s
      JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
     WHERE s.subject_id=proof_subject AND s.session_epoch=proof_epoch
       AND s.status='ACTIVE' AND a.verified_at IS NOT NULL
       AND a.contact_version=1 FOR UPDATE OF s,a;
    IF NOT FOUND THEN RETURN false; END IF;
    SELECT ss.last_seen_at,ss.expires_at INTO seen,absolute_expiry
      FROM mamony_identity.opaque_sessions ss
     WHERE ss.token_digest=p_session_digest AND ss.subject_id=proof_subject
       AND ss.subject_epoch=current_epoch AND ss.revoked_at IS NULL
     FOR UPDATE OF ss;
    IF NOT FOUND THEN RETURN false; END IF;
    stamp:=clock_timestamp();
    IF stamp>=absolute_expiry OR stamp>=seen+interval '30 minutes'
       OR stamp<seen THEN RETURN false; END IF;
    PERFORM 1 FROM mamony_identity.password_verifiers p
      WHERE p.subject_id=proof_subject FOR UPDATE OF p;
    IF NOT FOUND THEN RETURN false; END IF;
    UPDATE mamony_identity.password_login_grants SET consumed_at=stamp
     WHERE grant_digest=p_grant_digest AND consumed_at IS NULL
       AND expires_at>stamp;
    IF NOT FOUND THEN RETURN false; END IF;
    UPDATE mamony_identity.password_verifiers
       SET kdf_version=1,memory_kib=65536,iterations=3,parallelism=1,
           salt=p_new_salt,digest=p_new_digest
     WHERE subject_id=proof_subject
     RETURNING subject_id INTO changed_subject;
    IF NOT FOUND OR changed_subject IS DISTINCT FROM proof_subject THEN
        RETURN false;
    END IF;
    -- v14 trigger increments session_epoch in same transaction, invalidating
    -- all existing bearer tokens and earlier grants without token enumeration.
    INSERT INTO mamony_identity.security_outbox
        (event_id,subject_id,event_type,occurred_at)
    VALUES (p_event,proof_subject,'PASSWORD_CHANGED',stamp);
    RETURN true;
END;
$$;
REVOKE ALL ON FUNCTION
    mamony_identity.stage_password_change_grant(text,bigint,bytea),
    mamony_identity.rotate_password_verifier(bytea,bytea,bytea,bytea,text)
    FROM PUBLIC;
GRANT EXECUTE ON FUNCTION mamony_identity.stage_password_change_grant(text,bigint,bytea)
    TO mamony_identity_auth_grant_runtime;
GRANT USAGE ON SCHEMA mamony_identity TO mamony_identity_password_change_runtime;
GRANT EXECUTE ON FUNCTION
    mamony_identity.rotate_password_verifier(bytea,bytea,bytea,bytea,text)
    TO mamony_identity_password_change_runtime;
-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (15, '77a6b03db4d2bde43ba05410e059acfa72b44f183c971af05467b4bfcca22565');
