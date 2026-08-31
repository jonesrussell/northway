GOVULNCHECK_VERSION := v1.7.0
IMAGE ?= northway:local
VERSION ?= dev
REVISION ?= $(shell git rev-parse HEAD)$(if $(shell git status --porcelain --untracked-files=normal),-dirty)
BUILD_FLAGS = -trimpath -buildvcs=false -ldflags="-s -w -X github.com/jonesrussell/northway/internal/app.Version=$(VERSION) -X github.com/jonesrussell/northway/internal/app.Revision=$(REVISION)"

.PHONY: check contracts fmt fmt-check vet test integration race boundaries build arm64 vuln smoke container-smoke generate generate-check licenses licenses-check

check: contracts fmt-check vet test boundaries generate-check licenses-check build arm64 smoke

generate:
	python3 scripts/sqlc.py generate
generate-check:
	python3 scripts/sqlc.py check
licenses:
	python3 scripts/collect_licenses.py
licenses-check:
	python3 scripts/collect_licenses.py --check
integration:
	go test -count=1 -timeout=60s ./internal/sqlite

contracts:
	python3 scripts/validate_contracts.py
fmt:
	go fmt ./...
fmt-check:
	python3 scripts/check_format.py
vet:
	go vet ./...
test:
	go test -count=1 -timeout=60s ./...
race:
	go test -race -count=1 -timeout=60s ./...
boundaries:
	python3 -m unittest discover -s scripts -p 'test_check_boundaries.py'
	python3 scripts/check_boundaries.py
build:
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -o bin/northway ./cmd/northway
arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(BUILD_FLAGS) -o bin/northway-linux-arm64 ./cmd/northway
vuln:
	go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

smoke: build
	python3 scripts/smoke_runtime.py
	python3 scripts/smoke_storage.py

container-smoke:
	python3 scripts/smoke_container.py "$(IMAGE)" "$(REVISION)"
	python3 scripts/smoke_storage_container.py "$(IMAGE)" "$(REVISION)"
