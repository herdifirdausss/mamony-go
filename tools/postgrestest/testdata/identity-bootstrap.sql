-- Disposable PostgreSQL fixture only. The testgate enforces loopback and
-- the mamony_test_ database name BEFORE invoking this file.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mamony_identity_migrator') THEN
        CREATE ROLE mamony_identity_migrator LOGIN PASSWORD 'local-test-identity-migrator';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mamony_identity_runtime') THEN
        CREATE ROLE mamony_identity_runtime LOGIN PASSWORD 'local-test-identity-runtime';
    END IF;
END
$$;
ALTER ROLE mamony_identity_migrator LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD 'local-test-identity-migrator';
ALTER ROLE mamony_identity_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD 'local-test-identity-runtime';
DROP SCHEMA IF EXISTS mamony_identity CASCADE;
CREATE SCHEMA mamony_identity AUTHORIZATION mamony_identity_migrator;
REVOKE ALL ON SCHEMA mamony_identity FROM PUBLIC;
GRANT USAGE ON SCHEMA mamony_identity TO mamony_identity_runtime;
