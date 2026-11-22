-- Identity v24: offline saved recovery codes as a distinct, single-use
-- proof of prior possession IN ADDITION to the email recovery challenge.
-- Runtime does NOT issue/reissue/enroll codes, mount an HTTP endpoint or
-- create a supported production recovery ceremony. Codes in tests are
-- enrolled exclusively through privileged disposable ADMIN fixtures.
--
-- Harden the v21 assurance grant against "approve one challenge, redeem
-- another" by adding an immutable challenge-binding key. Fail closed on
-- migration if any legacy unbound grant already exists; operators MUST
-- manually review/revoke such old grants rather than silently binding them.
ALTER TABLE mamony_identity.password_recovery_assurance_grants
    ADD COLUMN challenge_id text NOT NULL
        REFERENCES mamony_identity.password_recovery_challenges(challenge_id);

-- The offline code is a 256-bit CSPRNG saved recovery code. Persist ONLY
-- a purpose-separated SHA-256 digest. Never share a recovery code with an
-- email flow or use the same code for a second recovery. Enrollment must
-- precede the recovery request and requires independent authentication.
CREATE TABLE mamony_identity.password_recovery_offline_codes (
    code_digest bytea PRIMARY KEY CHECK (octet_length(code_digest)=32),
    subject_id text NOT NULL REFERENCES mamony_identity.subjects(subject_id),
    enrolled_epoch bigint NOT NULL CHECK (enrolled_epoch>0),
    contact_version bigint NOT NULL CHECK (contact_version>0),
    enrolled_at timestamptz NOT NULL,
    consumed_at timestamptz NULL,
    CHECK (consumed_at IS NULL OR consumed_at>=enrolled_at)
);
CREATE INDEX password_recovery_offline_codes_subject
    ON mamony_identity.password_recovery_offline_codes(subject_id,enrolled_epoch)
    WHERE consumed_at IS NULL;
REVOKE ALL ON mamony_identity.password_recovery_offline_codes FROM PUBLIC;

-- Record an approval without putting an email address, raw code or
-- recovery token in security_outbox. The event type is owner-controlled.
ALTER TABLE mamony_identity.security_outbox
    DROP CONSTRAINT security_outbox_event_type_check;
ALTER TABLE mamony_identity.security_outbox
    ADD CONSTRAINT security_outbox_event_type_check
    CHECK (event_type IN ('ENROLLMENT_PENDING','EMAIL_PROOF_ISSUED',
        'CONTACT_VERIFIED','SESSION_CREATED','SESSION_REVOKED',
        'PASSWORD_CHANGED','PASSWORD_RECOVERED',
        'RECOVERY_ASSURANCE_APPROVED'));

