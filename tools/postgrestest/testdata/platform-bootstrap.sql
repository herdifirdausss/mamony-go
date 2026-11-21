-- Synthetic-only provisioning; safe only behind the testgate loopback checks.
-- Never run in a real application database.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mamony_platform_migrator') THEN
        CREATE ROLE mamony_platform_migrator LOGIN PASSWORD 'local-test-platform-migrator';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mamony_platform_runtime') THEN
        CREATE ROLE mamony_platform_runtime LOGIN PASSWORD 'local-test-platform-runtime';
    END IF;
END
$$;
ALTER ROLE mamony_platform_migrator LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD 'local-test-platform-migrator';
ALTER ROLE mamony_platform_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD 'local-test-platform-runtime';
DROP SCHEMA IF EXISTS mamony_platform CASCADE;
CREATE SCHEMA mamony_platform AUTHORIZATION mamony_platform_migrator;
REVOKE ALL ON SCHEMA mamony_platform FROM PUBLIC;
GRANT USAGE ON SCHEMA mamony_platform TO mamony_platform_runtime;
