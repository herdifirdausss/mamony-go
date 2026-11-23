-- Identity v28: a previously provisioned asymmetric cryptographic factor
-- authorizes a one-use recovery-code FIRST enrollment approval. No user,
-- HTTP or production role can provision/bind/rebind the factor public key.
-- The absent independent key registration ceremony remains DEFAULT DENY.
CREATE TABLE mamony_identity.recovery_code_factor_keys (
    subject_id text PRIMARY KEY REFERENCES mamony_identity.subjects(subject_id),
    public_key bytea NOT NULL UNIQUE CHECK (octet_length(public_key)=32),
    bound_epoch bigint NOT NULL CHECK (bound_epoch>0),
    bound_contact_version bigint NOT NULL CHECK (bound_contact_version=1),
    bound_at timestamptz NOT NULL,
    revoked_at timestamptz NULL
);
REVOKE ALL ON mamony_identity.recovery_code_factor_keys FROM PUBLIC;

CREATE TABLE mamony_identity.recovery_code_factor_challenges (
    challenge_id text PRIMARY KEY
        CHECK (challenge_id ~ '^fac_[A-Za-z0-9_-]{22}$'),
    subject_id text NOT NULL REFERENCES mamony_identity.subjects(subject_id),
    subject_epoch bigint NOT NULL CHECK (subject_epoch>0),
    nonce bytea NOT NULL UNIQUE CHECK (octet_length(nonce)=32),
    issued_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    failed_attempts integer NOT NULL DEFAULT 0 CHECK (failed_attempts BETWEEN 0 AND 5),
    consumed_at timestamptz NULL,
    CHECK (expires_at>issued_at AND expires_at<=issued_at+interval '3 minutes'),
    CHECK (consumed_at IS NULL OR consumed_at>=issued_at)
);
CREATE INDEX recovery_code_factor_challenge_subject
    ON mamony_identity.recovery_code_factor_challenges(subject_id,expires_at);
REVOKE ALL ON mamony_identity.recovery_code_factor_challenges FROM PUBLIC;

