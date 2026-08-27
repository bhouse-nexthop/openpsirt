# Everything CI runs is reachable from here, so a developer can reproduce any
# failure with one command using the same versions.

GO           ?= go
BIN          := bin/openpsirt
VERSION      ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT       ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE         ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG          := github.com/bhouse-nexthop/openpsirt/internal/version
LDFLAGS      := -s -w \
	-X '$(PKG).version=$(VERSION)' \
	-X '$(PKG).commit=$(COMMIT)' \
	-X '$(PKG).date=$(DATE)'

GOLANGCI_VERSION  ?= v2.13.1
GOVULNCHECK_VERSION ?= latest
GOLICENSES_VERSION  ?= latest

# Permissive only, for anything that ships. Build tooling is unrestricted.
ALLOWED_LICENCES := Apache-2.0,BSD-2-Clause,BSD-3-Clause,ISC,MIT,MPL-2.0

.PHONY: all build test vet lint fmt openapi run clean check tools govulncheck licences

all: check build

build:
	@mkdir -p bin
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/openpsirt

test:
	$(GO) test -race -count=1 ./...

vet:
	$(GO) vet ./...

lint:
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION) run

fmt:
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION) fmt

govulncheck:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

licences:
	$(GO) run github.com/google/go-licenses@$(GOLICENSES_VERSION) check ./... \
		--allowed_licenses=$(ALLOWED_LICENCES)

# The document is generated from the running registrations, never hand-written.
openapi:
	@mkdir -p docs/reference
	$(GO) run ./cmd/openpsirt -openapi > docs/reference/openapi.yaml
	@echo "wrote docs/reference/openapi.yaml"

check: vet lint test govulncheck licences

run:
	$(GO) run ./cmd/openpsirt

clean:
	rm -rf bin
