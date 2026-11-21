-- Mamony Go Identity Phase 1: pending registration authority only.
-- This forward-only SQL requires mamony_identity_migrator; never run at HTTP startup.
CREATE TABLE mamony_identity.migration_fingerprints (
    version bigint PRIMARY KEY CHECK (version > 0),
    sha256 text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    recorded_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE mamony_identity.subjects (
    subject_id text PRIMARY KEY CHECK (subject_id ~ '^sub_[A-Za-z0-9_-]{22}$'),
    status text NOT NULL CHECK (status = 'UNVERIFIED'),
    created_at timestamptz NOT NULL
);

CREATE TABLE mamony_identity.email_aliases (
    canonical_email text PRIMARY KEY CHECK (length(canonical_email) BETWEEN 3 AND 254 AND canonical_email = lower(canonical_email)),
    subject_id text NOT NULL UNIQUE REFERENCES mamony_identity.subjects(subject_id),
    contact_version bigint NOT NULL DEFAULT 1 CHECK (contact_version = 1),
    verified_at timestamptz NULL CHECK (verified_at IS NULL),
    created_at timestamptz NOT NULL
);

CREATE TABLE mamony_identity.password_verifiers (
    subject_id text PRIMARY KEY REFERENCES mamony_identity.subjects(subject_id),
    kdf_version smallint NOT NULL CHECK (kdf_version = 1),
    memory_kib integer NOT NULL CHECK (memory_kib = 65536),
    iterations integer NOT NULL CHECK (iterations = 3),
    parallelism smallint NOT NULL CHECK (parallelism = 1),
    salt bytea NOT NULL CHECK (octet_length(salt) = 16),
    digest bytea NOT NULL CHECK (octet_length(digest) = 32),
    created_at timestamptz NOT NULL
);

-- This is an identifier-only enrollment audit/event obligation, NOT a delivery
-- attempt, challenge token or promise that a user received a verification email.
CREATE TABLE mamony_identity.security_outbox (
    event_id text PRIMARY KEY CHECK (event_id ~ '^evt_[A-Za-z0-9_-]{22}$'),
    subject_id text NOT NULL REFERENCES mamony_identity.subjects(subject_id),
    event_type text NOT NULL CHECK (event_type = 'ENROLLMENT_PENDING'),
    occurred_at timestamptz NOT NULL
);

REVOKE ALL ON ALL TABLES IN SCHEMA mamony_identity FROM PUBLIC;
GRANT INSERT ON mamony_identity.subjects,
    mamony_identity.email_aliases,
    mamony_identity.password_verifiers,
    mamony_identity.security_outbox TO mamony_identity_runtime;

-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (1, 'd5dd2d2a68f3ad3fa01c6cf0ed8243f2b023e840b94cf8c09df215e26388f4e1');
