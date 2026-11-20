.PHONY: check fmt-check vet unit architecture docs skills research tools-policy archlint test-report api-compat build race lint security run

check: fmt-check vet unit architecture docs skills research tools-policy build

fmt-check:
	@out="$$(gofmt -l $$(find . -name '*.go' -not -path './vendor/*'))"; if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

vet:
	go vet ./...

unit:
	go test ./...

architecture:
	go run ./tools/archcheck

docs:
	go run ./tools/doccheck

skills:
	python3 tools/skillcheck.py
	python3 -m unittest discover -s tools -p 'test_*.py'

research:
	python3 tools/researchcheck.py

tools-policy:
	python3 tools/toolingcheck.py

archlint:
	@command -v go-arch-lint >/dev/null || { echo 'NOT RUN: install github.com/fe3dback/go-arch-lint@v1.19.2'; exit 2; }
	go-arch-lint check --project-path .

test-report:
	@command -v gotestsum >/dev/null || { echo 'NOT RUN: install gotest.tools/gotestsum@v1.13.0'; exit 2; }
	@mkdir -p test-reports
	gotestsum --format pkgname --junitfile test-reports/unit.xml --jsonfile test-reports/unit.json -- ./...

api-compat:
	bash scripts/check-openapi-compat.sh

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
