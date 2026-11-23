#!/bin/sh
# Run only on a disposable local Docker container. Requires docker and psql.
set -eu

if ! command -v docker >/dev/null 2>&1; then
  echo 'NOT RUN: docker missing' >&2
  exit 2
fi
if ! command -v psql >/dev/null 2>&1; then
  echo 'NOT RUN: PostgreSQL client (psql) missing' >&2
  exit 2
fi

container="$(docker run --rm -d -e POSTGRES_DB=mamony_test_local \
  -e POSTGRES_USER=mamony_test_admin -e POSTGRES_PASSWORD=local-test-admin \
  -p 127.0.0.1::5432 postgres:18.7)"
cleanup() {
  docker rm -f "$container" >/dev/null 2>&1 || true
}
trap cleanup EXIT HUP INT TERM

port="$(docker port "$container" 5432/tcp | sed -n '1s/.*://p')"
case "$port" in
  ''|*[!0-9]*) echo 'Cannot identify the ephemeral loopback port' >&2; exit 2 ;;
esac

attempt=0
until docker exec "$container" pg_isready -U mamony_test_admin -d mamony_test_local >/dev/null 2>&1; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 30 ]; then
    echo 'NOT RUN: disposable PostgreSQL did not become ready' >&2
    exit 2
  fi
  sleep 1
done

export TEST_DATABASE_ADMIN_URL="postgres://mamony_test_admin:local-test-admin@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_DATABASE_URL="postgres://mamony_test_runtime:local-test-runtime@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_PLATFORM_MIGRATION_URL="postgres://mamony_platform_migrator:local-test-platform-migrator@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_PLATFORM_RUNTIME_URL="postgres://mamony_platform_runtime:local-test-platform-runtime@127.0.0.1:$port/mamony_test_local?sslmode=disable"

export TEST_IDENTITY_MIGRATION_URL="postgres://mamony_identity_migrator:local-test-identity-migrator@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_IDENTITY_RUNTIME_URL="postgres://mamony_identity_runtime:local-test-identity-runtime@127.0.0.1:$port/mamony_test_local?sslmode=disable"

export TEST_IDENTITY_VERIFIER_URL="postgres://mamony_identity_verifier_runtime:local-test-identity-verifier@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_IDENTITY_AUTHENTICATOR_URL="postgres://mamony_identity_authenticator_runtime:local-test-identity-authenticator@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_IDENTITY_AUTH_GRANT_URL="postgres://mamony_identity_auth_grant_runtime:local-test-identity-auth-grant@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_IDENTITY_RECOVERY_NOTICE_URL="postgres://mamony_identity_recovery_notice_runtime:local-test-identity-recovery-notice@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_IDENTITY_RECOVERY_NOTICE_WEBHOOK_URL="postgres://mamony_identity_recovery_notice_webhook_runtime:local-test-identity-recovery-notice-webhook@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_IDENTITY_RECOVERY_NOTICE_MONITOR_URL="postgres://mamony_identity_recovery_notice_monitor_runtime:local-test-identity-recovery-notice-monitor@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_IDENTITY_RECOVERY_CODE_NOTICE_WEBHOOK_URL="postgres://mamony_identity_recovery_code_notice_webhook_runtime:local-test-identity-recovery-code-notice-webhook@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_IDENTITY_RECOVERY_CODE_NOTICE_URL="postgres://mamony_identity_recovery_code_notice_runtime:local-test-identity-recovery-code-notice@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_IDENTITY_RECOVERY_CODE_NOTICE_MONITOR_URL="postgres://mamony_identity_recovery_code_notice_monitor_runtime:local-test-identity-recovery-code-notice-monitor@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_IDENTITY_RECOVERY_FACTOR_ISSUER_URL="postgres://mamony_identity_recovery_factor_issuer_runtime:local-test-recovery-factor-issuer@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_IDENTITY_RECOVERY_FACTOR_VERIFIER_URL="postgres://mamony_identity_recovery_factor_verifier_runtime:local-test-recovery-factor-verifier@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_IDENTITY_PASSKEY_REGISTRATION_ISSUER_URL="postgres://mamony_identity_passkey_registration_issuer_runtime:local-test-passkey-registration-issuer@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_IDENTITY_PASSKEY_REGISTRATION_READER_URL="postgres://mamony_identity_passkey_registration_reader_runtime:local-test-passkey-registration-reader@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_IDENTITY_RECOVERY_CODE_MANAGER_URL="postgres://mamony_identity_recovery_code_manager_runtime:local-test-identity-recovery-code-manager@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_IDENTITY_RECOVERY_ASSURANCE_URL="postgres://mamony_identity_recovery_assurance_runtime:local-test-identity-recovery-assurance@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_IDENTITY_RECOVERY_REDEEMER_URL="postgres://mamony_identity_recovery_redeemer_runtime:local-test-identity-recovery-redeemer@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_IDENTITY_RECOVERY_DELIVERY_URL="postgres://mamony_identity_recovery_delivery_runtime:local-test-identity-recovery-delivery@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_IDENTITY_RECOVERY_ISSUER_URL="postgres://mamony_identity_recovery_issuer_runtime:local-test-identity-recovery-issuer@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_IDENTITY_PASSWORD_CHANGE_URL="postgres://mamony_identity_password_change_runtime:local-test-identity-password-change@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_IDENTITY_SESSION_URL="postgres://mamony_identity_session_runtime:local-test-identity-session@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_IDENTITY_NOTICE_MONITOR_URL="postgres://mamony_identity_notice_monitor_runtime:local-test-identity-notice-monitor@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_IDENTITY_NOTICE_WEBHOOK_URL="postgres://mamony_identity_notice_webhook_runtime:local-test-identity-notice-webhook@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_IDENTITY_SECURITY_NOTICE_URL="postgres://mamony_identity_security_notice_runtime:local-test-identity-security-notice@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_IDENTITY_DELIVERY_URL="postgres://mamony_identity_delivery_runtime:local-test-identity-delivery@127.0.0.1:$port/mamony_test_local?sslmode=disable"
export TEST_IDENTITY_MAINTENANCE_URL="postgres://mamony_identity_maintenance_runtime:local-test-identity-maintenance@127.0.0.1:$port/mamony_test_local?sslmode=disable"

make integration
