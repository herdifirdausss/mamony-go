-- Identity v21: internal-only, separate-password-recovery redemption.
-- NO HTTP route, user-facing page, live worker, assurance-approval issuer
-- or production authorization to enroll a recovery grant is introduced.
-- Privileged fixture approval is deliberately NOT a replacement for MFA.
ALTER TABLE mamony_identity.security_outbox
    DROP CONSTRAINT security_outbox_event_type_check;
ALTER TABLE mamony_identity.security_outbox
    ADD CONSTRAINT security_outbox_event_type_check
    CHECK (event_type IN ('ENROLLMENT_PENDING','EMAIL_PROOF_ISSUED',
        'CONTACT_VERIFIED','SESSION_CREATED','SESSION_REVOKED',
        'PASSWORD_CHANGED','PASSWORD_RECOVERED'));

-- Recovering with only a compromised inbox must NOT bypass stronger
-- authenticators. This table is default-DENY: no ordinary runtime role
-- can INSERT/UPDATE its grants, and no live proofing/approval path exists.
-- A future independently approved step-up/assurance flow must be designed.
CREATE TABLE mamony_identity.password_recovery_assurance_grants (
    subject_id text PRIMARY KEY REFERENCES mamony_identity.subjects(subject_id),
    subject_epoch bigint NOT NULL CHECK (subject_epoch>0),
    approved_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz NULL,
    CHECK (expires_at>approved_at AND
           expires_at<=approved_at+interval '10 minutes'),
    CHECK (consumed_at IS NULL OR consumed_at>=approved_at)
);
REVOKE ALL ON mamony_identity.password_recovery_assurance_grants FROM PUBLIC;

-- Token-free immutable notification obligation distinct from v16's
-- PASSWORD_CHANGED mail and separate from recovery bearer delivery.
CREATE TABLE mamony_identity.password_recovery_notices (
    event_id text PRIMARY KEY REFERENCES mamony_identity.security_outbox(event_id),
    subject_id text NOT NULL REFERENCES mamony_identity.subjects(subject_id),
    recipient text NOT NULL CHECK (length(recipient) BETWEEN 3 AND 254
        AND recipient=lower(recipient)),
    occurred_at timestamptz NOT NULL,
    status text NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING','ACCEPTED','DEAD')),
    attempts smallint NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 5),
    next_attempt_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    lease_id text NULL CHECK (lease_id IS NULL OR lease_id ~ '^lse_[A-Za-z0-9_-]{22}$'),
    leased_until timestamptz NULL,
    accepted_at timestamptz NULL,
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CHECK ((lease_id IS NULL)=(leased_until IS NULL)),
    CHECK (accepted_at IS NULL OR status='ACCEPTED')
);
CREATE INDEX password_recovery_notices_due
    ON mamony_identity.password_recovery_notices(next_attempt_at,event_id)
    WHERE status='PENDING';
REVOKE ALL ON mamony_identity.password_recovery_notices FROM PUBLIC;

-- A different notification trigger is necessary: v16 intentionally
-- allows PASSWORD_CHANGED only from the password-change runtime.
-- A forged PASSWORD_RECOVERED event by any other role must roll back.
CREATE FUNCTION mamony_identity.enqueue_password_recovery_notice()
RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE recipient_email text;
BEGIN -- PL/pgSQL function block; not migration transaction
    IF NEW.event_type<>'PASSWORD_RECOVERED' THEN RETURN NEW; END IF;
    IF session_user<>'mamony_identity_recovery_redeemer_runtime' THEN
        RAISE EXCEPTION 'recovery notice origin denied' USING ERRCODE='42501';
    END IF;
    SELECT a.canonical_email INTO recipient_email
      FROM mamony_identity.email_aliases a
      JOIN mamony_identity.subjects s ON s.subject_id=a.subject_id
     WHERE s.subject_id=NEW.subject_id AND s.status='ACTIVE'
       AND a.verified_at IS NOT NULL AND a.contact_version=1;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'recovery event missing verified recipient'
            USING ERRCODE='23514';
    END IF;
    INSERT INTO mamony_identity.password_recovery_notices
        (event_id,subject_id,recipient,occurred_at)
    VALUES (NEW.event_id,NEW.subject_id,recipient_email,NEW.occurred_at);
    RETURN NEW;
END;
$$;
REVOKE ALL ON FUNCTION mamony_identity.enqueue_password_recovery_notice()
FROM PUBLIC;
CREATE TRIGGER security_outbox_password_recovered_notice
AFTER INSERT ON mamony_identity.security_outbox FOR EACH ROW
EXECUTE FUNCTION mamony_identity.enqueue_password_recovery_notice();

-- The credential runtime has NO direct access to challenge, assurance,
-- verifier, session, alias or notice tables. Its only authority is EXECUTE.
-- Lock order is subject/alias -> challenge -> assurance -> verifier, which
-- matches issuer's subject -> challenge order and avoids their deadlock.
CREATE FUNCTION mamony_identity.redeem_password_recovery(
    p_digest bytea,p_new_salt bytea,p_new_digest bytea,p_event text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,pg_temp AS $$
DECLARE
    candidate_subject text;
    current_epoch bigint;
    token_epoch bigint;
    contact_email text;
    contact_version bigint;
    issued_alias text;
    stamp timestamptz;
BEGIN -- PL/pgSQL function block; not migration transaction
    IF session_user<>'mamony_identity_recovery_redeemer_runtime' THEN
        RAISE EXCEPTION 'password recovery redeemer origin denied'
            USING ERRCODE='42501';
    END IF;
    IF p_digest IS NULL OR octet_length(p_digest)<>32
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
      INTO current_epoch,contact_email,contact_version
      FROM mamony_identity.subjects s
      JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
     WHERE s.subject_id=candidate_subject AND s.status='ACTIVE'
       AND a.verified_at IS NOT NULL AND a.contact_version=1
     FOR UPDATE OF s,a;
    IF NOT FOUND THEN RETURN false; END IF;

    SELECT c.subject_epoch,c.canonical_email
      INTO token_epoch,issued_alias
      FROM mamony_identity.password_recovery_challenges c
     WHERE c.subject_id=candidate_subject AND c.token_digest=p_digest
       AND c.consumed_at IS NULL AND c.revoked_at IS NULL
       AND c.contact_version=contact_version
     FOR UPDATE OF c;
    IF NOT FOUND OR token_epoch<>current_epoch
       OR issued_alias IS DISTINCT FROM contact_email THEN
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
       AND subject_epoch=current_epoch AND expires_at>stamp;
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
REVOKE ALL ON FUNCTION mamony_identity.redeem_password_recovery(
    bytea,bytea,bytea,text) FROM PUBLIC;
GRANT USAGE ON SCHEMA mamony_identity
    TO mamony_identity_recovery_redeemer_runtime;
GRANT EXECUTE ON FUNCTION mamony_identity.redeem_password_recovery(
    bytea,bytea,bytea,text)
    TO mamony_identity_recovery_redeemer_runtime;

-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (21, '__REPLACE_SHA256__');
