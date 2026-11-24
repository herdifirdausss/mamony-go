-- Identity capability v36, migration 35: exact one-use possession commit.
-- INTERNAL/DRAFT ONLY. No WebAuthn login, recovery assurance or HTTP route.
-- The dedicated finisher role is a privileged verifier oracle under Profile A:
-- SQL cannot verify the client signature; trusted Go must first use the
-- maintained WebAuthn library against the ORIGINAL v34 evidence.
ALTER TABLE mamony_identity.passkey_credentials
    ADD COLUMN possession_verified_at timestamptz,
    ADD COLUMN possession_event_id text UNIQUE
        REFERENCES mamony_identity.security_outbox(event_id),
    ADD COLUMN possession_challenge_id text
        CHECK (possession_challenge_id IS NULL OR
               possession_challenge_id ~ '^pav_[A-Za-z0-9_-]{22}$');
ALTER TABLE mamony_identity.passkey_credentials
    DROP CONSTRAINT passkey_credentials_state_check;
ALTER TABLE mamony_identity.passkey_credentials
    ADD CONSTRAINT passkey_credentials_state_check
    CHECK (state IN ('PENDING_ASSERTION','POSSESSION_VERIFIED'));
ALTER TABLE mamony_identity.passkey_credentials
    ADD CONSTRAINT passkey_credentials_possession_evidence_check
    CHECK (
       (state='PENDING_ASSERTION' AND possession_verified_at IS NULL
        AND possession_event_id IS NULL AND possession_challenge_id IS NULL)
       OR
       (state='POSSESSION_VERIFIED' AND possession_verified_at IS NOT NULL
        AND possession_event_id IS NOT NULL AND possession_challenge_id IS NOT NULL)
    );

-- Original registration event/notice and activation event/notice are separate
-- immutable obligations. A PENDING row is not evidence of mail delivery.
CREATE TABLE mamony_identity.passkey_activation_notices (
    event_id text PRIMARY KEY REFERENCES mamony_identity.security_outbox(event_id),
    subject_id text NOT NULL REFERENCES mamony_identity.subjects(subject_id),
    recipient text NOT NULL
        CHECK (length(recipient) BETWEEN 3 AND 254 AND recipient=lower(recipient)),
    occurred_at timestamptz NOT NULL,
    status text NOT NULL DEFAULT 'PENDING' CHECK (status='PENDING')
);
REVOKE ALL ON mamony_identity.passkey_activation_notices FROM PUBLIC;
ALTER TABLE mamony_identity.security_outbox
    DROP CONSTRAINT security_outbox_event_type_check;
ALTER TABLE mamony_identity.security_outbox
    ADD CONSTRAINT security_outbox_event_type_check
    CHECK (event_type IN ('ENROLLMENT_PENDING','EMAIL_PROOF_ISSUED',
        'CONTACT_VERIFIED','SESSION_CREATED','SESSION_REVOKED',
        'PASSWORD_CHANGED','PASSWORD_RECOVERED','RECOVERY_ASSURANCE_APPROVED',
        'RECOVERY_CODES_ENROLLED','RECOVERY_CODES_ROTATED','RECOVERY_FACTOR_APPROVED',
        'PASSKEY_REGISTRATION_STAGED','PASSKEY_POSSESSION_VERIFIED'));

CREATE FUNCTION mamony_identity.enqueue_passkey_activation_notice()
RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE recipient_email text;
BEGIN
    IF NEW.event_type<>'PASSKEY_POSSESSION_VERIFIED' THEN RETURN NEW; END IF;
    IF session_user<>'mamony_identity_passkey_activation_finisher_runtime' THEN
        RAISE EXCEPTION 'passkey possession notice origin denied' USING ERRCODE='42501';
    END IF;
    SELECT a.canonical_email INTO recipient_email
      FROM mamony_identity.email_aliases a
      JOIN mamony_identity.subjects s ON s.subject_id=a.subject_id
     WHERE s.subject_id=NEW.subject_id AND s.status='ACTIVE'
       AND a.verified_at IS NOT NULL AND a.contact_version=1;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'passkey possession notice missing contact' USING ERRCODE='23514';
    END IF;
    INSERT INTO mamony_identity.passkey_activation_notices
        (event_id,subject_id,recipient,occurred_at)
    VALUES (NEW.event_id,NEW.subject_id,recipient_email,NEW.occurred_at);
    RETURN NEW;
END;
$$;
REVOKE ALL ON FUNCTION mamony_identity.enqueue_passkey_activation_notice() FROM PUBLIC;
CREATE TRIGGER security_outbox_passkey_possession_notice
AFTER INSERT ON mamony_identity.security_outbox FOR EACH ROW
EXECUTE FUNCTION mamony_identity.enqueue_passkey_activation_notice();

