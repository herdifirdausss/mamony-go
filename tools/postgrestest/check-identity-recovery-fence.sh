#!/usr/bin/env bash
# CI-only mechanism drill. Owns three fresh, network-isolated containers.
# No shared integration database, actual Identity data, production DSN or release.
set -euo pipefail

fail() { echo "IRF DRILL FAIL: $*" >&2; exit 1; }
[[ "$#" == 0 && "${GITHUB_ACTIONS:-}" == true && "${RUNNER_ENVIRONMENT:-}" == github-hosted ]] ||
  fail "GitHub-hosted CI only; no arguments"
command -v docker >/dev/null || fail "docker unavailable"
scratch="$(mktemp -d)"
chmod 700 "$scratch"
source_id='' target_id='' repeat_id='' idle_pid='' tx_pid=''
cleanup() {
  for id in "$source_id" "$target_id" "$repeat_id"; do
    if [[ -n "$id" ]]; then docker rm -f -v "$id" >/dev/null 2>&1 || true; fi
  done
  for pid in "$idle_pid" "$tx_pid"; do
    if [[ -n "$pid" ]]; then kill "$pid" >/dev/null 2>&1 || true; wait "$pid" 2>/dev/null || true; fi
  done
  rm -rf "$scratch"
}
trap cleanup EXIT
label="mamony.irf=$RANDOM-$RANDOM-$$"
admin=mamony_irf_admin
admin_pw=synthetic-irf-admin-only
runtime_pw=synthetic-irf-runtime-only
bootstrap="$(dirname "$0")/testdata/identity-bootstrap.sql"
# Extract names ONLY; never copy existing fixture credentials or run its reset.
sed -nE 's/^ALTER ROLE (mamony_identity_[a-z_]+|mamony_identity_runtime) LOGIN .*/\1/p' "$bootstrap" |
  LC_ALL=C sort -u > "$scratch/roles"
[[ -s "$scratch/roles" ]] || fail "empty role inventory"
grep -Fxq mamony_identity_migrator "$scratch/roles" || fail "missing migrator"
grep -Fxq mamony_identity_runtime "$scratch/roles" || fail "missing runtime"
role_count="$(wc -l < "$scratch/roles" | tr -d ' ')"

