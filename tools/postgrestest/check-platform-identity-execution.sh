#!/usr/bin/env bash
# CI-only isolated application test; never connect to an existing DB/container.
set -euo pipefail
umask 077
[[ "${GITHUB_ACTIONS:-}" == true && $# == 0 ]] || exit 1
work=$(mktemp -d)
id=''
cleanup() {
  if [[ "$id" =~ ^[a-f0-9]{64}$ ]]; then
    docker rm -f -v "$id" >/dev/null 2>&1 || true
  fi
  rm -rf -- "$work"
}
trap cleanup EXIT
CGO_ENABLED=0 go build -o "$work/migrate" ./cmd/mamony-identity-migrate
CGO_ENABLED=0 go test -tags integration -c -o "$work/identity.test" ./internal/modules/identity/internal/integration
id=$(timeout 90 docker run --rm -d --network none --label mamony.synthetic-rh03-exec=true \
  -e POSTGRES_DB=mamony_test_rh03_exec_source -e POSTGRES_USER=mamony_test_admin \
  -e POSTGRES_PASSWORD=local-test-rh03-exec-admin postgres:18.7 \
  -c archive_mode=on -c wal_level=replica \
  -c 'archive_command=mkdir -p /tmp/mamony-rh03-exec-archive && cp %p /tmp/mamony-rh03-exec-archive/%f.tmp && mv /tmp/mamony-rh03-exec-archive/%f.tmp /tmp/mamony-rh03-exec-archive/%f')
[[ "$id" =~ ^[a-f0-9]{64}$ ]] || exit 1
[[ "$(docker inspect -f '{{.HostConfig.NetworkMode}}|{{.Config.Image}}|{{index .Config.Labels "mamony.synthetic-rh03-exec"}}' "$id")" == 'none|postgres:18.7|true' ]] || exit 1
ready=false
for ((i=0; i<60; i++)); do
  if timeout 5 docker exec "$id" pg_isready -h 127.0.0.1 -U mamony_test_admin -d mamony_test_rh03_exec_source >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 1
done
[[ "$ready" == true ]] || exit 1
timeout 90 docker exec -i "$id" psql -U mamony_test_admin -d mamony_test_rh03_exec_source -Xq -v ON_ERROR_STOP=1 < tools/postgrestest/testdata/identity-bootstrap.sql
docker cp "$work/migrate" "$id:/tmp/rh03-migrate"
docker cp "$work/identity.test" "$id:/tmp/rh03-identity.test"
docker cp db/migrations/identity "$id:/tmp/rh03-migrations"
timeout 150 docker exec -u postgres \
  -e MAMONY_IDENTITY_MIGRATION_DSN=postgres://mamony_identity_migrator:local-test-identity-migrator@127.0.0.1:5432/mamony_test_rh03_exec_source?sslmode=disable \
  -e MAMONY_IDENTITY_MIGRATIONS_DIR=/tmp/rh03-migrations \
  "$id" /tmp/rh03-migrate
timeout 205 docker exec -u postgres \
  -e GITHUB_ACTIONS=true -e MAMONY_RH03_EXECUTION_FIXTURE=true \
  -e TEST_DATABASE_ADMIN_URL=postgres://mamony_test_admin:local-test-rh03-exec-admin@127.0.0.1:5432/mamony_test_rh03_exec_source?sslmode=disable \
  "$id" /tmp/rh03-identity.test -test.run '^TestIdentityRH03RecoveredPasswordBinding$' -test.v -test.timeout=190s