-- Original session user must be the distinct proofing principal. Its role
-- has schema USAGE + EXECUTE on this function ONLY. Neither app/API role
-- nor the issuer/delivery/redeemer has access to it.
CREATE FUNCTION mamony_identity.approve_password_recovery_offline_code(
    p_recovery_digest bytea,p_code_digest bytea,p_audit_event text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    candidate_subject text;
    current_epoch bigint;
    current_version bigint;
    contact_email text;
    challenge text;
    challenged_epoch bigint;
    challenged_version bigint;
    challenged_email text;
    challenged_requested timestamptz;
    challenged_expiry timestamptz;
    stamp timestamptz;
BEGIN -- PL/pgSQL function body; not a migration transaction
    IF session_user<>'mamony_identity_recovery_assurance_runtime' THEN
        RAISE EXCEPTION 'recovery assurance origin denied' USING ERRCODE='42501';
    END IF;
    IF p_recovery_digest IS NULL OR octet_length(p_recovery_digest)<>32
       OR p_code_digest IS NULL OR octet_length(p_code_digest)<>32
       OR p_audit_event IS NULL OR p_audit_event !~ '^evt_[A-Za-z0-9_-]{22}$'
    THEN RETURN false; END IF;

    -- Locate a subject *only* through the issued email recovery token.
    -- No subject, account, eligibility or challenge identifier is ever
    -- returned to the restricted caller. Recheck after locking subject.
    SELECT c.subject_id INTO candidate_subject
      FROM mamony_identity.password_recovery_challenges c
     WHERE c.token_digest=p_recovery_digest;
    IF NOT FOUND THEN RETURN false; END IF;

    SELECT s.session_epoch,a.contact_version,a.canonical_email
      INTO current_epoch,current_version,contact_email
      FROM mamony_identity.subjects s
      JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
     WHERE s.subject_id=candidate_subject AND s.status='ACTIVE'
       AND a.verified_at IS NOT NULL AND a.contact_version=1
     FOR UPDATE OF s,a;
    IF NOT FOUND THEN RETURN false; END IF;

    -- Lock order: subjects+alias -> challenge -> backup code -> grant,
    -- consistent with v19 issuance and v21 redemption. DB clock must be
    -- checked AFTER contended locks, never before them.
    SELECT c.challenge_id,c.subject_epoch,c.contact_version,c.canonical_email,
           c.requested_at,c.expires_at
      INTO challenge,challenged_epoch,challenged_version,challenged_email,
           challenged_requested,challenged_expiry
      FROM mamony_identity.password_recovery_challenges c
     WHERE c.token_digest=p_recovery_digest
       AND c.subject_id=candidate_subject
       AND c.consumed_at IS NULL AND c.revoked_at IS NULL
     FOR UPDATE OF c;
    IF NOT FOUND OR challenged_epoch<>current_epoch
       OR challenged_version<>current_version
       OR challenged_email IS DISTINCT FROM contact_email THEN
        RETURN false;
    END IF;
    stamp:=clock_timestamp();
    IF challenged_expiry<=stamp THEN RETURN false; END IF;

    PERFORM 1 FROM mamony_identity.password_recovery_offline_codes b
     WHERE b.code_digest=p_code_digest AND b.subject_id=candidate_subject
       AND b.enrolled_epoch=current_epoch
       AND b.contact_version=current_version
       AND b.enrolled_at<challenged_requested AND b.consumed_at IS NULL
     FOR UPDATE OF b;
    IF NOT FOUND THEN RETURN false; END IF;

    -- A previously unconsumed live approval for THIS challenge cannot
    -- burn another offline code. Approval for another already-revoked
    -- challenge can be replaced only by independently proving another
    -- saved offline code (no cross-challenge grant reuse).
    PERFORM 1 FROM mamony_identity.password_recovery_assurance_grants g
     WHERE g.subject_id=candidate_subject AND g.challenge_id=challenge
       AND g.consumed_at IS NULL AND g.expires_at>stamp
     FOR UPDATE OF g;
    IF FOUND THEN RETURN false; END IF;

    UPDATE mamony_identity.password_recovery_offline_codes
       SET consumed_at=stamp
     WHERE code_digest=p_code_digest AND consumed_at IS NULL;
    IF NOT FOUND THEN RETURN false; END IF;

    INSERT INTO mamony_identity.password_recovery_assurance_grants
        (subject_id,subject_epoch,challenge_id,approved_at,expires_at)
    VALUES (candidate_subject,current_epoch,challenge,stamp,
            LEAST(stamp+interval '5 minutes',challenged_expiry))
    ON CONFLICT (subject_id) DO UPDATE
      SET subject_epoch=EXCLUDED.subject_epoch,
          challenge_id=EXCLUDED.challenge_id,
          approved_at=EXCLUDED.approved_at,
          expires_at=EXCLUDED.expires_at,
          consumed_at=NULL;

    -- A late audit event ID collision rolls back BOTH the consumed offline
    -- code and grant. The return value never exposes account existence.
    INSERT INTO mamony_identity.security_outbox
        (event_id,subject_id,event_type,occurred_at)
    VALUES (p_audit_event,candidate_subject,'RECOVERY_ASSURANCE_APPROVED',stamp);
    RETURN true;
END;
$$;
REVOKE ALL ON FUNCTION mamony_identity.approve_password_recovery_offline_code(
    bytea,bytea,text) FROM PUBLIC;
GRANT USAGE ON SCHEMA mamony_identity
    TO mamony_identity_recovery_assurance_runtime;
GRANT EXECUTE ON FUNCTION mamony_identity.approve_password_recovery_offline_code(
    bytea,bytea,text)
    TO mamony_identity_recovery_assurance_runtime;

-- Replaces v21 definer without changing its EXECUTE owner and API. All
-- denial reasons stay internal false; neither challenge ID nor enrolled
-- code become visible to the redemption principal.
CREATE OR REPLACE FUNCTION mamony_identity.redeem_password_recovery(
    p_digest bytea,p_expected_email text,p_new_salt bytea,
    p_new_digest bytea,p_event text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    candidate_subject text;
    current_epoch bigint;
    token_epoch bigint;
    locked_challenge text;
    contact_email text;
    locked_contact_version bigint;
    issued_alias text;
    stamp timestamptz;
BEGIN -- PL/pgSQL function block; not migration transaction
    IF session_user<>'mamony_identity_recovery_redeemer_runtime' THEN
        RAISE EXCEPTION 'password recovery redeemer origin denied'
            USING ERRCODE='42501';
    END IF;
    IF p_digest IS NULL OR octet_length(p_digest)<>32
       OR p_expected_email IS NULL OR length(p_expected_email)<3
       OR length(p_expected_email)>254 OR p_expected_email<>lower(p_expected_email)
       OR p_new_salt IS NULL OR octet_length(p_new_salt)<>16
       OR p_new_digest IS NULL OR octet_length(p_new_digest)<>32
       OR p_event IS NULL OR p_event !~ '^evt_[A-Za-z0-9_-]{22}$'
    THEN RETURN false; END IF;

    -- Read the immutable subject pointer without locking a challenge row
    -- first. The DB role never exposes candidate_subject to Go or SQL
    -- callers; the subsequent locked challenge SELECT revalidates digest.
    SELECT c.subject_id INTO candidate_subject
      FROM mamony_identity.password_recovery_challenges c
     WHERE c.token_digest=p_digest;
    IF NOT FOUND THEN RETURN false; END IF;

    SELECT s.session_epoch,a.canonical_email,a.contact_version
      INTO current_epoch,contact_email,locked_contact_version
      FROM mamony_identity.subjects s
      JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
     WHERE s.subject_id=candidate_subject AND s.status='ACTIVE'
       AND a.verified_at IS NOT NULL AND a.contact_version=1
     FOR UPDATE OF s,a;
    IF NOT FOUND THEN RETURN false; END IF;

    SELECT c.challenge_id,c.subject_epoch,c.canonical_email
      INTO locked_challenge,token_epoch,issued_alias
      FROM mamony_identity.password_recovery_challenges c
     WHERE c.subject_id=candidate_subject AND c.token_digest=p_digest
       AND c.consumed_at IS NULL AND c.revoked_at IS NULL
       AND c.contact_version=locked_contact_version
     FOR UPDATE OF c;
    IF NOT FOUND OR token_epoch<>current_epoch
       OR issued_alias IS DISTINCT FROM contact_email
       OR p_expected_email IS DISTINCT FROM contact_email THEN
        RETURN false;
    END IF;
    stamp:=clock_timestamp();
    -- DB clock after row locks: expiration during contention is DENIED.
    IF NOT EXISTS (
        SELECT 1 FROM mamony_identity.password_recovery_challenges c
         WHERE c.token_digest=p_digest AND c.expires_at>stamp
    ) THEN RETURN false; END IF;

    -- No regular runtime can issue these grants. A migrated production
    -- system defaults to disabled until independent assurance is designed.
    PERFORM 1 FROM mamony_identity.password_recovery_assurance_grants g
      WHERE g.subject_id=candidate_subject AND g.subject_epoch=current_epoch
        AND g.challenge_id=locked_challenge
        AND g.consumed_at IS NULL AND g.expires_at>stamp
      FOR UPDATE OF g;
    IF NOT FOUND THEN RETURN false; END IF;

    PERFORM 1 FROM mamony_identity.password_verifiers v
      WHERE v.subject_id=candidate_subject FOR UPDATE OF v;
    IF NOT FOUND THEN RETURN false; END IF;

    UPDATE mamony_identity.password_recovery_challenges
       SET consumed_at=stamp
     WHERE token_digest=p_digest AND consumed_at IS NULL
       AND revoked_at IS NULL AND expires_at>stamp;
    IF NOT FOUND THEN RETURN false; END IF;
    UPDATE mamony_identity.password_recovery_assurance_grants
       SET consumed_at=stamp
     WHERE subject_id=candidate_subject AND consumed_at IS NULL
       AND subject_epoch=current_epoch AND challenge_id=locked_challenge
       AND expires_at>stamp;
    IF NOT FOUND THEN RETURN false; END IF;

    -- v14 trigger increments the subject session_epoch inside this same
    -- transaction, globally invalidating prior sessions/grants.
    UPDATE mamony_identity.password_verifiers
       SET kdf_version=1,memory_kib=65536,iterations=3,parallelism=1,
           salt=p_new_salt,digest=p_new_digest
     WHERE subject_id=candidate_subject;
    IF NOT FOUND THEN RETURN false; END IF;

    -- Prevent not-yet-claimed token mail from going out after redemption.
    UPDATE mamony_identity.password_recovery_deliveries
       SET status='DEAD',lease_id=NULL,leased_until=NULL,
           updated_at=stamp
     WHERE challenge_id=(SELECT c.challenge_id
         FROM mamony_identity.password_recovery_challenges c
         WHERE c.token_digest=p_digest)
       AND status='PENDING';

    -- One atomic event plus token-free recovery notice; a late event ID
    -- conflict or denied origin rolls back token consumption, grant, KDF
    -- rotation, epoch advance and the delivery queue transition together.
    INSERT INTO mamony_identity.security_outbox
        (event_id,subject_id,event_type,occurred_at)
    VALUES (p_event,candidate_subject,'PASSWORD_RECOVERED',stamp);
    RETURN true;
END;
$$;


-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (24, '__REPLACE_SHA256__');
