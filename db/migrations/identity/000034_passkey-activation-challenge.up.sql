-- Identity v34: unmounted, strictly one-use activation-assertion challenge.
-- Staged credentials remain PENDING_ASSERTION: this cannot verify an
-- authenticator signature, activate a credential, mint a session, or supply
-- independent recovery-factor assurance.
CREATE TABLE mamony_identity.passkey_activation_challenges (
    subject_id text PRIMARY KEY REFERENCES mamony_identity.subjects(subject_id),
    challenge_id text NOT NULL UNIQUE CHECK (challenge_id ~ '^pav_[A-Za-z0-9_-]{22}$'),
    credential_id bytea NOT NULL REFERENCES mamony_identity.passkey_credentials(credential_id),
    session_digest bytea NOT NULL CHECK (octet_length(session_digest)=32),
    challenge bytea NOT NULL UNIQUE CHECK (octet_length(challenge)=32),
    subject_epoch bigint NOT NULL CHECK (subject_epoch>0),
    contact_version bigint NOT NULL CHECK (contact_version=1),
    rp_id text NOT NULL CHECK (length(rp_id) BETWEEN 4 AND 253),
    origin text NOT NULL CHECK (origin='https://' || rp_id),
    policy_generation smallint NOT NULL CHECK (policy_generation=1),
    purpose text NOT NULL CHECK (purpose='PASSKEY_ACTIVATION_ASSERTION'),
    issued_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz NULL,
    CHECK (expires_at>issued_at AND expires_at<=issued_at+interval '3 minutes'),
    CHECK (consumed_at IS NULL OR consumed_at>=issued_at)
);
REVOKE ALL ON mamony_identity.passkey_activation_challenges FROM PUBLIC;