start() {
  docker run -d --network none --label "$label" \
    -e "POSTGRES_DB=$1" -e "POSTGRES_USER=$admin" -e "POSTGRES_PASSWORD=$admin_pw" \
    postgres:18.7 -c max_prepared_transactions=0
}
# Every privileged operation is restricted to a container created by this run.
target_ok() {
  local id="$1" db="$2"
  [[ "$id" =~ ^[[:xdigit:]]{64}$ && "$db" =~ ^mamony_test_irf_(source|target|repeat)$ ]] || return 1
  [[ "$(docker inspect --format '{{.Config.Image}}' "$id")" == postgres:18.7 ]] || return 1
  [[ "$(docker inspect --format '{{.HostConfig.NetworkMode}}' "$id")" == none ]] || return 1
  [[ "$(docker inspect --format '{{index .Config.Labels "mamony.irf"}}' "$id")" == "${label#*=}" ]] || return 1
  docker inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$id" |
    grep -Fx "POSTGRES_DB=$db" >/dev/null
}
admin_sql() {
  local id="$1" db="$2"; shift 2
  target_ok "$id" "$db" || fail "unexpected disposable target"
  docker exec -i -e "PGPASSWORD=$admin_pw" -e PGCONNECT_TIMEOUT=3 "$id" \
    psql -h 127.0.0.1 -U "$admin" -d "$db" -X -qAt -v ON_ERROR_STOP=1 "$@"
}
ready() {
  local id="$1" db="$2"
  for _ in $(seq 1 45); do
    if [[ "$(admin_sql "$id" "$db" -c 'SELECT current_database()' 2>/dev/null || true)" == "$db" ]]; then return; fi
    sleep 1
  done
  fail "database readiness not verified"
}
runtime_sql() {
  local id="$1" db="$2" role="$3"; shift 3
  target_ok "$id" "$db" || fail "unexpected runtime target"
  grep -Fxq "$role" "$scratch/roles" || fail "unknown runtime fixture"
  docker exec -i -e "PGPASSWORD=$runtime_pw" -e PGCONNECT_TIMEOUT=3 -e LC_ALL=C "$id" \
    psql -h 127.0.0.1 -U "$role" -d "$db" -X -qAt -v ON_ERROR_STOP=1 "$@"
}
check_inventory() {
  local id="$1" db="$2" expected="$3"
  admin_sql "$id" "$db" -c "SELECT rolname FROM pg_roles WHERE rolname LIKE 'mamony_identity_%' ORDER BY rolname" |
    LC_ALL=C sort > "$scratch/live"
  cmp -s "$expected" "$scratch/live" || return 1
  [[ "$(admin_sql "$id" "$db" -c "SELECT count(*) FROM pg_roles WHERE rolcanlogin AND rolname <> '$admin' AND rolname NOT LIKE 'mamony_identity_%'")" == 0 ]] || return 1
  [[ "$(admin_sql "$id" "$db" -c "SELECT count(*) FROM pg_auth_members m JOIN pg_roles r ON r.oid=m.member JOIN pg_roles g ON g.oid=m.roleid WHERE r.rolname LIKE 'mamony_identity_%' OR g.rolname LIKE 'mamony_identity_%'")" == 0 ]]
}
create_roles() {
  local id="$1" db="$2" mode="$3" role
  while IFS= read -r role; do
    [[ "$role" =~ ^mamony_identity_[a-z_]+$ ]] || fail "unexpected role name"
    if [[ "$mode" == LOGIN ]]; then
      printf "CREATE ROLE %s LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD '%s';\n" "$role" "$runtime_pw"
    else
      printf "CREATE ROLE %s NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;\n" "$role"
    fi
  done < "$scratch/roles" | admin_sql "$id" "$db"
  check_inventory "$id" "$db" "$scratch/roles" || fail "role inventory mismatch"
}
deny_all() {
  local id="$1" db="$2" role
  [[ "$(admin_sql "$id" "$db" -c "SELECT count(*) FROM pg_roles WHERE rolname LIKE 'mamony_identity_%' AND rolcanlogin")" == 0 ]] ||
    fail "role can still login"
  while IFS= read -r role; do
    if runtime_sql "$id" "$db" "$role" -c 'SELECT 1' > "$scratch/deny" 2>&1; then
      fail "unexpected successful runtime login"
    fi
    grep -Eq 'not permitted to log in|password authentication failed|permission denied for database' "$scratch/deny" ||
      fail "connection denial not established (unexpected failure)"
  done < "$scratch/roles"
  [[ "$(admin_sql "$id" "$db" -c 'SELECT current_database()')" == "$db" ]] || fail "admin inspection unavailable"
}

source_db=mamony_test_irf_source
target_db=mamony_test_irf_target
repeat_db=mamony_test_irf_repeat
source_id="$(start "$source_db")"
ready "$source_id" "$source_db"
create_roles "$source_id" "$source_db" LOGIN
while IFS= read -r role; do
  [[ "$(runtime_sql "$source_id" "$source_db" "$role" -c "SELECT current_user || '|' || current_database()")" == "$role|$source_db" ]] ||
    fail "positive role connection failed"
done < "$scratch/roles"
admin_sql "$source_id" "$source_db" -c 'CREATE TABLE public.irf_marker (id integer PRIMARY KEY); GRANT INSERT ON public.irf_marker TO mamony_identity_runtime'
runtime_sql "$source_id" "$source_db" mamony_identity_runtime -c 'INSERT INTO public.irf_marker VALUES (1)'
[[ "$(admin_sql "$source_id" "$source_db" -c 'SELECT count(*) FROM public.irf_marker')" == 1 ]] || fail "committed marker missing"
echo "IRF-F01 PASS: $role_count source principals connected; committed synthetic marker"

# Restore only the harmless marker, not Identity schema or role ACLs.
docker exec "$source_id" pg_dump -U "$admin" -d "$source_db" -Fc --no-owner --no-acl -t public.irf_marker > "$scratch/marker.dump"
[[ -s "$scratch/marker.dump" ]] || fail "empty marker backup"
held_target() {
  local id="$1" db="$2"
  create_roles "$id" "$db" NOLOGIN
  admin_sql "$id" "$db" -c "REVOKE CONNECT ON DATABASE $db FROM PUBLIC"
  docker exec -i "$id" pg_restore -U "$admin" -d "$db" --no-owner --no-acl --single-transaction --exit-on-error < "$scratch/marker.dump"
  [[ "$(admin_sql "$id" "$db" -c 'SELECT count(*) FROM public.irf_marker WHERE id=1')" == 1 ]] || fail "restore marker missing"
  [[ "$(admin_sql "$id" "$db" -c "SELECT count(*) FROM pg_authid WHERE rolname LIKE 'mamony_identity_%' AND rolpassword IS NOT NULL")" == 0 ]] || fail "target unexpectedly has runtime passwords"
  deny_all "$id" "$db"
}
target_id="$(start "$target_db")"
[[ "$target_id" != "$source_id" ]] || fail "same target"
ready "$target_id" "$target_db"
held_target "$target_id" "$target_db"
echo "IRF-F02 PASS: isolated restored marker; all fixture roles denied"

