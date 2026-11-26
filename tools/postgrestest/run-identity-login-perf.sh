#!/usr/bin/env bash
# Run one synthetic login performance case against a new local PostgreSQL cluster.
set -euo pipefail
umask 077

if [[ $# -ne 1 || ! "$1" =~ ^[14]$ ]]; then
  printf 'Usage: %s 1|4\n' "$0" >&2
  exit 2
fi
export MAMONY_IDENTITY_PERF_WORKERS="$1"
profile="${MAMONY_IDENTITY_PERF_PROFILE:-0}"
if [[ "$profile" != 0 && "$profile" != 1 ]]; then
  printf 'Set MAMONY_IDENTITY_PERF_PROFILE to 0 or 1.\n' >&2
  exit 2
fi
for command in go initdb pg_ctl postgres psql python3 taskset; do
  command -v "$command" >/dev/null || { printf 'Missing command: %s\n' "$command" >&2; exit 2; }
done
if [[ ! "$(postgres --version)" =~ PostgreSQL\)\ 18\.7(\ |$) ]]; then
  printf 'Use PostgreSQL 18.7 for this baseline.\n' >&2
  exit 2
fi

cd -- "$(dirname -- "$0")/../.."
cpus="${MAMONY_IDENTITY_PERF_CPU_SET:-$(python3 -c 'import os; print(",".join(map(str, sorted(os.sched_getaffinity(0))[:2])))')}"
taskset -c "$cpus" python3 -c 'import os, sys; sys.exit(0 if len(os.sched_getaffinity(0)) == 2 else "Select exactly two available CPUs.")'
printf 'Identity performance fixture: PostgreSQL 18.7, logical CPUs %s, Go memory target 256 MiB\n' "$cpus"
go version

work=$(mktemp -d /tmp/mamony-identity-perf-XXXXXXXX)
cleanup() {
  if [[ -s "$work/data/postmaster.pid" ]]; then
    pg_ctl -D "$work/data" -m fast -w -t 15 stop >/dev/null || {
      printf 'PostgreSQL did not stop. Retaining disposable workspace %s\n' "$work" >&2
      return 1
    }
  fi
  rm -rf -- "$work"
}
trap cleanup EXIT

# Use only explicit local connection settings for this new cluster.
for variable in ${!PG@}; do
  unset "$variable"
done
admin_password=$(python3 -c 'import secrets; print(secrets.token_hex(24))')
printf '%s\n' "$admin_password" > "$work/password"
initdb -D "$work/data" -U mamony_test_admin --pwfile="$work/password" \
  --auth=scram-sha-256 --no-locale --encoding=UTF8 >/dev/null
rm -- "$work/password"
port=$(python3 - <<'PY'
import socket
with socket.socket() as sock:
    sock.bind(("127.0.0.1", 0))
    print(sock.getsockname()[1])
PY
)
if ! taskset -c "$cpus" pg_ctl -D "$work/data" -l "$work/postgres.log" -w -t 20 start \
  -o "-h 127.0.0.1 -p $port -c unix_socket_directories= -c shared_buffers=32MB -c max_connections=20 -c jit=off" >/dev/null; then
  cat -- "$work/postgres.log" >&2
  exit 1
fi

psql_admin() {
  PGPASSWORD="$admin_password" psql -X -v ON_ERROR_STOP=1 -h 127.0.0.1 -p "$port" -U mamony_test_admin "$@"
}
if [[ "$(psql_admin -d postgres -Atc 'SHOW data_directory')" != "$work/data" ]]; then
  printf 'The database server does not match the disposable workspace.\n' >&2
  exit 1
fi
psql_admin -d postgres -c 'CREATE DATABASE mamony_test_identity_perf' >/dev/null
psql_admin -d mamony_test_identity_perf -f tools/postgrestest/testdata/identity-bootstrap.sql >/dev/null
unset admin_password

target="127.0.0.1:$port/mamony_test_identity_perf?sslmode=disable"
MAMONY_IDENTITY_MIGRATION_DSN="postgres://mamony_identity_migrator:local-test-identity-migrator@$target" \
  MAMONY_IDENTITY_MIGRATIONS_DIR=db/migrations/identity \
  go run ./cmd/mamony-identity-migrate
export TEST_IDENTITY_RUNTIME_URL="postgres://mamony_identity_runtime:local-test-identity-runtime@$target"
export TEST_IDENTITY_VERIFIER_URL="postgres://mamony_identity_verifier_runtime:local-test-identity-verifier@$target"
export TEST_IDENTITY_AUTHENTICATOR_URL="postgres://mamony_identity_authenticator_runtime:local-test-identity-authenticator@$target"
export GOMAXPROCS=2 GOMEMLIMIT=256MiB

go test -tags integration,identity_perf -c -o "$work/login-perf.test" \
  ./internal/modules/identity/internal/integration
python3 - "$cpus" "$work" "$profile" <<'PY'
import pathlib
import resource
import subprocess
import sys

cpus, directory, profile = sys.argv[1:]
work = pathlib.Path(directory)
binary = work / "login-perf.test"
block = work / "block.pprof"
command = ["taskset", "-c", cpus, str(binary), "-test.v",
           "-test.run", "^TestIdentityLoginPerformance$", "-test.timeout", "90s"]
if profile == "1":
    command.extend(["-test.blockprofile", str(block), "-test.blockprofilerate", "1"])
subprocess.run(command, check=True)
print(f"identity_login_perf process_peak_rss_kib={resource.getrusage(resource.RUSAGE_CHILDREN).ru_maxrss}",
      flush=True)
if profile == "1":
    subprocess.run(["go", "tool", "pprof", "-top", "-cum", "-nodecount=0",
                    "-focus=PasswordHasher.*acquire", str(binary), str(block)], check=True)
PY