-- Per-subject one outstanding activation challenge. Security checks are
-- performed against the exact original registration intent (including the
-- originating session digest and consumed registration nonce/admission).
-- Because v30 overwrites completed intents, a newer registration Begin
-- deliberately invalidates the earlier PENDING credential's activation path:
-- no weaker fallback or independent account-control claim is allowed.
CREATE FUNCTION mamony_identity.issue_passkey_activation_challenge(
    p_session_digest bytea,p_credential_id bytea,p_challenge_id text,
    p_challenge bytea,p_rp_id text,p_origin text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    account_subject text;
    current_epoch bigint;
    current_contact bigint;
    stored_epoch bigint;
    seen timestamptz;
    session_expiry timestamptz;
    credential_created timestamptz;
    stamp timestamptz;
    affected integer;
BEGIN -- PL/pgSQL block, not a migration transaction
    IF session_user<>'mamony_identity_passkey_activation_issuer_runtime' THEN
        RAISE EXCEPTION 'passkey activation issue origin denied' USING ERRCODE='42501';
    END IF;
    IF p_session_digest IS NULL OR octet_length(p_session_digest)<>32
       OR p_credential_id IS NULL OR octet_length(p_credential_id) NOT BETWEEN 16 AND 1023
       OR p_challenge_id IS NULL OR p_challenge_id !~ '^pav_[A-Za-z0-9_-]{22}$'
       OR p_challenge IS NULL OR octet_length(p_challenge)<>32
       OR p_rp_id IS NULL OR length(p_rp_id) NOT BETWEEN 4 AND 253
       OR p_rp_id !~ '^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$'
       OR p_origin IS NULL OR p_origin IS DISTINCT FROM 'https://' || p_rp_id
    THEN RETURN false; END IF;
    SELECT ss.subject_id INTO account_subject
      FROM mamony_identity.opaque_sessions ss
     WHERE ss.token_digest=p_session_digest;
    IF NOT FOUND THEN RETURN false; END IF;
    -- Same lock order as v33: subject/alias -> session -> credential ->
    -- activation challenge; check clock AFTER locks.
    SELECT s.session_epoch,a.contact_version INTO current_epoch,current_contact
      FROM mamony_identity.subjects s
      JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
     WHERE s.subject_id=account_subject AND s.status='ACTIVE'
       AND a.verified_at IS NOT NULL AND a.contact_version=1
     FOR UPDATE OF s,a;
    IF NOT FOUND THEN RETURN false; END IF;
    SELECT ss.subject_epoch,ss.last_seen_at,ss.expires_at
      INTO stored_epoch,seen,session_expiry
      FROM mamony_identity.opaque_sessions ss
     WHERE ss.token_digest=p_session_digest AND ss.subject_id=account_subject
       AND ss.subject_epoch=current_epoch AND ss.revoked_at IS NULL
     FOR UPDATE OF ss;
    IF NOT FOUND OR stored_epoch<>current_epoch THEN RETURN false; END IF;

    SELECT c.created_at INTO credential_created
      FROM mamony_identity.passkey_credentials c
      JOIN mamony_identity.passkey_registration_intents i
        ON i.intent_id=c.intent_id AND i.subject_id=c.subject_id
      JOIN mamony_identity.passkey_registration_admissions d
        ON d.intent_id=c.intent_id AND d.subject_id=c.subject_id
      JOIN mamony_identity.security_outbox o ON o.event_id=c.event_id
      JOIN mamony_identity.passkey_registration_notices n
        ON n.event_id=c.event_id AND n.subject_id=c.subject_id
     WHERE c.credential_id=p_credential_id AND c.subject_id=account_subject
       AND c.state='PENDING_ASSERTION'
       AND c.provenance='PASSWORD_REAUTH_ONLY'
       AND c.subject_epoch=current_epoch AND c.contact_version=current_contact
       AND c.rp_id=p_rp_id AND c.origin=p_origin
       AND i.session_digest=p_session_digest
       AND i.subject_epoch=current_epoch AND i.contact_version=current_contact
       AND i.consumed_at IS NOT NULL
       AND d.session_digest=p_session_digest
       AND d.subject_epoch=current_epoch AND d.contact_version=current_contact
       AND d.provenance='PASSWORD_REAUTH_ONLY' AND d.consumed_at IS NOT NULL
       AND o.event_type='PASSKEY_REGISTRATION_STAGED'
     FOR SHARE OF c;
    IF NOT FOUND THEN RETURN false; END IF;
    stamp:=clock_timestamp();
    IF stamp>=session_expiry OR stamp>=seen+interval '30 minutes'
       OR stamp<seen OR stamp<credential_created
       OR stamp>=credential_created+interval '10 minutes'
    THEN RETURN false; END IF;

    INSERT INTO mamony_identity.passkey_activation_challenges (
        subject_id,challenge_id,credential_id,session_digest,challenge,
        subject_epoch,contact_version,rp_id,origin,policy_generation,
        purpose,issued_at,expires_at)
    VALUES(account_subject,p_challenge_id,p_credential_id,p_session_digest,
           p_challenge,current_epoch,current_contact,p_rp_id,p_origin,1,
           'PASSKEY_ACTIVATION_ASSERTION',stamp,LEAST(
             stamp+interval '3 minutes',credential_created+interval '10 minutes'))
    ON CONFLICT(subject_id) DO UPDATE SET
        challenge_id=EXCLUDED.challenge_id,
        credential_id=EXCLUDED.credential_id,
        session_digest=EXCLUDED.session_digest,
        challenge=EXCLUDED.challenge,
        subject_epoch=EXCLUDED.subject_epoch,
        contact_version=EXCLUDED.contact_version,
        rp_id=EXCLUDED.rp_id,
        origin=EXCLUDED.origin,
        policy_generation=EXCLUDED.policy_generation,
        purpose=EXCLUDED.purpose,
        issued_at=EXCLUDED.issued_at,
        expires_at=EXCLUDED.expires_at,
        consumed_at=NULL
    WHERE mamony_identity.passkey_activation_challenges.expires_at<=stamp
       OR mamony_identity.passkey_activation_challenges.consumed_at IS NOT NULL;
    GET DIAGNOSTICS affected=ROW_COUNT;
    RETURN affected=1;
END;
$$;

-- Read-only expected assertion context. It returns a specific PENDING
-- credential's public key, flags and counter only for the original live
-- staging session and currently unspent/nonexpired activation nonce.
-- Neither returning an assertion context nor issuing a challenge proves
-- possession, and the reader cannot consume or activate credentials.
CREATE FUNCTION mamony_identity.read_passkey_activation_challenge(
    p_session_digest bytea,p_challenge_id text)
RETURNS TABLE(subject_id text,subject_epoch bigint,challenge bytea,
    credential_id bytea,public_key_cose bytea,aaguid bytea,
    sign_count bigint,backup_eligible boolean,backup_state boolean,
    rp_id text,origin text,policy_generation smallint)
LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    account_subject text;
    current_epoch bigint;
    current_contact bigint;
    stored_epoch bigint;
    seen timestamptz;
    session_expiry timestamptz;
    stamp timestamptz;
BEGIN -- PL/pgSQL function body, not migration transaction
    IF session_user<>'mamony_identity_passkey_activation_reader_runtime' THEN
        RAISE EXCEPTION 'passkey activation reader origin denied' USING ERRCODE='42501';
    END IF;
    IF p_session_digest IS NULL OR octet_length(p_session_digest)<>32
       OR p_challenge_id IS NULL OR p_challenge_id !~ '^pav_[A-Za-z0-9_-]{22}$'
    THEN RETURN; END IF;
    SELECT ss.subject_id INTO account_subject
      FROM mamony_identity.opaque_sessions ss
     WHERE ss.token_digest=p_session_digest;
    IF NOT FOUND THEN RETURN; END IF;
    SELECT s.session_epoch,a.contact_version INTO current_epoch,current_contact
      FROM mamony_identity.subjects s
      JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
     WHERE s.subject_id=account_subject AND s.status='ACTIVE'
       AND a.verified_at IS NOT NULL AND a.contact_version=1
     FOR SHARE OF s,a;
    IF NOT FOUND THEN RETURN; END IF;
    SELECT ss.subject_epoch,ss.last_seen_at,ss.expires_at
      INTO stored_epoch,seen,session_expiry
      FROM mamony_identity.opaque_sessions ss
     WHERE ss.token_digest=p_session_digest AND ss.subject_id=account_subject
       AND ss.subject_epoch=current_epoch AND ss.revoked_at IS NULL
     FOR SHARE OF ss;
    IF NOT FOUND OR stored_epoch<>current_epoch THEN RETURN; END IF;
    stamp:=clock_timestamp();
    IF stamp>=session_expiry OR stamp>=seen+interval '30 minutes'
       OR stamp<seen THEN RETURN; END IF;
    RETURN QUERY
    SELECT account_subject,current_epoch,a.challenge,c.credential_id,
           c.public_key_cose,c.aaguid,c.sign_count,c.backup_eligible,
           c.backup_state,c.rp_id,c.origin,a.policy_generation
      FROM mamony_identity.passkey_credentials c
      JOIN mamony_identity.passkey_activation_challenges a
        ON a.credential_id=c.credential_id AND a.subject_id=c.subject_id
      JOIN mamony_identity.passkey_registration_intents i
        ON i.intent_id=c.intent_id AND i.subject_id=c.subject_id
      JOIN mamony_identity.passkey_registration_admissions d
        ON d.intent_id=c.intent_id AND d.subject_id=c.subject_id
      JOIN mamony_identity.security_outbox o ON o.event_id=c.event_id
      JOIN mamony_identity.passkey_registration_notices n
        ON n.event_id=c.event_id AND n.subject_id=c.subject_id
     WHERE a.subject_id=account_subject AND a.challenge_id=p_challenge_id
       AND a.session_digest=p_session_digest
       AND a.subject_epoch=current_epoch AND a.contact_version=current_contact
       AND a.policy_generation=1 AND a.purpose='PASSKEY_ACTIVATION_ASSERTION'
       AND a.consumed_at IS NULL AND a.issued_at<=stamp AND a.expires_at>stamp
       AND c.state='PENDING_ASSERTION' AND c.provenance='PASSWORD_REAUTH_ONLY'
       AND c.subject_epoch=current_epoch AND c.contact_version=current_contact
       AND c.rp_id=a.rp_id AND c.origin=a.origin
       AND c.created_at<=stamp AND c.created_at+interval '10 minutes'>stamp
       AND i.session_digest=p_session_digest AND i.consumed_at IS NOT NULL
       AND i.subject_epoch=current_epoch AND i.contact_version=current_contact
       AND d.session_digest=p_session_digest AND d.consumed_at IS NOT NULL
       AND d.subject_epoch=current_epoch AND d.contact_version=current_contact
       AND d.provenance='PASSWORD_REAUTH_ONLY'
       AND o.event_type='PASSKEY_REGISTRATION_STAGED'
     FOR SHARE OF c,a;
END;
$$;
REVOKE ALL ON FUNCTION mamony_identity.issue_passkey_activation_challenge(
    bytea,bytea,text,bytea,text,text) FROM PUBLIC;
REVOKE ALL ON FUNCTION mamony_identity.read_passkey_activation_challenge(
    bytea,text) FROM PUBLIC;
GRANT USAGE ON SCHEMA mamony_identity
    TO mamony_identity_passkey_activation_issuer_runtime,
       mamony_identity_passkey_activation_reader_runtime;
GRANT EXECUTE ON FUNCTION mamony_identity.issue_passkey_activation_challenge(
    bytea,bytea,text,bytea,text,text)
    TO mamony_identity_passkey_activation_issuer_runtime;
GRANT EXECUTE ON FUNCTION mamony_identity.read_passkey_activation_challenge(
    bytea,text) TO mamony_identity_passkey_activation_reader_runtime;

-- Explicit NO activation mutation, assertion verification or public route.
-- Pending credential status remains PENDING_ASSERTION as in v33.

-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (34, '3452edcc82bfd809db0ff56ad783418e5e5048c05b0a1017eb7e9182437f51a8');
