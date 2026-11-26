#!/usr/bin/env bash
# CI-only: logical dump of the already-verified v36 fixture, restored into a
# SECOND isolated PostgreSQL 18.7 container/database. Compares full credential,
# challenge, audit and notice snapshots; not PITR, failover or rollback testing.
set -euo pipefail

fail() {
  echo "PASSKEY BACKUP-RESTORE FAIL: $*" >&2
  exit 1
}

expected_dsn='postgres://mamony_test_admin:local-test-admin@127.0.0.1:5432/mamony_test_ci?sslmode=disable'
[[ "$GITHUB_ACTIONS" == "true" ]] || fail "GitHub Actions disposable service required"
[[ "$TEST_DATABASE_ADMIN_URL" == "$expected_dsn" ]] ||
  fail "source is not the exact disposable CI database"
[[ "$PG_SERVICE_CONTAINER_ID" =~ ^[[:xdigit:]]{12,64}$ ]] ||
  fail "invalid source service-container ID"
command -v docker >/dev/null || fail "docker missing"
command -v psql >/dev/null || fail "psql missing"

source_id="$PG_SERVICE_CONTAINER_ID"
[[ "$(docker inspect --format '{{.Config.Image}}' "$source_id")" == 'postgres:18.7' ]] ||
  fail "unexpected source PostgreSQL image"
[[ "$(docker inspect --format '{{.State.Running}}' "$source_id")" == 'true' ]] ||
  fail "source PostgreSQL not running"
docker inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$source_id" |
  grep -Fxq 'POSTGRES_DB=mamony_test_ci' ||
  fail "source PostgreSQL is not the disposable CI database"
docker inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$source_id" |
  grep -Fxq 'POSTGRES_USER=mamony_test_admin' ||
  fail "source PostgreSQL role mismatch"

script_dir="$(cd "$(dirname "$0")" && pwd)"
snapshot_sql="$script_dir/passkey-recovery-snapshot.sql"
[[ -f "$snapshot_sql" ]] || fail "missing shared snapshot query"

scratch="$(mktemp -d)"
chmod 700 "$scratch"
restore_id=''
cleanup() {
  if [[ -n "$restore_id" ]]; then
    docker rm -f "$restore_id" >/dev/null 2>&1 || true
  fi
  rm -rf "$scratch"
}
trap cleanup EXIT

source_psql() {
  docker exec -i "$source_id" psql -U mamony_test_admin -d mamony_test_ci \
    -X -q -A -t -v ON_ERROR_STOP=1 "$@"
}

snapshot_source() {
  source_psql < "$snapshot_sql"
}

validate_snapshot() {
  local data="$1"
  local verified linked notices events digest
  IFS='|' read -r verified linked notices events digest <<<"$data"
  [[ "$verified" =~ ^[0-9]+$ && "$linked" =~ ^[0-9]+$ &&
     "$notices" =~ ^[0-9]+$ && "$events" =~ ^[0-9]+$ &&
     "$digest" =~ ^[0-9a-f]{32}$ ]] ||
    fail "invalid restore snapshot structure"
  (( verified >= 1 )) || fail "missing real committed possession fixture"
  [[ "$verified" == "$linked" && "$verified" == "$notices" &&
     "$verified" == "$events" ]] ||
    fail "not all verified credentials have one linked consumed challenge, audit and pending notice"
}

[[ "$(source_psql -c 'SELECT current_database()')" == 'mamony_test_ci' ]] ||
  fail "wrong source database"
before="$(snapshot_source)"
validate_snapshot "$before"

# PostgreSQL 18's own pg_dump/pg_restore run inside the containers. The
# ubuntu-latest runner's client may be older than the v18 server.
docker exec "$source_id" pg_dump -U mamony_test_admin \
  -d mamony_test_ci --format=custom --no-owner --no-acl \
  > "$scratch/source.dump"
[[ -s "$scratch/source.dump" ]] || fail "logical dump is empty"

# All writes have finished. Confirm dump did not race a concurrent mutation.
[[ "$(snapshot_source)" == "$before" ]] ||
  fail "source changed while taking logical dump"

# Restore RLS policy role identifiers, with NOLOGIN and no passwords. Grants
# and ownership are intentionally NOT being tested by this logical restore.
source_psql -c "SELECT format('CREATE ROLE %I NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;', rolname) FROM pg_catalog.pg_roles WHERE rolname LIKE 'mamony_%' AND rolname <> 'mamony_test_admin' ORDER BY rolname" > "$scratch/roles.sql"

restore_id="$(docker run --rm -d \
  -e POSTGRES_DB=mamony_test_restore \
  -e POSTGRES_USER=mamony_test_admin \
  -e POSTGRES_PASSWORD=local-test-restore-admin \
  -p 127.0.0.1::5432 postgres:18.7)"
[[ "$restore_id" =~ ^[[:xdigit:]]{12,64}$ && "$restore_id" != "$source_id" ]] ||
  fail "restore is not a separate disposable container"
[[ "$(docker inspect --format '{{.Config.Image}}' "$restore_id")" == 'postgres:18.7' ]] ||
  fail "unexpected restore image"
docker inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$restore_id" |
  grep -Fxq 'POSTGRES_DB=mamony_test_restore' ||
  fail "unexpected restore database"

ready=false
for attempt in $(seq 1 60); do
  if docker exec "$restore_id" pg_isready -U mamony_test_admin \
    -d mamony_test_restore >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 1
done
[[ "$ready" == "true" ]] || fail "independent PostgreSQL restore instance did not start"

docker exec -i "$restore_id" psql -X -q -v ON_ERROR_STOP=1 \
  -U mamony_test_admin -d mamony_test_restore < "$scratch/roles.sql"
# Restore a complete logical DB with PostgreSQL's v18 tools. The new
# instance intentionally lacks original runtime role passwords and grants,
# so this test certifies data consistency, NOT access-policy restoration.
docker exec -i "$restore_id" pg_restore -U mamony_test_admin \
  -d mamony_test_restore --no-owner --no-acl --single-transaction \
  --exit-on-error < "$scratch/source.dump"

restored_db="$(docker exec "$restore_id" psql -U mamony_test_admin \
  -d mamony_test_restore -X -q -A -t -v ON_ERROR_STOP=1 \
  -c 'SELECT current_database()')"
[[ "$restored_db" == 'mamony_test_restore' ]] ||
  fail "restore snapshot points to the source DB"

after="$(docker exec -i "$restore_id" psql -U mamony_test_admin \
  -d mamony_test_restore -X -q -A -t -v ON_ERROR_STOP=1 < "$snapshot_sql")"
validate_snapshot "$after"
[[ "$before" == "$after" ]] ||
  fail "restored credential, challenge, audit or notice snapshot differs"

echo "PASSKEY BACKUP-RESTORE PASS: distinct PostgreSQL 18 container restored all verified credentials, linked consumed challenges, possession audit and queued notice rows"
