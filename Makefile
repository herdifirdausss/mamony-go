.PHONY: check fmt-check vet unit architecture docs skills archlint test-report build race lint security run test-repeat coverage integration integration-local migrations-check migrate-platform migrate-identity

check: fmt-check vet unit architecture docs skills migrations-check build


fmt-check:
	@out="$(gofmt -l $(find . -name '*.go' -not -path './vendor/*'))"; if [ -n "$out" ]; then echo "gofmt needed:"; echo "$out"; exit 1; fi

vet:
	go vet ./...

unit:
	go test ./...

# Re-run tests with randomized order to expose hidden shared state; no coverage claim.
test-repeat:
	go test -shuffle=on -count=3 ./...

# Coverage is diagnostic evidence, never a substitute for negative behavior tests.
coverage:
	@mkdir -p test-reports
	go test -coverprofile=test-reports/coverage.out ./...
	go tool cover -func=test-reports/coverage.out

# Run only tagged PostgreSQL tests with strict URL guards and nonempty PASS
# evidence from the exact required test names (not a package-level PASS).
integration:
	go run ./tools/testgate

# Start a disposable, loopback-only PostgreSQL container and clean it up.
integration-local:
	sh tools/postgrestest/run-local.sh

architecture:
	go run ./tools/archcheck

docs:
	go run ./tools/doccheck

skills:
	python3 tools/skillcheck.py
	python3 -B -m unittest discover -s tools -p 'test_*.py'

archlint:
	@command -v go-arch-lint >/dev/null || { echo 'NOT RUN: install github.com/fe3dback/go-arch-lint@v1.19.2'; exit 2; }
	go-arch-lint check --project-path .

test-report:
	@command -v gotestsum >/dev/null || { echo 'NOT RUN: install gotest.tools/gotestsum@v1.13.0'; exit 2; }
	@mkdir -p test-reports
	gotestsum --format pkgname --junitfile test-reports/unit.xml --jsonfile test-reports/unit.json -- ./...

build:
	go build ./...
	go mod verify
	go mod tidy -diff

race:
	go test -race ./...

lint:
	@command -v golangci-lint >/dev/null || { echo 'NOT RUN: install golangci-lint v2'; exit 2; }
	golangci-lint config verify
	golangci-lint run ./...

security:
	@command -v govulncheck >/dev/null || { echo 'NOT RUN: install govulncheck'; exit 2; }
	govulncheck ./...

run:
	go run ./cmd/mamony

# Static verification of committed SQL migration trailers (no DB required).
migrations-check:
	go test -count=1 -run TestRepositoryMigrationFiles ./internal/platform/migration

# Production migrations run separately from the HTTP process with dedicated role.
# MAMONY_MIGRATION_DSN must come from a provisioned migration secret source.
migrate-platform:
	go run ./cmd/mamony-migrate

# Identity migrations use separate credentials and never run at HTTP startup.
migrate-identity:
	go run ./cmd/mamony-identity-migrate
