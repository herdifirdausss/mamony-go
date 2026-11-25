#!/usr/bin/env bash
# CI-only RH-03 candidate: read-only synthetic source, isolated logical restore.
set -euo pipefail
umask 077
fail() { echo "RH-03 LAB FAIL: $*" >&2; exit 1; }
[[ "$GITHUB_ACTIONS" == true && $# == 0 ]] || fail "CI-only fixture required"
[[ "$TEST_DATABASE_ADMIN_URL" == 'postgres://mamony_test_admin:local-test-admin@127.0.0.1:5432/mamony_test_ci?sslmode=disable' ]] || fail "unexpected source fixture"
source_id="$PG_SERVICE_CONTAINER_ID"
[[ "$source_id" =~ ^[a-f0-9]{64}$ ]] || fail "invalid service identity"
command -v docker >/dev/null || fail "docker missing"
[[ "$(docker inspect -f '{{.Config.Image}}|{{.State.Running}}' "$source_id")" == 'postgres:18.7|true' ]] || fail "unexpected source image/state"
source_env=$(docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "$source_id")
grep -Fxq 'POSTGRES_DB=mamony_test_ci' <<<"$source_env" || fail "wrong source database"
grep -Fxq 'POSTGRES_USER=mamony_test_admin' <<<"$source_env" || fail "wrong source role"
unset source_env

work=$(mktemp -d)
target_id=''
cleanup() {
  if [[ "$target_id" =~ ^[a-f0-9]{64}$ && "$target_id" != "$source_id" ]]; then
    docker rm -f -v "$target_id" >/dev/null 2>&1 || true
  fi
  rm -rf -- "$work"
}
trap cleanup EXIT
snapshot_sql="$(dirname "$0")/identity-reconciliation-snapshot.sql"
[[ -s "$snapshot_sql" ]] || fail "missing snapshot query"

source_sql() {
  timeout 90 docker exec -i "$source_id" psql -U mamony_test_admin -d mamony_test_ci -XqAt -F '|' -v ON_ERROR_STOP=1 "$@"
}
target_sql() {
  timeout 90 docker exec -i "$target_id" psql -U mamony_test_admin -d mamony_test_rh03 -XqAt -F '|' -v ON_ERROR_STOP=1 "$@"
}
[[ "$(source_sql -c 'SELECT current_database()')" == mamony_test_ci ]] || fail "wrong source"
[[ "$(source_sql -c 'SELECT max(version) FROM mamony_identity.migration_fingerprints')" == 34 ]] || fail "expected main migration fixture missing"
source_sql < "$snapshot_sql" > "$work/before"
# Reject vacuous or malformed fixture snapshots; all expected records must exist.
validate_snapshot() {
  local file=$1
  [[ -s "$file" ]] || return 1
  [[ "$(wc -l < "$file")" -eq 18 ]] || return 1
  awk -F '|' 'NF != 3 || $1 !~ /^[a-z_]+$/ || $2 !~ /^[0-9]+$/ || length($3) != 64 || $3 !~ /^[0-9a-f]+$/ || seen[$1]++ {bad=1} END {exit bad}' "$file" || return 1
  local table
  for table in subjects email_aliases password_verifiers recovery_code_factor_keys password_recovery_offline_codes passkey_credentials security_outbox password_change_notices password_recovery_notices password_recovery_code_notices passkey_registration_notices opaque_sessions migration_fingerprints; do
    awk -F '|' -v t="$table" '$1==t && $2>0 {found=1} END {exit !found}' "$file" || return 1
  done
}
validate_snapshot "$work/before" || fail "missing or invalid committed fixture"
# Existing integration gate has completed before this step; no source writes.
timeout 120 docker exec "$source_id" pg_dump -U mamony_test_admin -d mamony_test_ci --format=custom --no-owner --no-acl > "$work/source.dump"
[[ -s "$work/source.dump" ]] || fail "empty logical dump"
source_sql < "$snapshot_sql" > "$work/after-dump"
cmp -s "$work/before" "$work/after-dump" || fail "source changed during observation"
# Recreate only role identifiers, without credentials or login permission.
role_filter="left(rolname,7)='mamony_' AND rolname <> 'mamony_test_admin'"
source_sql -c "SELECT rolname FROM pg_roles WHERE $role_filter ORDER BY rolname" > "$work/role-names"
source_sql -c "SELECT format('CREATE ROLE %I NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;', rolname) FROM pg_roles WHERE $role_filter ORDER BY rolname" > "$work/roles.sql"
[[ -s "$work/role-names" && -s "$work/roles.sql" ]] || fail "missing role inventory"

target_id=$(timeout 90 docker run --rm -d --network none --label mamony.synthetic-rh03=true \
  -e POSTGRES_DB=mamony_test_rh03 -e POSTGRES_USER=mamony_test_admin \
  -e POSTGRES_PASSWORD=local-test-rh03-admin postgres:18.7)
[[ "$target_id" =~ ^[a-f0-9]{64}$ && "$target_id" != "$source_id" ]] || fail "restore target not distinct"
[[ "$(docker inspect -f '{{.HostConfig.NetworkMode}}|{{.Config.Image}}|{{index .Config.Labels "mamony.synthetic-rh03"}}' "$target_id")" == 'none|postgres:18.7|true' ]] || fail "unexpected target containment"
ready=false
for ((i=0; i<60; i++)); do
  # TCP readiness inside the target distinguishes the final postmaster from
  # the image's temporary socket-only initialization server.
  if timeout 5 docker exec "$target_id" pg_isready -h 127.0.0.1 -U mamony_test_admin -d mamony_test_rh03 >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 1
done
[[ "$ready" == true ]] || fail "target not ready"
[[ "$(target_sql -c 'SELECT current_database()')" == mamony_test_rh03 ]] || fail "wrong target database"
target_sql < "$work/roles.sql" >/dev/null
timeout 120 docker exec -i "$target_id" pg_restore -U mamony_test_admin -d mamony_test_rh03 --no-owner --no-acl --single-transaction --exit-on-error < "$work/source.dump"
target_sql < "$snapshot_sql" > "$work/restored"
validate_snapshot "$work/restored" || fail "invalid restored fixture"

# Only compare against the process-local controlled observation; matching
# evidence is never an authorization to release the target.
matches_observation() {
  validate_snapshot "$1" && cmp -s "$work/before" "$1"
}
matches_observation "$work/restored" || fail "restored rows differ"
echo "RH-03a1 PASS: selected committed Identity binding, audit and notice rows match logical restore"
target_sql -c "SELECT rolname FROM pg_roles WHERE $role_filter ORDER BY rolname" > "$work/target-roles"
cmp -s "$work/role-names" "$work/target-roles" || fail "target role inventory mismatch"
[[ "$(target_sql -c "SELECT count(*) FROM pg_roles WHERE ($role_filter) AND rolcanlogin")" == 0 ]] || fail "unexpected login-enabled target role"
echo "RH-03b1 PASS: selected temporary authority rows match; isolated target has NOLOGIN fixture roles (not application invalidation)"

head -n 1 "$work/before" > "$work/truncated"
awk -F '|' 'BEGIN {OFS="|"} NR==1 {$2=$2+1} {print}' "$work/before" > "$work/contradictory"
for candidate in "$work/absent" "$work/truncated" "$work/contradictory"; do
  if matches_observation "$candidate"; then fail "incomplete or contradictory evidence accepted"; fi
done
matches_observation "$work/restored" || fail "positive observation changed"
[[ "$(target_sql -c "SELECT count(*) FROM pg_roles WHERE ($role_filter) AND rolcanlogin")" == 0 ]] || fail "target hold changed"
echo "RH-03c1 PASS: missing, truncated and contradictory comparison rejected; target remains HOLD"
