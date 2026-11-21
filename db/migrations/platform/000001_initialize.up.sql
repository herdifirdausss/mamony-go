-- Mamony Go platform foundation. This file is forward-only.
-- Only the dedicated platform migration role may execute it.
CREATE TABLE mamony_platform.migration_fingerprints (
    version bigint PRIMARY KEY CHECK (version > 0),
    sha256 text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    recorded_at timestamptz NOT NULL DEFAULT now()
);
REVOKE ALL ON mamony_platform.migration_fingerprints FROM PUBLIC;

-- mamony:integrity:v1
INSERT INTO mamony_platform.migration_fingerprints (version, sha256) VALUES (1, '6a9be3034e4f5834951341ac30f4e1f6d2a6f2528b933f74817d5831cd1fb810');
