#!/usr/bin/env bash
# Disposable GitHub Actions fixture only. Run AFTER make integration has
# populated real v36 passkey state. Force-crashes ONLY the explicitly named
# PostgreSQL service container, then verifies persisted SQL invariants.
# NOT a backup restore, PITR, replica failover, or production test.
set -euo pipefail

fail() {
  echo "PASSKEY CRASH-RECOVERY FAIL: $*" >&2
  exit 1
}

expected_dsn='postgres://mamony_test_admin:local-test-admin@127.0.0.1:5432/mamony_test_ci?sslmode=disable'
[[ "$GITHUB_ACTIONS" == "true" ]] || fail "requires GitHub Actions disposable service"
[[ "$TEST_DATABASE_ADMIN_URL" == "$expected_dsn" ]] ||
  fail "admin DSN is not the exact disposable CI database"
[[ "$PG_SERVICE_CONTAINER_ID" =~ ^[[:xdigit:]]{12,64}$ ]] ||
  fail "missing GitHub Actions postgres service container ID"
command -v docker >/dev/null || fail "docker missing"
command -v psql >/dev/null || fail "psql missing"

container="$PG_SERVICE_CONTAINER_ID"
[[ "$(docker inspect --format '{{.Config.Image}}' "$container")" == "postgres:18.7" ]] ||
  fail "unexpected service container image"
docker inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$container" |
  grep -Fxq 'POSTGRES_DB=mamony_test_ci' ||
  fail "container database is not the disposable CI fixture"
docker inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$container" |
  grep -Fxq 'POSTGRES_USER=mamony_test_admin' ||
  fail "container database role is not the disposable CI fixture"
[[ "$(docker inspect --format '{{.State.Running}}' "$container")" == "true" ]] ||
  fail "PostgreSQL service container is not running"

psql_ci() {
  psql -X -q -A -t -v ON_ERROR_STOP=1 --dbname="$TEST_DATABASE_ADMIN_URL" "$@"
}

[[ "$(psql_ci -c 'SELECT current_database()')" == "mamony_test_ci" ]] ||
  fail "connected to the wrong database"
[[ "$(psql_ci -c "SELECT current_setting('fsync') || '|' || current_setting('synchronous_commit')")" == "on|on" ]] ||
  fail "durability settings differ from fsync=on, synchronous_commit=on"

# Check ALL staged/verified passkeys, activation challenges, activation notices,
# and possession audits. Counts force at least one real v36 activation and an
# exact linked audit+notice+consumed challenge for every verified credential.
snapshot() {
  psql_ci -f "$(dirname "$0")/passkey-recovery-snapshot.sql"
}

before="$(snapshot)"
IFS='|' read -r verified linked notices events digest <<<"$before"
[[ "$verified" =~ ^[0-9]+$ && "$linked" =~ ^[0-9]+$ &&
   "$notices" =~ ^[0-9]+$ && "$events" =~ ^[0-9]+$ &&
   "$digest" =~ ^[0-9a-f]{32}$ ]] ||
  fail "malformed pre-crash SQL snapshot"
(( verified >= 1 )) || fail "no committed passkey possession fixtures"
[[ "$verified" == "$linked" && "$verified" == "$notices" &&
   "$verified" == "$events" ]] ||
  fail "pre-crash verified credentials lack a unique linked audit, notice or consumed challenge"

echo "Passkey pre-crash snapshot: verified=$verified, audit=$events, notices=$notices"
# SIGKILL prevents graceful shutdown. Restart SAME container / same data volume.
docker kill --signal=KILL "$container" >/dev/null
if [[ "$(docker inspect --format '{{.State.Running}}' "$container")" != "true" ]]; then
  docker start "$container" >/dev/null
fi

ready=false
for attempt in $(seq 1 60); do
  if psql_ci -c "SELECT 1" >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 1
done
[[ "$ready" == "true" ]] || fail "PostgreSQL was not ready after SIGKILL and restart"
[[ "$(psql_ci -c 'SELECT current_database()')" == "mamony_test_ci" ]] ||
  fail "database identity changed after restart"

after="$(snapshot)"
[[ "$after" == "$before" ]] ||
  fail "passkey credential, challenge, audit or notice state changed across PostgreSQL SIGKILL/restart"

echo "PASSKEY CRASH-RECOVERY PASS: real v36 commits, consumed challenges, and pending notices survived PostgreSQL SIGKILL/restart without partial state"