-- 18 parameters, all asserted by trusted Go after maintained-library
-- verification. No caller-selected subject or role; the session digest
-- derives the account. Snapshot fields must match fresh locked DB rows.
CREATE FUNCTION mamony_identity.finish_verified_passkey_possession(
    p_session_digest bytea,p_challenge_id text,p_expected_nonce bytea,
    p_expected_epoch bigint,p_rp_id text,p_origin text,
    p_credential_id bytea,p_public_key_cose bytea,p_aaguid bytea,
    p_expected_count bigint,p_expected_be boolean,p_expected_bs boolean,
    p_signed_count bigint,p_signed_be boolean,p_signed_bs boolean,
    p_signed_up boolean,p_signed_uv boolean,p_event text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    account_subject text;
    current_epoch bigint;
    current_contact bigint;
    session_epoch bigint;
    seen timestamptz;
    session_expiry timestamptz;
    credential_created timestamptz;
    original_intent text;
    original_event text;
    challenge_issued timestamptz;
    challenge_expires timestamptz;
    stamp timestamptz;
    affected integer;
BEGIN
    IF session_user<>'mamony_identity_passkey_activation_finisher_runtime' THEN
        RAISE EXCEPTION 'passkey possession finisher origin denied'
            USING ERRCODE='42501';
    END IF;
    IF p_session_digest IS NULL OR octet_length(p_session_digest)<>32
       OR p_challenge_id IS NULL OR p_challenge_id !~ '^pav_[A-Za-z0-9_-]{22}$'
       OR p_expected_nonce IS NULL OR octet_length(p_expected_nonce)<>32
       OR p_expected_epoch IS NULL OR p_expected_epoch<=0
       OR p_rp_id IS NULL OR length(p_rp_id) NOT BETWEEN 4 AND 253
       OR p_rp_id !~ '^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$'
       OR p_origin IS NULL OR p_origin IS DISTINCT FROM 'https://' || p_rp_id
       OR p_credential_id IS NULL OR octet_length(p_credential_id) NOT BETWEEN 16 AND 1023
       OR p_public_key_cose IS NULL OR octet_length(p_public_key_cose) NOT BETWEEN 16 AND 2048
       OR p_aaguid IS NULL OR octet_length(p_aaguid)<>16
       OR p_expected_count IS NULL OR p_expected_count NOT BETWEEN 0 AND 4294967295
       OR p_signed_count IS NULL OR p_signed_count NOT BETWEEN 0 AND 4294967295
       OR p_expected_be IS NULL OR p_expected_bs IS NULL
       OR (p_expected_bs AND NOT p_expected_be)
       OR p_signed_be IS DISTINCT FROM p_expected_be OR p_signed_bs IS NULL
       OR (p_signed_bs AND NOT p_signed_be)
       OR p_signed_up IS DISTINCT FROM true OR p_signed_uv IS DISTINCT FROM true
       OR ((p_expected_count<>0 OR p_signed_count<>0)
           AND p_signed_count<=p_expected_count)
       OR p_event IS NULL OR p_event !~ '^evt_[A-Za-z0-9_-]{22}$'
    THEN RETURN false; END IF;

    -- Existing issuer/stager lock order: subject+alias -> session ->
    -- credential -> challenge. No multi-table query is relied on to
    -- implicitly acquire credential and challenge locks in that order.
    SELECT ss.subject_id INTO account_subject
      FROM mamony_identity.opaque_sessions ss
     WHERE ss.token_digest=p_session_digest;
    IF NOT FOUND THEN RETURN false; END IF;
    SELECT s.session_epoch,a.contact_version INTO current_epoch,current_contact
      FROM mamony_identity.subjects s
      JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
     WHERE s.subject_id=account_subject AND s.status='ACTIVE'
       AND a.verified_at IS NOT NULL AND a.contact_version=1
       AND s.session_epoch=p_expected_epoch
     FOR UPDATE OF s,a;
    IF NOT FOUND THEN RETURN false; END IF;
    SELECT ss.subject_epoch,ss.last_seen_at,ss.expires_at
      INTO session_epoch,seen,session_expiry
      FROM mamony_identity.opaque_sessions ss
     WHERE ss.token_digest=p_session_digest AND ss.subject_id=account_subject
       AND ss.subject_epoch=current_epoch AND ss.revoked_at IS NULL
     FOR UPDATE OF ss;
    IF NOT FOUND OR session_epoch<>current_epoch THEN RETURN false; END IF;

    SELECT c.created_at,c.intent_id,c.event_id
      INTO credential_created,original_intent,original_event
      FROM mamony_identity.passkey_credentials c
     WHERE c.credential_id=p_credential_id AND c.subject_id=account_subject
       AND c.state='PENDING_ASSERTION'
       AND c.subject_epoch=current_epoch AND c.contact_version=current_contact
       AND c.provenance='PASSWORD_REAUTH_ONLY'
       AND c.rp_id=p_rp_id AND c.origin=p_origin
       AND c.public_key_cose=p_public_key_cose AND c.aaguid=p_aaguid
       AND c.sign_count=p_expected_count
       AND c.backup_eligible=p_expected_be AND c.backup_state=p_expected_bs
     FOR UPDATE OF c;
    IF NOT FOUND THEN RETURN false; END IF;

    SELECT a.issued_at,a.expires_at
      INTO challenge_issued,challenge_expires
      FROM mamony_identity.passkey_activation_challenges a
     WHERE a.subject_id=account_subject AND a.challenge_id=p_challenge_id
       AND a.credential_id=p_credential_id
       AND a.session_digest=p_session_digest
       AND a.challenge=p_expected_nonce
       AND a.subject_epoch=current_epoch AND a.contact_version=current_contact
       AND a.rp_id=p_rp_id AND a.origin=p_origin
       AND a.policy_generation=1 AND a.purpose='PASSKEY_ACTIVATION_ASSERTION'
       AND a.consumed_at IS NULL
     FOR UPDATE OF a;
    IF NOT FOUND THEN RETURN false; END IF;

    -- Credential MUST still be backed by its original consumed v30/v32
    -- registration and one original mandatory stage audit/notice.
    PERFORM 1
      FROM mamony_identity.passkey_registration_intents i
      JOIN mamony_identity.passkey_registration_admissions d
        ON d.subject_id=i.subject_id AND d.intent_id=i.intent_id
      JOIN mamony_identity.security_outbox o
        ON o.event_id=original_event AND o.subject_id=account_subject
      JOIN mamony_identity.passkey_registration_notices n
        ON n.event_id=original_event AND n.subject_id=account_subject
     WHERE i.subject_id=account_subject AND i.intent_id=original_intent
       AND i.session_digest=p_session_digest
       AND i.subject_epoch=current_epoch AND i.contact_version=current_contact
       AND i.purpose='PASSKEY_REGISTRATION' AND i.policy_generation=1
       AND i.rp_id=p_rp_id AND i.origin=p_origin AND i.consumed_at IS NOT NULL
       AND d.session_digest=p_session_digest
       AND d.subject_epoch=current_epoch AND d.contact_version=current_contact
       AND d.provenance='PASSWORD_REAUTH_ONLY' AND d.consumed_at IS NOT NULL
       AND o.event_type='PASSKEY_REGISTRATION_STAGED';
    IF NOT FOUND THEN RETURN false; END IF;

    -- DB clock sampled AFTER locks: delayed/revoked/expired requests
    -- cannot inherit the earlier v35 read's snapshot validity.
    stamp:=clock_timestamp();
    IF stamp<seen OR stamp>=session_expiry OR stamp>=seen+interval '30 minutes'
       OR stamp<credential_created OR stamp>=credential_created+interval '10 minutes'
       OR stamp<challenge_issued OR stamp>=challenge_expires
    THEN RETURN false; END IF;

    -- Event insertion triggers the mandatory recipient-verified notice.
    -- A collision or notice failure aborts the caller's single SQL Tx.
    INSERT INTO mamony_identity.security_outbox
        (event_id,subject_id,event_type,occurred_at)
    VALUES (p_event,account_subject,'PASSKEY_POSSESSION_VERIFIED',stamp);

    UPDATE mamony_identity.passkey_credentials
       SET state='POSSESSION_VERIFIED',sign_count=p_signed_count,
           backup_state=p_signed_bs,possession_verified_at=stamp,
           possession_event_id=p_event,possession_challenge_id=p_challenge_id
     WHERE credential_id=p_credential_id AND subject_id=account_subject
       AND state='PENDING_ASSERTION' AND sign_count=p_expected_count
       AND backup_eligible=p_signed_be;
    GET DIAGNOSTICS affected=ROW_COUNT;
    IF affected<>1 THEN
        RAISE EXCEPTION 'passkey possession credential update fence'
            USING ERRCODE='40001';
    END IF;
    UPDATE mamony_identity.passkey_activation_challenges
       SET consumed_at=stamp
     WHERE subject_id=account_subject AND challenge_id=p_challenge_id
       AND challenge=p_expected_nonce AND consumed_at IS NULL;
    GET DIAGNOSTICS affected=ROW_COUNT;
    IF affected<>1 THEN
        RAISE EXCEPTION 'passkey possession challenge consume fence'
            USING ERRCODE='40001';
    END IF;
    RETURN true;
END;
$$;

REVOKE ALL ON FUNCTION mamony_identity.finish_verified_passkey_possession(
    bytea,text,bytea,bigint,text,text,bytea,bytea,bytea,
    bigint,boolean,boolean,bigint,boolean,boolean,boolean,boolean,text) FROM PUBLIC;
GRANT USAGE ON SCHEMA mamony_identity
    TO mamony_identity_passkey_activation_finisher_runtime;
GRANT EXECUTE ON FUNCTION mamony_identity.finish_verified_passkey_possession(
    bytea,text,bytea,bigint,text,text,bytea,bytea,bytea,
    bigint,boolean,boolean,bigint,boolean,boolean,boolean,boolean,text)
    TO mamony_identity_passkey_activation_finisher_runtime;

-- No credential may authenticate: login and independent recovery remain
-- unimplemented and all public/private router inventories are unchanged.

-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (35, '38e0fa55a039b8d61852103db3dd1d732ae0335abaf7112590ed755d525b038c');
