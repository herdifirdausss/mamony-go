-- Identity v31: a separate UNMOUNTED, read-only registration challenge
-- evidence function, exclusively for a future maintained WebAuthn verifier.
-- This DOES NOT verify clientDataJSON/attestation or grant credential binding.
-- The challenge is public, but session/subject binding is not a public API.
CREATE FUNCTION mamony_identity.read_passkey_registration_intent(
    p_session_digest bytea,p_intent_id text)
RETURNS TABLE(subject_id text,subject_epoch bigint,challenge bytea,
              rp_id text,origin text,policy_generation smallint)
LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    account_subject text;
    current_epoch bigint;
    current_contact_version bigint;
    stored_epoch bigint;
    session_last_seen timestamptz;
    session_expires timestamptz;
    expected_nonce bytea;
    expected_rp text;
    expected_origin text;
    expected_policy smallint;
    issued timestamptz;
    expiry timestamptz;
    stamp timestamptz;
BEGIN -- PL/pgSQL function body; not migration transaction control
    IF session_user<>'mamony_identity_passkey_registration_reader_runtime' THEN
        RAISE EXCEPTION 'passkey registration evidence origin denied'
            USING ERRCODE='42501';
    END IF;
    IF p_session_digest IS NULL OR octet_length(p_session_digest)<>32
       OR p_intent_id IS NULL OR p_intent_id !~ '^wrg_[A-Za-z0-9_-]{22}$'
    THEN RETURN; END IF;

    -- A session digest (not an intent ID or caller-selected subject) resolves
    -- the account. Lock order agrees with the v30 issuer and session writers:
    -- subject+verified contact -> session -> one-subject intent row. Recheck
    -- DB clock AFTER acquiring locks. No session, secret, or private credential
    -- data is exposed to SQL roles.
    SELECT ss.subject_id INTO account_subject
      FROM mamony_identity.opaque_sessions ss
     WHERE ss.token_digest=p_session_digest;
    IF NOT FOUND THEN RETURN; END IF;

    SELECT s.session_epoch,a.contact_version
      INTO current_epoch,current_contact_version
      FROM mamony_identity.subjects s
      JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
     WHERE s.subject_id=account_subject AND s.status='ACTIVE'
       AND a.verified_at IS NOT NULL AND a.contact_version=1
     FOR SHARE OF s,a;
    IF NOT FOUND THEN RETURN; END IF;

    SELECT ss.subject_epoch,ss.last_seen_at,ss.expires_at
      INTO stored_epoch,session_last_seen,session_expires
      FROM mamony_identity.opaque_sessions ss
     WHERE ss.token_digest=p_session_digest
       AND ss.subject_id=account_subject AND ss.revoked_at IS NULL
     FOR SHARE OF ss;
    IF NOT FOUND OR stored_epoch<>current_epoch THEN RETURN; END IF;

    SELECT i.challenge,i.rp_id,i.origin,i.policy_generation,
           i.issued_at,i.expires_at
      INTO expected_nonce,expected_rp,expected_origin,expected_policy,
           issued,expiry
      FROM mamony_identity.passkey_registration_intents i
     WHERE i.subject_id=account_subject AND i.intent_id=p_intent_id
       AND i.session_digest=p_session_digest
       AND i.subject_epoch=current_epoch
       AND i.contact_version=current_contact_version
       AND i.policy_generation=1 AND i.purpose='PASSKEY_REGISTRATION'
       AND i.consumed_at IS NULL
     FOR SHARE OF i;
    IF NOT FOUND THEN RETURN; END IF;

    stamp:=clock_timestamp();
    IF stamp>=session_expires
       OR stamp>=session_last_seen+interval '30 minutes'
       OR stamp<session_last_seen OR stamp<issued OR stamp>=expiry
       OR expected_origin IS DISTINCT FROM 'https://' || expected_rp
    THEN RETURN; END IF;

    RETURN QUERY SELECT account_subject,current_epoch,expected_nonce,
                        expected_rp,expected_origin,expected_policy;
END;
$$;

REVOKE ALL ON FUNCTION mamony_identity.read_passkey_registration_intent(
    bytea,text) FROM PUBLIC;
GRANT USAGE ON SCHEMA mamony_identity
    TO mamony_identity_passkey_registration_reader_runtime;
GRANT EXECUTE ON FUNCTION mamony_identity.read_passkey_registration_intent(
    bytea,text) TO mamony_identity_passkey_registration_reader_runtime;

-- Intentionally NO finish/consume/registration/credential table or role.
-- WebAuthn attestation validation AND independent binding authority remain
-- default-deny future separate ADRs and code reviews.

-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (31, 'XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX');
