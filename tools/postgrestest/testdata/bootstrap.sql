-- Disposable integration fixture ONLY: never apply to a real schema.
-- The invoking role must be the dedicated mamony_test_admin superuser.
\set ON_ERROR_STOP on
BEGIN;
DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'mamony_test_owner') THEN
    CREATE ROLE mamony_test_owner LOGIN PASSWORD 'local-test-owner';
  END IF;
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'mamony_test_runtime') THEN
    CREATE ROLE mamony_test_runtime LOGIN PASSWORD 'local-test-runtime';
  END IF;
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'mamony_test_elevated_runtime') THEN
    CREATE ROLE mamony_test_elevated_runtime LOGIN PASSWORD 'local-test-elevated';
  END IF;
END
$$;
ALTER ROLE mamony_test_owner LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD 'local-test-owner';
ALTER ROLE mamony_test_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD 'local-test-runtime';
-- Deliberately privileged negative fixture: expected runtime pool refusal.
-- Never grant this account schema privileges or expose it outside tests.
ALTER ROLE mamony_test_elevated_runtime LOGIN NOSUPERUSER CREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD 'local-test-elevated';

DROP SCHEMA IF EXISTS mamony_test_fixture CASCADE;
CREATE SCHEMA mamony_test_fixture AUTHORIZATION mamony_test_owner;
SET ROLE mamony_test_owner;
CREATE TABLE mamony_test_fixture.records (
  id integer PRIMARY KEY,
  tenant_id text NOT NULL,
  note text NOT NULL
);
ALTER TABLE mamony_test_fixture.records ENABLE ROW LEVEL SECURITY;
ALTER TABLE mamony_test_fixture.records FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_records ON mamony_test_fixture.records
  FOR ALL TO mamony_test_owner, mamony_test_runtime
  USING (tenant_id = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
RESET ROLE;

-- Data setup is deliberately performed by the disposable bootstrap admin;
-- application runtime and the table owner remain subject to FORCE RLS.
INSERT INTO mamony_test_fixture.records (id, tenant_id, note)
  VALUES (1, 'tenant_a', 'fixture-a'), (2, 'tenant_b', 'fixture-b');
GRANT USAGE ON SCHEMA mamony_test_fixture TO mamony_test_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON mamony_test_fixture.records TO mamony_test_runtime;
COMMIT;
