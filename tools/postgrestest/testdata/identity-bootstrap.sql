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
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mamony_identity_verifier_runtime') THEN
        CREATE ROLE mamony_identity_verifier_runtime LOGIN PASSWORD 'local-test-identity-verifier';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mamony_identity_authenticator_runtime') THEN
        CREATE ROLE mamony_identity_authenticator_runtime LOGIN PASSWORD 'local-test-identity-authenticator';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mamony_identity_auth_grant_runtime') THEN
        CREATE ROLE mamony_identity_auth_grant_runtime LOGIN PASSWORD 'local-test-identity-auth-grant';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mamony_identity_recovery_notice_runtime') THEN
        CREATE ROLE mamony_identity_recovery_notice_runtime LOGIN PASSWORD 'local-test-identity-recovery-notice';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mamony_identity_recovery_notice_webhook_runtime') THEN
        CREATE ROLE mamony_identity_recovery_notice_webhook_runtime LOGIN PASSWORD 'local-test-identity-recovery-notice-webhook';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mamony_identity_recovery_notice_monitor_runtime') THEN
        CREATE ROLE mamony_identity_recovery_notice_monitor_runtime LOGIN PASSWORD 'local-test-identity-recovery-notice-monitor';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mamony_identity_recovery_assurance_runtime') THEN
        CREATE ROLE mamony_identity_recovery_assurance_runtime LOGIN PASSWORD 'local-test-identity-recovery-assurance';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mamony_identity_recovery_redeemer_runtime') THEN
        CREATE ROLE mamony_identity_recovery_redeemer_runtime LOGIN PASSWORD 'local-test-identity-recovery-redeemer';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mamony_identity_recovery_delivery_runtime') THEN
        CREATE ROLE mamony_identity_recovery_delivery_runtime LOGIN PASSWORD 'local-test-identity-recovery-delivery';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mamony_identity_recovery_issuer_runtime') THEN
        CREATE ROLE mamony_identity_recovery_issuer_runtime LOGIN PASSWORD 'local-test-identity-recovery-issuer';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mamony_identity_password_change_runtime') THEN
        CREATE ROLE mamony_identity_password_change_runtime LOGIN PASSWORD 'local-test-identity-password-change';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mamony_identity_session_runtime') THEN
        CREATE ROLE mamony_identity_session_runtime LOGIN PASSWORD 'local-test-identity-session';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mamony_identity_notice_monitor_runtime') THEN
        CREATE ROLE mamony_identity_notice_monitor_runtime LOGIN PASSWORD 'local-test-identity-notice-monitor';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mamony_identity_notice_webhook_runtime') THEN
        CREATE ROLE mamony_identity_notice_webhook_runtime LOGIN PASSWORD 'local-test-identity-notice-webhook';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mamony_identity_security_notice_runtime') THEN
        CREATE ROLE mamony_identity_security_notice_runtime LOGIN PASSWORD 'local-test-identity-security-notice';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mamony_identity_delivery_runtime') THEN
        CREATE ROLE mamony_identity_delivery_runtime LOGIN PASSWORD 'local-test-identity-delivery';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mamony_identity_maintenance_runtime') THEN
        CREATE ROLE mamony_identity_maintenance_runtime LOGIN PASSWORD 'local-test-identity-maintenance';
    END IF;
END
$$;
ALTER ROLE mamony_identity_migrator LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD 'local-test-identity-migrator';
ALTER ROLE mamony_identity_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD 'local-test-identity-runtime';
ALTER ROLE mamony_identity_verifier_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD 'local-test-identity-verifier';
ALTER ROLE mamony_identity_authenticator_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD 'local-test-identity-authenticator';
ALTER ROLE mamony_identity_auth_grant_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD 'local-test-identity-auth-grant';
ALTER ROLE mamony_identity_recovery_notice_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD 'local-test-identity-recovery-notice';
ALTER ROLE mamony_identity_recovery_notice_webhook_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD 'local-test-identity-recovery-notice-webhook';
ALTER ROLE mamony_identity_recovery_notice_monitor_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD 'local-test-identity-recovery-notice-monitor';
ALTER ROLE mamony_identity_recovery_assurance_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD 'local-test-identity-recovery-assurance';
ALTER ROLE mamony_identity_recovery_redeemer_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD 'local-test-identity-recovery-redeemer';
ALTER ROLE mamony_identity_recovery_delivery_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD 'local-test-identity-recovery-delivery';
ALTER ROLE mamony_identity_recovery_issuer_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD 'local-test-identity-recovery-issuer';
ALTER ROLE mamony_identity_password_change_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD 'local-test-identity-password-change';
ALTER ROLE mamony_identity_session_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD 'local-test-identity-session';
ALTER ROLE mamony_identity_notice_monitor_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD 'local-test-identity-notice-monitor';
ALTER ROLE mamony_identity_notice_webhook_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD 'local-test-identity-notice-webhook';
ALTER ROLE mamony_identity_security_notice_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD 'local-test-identity-security-notice';
ALTER ROLE mamony_identity_delivery_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD 'local-test-identity-delivery';
ALTER ROLE mamony_identity_maintenance_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD 'local-test-identity-maintenance';
DROP SCHEMA IF EXISTS mamony_identity CASCADE;
CREATE SCHEMA mamony_identity AUTHORIZATION mamony_identity_migrator;
REVOKE ALL ON SCHEMA mamony_identity FROM PUBLIC;
GRANT USAGE ON SCHEMA mamony_identity TO mamony_identity_runtime;
