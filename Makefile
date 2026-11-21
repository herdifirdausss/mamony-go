.PHONY: check fmt-check vet unit architecture docs skills archlint test-report build race lint security run test-repeat coverage integration

check: fmt-check vet unit architecture docs skills build

fmt-check:
	@out="$$(gofmt -l $$(find . -name '*.go' -not -path './vendor/*'))"; if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

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

# Fail closed until a real, disposable PostgreSQL integration harness is
# implemented and a CI job verifies that its tests actually execute.
# File existence and TEST_DATABASE_URL alone cannot prove any test ran.
integration:
	@echo 'NOT RUN: PostgreSQL test harness and enforced integration CI are not implemented'; exit 2

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
