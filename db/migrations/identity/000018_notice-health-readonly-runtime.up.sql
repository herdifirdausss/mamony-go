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
INSERT INTO mamony_identity.migration_fingerprints (version, sha256) VALUES (18, '9eae74a5ebafdef85c8f42e2cd8b150f1453249e5cd44bb285576f6829afba3b');
