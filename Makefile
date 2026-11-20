.PHONY: check fmt-check vet unit architecture docs skills build race lint security run

check: fmt-check vet unit architecture docs skills build

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
