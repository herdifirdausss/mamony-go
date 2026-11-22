-- v14: credential/contact updates invalidate all previous session and grant epochs.
-- Existing registration INSERT is not a credential rotation. No public mutation
-- route, password reset, email change or new SQL runtime role is introduced.
CREATE FUNCTION mamony_identity.bump_epoch_on_auth_material()
RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER
SET search_path = pg_catalog, pg_temp
AS $$
BEGIN -- PL/pgSQL function block; not a migration transaction
    IF TG_OP = 'UPDATE' THEN
        IF NEW.subject_id IS DISTINCT FROM OLD.subject_id THEN
            RAISE EXCEPTION 'identity auth material cannot change owner'
                USING ERRCODE='23514';
        END IF;
    END IF;
    UPDATE mamony_identity.subjects SET session_epoch=session_epoch+1
      WHERE subject_id=OLD.subject_id;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'auth material subject not found' USING ERRCODE='23503';
    END IF;
    IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END;
$$;
REVOKE ALL ON FUNCTION mamony_identity.bump_epoch_on_auth_material() FROM PUBLIC;
CREATE TRIGGER password_verifier_changed_bumps_epoch
AFTER UPDATE OF subject_id,kdf_version,memory_kib,iterations,parallelism,salt,digest
ON mamony_identity.password_verifiers
FOR EACH ROW WHEN (
  ROW(OLD.subject_id,OLD.kdf_version,OLD.memory_kib,OLD.iterations,OLD.parallelism,OLD.salt,OLD.digest)
  IS DISTINCT FROM
  ROW(NEW.subject_id,NEW.kdf_version,NEW.memory_kib,NEW.iterations,NEW.parallelism,NEW.salt,NEW.digest))
EXECUTE FUNCTION mamony_identity.bump_epoch_on_auth_material();
CREATE TRIGGER password_verifier_removed_bumps_epoch
AFTER DELETE ON mamony_identity.password_verifiers FOR EACH ROW
EXECUTE FUNCTION mamony_identity.bump_epoch_on_auth_material();
CREATE TRIGGER email_alias_changed_bumps_epoch
AFTER UPDATE OF canonical_email,verified_at,contact_version,subject_id
ON mamony_identity.email_aliases
FOR EACH ROW WHEN (
  ROW(OLD.canonical_email,OLD.verified_at,OLD.contact_version,OLD.subject_id)
  IS DISTINCT FROM
  ROW(NEW.canonical_email,NEW.verified_at,NEW.contact_version,NEW.subject_id))
EXECUTE FUNCTION mamony_identity.bump_epoch_on_auth_material();
CREATE TRIGGER email_alias_removed_bumps_epoch
AFTER DELETE ON mamony_identity.email_aliases FOR EACH ROW
EXECUTE FUNCTION mamony_identity.bump_epoch_on_auth_material();

-- Read the subject epoch in the SAME SELECT snapshot as the verifier, before
-- Argon2id. A stale successful comparison must NOT stage a new-epoch grant.
GRANT SELECT (session_epoch) ON mamony_identity.subjects
  TO mamony_identity_authenticator_runtime;
DROP FUNCTION mamony_identity.stage_password_auth_grant(text,bytea);
CREATE FUNCTION mamony_identity.stage_password_auth_grant(
  p_subject text, p_expected_epoch bigint, p_digest bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path = pg_catalog, pg_temp
AS $$
DECLARE
    current_epoch bigint;
    stamp timestamptz;
BEGIN -- PL/pgSQL function block; not a migration transaction
    IF p_subject IS NULL OR p_subject !~ '^sub_[A-Za-z0-9_-]{22}$'
       OR p_expected_epoch IS NULL OR p_expected_epoch <= 0
       OR p_digest IS NULL OR octet_length(p_digest) <> 32 THEN
        RETURN false;
    END IF;
    SELECT s.session_epoch INTO current_epoch
      FROM mamony_identity.subjects s
      JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
     WHERE s.subject_id=p_subject AND s.status='ACTIVE'
       AND a.verified_at IS NOT NULL AND a.contact_version=1
       AND s.session_epoch=p_expected_epoch
     FOR UPDATE OF s,a;
    IF NOT FOUND THEN RETURN false; END IF;
    stamp:=clock_timestamp();
    INSERT INTO mamony_identity.password_login_grants
      (grant_digest,subject_id,subject_epoch,issued_at,expires_at)
    VALUES (p_digest,p_subject,current_epoch,stamp,stamp+interval '30 seconds');
    RETURN true;
END;
$$;
REVOKE ALL ON FUNCTION mamony_identity.stage_password_auth_grant(text,bigint,bytea)
  FROM PUBLIC;
GRANT EXECUTE ON FUNCTION mamony_identity.stage_password_auth_grant(text,bigint,bytea)
  TO mamony_identity_auth_grant_runtime;
-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (14, 'a99c08c12f3cc9db665a83161cdcaab7ed6ceb54b6e63d2af1904aa352fb2c66');
