-- Identity v18: least-privilege read-only security notice health observer.
-- Monitoring SQL credentials must never inherit maintenance PURGE privileges.
-- Database role must be provisioned independently by trusted infrastructure.
GRANT USAGE ON SCHEMA mamony_identity
    TO mamony_identity_notice_monitor_runtime;
GRANT EXECUTE ON FUNCTION mamony_identity.security_notice_health()
    TO mamony_identity_notice_monitor_runtime;
REVOKE ALL ON FUNCTION mamony_identity.purge_old_mailgun_security_receipts()
    FROM mamony_identity_notice_monitor_runtime;
REVOKE ALL ON FUNCTION mamony_identity.purge_old_password_change_notices()
    FROM mamony_identity_notice_monitor_runtime;
-- mamony:integrity:v1
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (18, '84c6dfce94cdc5dbc625359a44f1eb900825a994822e8031c7c18d5ae6e2fd83');
