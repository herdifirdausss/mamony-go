-- Identity v5: privacy-preserving, distributed anonymous request admission.
-- These counters are keyed by external HMAC, never raw email addresses or IPs.
-- No public Identity routes or email delivery are enabled by this migration.
CREATE TABLE mamony_identity.anonymous_admission_buckets (
    action text NOT NULL CHECK (action IN ('REGISTER', 'RESEND')),
    scope text NOT NULL CHECK (scope IN ('CLIENT_MINUTE', 'CLIENT_HOUR', 'EMAIL_HOUR', 'EMAIL_DAY')),
    key_digest bytea NOT NULL CHECK (octet_length(key_digest) = 32),
    window_start timestamptz NOT NULL,
    hits smallint NOT NULL CHECK (hits BETWEEN 1 AND 80),
    PRIMARY KEY (action, scope, key_digest, window_start)
);

CREATE INDEX anonymous_admission_buckets_cleanup
    ON mamony_identity.anonymous_admission_buckets (window_start);

REVOKE ALL ON mamony_identity.anonymous_admission_buckets FROM PUBLIC;
-- The registration runtime is trusted for bounded pre-registration admission;
-- it cannot delete counters, create tables or mutate subject verification.
GRANT SELECT ON mamony_identity.anonymous_admission_buckets
    TO mamony_identity_runtime;
GRANT INSERT (action, scope, key_digest, window_start, hits)
    ON mamony_identity.anonymous_admission_buckets TO mamony_identity_runtime;
GRANT UPDATE (hits)
    ON mamony_identity.anonymous_admission_buckets TO mamony_identity_runtime;

-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (5, '5bee9742ad52d937f3e6dc450f1816eabc580bd69961d816bc95f77063cea2d7');
