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
physical_id=''
cleanup() {
  if [[ "$physical_id" =~ ^[a-f0-9]{64}$ && "$physical_id" != "$source_id" && "$physical_id" != "$target_id" ]]; then
    docker rm -f -v "$physical_id" >/dev/null 2>&1 || true
  fi
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
  -e POSTGRES_PASSWORD=local-test-rh03-admin postgres:18.7 \
  -c archive_mode=on -c wal_level=replica \
  -c "archive_command=mkdir -p /tmp/mamony-rh03-archive && cp %p /tmp/mamony-rh03-archive/%f.tmp && mv /tmp/mamony-rh03-archive/%f.tmp /tmp/mamony-rh03-archive/%f")
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

# RH-03a2: physical recovery of the already-held, logically seeded fixture.
# No changes to the original CI service or existing migration files.
# The following two state transitions are controlled fixture data changes,
# not evidence that application APIs enforce these transitions.
lab_pg() {
  local id=$1
  shift
  timeout 90 docker exec -u postgres "$id" "$@"
}
[[ "$(target_sql -c 'SHOW archive_mode')" == on ]] || fail "lab archive not enabled"
lab_pg "$target_id" pg_basebackup -U mamony_test_admin -D /tmp/mamony-rh03-base -X stream -c fast
lab_pg "$target_id" pg_verifybackup /tmp/mamony-rh03-base
target_sql -c 'SELECT pg_switch_wal()' >/dev/null
# Select only existing synthetic rows in the disposable, NOLOGIN copy.
# Record exact count of each changed row and commit both in one transaction.
changes=$(target_sql <<'SQL'
BEGIN;
WITH updated AS (
  UPDATE mamony_identity.opaque_sessions
  SET revoked_at = greatest(issued_at, clock_timestamp())
  WHERE token_digest = (
    SELECT token_digest FROM mamony_identity.opaque_sessions
    WHERE revoked_at IS NULL ORDER BY token_digest LIMIT 1
  )
  RETURNING 1
)
SELECT count(*) FROM updated;
WITH updated AS (
  UPDATE mamony_identity.password_recovery_offline_codes
  SET consumed_at = greatest(enrolled_at, clock_timestamp())
  WHERE code_digest = (
    SELECT code_digest FROM mamony_identity.password_recovery_offline_codes
    WHERE consumed_at IS NULL ORDER BY code_digest LIMIT 1
  )
  RETURNING 1
)
SELECT count(*) FROM updated;
COMMIT;
SQL
)
[[ "$changes" == $'1\n1' ]] || fail "controlled fixture transitions missing"
target_sql < "$snapshot_sql" > "$work/native-boundary"
validate_snapshot "$work/native-boundary" || fail "invalid native boundary observation"
for table in opaque_sessions password_recovery_offline_codes; do
  [[ "$(grep -F "$table|" "$work/restored")" != "$(grep -F "$table|" "$work/native-boundary")" ]] || fail "fixture transition not reflected"
done
# All other observed binding, audit and notice rows must remain unchanged.
grep -Ev '^(opaque_sessions|password_recovery_offline_codes)\|' "$work/restored" > "$work/native-unchanged-before"
grep -Ev '^(opaque_sessions|password_recovery_offline_codes)\|' "$work/native-boundary" > "$work/native-unchanged-after"
cmp -s "$work/native-unchanged-before" "$work/native-unchanged-after" || fail "unexpected fixture state change"
boundary=$(target_sql -c "SELECT pg_create_restore_point('mamony_rh03_boundary')")
[[ "$boundary" =~ ^[0-9A-F]+/[0-9A-F]+$ ]] || fail "invalid native boundary"
segment=$(target_sql -c "SELECT pg_walfile_name('$boundary'::pg_lsn)")
[[ "$segment" =~ ^[0-9A-F]{24}$ ]] || fail "invalid native WAL segment"
lab_pg "$target_id" test ! -e "/tmp/mamony-rh03-base/pg_wal/$segment" || fail "boundary already in base WAL"
target_sql -c 'SELECT pg_switch_wal()' >/dev/null
for ((i=0; i<45; i++)); do
  if lab_pg "$target_id" test -s "/tmp/mamony-rh03-archive/$segment"; then break; fi
  sleep 1