# One idle session and one known uncommitted transaction in the source.
mkfifo "$scratch/idle-input"
exec 3<>"$scratch/idle-input"
runtime_sql "$source_id" "$source_db" mamony_identity_runtime < "$scratch/idle-input" > "$scratch/idle-log" 2>&1 &
idle_pid=$!
printf "SET application_name='irf_idle';\nSELECT 1;\n" >&3
runtime_sql "$source_id" "$source_db" mamony_identity_runtime -c \
  "SET application_name='irf_tx'; BEGIN; INSERT INTO public.irf_marker VALUES (2); SELECT pg_sleep(60); COMMIT;" > "$scratch/tx-log" 2>&1 &
tx_pid=$!
observed=false
for _ in $(seq 1 20); do
  if [[ "$(admin_sql "$source_id" "$source_db" -c "SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND usename='mamony_identity_runtime' AND ((application_name='irf_idle' AND state='idle') OR (application_name='irf_tx' AND state='active' AND wait_event='PgSleep'))")" == 2 ]]; then observed=true; break; fi
  sleep 1
done
[[ "$observed" == true ]] || fail "existing sessions not established"
[[ "$(admin_sql "$source_id" "$source_db" -c 'SHOW max_prepared_transactions')" == 0 ]] || fail "prepared transaction policy mismatch"
[[ "$(admin_sql "$source_id" "$source_db" -c 'SELECT count(*) FROM pg_prepared_xacts')" == 0 ]] || fail "unexpected prepared transaction"
# Only this new disposable instance; no admin or other service session touched.
while IFS= read -r role; do printf 'ALTER ROLE %s NOLOGIN;\n' "$role"; done < "$scratch/roles" |
  admin_sql "$source_id" "$source_db"
deny_all "$source_id" "$source_db"
admin_sql "$source_id" "$source_db" -c "SELECT pg_terminate_backend(pid, 5000) FROM pg_stat_activity WHERE datname=current_database() AND usename LIKE 'mamony_identity_%' AND pid <> pg_backend_pid()" > "$scratch/signals"
[[ "$(grep -c '^t$' "$scratch/signals")" == 2 && "$(wc -l < "$scratch/signals" | tr -d ' ')" == 2 ]] ||
  fail "termination did not confirm both backends"
[[ "$(admin_sql "$source_id" "$source_db" -c "SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND usename LIKE 'mamony_identity_%'")" == 0 ]] ||
  fail "runtime connections remain"
exec 3>&-
# Client error is not by itself outcome evidence; confirm state separately.
if wait "$tx_pid"; then fail "uncommitted client unexpectedly succeeded"; fi
tx_pid=''
if wait "$idle_pid"; then fail "idle client unexpectedly succeeded"; fi
idle_pid=''
deny_all "$source_id" "$source_db"
echo "IRF-F03 PASS: fresh admission denied and two observed backends terminated"
[[ "$(admin_sql "$source_id" "$source_db" -c 'SELECT string_agg(id::text, chr(44) ORDER BY id) FROM public.irf_marker')" == 1 ]] ||
  fail "committed/uncommitted marker invariant failed"
echo "IRF-F04 PASS: committed marker retained; known uncommitted marker rolled back"

# Selected negative preconditions, not a simulated recovery-controller timeout.
if target_ok "$source_id" "$target_db"; then fail "wrong-target guard accepted"; fi
head -n -1 "$scratch/roles" > "$scratch/incomplete"
if check_inventory "$source_id" "$source_db" "$scratch/incomplete"; then fail "incomplete inventory accepted"; fi
echo "IRF-F05 PARTIAL: wrong target and incomplete inventory refused; timeout NOT RUN"
repeat_id="$(start "$repeat_db")"
[[ "$repeat_id" != "$source_id" && "$repeat_id" != "$target_id" ]] || fail "repeat target not distinct"
ready "$repeat_id" "$repeat_db"
held_target "$repeat_id" "$repeat_db"
echo "IRF-F06 PARTIAL: second fresh restore held; release authority NOT RUN"
echo "IRF MECHANISM DRILL PASS: synthetic connection and transaction isolation only; no Identity recovery release"
