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

# Every tool is pinned. "latest" resolves at run time, so CI would not be
# reproducible against last week's run and a compromised upstream release would
# execute here on the day it shipped.
GOLANGCI_VERSION    ?= v2.13.1
GOVULNCHECK_VERSION ?= v1.7.0
GOLICENSES_VERSION  ?= v1.6.0
CDXGOMOD_VERSION    ?= v1.12.0

# Permissive only, for anything that ships. Build tooling is unrestricted.
ALLOWED_LICENSES := Apache-2.0,BSD-2-Clause,BSD-3-Clause,ISC,MIT,MPL-2.0

# Modules the classifier cannot read, whose license has been checked by hand.
# Each entry needs a reason. Lowering the confidence threshold instead would
# silently accept every other unreadable license too, which is the opposite of
# what this check is for.
#
#   modernc.org/mathutil  BSD-3-Clause. Verified by reading LICENSE: three
#                         clauses and the standard disclaimer. The classifier
#                         fails on its wording, which says "neither the names
#                         of the authors" where the canonical text says
#                         "neither the name of the copyright holder".
LICENSE_EXCEPTIONS := modernc.org/mathutil

.PHONY: unreachable all build test vet lint fmt openapi openapi-current run clean check check-packaging tools govulncheck licenses sbom

all: check build

build:
	@mkdir -p bin
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/openpsirt

# One package at a time. Packages otherwise run in parallel against the same
# database server, and the rollback test drops every table while another
# package is using them. Isolating each package into its own database would
# also work; serializing is one flag, and the suite is seconds long.
test:
	$(GO) test -race -count=1 -p 1 ./...

vet:
	$(GO) vet ./...

lint:
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION) run

fmt:
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION) fmt

govulncheck:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

licenses:
	$(GO) run github.com/google/go-licenses@$(GOLICENSES_VERSION) check ./... \
		--allowed_licenses=$(ALLOWED_LICENSES) \
		$(foreach m,$(LICENSE_EXCEPTIONS),--ignore=$(m))

# We ingest SBOMs, so we publish one for ourselves. CycloneDX because that is
# the format this project treats as authoritative on the way in.
sbom:
	@mkdir -p bin
	$(GO) run github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@$(CDXGOMOD_VERSION) \
		app -main cmd/openpsirt -json -packages -output bin/openpsirt.cdx.json .
	@echo "wrote bin/openpsirt.cdx.json"

# The document is generated from the running registrations, never hand-written.
openapi:
	@mkdir -p docs/reference
	$(GO) run ./cmd/openpsirt -openapi > docs/reference/openapi.yaml
	@echo "wrote docs/reference/openapi.yaml"

# Everything CI runs, reachable from one command. Container and chart checks
# are included because CI runs them; omitting them meant four of nine jobs
# could not be reproduced locally.
check: build vet lint unreachable test govulncheck licenses openapi-current sbom

# Exported code nothing reaches. The analysis gate only reports unexported
# symbols, which left ten real defects invisible in one review — a store method
# with no route to it, a renderer nothing rendered with, a rule enforced in a
# second place nothing called. Every one looked like working code.
unreachable:
	$(GO) run ./internal/tools/unreachable

# CI fails when the committed document has drifted, so check the same thing.
openapi-current: openapi
	@git diff --exit-code -- docs/reference/openapi.yaml \
	  || { echo "docs/reference/openapi.yaml is stale: commit the regenerated file"; exit 1; }

# Requires docker and helm. Skipped by "check" so that a machine without them
# can still run everything else.
check-packaging:
	docker build -q -t openpsirt:check . >/dev/null
	docker run --rm openpsirt:check -version
	@test "$$(docker run --rm --entrypoint id openpsirt:check -u)" != "0" \
	  || { echo "image runs as root"; exit 1; }
	@docker run --rm --entrypoint /usr/local/bin/grype openpsirt:check version >/dev/null \
	  || { echo "image carries no working scanner, so it could ingest and never scan"; exit 1; }
	helm lint deploy/helm/openpsirt --set database.url=postgres://u:p@h:5432/d \
	  --set auth.bootstrapAdmins='{admin}' --set auth.trustedHeader.name=X-Forwarded-User \
	  --set auth.trustedHeader.sources='{10.0.0.0/8}'
	helm template t deploy/helm/openpsirt --set database.existingSecret=s \
	  --set auth.bootstrapAdmins='{admin}' --set auth.baseURL=https://psirt.example.com \
	  --set auth.oidc.issuer=https://id.example.com --set auth.oidc.clientID=abc \
	  --set auth.oidc.clientSecret=shh >/dev/null
	# An install that cannot reach a login is not an install. Each of these
	# refuses at template time rather than producing a deployment that starts,
	# fails its own administration check, and crash-loops with the reason in a
	# log nobody is watching yet.
	@for missing in \
	  "nobody can administer:--set database.existingSecret=s" \
	  "no way to sign in:--set database.existingSecret=s --set auth.bootstrapAdmins={admin}" \
	  "no address to return to:--set database.existingSecret=s --set auth.bootstrapAdmins={admin} --set auth.oidc.issuer=https://id.example.com" \
	  "a header anybody can set:--set database.existingSecret=s --set auth.bootstrapAdmins={admin} --set auth.trustedHeader.name=X-User"; do \
	  what="$${missing%%:*}"; args="$${missing#*:}"; \
	  if helm template t deploy/helm/openpsirt $$args >/dev/null 2>&1; then \
	    echo "the chart accepted an install with $$what"; exit 1; \
	  fi; \
	done
	@echo "the chart refuses every install that could not be signed into"

run:
	$(GO) run ./cmd/openpsirt

clean:
	rm -rf bin
