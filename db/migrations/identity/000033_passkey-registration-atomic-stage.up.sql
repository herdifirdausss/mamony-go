-- Identity v33: stage a verified WebAuthn registration candidate atomically.
-- There is NO usable passkey, assertion login, independent recovery factor,
-- public route, or security-mail sender. Only PENDING_ASSERTION is persisted.
-- The dedicated staging SQL role is a privileged assertion oracle: trusted
-- Go MUST first validate the complete response via go-webauthn against v31.
CREATE TABLE mamony_identity.passkey_credentials (
    credential_id bytea PRIMARY KEY CHECK (octet_length(credential_id) BETWEEN 16 AND 1023),
    subject_id text NOT NULL REFERENCES mamony_identity.subjects(subject_id),
    intent_id text NOT NULL UNIQUE CHECK (intent_id ~ '^wrg_[A-Za-z0-9_-]{22}$'),
    public_key_cose bytea NOT NULL
        CHECK (octet_length(public_key_cose) BETWEEN 16 AND 2048),
    aaguid bytea NOT NULL CHECK (octet_length(aaguid)=16),
    rp_id text NOT NULL CHECK (length(rp_id) BETWEEN 4 AND 253),
    origin text NOT NULL CHECK (origin='https://' || rp_id),
    subject_epoch bigint NOT NULL CHECK (subject_epoch>0),
    contact_version bigint NOT NULL CHECK (contact_version=1),
    attestation_format text NOT NULL CHECK (length(attestation_format) BETWEEN 1 AND 64),
    attestation_type text NOT NULL CHECK (length(attestation_type) BETWEEN 1 AND 64),
    user_present boolean NOT NULL CHECK (user_present),
    user_verified boolean NOT NULL CHECK (user_verified),
    backup_eligible boolean NOT NULL,
    backup_state boolean NOT NULL,
    sign_count bigint NOT NULL CHECK (sign_count BETWEEN 0 AND 4294967295),
    provenance text NOT NULL CHECK (provenance='PASSWORD_REAUTH_ONLY'),
    state text NOT NULL CHECK (state='PENDING_ASSERTION'),
    event_id text NOT NULL UNIQUE REFERENCES mamony_identity.security_outbox(event_id),
    created_at timestamptz NOT NULL,
    CHECK (NOT backup_state OR backup_eligible)
);
CREATE INDEX passkey_credentials_subject_pending
    ON mamony_identity.passkey_credentials(subject_id,created_at);
REVOKE ALL ON mamony_identity.passkey_credentials FROM PUBLIC;

-- Notification is ENQUEUED in the same transaction as consumed proofs and
-- staged credential; no mail worker exists. This is NOT a delivery claim.
CREATE TABLE mamony_identity.passkey_registration_notices (
    event_id text PRIMARY KEY REFERENCES mamony_identity.security_outbox(event_id),
    subject_id text NOT NULL REFERENCES mamony_identity.subjects(subject_id),
    recipient text NOT NULL CHECK (length(recipient) BETWEEN 3 AND 254
        AND recipient=lower(recipient)),
    occurred_at timestamptz NOT NULL,
    status text NOT NULL DEFAULT 'PENDING' CHECK (status='PENDING')
);
REVOKE ALL ON mamony_identity.passkey_registration_notices FROM PUBLIC;

ALTER TABLE mamony_identity.security_outbox
    DROP CONSTRAINT security_outbox_event_type_check;
ALTER TABLE mamony_identity.security_outbox
    ADD CONSTRAINT security_outbox_event_type_check
    CHECK (event_type IN ('ENROLLMENT_PENDING','EMAIL_PROOF_ISSUED',
        'CONTACT_VERIFIED','SESSION_CREATED','SESSION_REVOKED',
        'PASSWORD_CHANGED','PASSWORD_RECOVERED','RECOVERY_ASSURANCE_APPROVED',
        'RECOVERY_CODES_ENROLLED','RECOVERY_CODES_ROTATED','RECOVERY_FACTOR_APPROVED',
        'PASSKEY_REGISTRATION_STAGED'));

CREATE FUNCTION mamony_identity.enqueue_passkey_registration_stage_notice()
RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE recipient_email text;
BEGIN -- PL/pgSQL trigger, not migration transaction control
    IF NEW.event_type<>'PASSKEY_REGISTRATION_STAGED' THEN RETURN NEW; END IF;
    IF session_user<>'mamony_identity_passkey_registration_stage_runtime' THEN
        RAISE EXCEPTION 'passkey staging notice origin denied' USING ERRCODE='42501';
    END IF;
    SELECT a.canonical_email INTO recipient_email
      FROM mamony_identity.email_aliases a
      JOIN mamony_identity.subjects s ON s.subject_id=a.subject_id
     WHERE s.subject_id=NEW.subject_id AND s.status='ACTIVE'
       AND a.verified_at IS NOT NULL AND a.contact_version=1;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'passkey notice missing verified contact' USING ERRCODE='23514';
    END IF;
    INSERT INTO mamony_identity.passkey_registration_notices
        (event_id,subject_id,recipient,occurred_at)
    VALUES (NEW.event_id,NEW.subject_id,recipient_email,NEW.occurred_at);
    RETURN NEW;
