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

NPM ?= npm

# A throwaway deployment to click around in. Everything about it is
# overridable, because the two settings people get wrong are the host they
# browse to and who they arrive as.
#
#   DEMO_HOST   what you type in the browser. Must match what the forgery
#               guard is told to expect, which is why it appears three times
#               below rather than once
#   DEMO_USER   who you arrive as. The trusted-header path prefixes it, so the
#               administrator is proxy:$(DEMO_USER)
DEMO_HOST ?= localhost
DEMO_PORT ?= 5173
DEMO_API  ?= 127.0.0.1:8080
DEMO_USER ?= dev
DEMO_DIR  ?= $(HOME)/openpsirt-dev
DEMO_DB   ?= $(DEMO_DIR)/dev.db
DEMO_URL  := http://$(DEMO_HOST):$(DEMO_PORT)

.PHONY: unreachable all build test vet lint fmt openapi openapi-current run clean check check-packaging tools govulncheck licenses sbom web web-deps web-api web-check demo demo-up demo-down demo-seed demo-reset demo-status

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
check: build vet lint unreachable test govulncheck licenses openapi-current sbom web-check

# The interface, built into the directory the binary embeds. Kept out of
# "build" so a checkout with no node toolchain still produces a working
# API-only binary — the embed tolerates an empty directory on purpose.
web: web-deps
	$(NPM) --prefix web run build
	# The build output only. The directory itself and its two dotfiles are
	# tracked, because the embed needs the directory to exist in a fresh
	# clone — removing the directory wholesale would delete them.
	rm -rf internal/webui/dist/assets internal/webui/dist/index.html
	cp -r web/dist/. internal/webui/dist/
	@git check-ignore -q internal/webui/dist/index.html \
	  || { echo "internal/webui/dist is not ignored: build output would be committed"; exit 1; }

# Reproducible, like every other dependency here: npm ci installs exactly what
# the lockfile pins rather than re-resolving ranges at build time.
web-deps:
	$(NPM) --prefix web ci

# The client is generated from the committed document (UIX-19), so a drifted
# document is a compile error in the interface rather than a runtime surprise.
web-api: openapi
	$(NPM) --prefix web run api
	@git diff --exit-code -- web/src/api/schema.d.ts \
	  || { echo "web/src/api/schema.d.ts is stale: run make web-api and commit it"; exit 1; }

# What CI runs for the interface. Skipped with a note rather than failing where
# there is no node, so the Go half still gates on a machine without it.
web-check:
	@command -v $(NPM) >/dev/null 2>&1 \
	  || { echo "npm not found, so the interface is unchecked here"; exit 0; }
	$(MAKE) web-deps
	$(NPM) --prefix web run typecheck
	$(NPM) --prefix web test
	$(MAKE) web-api

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

# Bring up something to look at, seed it, and say where to go.
demo: build web demo-up demo-seed demo-status

demo-up: demo-down
	@mkdir -p $(DEMO_DIR)
	@OPENPSIRT_DATABASE_URL="sqlite://$(DEMO_DB)" \
	 OPENPSIRT_ADDR="$(DEMO_API)" \
	 OPENPSIRT_PLAIN_HTTP=1 \
	 OPENPSIRT_BOOTSTRAP_ADMINS="proxy:$(DEMO_USER)" \
	 OPENPSIRT_TRUSTED_HEADER="X-User" \
	 OPENPSIRT_TRUSTED_SOURCES="127.0.0.0/8" \
	 OPENPSIRT_BASE_URL="$(DEMO_URL)" \
	 nohup ./$(BIN) > $(DEMO_DIR)/api.log 2>&1 & echo $$! > $(DEMO_DIR)/api.pid
	@OPENPSIRT_DEV_USER="$(DEMO_USER)" \
	 OPENPSIRT_DEV_HOSTS="$(DEMO_HOST)" \
	 nohup $(NPM) --prefix web run dev -- --host --port $(DEMO_PORT) \
	   > $(DEMO_DIR)/web.log 2>&1 & echo $$! > $(DEMO_DIR)/web.pid
	@sleep 6
	@echo "api  $(DEMO_API)   log $(DEMO_DIR)/api.log"
	@echo "web  $(DEMO_URL)   log $(DEMO_DIR)/web.log"

demo-down:
	@-pkill -f "$(BIN)" 2>/dev/null || true
	@-pkill -f "vite.*--port $(DEMO_PORT)" 2>/dev/null || true
	@rm -f $(DEMO_DIR)/api.pid $(DEMO_DIR)/web.pid
	@sleep 1

# Declares somewhere to file scans against and sends the full-size fixture.
# Idempotent: declaring something that exists succeeds and changes nothing, so
# this can be run again without tearing anything down.
demo-seed:
	@command -v xz >/dev/null || { echo "xz is needed to read the fixture"; exit 1; }
	@xz -dc internal/sbom/testdata/switch-image.cdx.json.xz > $(DEMO_DIR)/image.cdx.json
	@for spec in \
	  '/v1/products|{"name":"sonic","display_name":"SONiC"}' \
	  '/v1/products/sonic/streams|{"name":"master","kind":"branch"}' \
	  '/v1/products/sonic/variants|{"name":"broadcom","customer_facing":true}'; do \
	  path=$${spec%%|*}; body=$${spec#*|}; \
	  curl -sS --noproxy '*' -o /dev/null -w "  $$path %{http_code}\n" \
	    -X POST -H "X-User: $(DEMO_USER)" -H "Origin: $(DEMO_URL)" \
	    -H 'Content-Type: application/json' -d "$$body" \
	    "http://$(DEMO_API)$$path"; \
	done
	@curl -sS --noproxy '*' -o /dev/null -w "  upload %{http_code}\n" \
	  -X POST -H "X-User: $(DEMO_USER)" -H "Origin: $(DEMO_URL)" \
	  -F "inventory=@$(DEMO_DIR)/image.cdx.json" \
	  "http://$(DEMO_API)/v1/products/sonic/streams/master/variants/broadcom/scans"
	@echo "  the scan runs in the background; make demo-status shows when it lands"

demo-status:
	@printf "  scan   "; curl -sS --noproxy '*' -H "X-User: $(DEMO_USER)" \
	  "http://$(DEMO_API)/v1/products/sonic/streams/master/variants/broadcom/scans" \
	  | sed -e 's/.*"state":"\([a-z]*\)".*/\1/' -e 's/^{.*/no scans yet/'
	@printf "  open   "; curl -sS --noproxy '*' -H "X-User: $(DEMO_USER)" \
	  "http://$(DEMO_API)/v1/products/sonic/streams/master/variants/broadcom/findings?limit=1" \
	  | sed -e 's/.*"total":\([0-9]*\).*/\1 findings/' -e 's/^{.*/unreadable/'
	@echo "  open $(DEMO_URL) — you arrive as proxy:$(DEMO_USER), no sign-in"

# Start over. The database is the only state, so removing it is the whole of it.
demo-reset: demo-down
	@rm -f $(DEMO_DB)
	@echo "  removed $(DEMO_DB)"

clean-web:
	rm -rf web/node_modules web/dist internal/webui/dist/assets internal/webui/dist/index.html

clean:
	rm -rf bin
