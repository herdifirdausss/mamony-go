#!/usr/bin/env bash
# RH-01: isolated synthetic PostgreSQL physical backup/recovery only.
set -euo pipefail
[[ "${GITHUB_ACTIONS:-}" == true && $# == 0 ]] || { echo "CI-only synthetic recovery lab" >&2; exit 1; }
command -v docker >/dev/null
root=/tmp/mamony-pitr
work=$(mktemp -d)
source_id=
target_id=
cleanup() {
  local id
  for id in "$target_id" "$source_id"; do
    if [[ "$id" =~ ^[a-f0-9]{64}$ ]]; then
      docker rm -f -v "$id" >/dev/null 2>&1 || true
    fi
  done
  rm -rf -- "$work"
}
trap cleanup EXIT
fail() { echo "RH-01 FAIL: $*" >&2; exit 1; }
new_container() {
  docker run --rm -d --network none --label mamony.synthetic-pitr=true --entrypoint sleep postgres:18.7 infinity
}
check_container() {
  local id=$1
  [[ "$id" =~ ^[a-f0-9]{64}$ ]] || fail "invalid container identity"
  [[ "$(docker inspect -f '{{.HostConfig.NetworkMode}}|{{.Config.Image}}|{{index .Config.Labels "mamony.synthetic-pitr"}}' "$id")" == "none|postgres:18.7|true" ]] || fail "unexpected lab containment"
}
pg() {
  local id=$1
  shift
  timeout 90 docker exec -u postgres "$id" "$@"
}
sql() {
  local id=$1
  shift
  pg "$id" psql -XAt -v ON_ERROR_STOP=1 -h "$root/socket" -U lab_admin -d postgres "$@"
}
prepare() {
  docker exec "$1" mkdir -p "$root/socket" "$root/archive"
  docker exec "$1" chown -R postgres:postgres "$root"
}
source_id=$(new_container)
check_container "$source_id"
prepare "$source_id"
pg "$source_id" initdb -D "$root/data" -U lab_admin --auth-local=trust --auth-host=trust >/dev/null
docker exec -i -u postgres "$source_id" sh -c "cat >> $root/data/postgresql.conf" <<'CONF'
listen_addresses = ''
unix_socket_directories = '/tmp/mamony-pitr/socket'
wal_level = replica
archive_mode = on
archive_command = 'cp %p /tmp/mamony-pitr/archive/%f.tmp && mv /tmp/mamony-pitr/archive/%f.tmp /tmp/mamony-pitr/archive/%f'
CONF
pg "$source_id" pg_ctl -D "$root/data" -l "$root/source.log" -w -t 30 start >/dev/null
sql "$source_id" -c "CREATE TABLE lab_marker (id integer PRIMARY KEY); INSERT INTO lab_marker VALUES (1);" >/dev/null
started=$SECONDS
pg "$source_id" pg_basebackup -h "$root/socket" -U lab_admin -D "$root/base" -X stream -c fast
pg "$source_id" pg_verifybackup "$root/base"
echo "RH-01a PASS: native backup verified; elapsed_seconds=$((SECONDS-started))"
# Force the post-backup boundary into WAL beyond the streamed backup.
sql "$source_id" -c "SELECT pg_switch_wal();" >/dev/null
sql "$source_id" -c "INSERT INTO lab_marker VALUES (2);" >/dev/null
sql "$source_id" -c "BEGIN; INSERT INTO lab_marker VALUES (3); ROLLBACK;" >/dev/null
boundary=$(sql "$source_id" -c "SELECT pg_create_restore_point('mamony_lab_boundary');")
[[ "$boundary" =~ ^[0-9A-F]+/[0-9A-F]+$ ]] || fail "invalid boundary"
segment=$(sql "$source_id" -c "SELECT pg_walfile_name('$boundary'::pg_lsn);")
[[ "$segment" =~ ^[0-9A-F]{24}$ ]] || fail "invalid archive segment"
pg "$source_id" test ! -f "$root/base/pg_wal/$segment" || fail "target segment already in base backup"
# The later committed row must not appear at the selected recovery point.
sql "$source_id" -c "INSERT INTO lab_marker VALUES (4); SELECT pg_switch_wal();" >/dev/null
for ((i=0; i<45; i++)); do
  if pg "$source_id" test -s "$root/archive/$segment"; then break; fi
  sleep 1
done
pg "$source_id" test -s "$root/archive/$segment" || fail "required archive segment absent"
lineage=$(sql "$source_id" -c "SELECT system_identifier::text FROM pg_control_system();")
timeline=$(sql "$source_id" -c "SELECT timeline_id FROM pg_control_checkpoint();")
[[ "$lineage" =~ ^[0-9]+$ && "$timeline" =~ ^[0-9]+$ ]] || fail "invalid source lineage"
[[ "$(sql "$source_id" -c "SELECT string_agg(id::text, ',' ORDER BY id) FROM lab_marker;")" == "1,2,4" ]] || fail "source markers unexpected"
printf "%s\n" "$lineage" "$timeline" "$boundary" "$segment" "1,2" > "$work/boundary.txt"
pg "$source_id" pg_ctl -D "$root/data" -m fast -w -t 30 stop >/dev/null
mkdir "$work/base" "$work/archive"
docker cp "$source_id:$root/base/." "$work/base"
docker cp "$source_id:$root/archive/." "$work/archive"
[[ -s "$work/archive/$segment" && -s "$work/base/backup_manifest" ]] || fail "missing copied evidence"
echo "RH-01b PASS: synthetic boundary and required post-backup WAL recorded"
target_id=$(new_container)
check_container "$target_id"
[[ "$target_id" != "$source_id" ]] || fail "target not distinct"
prepare "$target_id"
docker exec "$target_id" mkdir "$root/data"
docker cp "$work/base/." "$target_id:$root/data"
docker cp "$work/archive/." "$target_id:$root/archive"
docker exec "$target_id" chown -R postgres:postgres "$root"
docker exec "$target_id" chmod 700 "$root/data"
pg "$target_id" pg_verifybackup "$root/data"
mapfile -t record < "$work/boundary.txt"
[[ ${#record[@]} == 5 && "${record[0]}" == "$lineage" && "${record[1]}" == "$timeline" && "${record[2]}" == "$boundary" && "${record[3]}" == "$segment" && "${record[4]}" == "1,2" ]] || fail "boundary record mismatch"
docker exec -i -u postgres "$target_id" sh -c "cat >> $root/data/postgresql.conf" <<CONF
archive_mode = off
restore_command = 'cp /tmp/mamony-pitr/archive/%f %p'
recovery_target_name = 'mamony_lab_boundary'
recovery_target_timeline = '$timeline'
recovery_target_action = 'pause'
hot_standby = on
CONF
pg "$target_id" touch "$root/data/recovery.signal"
started=$SECONDS
pg "$target_id" pg_ctl -D "$root/data" -l "$root/target.log" -w -t 45 start >/dev/null || { pg "$target_id" cat "$root/target.log"; fail "target start failed"; }
state=
for ((i=0; i<45; i++)); do
  state=$(sql "$target_id" -c "SELECT pg_is_in_recovery() AND pg_get_wal_replay_pause_state() = 'paused';" 2>/dev/null || true)
  [[ "$state" != t ]] || break
  sleep 1
done
[[ "$state" == t ]] || fail "target did not reach paused recovery"
[[ "$(sql "$target_id" -c "SELECT system_identifier::text FROM pg_control_system();")" == "$lineage" ]] || fail "physical lineage mismatch"
[[ "$(sql "$target_id" -c "SELECT string_agg(id::text, ',' ORDER BY id) FROM lab_marker;")" == "${record[4]}" ]] || fail "restored markers unexpected"
[[ "$(sql "$target_id" -c "SELECT pg_last_wal_replay_lsn() >= '$boundary'::pg_lsn;")" == t ]] || fail "replay boundary not reached"
pg "$target_id" grep -F "recovery stopping at restore point" "$root/target.log" >/dev/null || fail "named target not confirmed"
echo "RH-01c PASS: native named-target recovery paused; committed markers present, rolled-back and later rows absent; elapsed_seconds=$((SECONDS-started))"
echo "RH-01 PASS: synthetic physical recovery only; no promotion or release"
echo "RH-02 NOT RUN: gap and evidence failure tests pending"
