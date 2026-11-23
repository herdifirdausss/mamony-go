-- Identity delivery v3: encrypted retry envelopes, never plaintext bearer tokens.
-- The encryption key and the email provider credentials live outside PostgreSQL.
-- This migration is forward-only and does NOT activate a public HTTP route.
CREATE TABLE mamony_identity.email_proof_deliveries (
    challenge_id text PRIMARY KEY REFERENCES mamony_identity.email_verification_challenges(challenge_id),
    key_id text NOT NULL CHECK (key_id ~ '^[A-Za-z0-9_-]{1,32}$'),
    sealed_token bytea NOT NULL CHECK (octet_length(sealed_token) BETWEEN 60 AND 256),
    status text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'ACCEPTED', 'DEAD')),
    attempts smallint NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 5),
    next_attempt_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    lease_id text NULL CHECK (lease_id IS NULL OR lease_id ~ '^lse_[A-Za-z0-9_-]{22}$'),
    leased_until timestamptz NULL,
    accepted_at timestamptz NULL,
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CHECK ((lease_id IS NULL) = (leased_until IS NULL)),
    CHECK (accepted_at IS NULL OR status = 'ACCEPTED')
);

CREATE INDEX email_proof_deliveries_due ON mamony_identity.email_proof_deliveries(next_attempt_at, challenge_id)
    WHERE status = 'PENDING';

REVOKE ALL ON mamony_identity.email_proof_deliveries FROM PUBLIC;
-- Only the verifier can create ciphertext jobs alongside issuing a digest.
GRANT INSERT (challenge_id, key_id, sealed_token)
    ON mamony_identity.email_proof_deliveries TO mamony_identity_verifier_runtime;

-- The worker cannot read password verifiers, change subject/contact state,
-- issue a new challenge, delete an audit event or access migration history.
GRANT USAGE ON SCHEMA mamony_identity TO mamony_identity_delivery_runtime;
GRANT SELECT (subject_id, status) ON mamony_identity.subjects
    TO mamony_identity_delivery_runtime;
GRANT SELECT (subject_id, canonical_email, contact_version, verified_at)
    ON mamony_identity.email_aliases TO mamony_identity_delivery_runtime;
GRANT SELECT (challenge_id, subject_id, canonical_email, contact_version,
              expires_at, consumed_at, revoked_at)
    ON mamony_identity.email_verification_challenges TO mamony_identity_delivery_runtime;
GRANT SELECT ON mamony_identity.email_proof_deliveries
    TO mamony_identity_delivery_runtime;
GRANT UPDATE (status, attempts, next_attempt_at, lease_id, leased_until,
              accepted_at, updated_at)
    ON mamony_identity.email_proof_deliveries TO mamony_identity_delivery_runtime;

-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (3, '0b1c6db20df6ec99d8965eb892055d257716c99b8e9a5a2d72f9fce09f575efa');
