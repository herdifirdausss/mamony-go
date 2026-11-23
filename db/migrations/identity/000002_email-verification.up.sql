-- Mamony Go Identity Phase 1: verified contact under a distinct restricted runtime.
-- v1 is immutable. v2 authorizes state transitions only through the verifier usecase.
ALTER TABLE mamony_identity.subjects DROP CONSTRAINT subjects_status_check;
ALTER TABLE mamony_identity.subjects ADD CONSTRAINT subjects_status_check
    CHECK (status IN ('UNVERIFIED', 'ACTIVE', 'SUSPENDED', 'FROZEN', 'DELETED'));
ALTER TABLE mamony_identity.subjects ALTER COLUMN status SET DEFAULT 'UNVERIFIED';

ALTER TABLE mamony_identity.email_aliases DROP CONSTRAINT email_aliases_verified_at_check;

ALTER TABLE mamony_identity.security_outbox DROP CONSTRAINT security_outbox_event_type_check;
ALTER TABLE mamony_identity.security_outbox ADD CONSTRAINT security_outbox_event_type_check
    CHECK (event_type IN ('ENROLLMENT_PENDING', 'EMAIL_PROOF_ISSUED', 'CONTACT_VERIFIED'));

CREATE TABLE mamony_identity.email_verification_challenges (
    challenge_id text PRIMARY KEY CHECK (challenge_id ~ '^vch_[A-Za-z0-9_-]{22}$'),
    subject_id text NOT NULL REFERENCES mamony_identity.subjects(subject_id),
    canonical_email text NOT NULL REFERENCES mamony_identity.email_aliases(canonical_email),
    contact_version bigint NOT NULL CHECK (contact_version = 1),
    purpose text NOT NULL CHECK (purpose = 'EMAIL_VERIFY'),
    token_digest bytea NOT NULL UNIQUE CHECK (octet_length(token_digest) = 32),
    requested_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz NULL,
    revoked_at timestamptz NULL,
    CHECK (expires_at > requested_at),
    CHECK (consumed_at IS NULL OR revoked_at IS NULL)
);
CREATE UNIQUE INDEX email_verify_one_live_per_subject
    ON mamony_identity.email_verification_challenges(subject_id, purpose)
    WHERE consumed_at IS NULL AND revoked_at IS NULL;

-- Registration runtime had full-table INSERT in v1. Revoke it BEFORE
-- allowing ACTIVE/verified states; grant only fields that cannot bypass
-- the verifier and rely on server-set defaults for protected state.
ALTER TABLE mamony_identity.security_outbox
    ALTER COLUMN event_type SET DEFAULT 'ENROLLMENT_PENDING';
REVOKE INSERT ON mamony_identity.subjects, mamony_identity.email_aliases,
    mamony_identity.security_outbox FROM mamony_identity_runtime;
GRANT INSERT (subject_id, created_at) ON mamony_identity.subjects
    TO mamony_identity_runtime;
GRANT INSERT (canonical_email, subject_id, created_at)
    ON mamony_identity.email_aliases TO mamony_identity_runtime;
GRANT INSERT (event_id, subject_id, occurred_at)
    ON mamony_identity.security_outbox TO mamony_identity_runtime;

REVOKE ALL ON mamony_identity.email_verification_challenges FROM PUBLIC;

-- The verifier has a separate bounded credential; it cannot read password
-- verifier rows or migration history, create tables, or delete security events.
GRANT USAGE ON SCHEMA mamony_identity TO mamony_identity_verifier_runtime;
GRANT SELECT ON mamony_identity.subjects, mamony_identity.email_aliases,
    mamony_identity.email_verification_challenges TO mamony_identity_verifier_runtime;
GRANT UPDATE (status) ON mamony_identity.subjects TO mamony_identity_verifier_runtime;
GRANT UPDATE (verified_at) ON mamony_identity.email_aliases TO mamony_identity_verifier_runtime;
GRANT INSERT ON mamony_identity.email_verification_challenges,
    mamony_identity.security_outbox TO mamony_identity_verifier_runtime;
GRANT UPDATE (consumed_at, revoked_at) ON mamony_identity.email_verification_challenges
    TO mamony_identity_verifier_runtime;

-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (2, '06f2ca92610637a029250e759b8d2ec0231612e13e30f954097aba9a6ffc73d1');