-- Issuance is impossible without fresh existing Argon2id password proof,
-- committed via the purpose-separated v25 RECOVERY_CODE_ROTATION grant.
-- Grant -> subject/alias -> session -> pre-provisioned key -> challenge.
CREATE FUNCTION mamony_identity.issue_recovery_code_factor_challenge(
    p_session bytea,p_grant bytea,p_id text,p_nonce bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE expected_subject text; expected_epoch bigint;
        current_epoch bigint; contact_version bigint;
        latest_seen timestamptz; expiry timestamptz; stamp timestamptz;
BEGIN -- PL/pgSQL function block, not a migration transaction
    IF session_user<>'mamony_identity_recovery_factor_issuer_runtime' THEN
        RAISE EXCEPTION 'factor challenge issuer origin denied' USING ERRCODE='42501';
    END IF;
    IF p_session IS NULL OR octet_length(p_session)<>32
       OR p_grant IS NULL OR octet_length(p_grant)<>32
       OR p_id IS NULL OR p_id !~ '^fac_[A-Za-z0-9_-]{22}$'
       OR p_nonce IS NULL OR octet_length(p_nonce)<>32 THEN RETURN false; END IF;
    SELECT g.subject_id,g.subject_epoch INTO expected_subject,expected_epoch
      FROM mamony_identity.password_login_grants g
     WHERE g.grant_digest=p_grant AND g.purpose='RECOVERY_CODE_ROTATION'
       AND g.consumed_at IS NULL FOR UPDATE OF g;
    IF NOT FOUND THEN RETURN false; END IF;
    SELECT s.session_epoch,a.contact_version INTO current_epoch,contact_version
      FROM mamony_identity.subjects s
      JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
     WHERE s.subject_id=expected_subject AND s.status='ACTIVE'
       AND s.session_epoch=expected_epoch
       AND a.verified_at IS NOT NULL AND a.contact_version=1
     FOR UPDATE OF s,a;
    IF NOT FOUND THEN RETURN false; END IF;
    SELECT ss.last_seen_at,ss.expires_at INTO latest_seen,expiry
      FROM mamony_identity.opaque_sessions ss
     WHERE ss.token_digest=p_session AND ss.subject_id=expected_subject
       AND ss.subject_epoch=current_epoch AND ss.revoked_at IS NULL
     FOR UPDATE OF ss;
    IF NOT FOUND THEN RETURN false; END IF;
    stamp:=clock_timestamp();
    IF stamp>=expiry OR stamp>=latest_seen+interval '30 minutes'
       OR stamp<latest_seen THEN RETURN false; END IF;
    PERFORM 1 FROM mamony_identity.password_login_grants g
     WHERE g.grant_digest=p_grant AND g.subject_epoch=current_epoch
       AND g.purpose='RECOVERY_CODE_ROTATION'
       AND g.consumed_at IS NULL AND g.expires_at>stamp FOR UPDATE OF g;
    IF NOT FOUND THEN RETURN false; END IF;
    PERFORM 1 FROM mamony_identity.recovery_code_factor_keys k
     WHERE k.subject_id=expected_subject AND k.bound_epoch=current_epoch
       AND k.bound_contact_version=contact_version AND k.revoked_at IS NULL
     FOR UPDATE OF k;
    IF NOT FOUND THEN RETURN false; END IF;
    IF EXISTS(SELECT 1 FROM mamony_identity.password_recovery_offline_codes c
        WHERE c.subject_id=expected_subject) THEN RETURN false; END IF;
    UPDATE mamony_identity.password_login_grants SET consumed_at=stamp
     WHERE grant_digest=p_grant AND consumed_at IS NULL;
    IF NOT FOUND THEN RETURN false; END IF;
    INSERT INTO mamony_identity.recovery_code_factor_challenges
        (challenge_id,subject_id,subject_epoch,nonce,issued_at,expires_at)
    VALUES(p_id,expected_subject,current_epoch,p_nonce,stamp,stamp+interval '3 minutes');
    RETURN true;
END;
$$;

-- Verifier role may read ONLY current, nonexpired challenge+public key.
-- Neither challenge/session nor signing key is accessible through table SELECT.
CREATE FUNCTION mamony_identity.read_recovery_code_factor_challenge(p_id text)
RETURNS TABLE(subject_id text,subject_epoch bigint,nonce bytea,public_key bytea)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $$
BEGIN -- PL/pgSQL function block, not a migration transaction
    IF session_user<>'mamony_identity_recovery_factor_verifier_runtime' THEN
        RAISE EXCEPTION 'factor challenge verifier origin denied' USING ERRCODE='42501';
    END IF;
    IF p_id IS NULL OR p_id !~ '^fac_[A-Za-z0-9_-]{22}$' THEN RETURN; END IF;
    RETURN QUERY
    SELECT c.subject_id,c.subject_epoch,c.nonce,k.public_key
      FROM mamony_identity.recovery_code_factor_challenges c
      JOIN mamony_identity.recovery_code_factor_keys k ON k.subject_id=c.subject_id
      JOIN mamony_identity.subjects s ON s.subject_id=c.subject_id
      JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
     WHERE c.challenge_id=p_id AND c.consumed_at IS NULL
       AND c.failed_attempts<5 AND c.expires_at>clock_timestamp()
       AND s.status='ACTIVE' AND c.subject_epoch=s.session_epoch
       AND k.bound_epoch=s.session_epoch
       AND k.bound_contact_version=a.contact_version
       AND k.revoked_at IS NULL
       AND a.verified_at IS NOT NULL AND a.contact_version=1;
END;
$$;

-- Invalid signatures burn one attempt; the verifier NEVER stores bad
-- signatures or produces any approval from a failed cryptographic check.
CREATE FUNCTION mamony_identity.fail_recovery_code_factor_challenge(p_id text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
BEGIN -- PL/pgSQL function block, not a migration transaction
    IF session_user<>'mamony_identity_recovery_factor_verifier_runtime' THEN
        RAISE EXCEPTION 'factor challenge failure origin denied' USING ERRCODE='42501';
    END IF;
    IF p_id IS NULL OR p_id !~ '^fac_[A-Za-z0-9_-]{22}$' THEN RETURN false; END IF;
    UPDATE mamony_identity.recovery_code_factor_challenges c
       SET failed_attempts=failed_attempts+1
     WHERE c.challenge_id=p_id AND c.consumed_at IS NULL
       AND c.expires_at>clock_timestamp() AND c.failed_attempts<5;
    RETURN FOUND;
END;
$$;

-- Go verifier MUST first validate Ed25519 signature over a purpose-tagged,
-- length-unambiguous (challenge id, subject id, epoch, 256-bit nonce) message.
-- SQL role is a trusted verifier: compromise of that credential can forge
-- approval *only for a previously issued, fresh, unconsumed challenge*.
-- There is deliberately NO key provisioning capability granted to SQL roles.
CREATE FUNCTION mamony_identity.complete_recovery_code_factor_challenge(
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
        (subject_id,subject_epoch,proof_digest,approved_at,expires_at)
    VALUES(account_subject,current_epoch,p_approval_digest,stamp,stamp+interval '5 minutes')
    ON CONFLICT(subject_id) DO UPDATE SET
        subject_epoch=EXCLUDED.subject_epoch,
        proof_digest=EXCLUDED.proof_digest,
        approved_at=EXCLUDED.approved_at,
        expires_at=EXCLUDED.expires_at,
        consumed_at=NULL
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
ALTER TABLE mamony_identity.security_outbox DROP CONSTRAINT security_outbox_event_type_check;
ALTER TABLE mamony_identity.security_outbox ADD CONSTRAINT security_outbox_event_type_check
CHECK (event_type IN ('ENROLLMENT_PENDING','EMAIL_PROOF_ISSUED',
    'CONTACT_VERIFIED','SESSION_CREATED','SESSION_REVOKED',
    'PASSWORD_CHANGED','PASSWORD_RECOVERED','RECOVERY_ASSURANCE_APPROVED',
    'RECOVERY_CODES_ENROLLED','RECOVERY_CODES_ROTATED','RECOVERY_FACTOR_APPROVED'));

REVOKE ALL ON FUNCTION
    mamony_identity.issue_recovery_code_factor_challenge(bytea,bytea,text,bytea),
    mamony_identity.read_recovery_code_factor_challenge(text),
    mamony_identity.fail_recovery_code_factor_challenge(text),
    mamony_identity.complete_recovery_code_factor_challenge(text,bytea,bytea,text)
FROM PUBLIC;
GRANT USAGE ON SCHEMA mamony_identity TO
    mamony_identity_recovery_factor_issuer_runtime,
    mamony_identity_recovery_factor_verifier_runtime;
GRANT EXECUTE ON FUNCTION
    mamony_identity.issue_recovery_code_factor_challenge(bytea,bytea,text,bytea)
TO mamony_identity_recovery_factor_issuer_runtime;
GRANT EXECUTE ON FUNCTION
    mamony_identity.read_recovery_code_factor_challenge(text),
    mamony_identity.fail_recovery_code_factor_challenge(text),
    mamony_identity.complete_recovery_code_factor_challenge(text,bytea,bytea,text)
TO mamony_identity_recovery_factor_verifier_runtime;

-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (28, 'd1ce519db0499e53a48cadc320ef128fb2a4aeb5f49c5b21d8613c444d68e858');