done
lab_pg "$target_id" test -s "/tmp/mamony-rh03-archive/$segment" || fail "native boundary WAL not archived"
lineage=$(target_sql -c 'SELECT system_identifier::text FROM pg_control_system()')
timeline=$(target_sql -c 'SELECT timeline_id FROM pg_control_checkpoint()')
[[ "$lineage" =~ ^[0-9]+$ && "$timeline" =~ ^[0-9]+$ ]] || fail "invalid native lineage"
target_sql < "$snapshot_sql" > "$work/native-boundary-confirmed"
cmp -s "$work/native-boundary" "$work/native-boundary-confirmed" || fail "native source observation changed"

mkdir "$work/native-base" "$work/native-archive"
docker cp "$target_id:/tmp/mamony-rh03-base/." "$work/native-base"
docker cp "$target_id:/tmp/mamony-rh03-archive/." "$work/native-archive"
[[ -s "$work/native-base/backup_manifest" && -s "$work/native-archive/$segment" ]] || fail "native evidence missing"

physical_id=$(timeout 90 docker run --rm -d --network none --label mamony.synthetic-rh03-native=true --entrypoint sleep postgres:18.7 infinity)
[[ "$physical_id" =~ ^[a-f0-9]{64}$ && "$physical_id" != "$source_id" && "$physical_id" != "$target_id" ]] || fail "native target not distinct"
[[ "$(docker inspect -f '{{.HostConfig.NetworkMode}}|{{.Config.Image}}|{{index .Config.Labels "mamony.synthetic-rh03-native"}}' "$physical_id")" == 'none|postgres:18.7|true' ]] || fail "native containment mismatch"
root=/tmp/mamony-rh03-native
docker exec "$physical_id" mkdir -p "$root/data" "$root/archive" "$root/socket"
docker cp "$work/native-base/." "$physical_id:$root/data"
docker cp "$work/native-archive/." "$physical_id:$root/archive"
docker exec "$physical_id" chown -R postgres:postgres "$root"
docker exec "$physical_id" chmod 700 "$root/data"
lab_pg "$physical_id" pg_verifybackup "$root/data"
docker exec -i -u postgres "$physical_id" sh -c "cat >> $root/data/postgresql.conf" <<CONF
listen_addresses = ''
unix_socket_directories = '$root/socket'
archive_mode = off
restore_command = 'cp $root/archive/%f %p'
recovery_target_name = 'mamony_rh03_boundary'
recovery_target_timeline = '$timeline'
recovery_target_action = 'pause'
hot_standby = on
CONF
lab_pg "$physical_id" touch "$root/data/recovery.signal"
native_sql() {
  timeout 90 docker exec -i -u postgres "$physical_id" psql -h "$root/socket" -U mamony_test_admin -d mamony_test_rh03 -XqAt -F '|' -v ON_ERROR_STOP=1 "$@"
}
lab_pg "$physical_id" pg_ctl -D "$root/data" -l "$root/recovery.log" -w -t 45 start >/dev/null || fail "native recovery start failed"
state=''
for ((i=0; i<45; i++)); do
  state=$(native_sql -c "SELECT pg_is_in_recovery() AND pg_get_wal_replay_pause_state()='paused'" 2>/dev/null || true)
  [[ "$state" != t ]] || break
  sleep 1
done
[[ "$state" == t ]] || fail "native target not paused"
[[ "$(native_sql -c 'SELECT system_identifier::text FROM pg_control_system()')" == "$lineage" ]] || fail "native lineage mismatch"
[[ "$(native_sql -c "SELECT pg_last_wal_replay_lsn() >= '$boundary'::pg_lsn")" == t ]] || fail "native boundary not reached"
lab_pg "$physical_id" grep -F 'recovery stopping at restore point' "$root/recovery.log" >/dev/null || fail "native named target not confirmed"
native_sql < "$snapshot_sql" > "$work/native-restored"
validate_snapshot "$work/native-restored" || fail "invalid native target observation"
cmp -s "$work/native-boundary" "$work/native-restored" || fail "native Identity observation differs"
if cmp -s "$work/restored" "$work/native-restored"; then fail "pre-boundary state unexpectedly matched"; fi
[[ "$(native_sql -c "SELECT count(*) FROM pg_roles WHERE ($role_filter) AND rolcanlogin")" == 0 ]] || fail "native role hold missing"
native_sql -c "SELECT rolname FROM pg_roles WHERE $role_filter ORDER BY rolname" > "$work/native-roles"
cmp -s "$work/role-names" "$work/native-roles" || fail "native role inventory mismatch"
echo "RH-03a2 PASS: native named-boundary recovery of held Identity fixture; post-backup session revocation and code consumption preserved; original selected binding, audit and notice rows unchanged"
