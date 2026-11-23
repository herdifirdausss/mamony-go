-- Identity v30: durable, UNMOUNTED WebAuthn registration intent issuance.
-- Deliberately NO credential table, verifier, finish/consume function, trusted
-- binding authority, credential insert permission, public route or assurance.
-- An issued challenge is NOT an authenticator registration approval.
CREATE TABLE mamony_identity.passkey_registration_intents (
    subject_id text PRIMARY KEY REFERENCES mamony_identity.subjects(subject_id),
    intent_id text NOT NULL UNIQUE CHECK (intent_id ~ '^wrg_[A-Za-z0-9_-]{22}$'),
    session_digest bytea NOT NULL CHECK (octet_length(session_digest)=32),
    challenge bytea NOT NULL UNIQUE CHECK (octet_length(challenge)=32),
    subject_epoch bigint NOT NULL CHECK (subject_epoch>0),
    contact_version bigint NOT NULL CHECK (contact_version=1),
    rp_id text NOT NULL CHECK (length(rp_id) BETWEEN 4 AND 253),
    origin text NOT NULL CHECK (origin='https://' || rp_id),
    policy_generation smallint NOT NULL CHECK (policy_generation=1),
    purpose text NOT NULL CHECK (purpose='PASSKEY_REGISTRATION'),
    issued_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz NULL,
    CHECK (expires_at>issued_at
       AND expires_at<=issued_at+interval '3 minutes'),
    CHECK (consumed_at IS NULL OR consumed_at>=issued_at)
);
REVOKE ALL ON mamony_identity.passkey_registration_intents FROM PUBLIC;

-- This role can ONLY stage a challenge for a genuinely live session. It
-- cannot bind a credential or mark an intent verified/consumed; those
-- operations require a future reviewed WebAuthn verifier AND distinct
-- independent registration authority. No password/session-only promotion
-- into trusted recovery-code eligibility.
CREATE FUNCTION mamony_identity.issue_passkey_registration_intent(
    p_session_digest bytea,p_intent_id text,p_challenge bytea,
    p_rp_id text,p_origin text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    account_subject text;
    current_epoch bigint;
    current_contact_version bigint;
    stored_epoch bigint;
    session_last_seen timestamptz;
    session_expires timestamptz;
    stamp timestamptz;
    updated integer;
BEGIN -- PL/pgSQL function body; not a migration transaction
    IF session_user<>'mamony_identity_passkey_registration_issuer_runtime' THEN
        RAISE EXCEPTION 'passkey registration issuer origin denied'
            USING ERRCODE='42501';
    END IF;
    IF p_session_digest IS NULL OR octet_length(p_session_digest)<>32
       OR p_intent_id IS NULL OR p_intent_id !~ '^wrg_[A-Za-z0-9_-]{22}$'
       OR p_challenge IS NULL OR octet_length(p_challenge)<>32
       OR p_rp_id IS NULL OR length(p_rp_id) NOT BETWEEN 4 AND 253
       OR p_rp_id !~ '^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$'
       OR p_origin IS NULL OR p_origin IS DISTINCT FROM 'https://' || p_rp_id
    THEN RETURN false; END IF;

    -- Read-only subject pointer first, then subject/contact, then session:
    -- same ordering as existing authenticated state writers. DB clock checked
    -- AFTER locks to reject expiration during lock contention.
    SELECT ss.subject_id INTO account_subject
      FROM mamony_identity.opaque_sessions ss
     WHERE ss.token_digest=p_session_digest;
    IF NOT FOUND THEN RETURN false; END IF;
    SELECT s.session_epoch,a.contact_version INTO current_epoch,current_contact_version
      FROM mamony_identity.subjects s
      JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
     WHERE s.subject_id=account_subject AND s.status='ACTIVE'
       AND a.verified_at IS NOT NULL AND a.contact_version=1
     FOR UPDATE OF s,a;
    IF NOT FOUND THEN RETURN false; END IF;
    SELECT ss.subject_epoch,ss.last_seen_at,ss.expires_at
      INTO stored_epoch,session_last_seen,session_expires
      FROM mamony_identity.opaque_sessions ss
     WHERE ss.token_digest=p_session_digest
       AND ss.subject_id=account_subject AND ss.revoked_at IS NULL
     FOR UPDATE OF ss;
    IF NOT FOUND OR stored_epoch<>current_epoch THEN RETURN false; END IF;
    stamp:=clock_timestamp();
    IF stamp>=session_expires OR stamp>=session_last_seen+interval '30 minutes'
       OR stamp<session_last_seen THEN RETURN false; END IF;

    -- One outstanding intent per subject, including across sessions.
    -- An old expired/consumed intent can be superseded, but a fresh intent
    -- cannot be silently replaced and concurrent calls have one winner.
    INSERT INTO mamony_identity.passkey_registration_intents (
        subject_id,intent_id,session_digest,challenge,subject_epoch,
        contact_version,rp_id,origin,policy_generation,purpose,issued_at,expires_at)
    VALUES (account_subject,p_intent_id,p_session_digest,p_challenge,current_epoch,
        current_contact_version,p_rp_id,p_origin,1,'PASSKEY_REGISTRATION',
        stamp,stamp+interval '3 minutes')
    ON CONFLICT(subject_id) DO UPDATE SET
        intent_id=EXCLUDED.intent_id,
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
    WHERE mamony_identity.passkey_registration_intents.expires_at<=stamp
       OR mamony_identity.passkey_registration_intents.consumed_at IS NOT NULL;
    GET DIAGNOSTICS updated=ROW_COUNT;
    RETURN updated=1;
END;
$$;

REVOKE ALL ON FUNCTION mamony_identity.issue_passkey_registration_intent(
    bytea,text,bytea,text,text) FROM PUBLIC;
GRANT USAGE ON SCHEMA mamony_identity
    TO mamony_identity_passkey_registration_issuer_runtime;
GRANT EXECUTE ON FUNCTION mamony_identity.issue_passkey_registration_intent(
    bytea,text,bytea,text,text)
    TO mamony_identity_passkey_registration_issuer_runtime;

-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (30, 'XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX');