END;
$$;
REVOKE ALL ON FUNCTION
    mamony_identity.enqueue_passkey_registration_stage_notice() FROM PUBLIC;
CREATE TRIGGER security_outbox_passkey_registration_staged_notice
AFTER INSERT ON mamony_identity.security_outbox FOR EACH ROW
EXECUTE FUNCTION mamony_identity.enqueue_passkey_registration_stage_notice();

-- This executor must be isolated from the v30 issuer, v31 evidence reader
-- and v32 password reauthentication admission runtime. No direct table DML
-- is granted: it can only stage a non-authenticating PENDING_ASSERTION.
-- Lock order: subject+verified alias -> same session -> current intent ->
-- same-intent admission; DB clock checked only AFTER all those row locks.
CREATE FUNCTION mamony_identity.stage_verified_passkey_credential(
    p_session_digest bytea,p_intent_id text,p_expected_nonce bytea,
    p_expected_epoch bigint,p_rp_id text,p_origin text,
    p_credential_id bytea,p_public_key_cose bytea,p_aaguid bytea,
    p_attestation_format text,p_attestation_type text,
    p_up boolean,p_uv boolean,p_backup_eligible boolean,
    p_backup_state boolean,p_sign_count bigint,p_event text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    account_subject text;
    current_epoch bigint;
    current_contact_version bigint;
    stored_session_epoch bigint;
    seen timestamptz;
    session_expiry timestamptz;
    issued timestamptz;
    intent_expiry timestamptz;
    admission_approved timestamptz;
    admission_expiry timestamptz;
    stamp timestamptz;
BEGIN -- PL/pgSQL function, not migration transaction control
    IF session_user<>'mamony_identity_passkey_registration_stage_runtime' THEN
        RAISE EXCEPTION 'passkey staging origin denied' USING ERRCODE='42501';
    END IF;
    IF p_session_digest IS NULL OR octet_length(p_session_digest)<>32
       OR p_intent_id IS NULL OR p_intent_id !~ '^wrg_[A-Za-z0-9_-]{22}$'
       OR p_expected_nonce IS NULL OR octet_length(p_expected_nonce)<>32
       OR p_expected_epoch IS NULL OR p_expected_epoch<=0
       OR p_rp_id IS NULL OR length(p_rp_id) NOT BETWEEN 4 AND 253
       OR p_origin IS NULL OR p_origin IS DISTINCT FROM 'https://' || p_rp_id
       OR p_credential_id IS NULL OR octet_length(p_credential_id) NOT BETWEEN 16 AND 1023
       OR p_public_key_cose IS NULL OR octet_length(p_public_key_cose) NOT BETWEEN 16 AND 2048
       OR p_aaguid IS NULL OR octet_length(p_aaguid)<>16
       OR p_attestation_format IS NULL
       OR length(p_attestation_format) NOT BETWEEN 1 AND 64
       OR p_attestation_type IS NULL OR length(p_attestation_type) NOT BETWEEN 1 AND 64
       OR p_up IS DISTINCT FROM true OR p_uv IS DISTINCT FROM true
       OR p_backup_eligible IS NULL OR p_backup_state IS NULL
       OR (p_backup_state AND NOT p_backup_eligible)
       OR p_sign_count IS NULL OR p_sign_count NOT BETWEEN 0 AND 4294967295
       OR p_event IS NULL OR p_event !~ '^evt_[A-Za-z0-9_-]{22}$'
    THEN RETURN false; END IF;

    -- The bearer digest locates the subject; no caller-chosen subject.
    SELECT ss.subject_id INTO account_subject
      FROM mamony_identity.opaque_sessions ss
     WHERE ss.token_digest=p_session_digest;
    IF NOT FOUND THEN RETURN false; END IF;
    SELECT s.session_epoch,a.contact_version INTO current_epoch,current_contact_version
      FROM mamony_identity.subjects s
      JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
     WHERE s.subject_id=account_subject AND s.status='ACTIVE'
       AND a.verified_at IS NOT NULL AND a.contact_version=1
       AND s.session_epoch=p_expected_epoch
     FOR UPDATE OF s,a;
    IF NOT FOUND THEN RETURN false; END IF;
    SELECT ss.subject_epoch,ss.last_seen_at,ss.expires_at
      INTO stored_session_epoch,seen,session_expiry
      FROM mamony_identity.opaque_sessions ss
     WHERE ss.token_digest=p_session_digest AND ss.subject_id=account_subject
       AND ss.subject_epoch=current_epoch AND ss.revoked_at IS NULL
     FOR UPDATE OF ss;
    IF NOT FOUND OR stored_session_epoch<>current_epoch THEN RETURN false; END IF;
    SELECT i.issued_at,i.expires_at INTO issued,intent_expiry
      FROM mamony_identity.passkey_registration_intents i
     WHERE i.subject_id=account_subject AND i.intent_id=p_intent_id
       AND i.session_digest=p_session_digest AND i.subject_epoch=current_epoch
       AND i.contact_version=current_contact_version
       AND i.purpose='PASSKEY_REGISTRATION'
       AND i.policy_generation=1 AND i.rp_id=p_rp_id AND i.origin=p_origin
       AND i.challenge=p_expected_nonce AND i.consumed_at IS NULL
     FOR UPDATE OF i;
    IF NOT FOUND THEN RETURN false; END IF;
    SELECT a.approved_at,a.expires_at INTO admission_approved,admission_expiry
      FROM mamony_identity.passkey_registration_admissions a
     WHERE a.subject_id=account_subject AND a.intent_id=p_intent_id
       AND a.session_digest=p_session_digest
       AND a.subject_epoch=current_epoch
       AND a.contact_version=current_contact_version
       AND a.provenance='PASSWORD_REAUTH_ONLY' AND a.consumed_at IS NULL
     FOR UPDATE OF a;
    IF NOT FOUND THEN RETURN false; END IF;
    stamp:=clock_timestamp();
    IF stamp>=session_expiry OR stamp>=seen+interval '30 minutes'
       OR stamp<seen OR stamp<issued OR stamp>=intent_expiry
       OR stamp<admission_approved OR stamp>=admission_expiry
       OR admission_expiry>intent_expiry THEN RETURN false; END IF;

    -- If ANY later step fails (duplicate credential/event, invalid notice
    -- recipient, revocation, missing row) the function raises and the
    -- caller's database transaction MUST roll back. No partial consumes.
    INSERT INTO mamony_identity.security_outbox
        (event_id,subject_id,event_type,occurred_at)
    VALUES (p_event,account_subject,'PASSKEY_REGISTRATION_STAGED',stamp);
    -- The origin-gated trigger synchronously creates one mandatory notice.
    INSERT INTO mamony_identity.passkey_credentials (
        credential_id,subject_id,intent_id,public_key_cose,aaguid,rp_id,origin,
        subject_epoch,contact_version,attestation_format,attestation_type,
        user_present,user_verified,backup_eligible,backup_state,sign_count,
        provenance,state,event_id,created_at)
    VALUES (p_credential_id,account_subject,p_intent_id,p_public_key_cose,
        p_aaguid,p_rp_id,p_origin,current_epoch,current_contact_version,
        p_attestation_format,p_attestation_type,true,true,p_backup_eligible,
        p_backup_state,p_sign_count,'PASSWORD_REAUTH_ONLY','PENDING_ASSERTION',
        p_event,stamp);
    UPDATE mamony_identity.passkey_registration_intents
       SET consumed_at=stamp
     WHERE subject_id=account_subject AND intent_id=p_intent_id
       AND session_digest=p_session_digest AND consumed_at IS NULL;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'passkey intent consumption fence' USING ERRCODE='40001';
    END IF;
    UPDATE mamony_identity.passkey_registration_admissions
       SET consumed_at=stamp
     WHERE subject_id=account_subject AND intent_id=p_intent_id
       AND session_digest=p_session_digest AND consumed_at IS NULL;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'passkey admission consumption fence' USING ERRCODE='40001';
    END IF;
    RETURN true;
END;
$$;

REVOKE ALL ON FUNCTION mamony_identity.stage_verified_passkey_credential(
    bytea,text,bytea,bigint,text,text,bytea,bytea,bytea,
    text,text,boolean,boolean,boolean,boolean,bigint,text) FROM PUBLIC;
GRANT USAGE ON SCHEMA mamony_identity
    TO mamony_identity_passkey_registration_stage_runtime;
GRANT EXECUTE ON FUNCTION mamony_identity.stage_verified_passkey_credential(
    bytea,text,bytea,bigint,text,text,bytea,bytea,bytea,
    text,text,boolean,boolean,boolean,boolean,bigint,text)
    TO mamony_identity_passkey_registration_stage_runtime;

-- No activation/attestation trust upgrade, no login, no recovery bootstrap,
-- no worker delivering notices and no public route. Staging role must NEVER
-- be shared with the HTTP runtime or password-auth/nonce issuer/read roles.

-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (33, '0713a7a5daebb3b03a7694a7ec2825feac6a48170b8ba5d51903e8c8d3a0b611');
