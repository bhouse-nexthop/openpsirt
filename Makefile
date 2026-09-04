# Everything CI runs is reachable from here, so a developer can reproduce any
# failure with one command using the same versions.

# bash with pipefail, because a recipe that pipes takes the *last* command's
# status by default. "go test | tee" therefore reported the status of tee,
# which is always 0 — so the engines gate announced "every engine ran" while
# the suite it had just run was failing. That is the exact shape of bug the
# gate exists to catch, in the gate itself.
SHELL       := /usr/bin/env bash
.SHELLFLAGS := -eo pipefail -c

# Machine-local settings, chiefly the database URLs below. Ignored by git, so
# a checkout with one is not different from a checkout without one. Absent is
# fine: the dash means "if it exists".
-include local.mk

GO           ?= go
# The engines the suite runs against. Set in local.mk or in the environment;
# exported so they reach the test process either way, since a make variable is
# not an environment variable and the suite reads the environment.
#
# Unset means SQLite alone, which passes every portability trap the design is
# written around by never reaching one. "check" says so rather than letting a
# one-engine pass read as a four-engine pass; "check-engines" refuses.
export OPENPSIRT_TEST_POSTGRES_URL
export OPENPSIRT_TEST_MYSQL_URL
export OPENPSIRT_TEST_MARIADB_URL
export OPENPSIRT_TEST_TOO_OLD_URL
ENGINES_SET := $(strip $(OPENPSIRT_TEST_POSTGRES_URL)$(OPENPSIRT_TEST_MYSQL_URL)$(OPENPSIRT_TEST_MARIADB_URL))
# Named one at a time, because "all three are missing" and "one is missing" are
# different states and only the first used to be reported. With postgres alone
# configured the warning stayed silent and the run tested two engines of four.
ENGINES_MISSING := $(strip \
  $(if $(OPENPSIRT_TEST_POSTGRES_URL),,postgres) \
  $(if $(OPENPSIRT_TEST_MYSQL_URL),,mysql) \
  $(if $(OPENPSIRT_TEST_MARIADB_URL),,mariadb))

# The servers those URLs point at, as containers. Pinned for the same reason
# every tool below is, and to the versions CI runs: an engine that has drifted
# from CI's turns "it passed locally" into a different claim from "it passes".
# Two pinned lists in two files drift silently, so "engines-check" asserts they
# still agree rather than trusting that whoever moved one moved the other.
DOCKER               ?= docker
ENGINE_PG_IMAGE      ?= postgres:16-alpine
ENGINE_MYSQL_IMAGE   ?= mysql:8.4
ENGINE_MARIADB_IMAGE ?= mariadb:11.4
# Deliberately below the supported floor, so the refusal to run against an old
# server is exercised rather than skipped.
ENGINE_FLOOR_IMAGE   ?= postgres:13-alpine
ENGINE_PG_PORT       ?= 5432
ENGINE_MYSQL_PORT    ?= 3306
ENGINE_MARIADB_PORT  ?= 3307
ENGINE_FLOOR_PORT    ?= 5433
ENGINE_PREFIX        ?= openpsirt
# Seconds to wait for a server to start accepting connections. A container
# reported "Up" is not one that answers yet, and MySQL takes the longest.
ENGINE_WAIT          ?= 120

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
#   DEMO_HOST   where this waits for the container to answer, and what the
#               status line prints. Not what you have to type: the demo is not
#               told a base address, so it answers on whatever name you reach
#               it by
#   DEMO_USER   who you arrive as. The trusted-header path prefixes it, so the
#               administrator is proxy:$(DEMO_USER)
DEMO_HOST ?= localhost
DEMO_PORT ?= 8080
DEMO_USER ?= dev
# The rest of the cast, one per line as port:identity:roles.
#
# **One person cannot demonstrate this tool.** Approving your own claim is
# refused, because a control one person completes alone is not one (TRI-41) —
# so a demo with a single identity can propose a judgment and can never show it
# agreed to, and the record an auditor reads says "same person" against every
# row. Somebody has to be somebody else.
#
# A port each rather than a switcher: the trusted-header path is a proxy
# stating who you are, and the smallest honest version of two people is two
# doors. Open two browser windows and you are two people, which is also what
# lets one of them watch the other's claim arrive.
#
# Roles are granted per product, so the seed grants each of these on every
# product it declares. Nobody is an administrator but the first: an approver
# who could also change the settings would not be exercising anything.
DEMO_CAST ?= 8081:ana:public-read,public-triage,approver \
             8082:ben:public-read,public-triage,approver
# What the demo and the dev loop seed, one build per entry:
#   inventory,product,display name,branch,variant
# The inventory is an .xz of a CycloneDX file under the SBOM package's
# testdata. Adding a variant is adding a line — a second variant of the same
# product is what exercises decisions carrying across variants (REL-01,
# REL-09), so the mellanox build of the switch image belongs here once it is
# saved beside the broadcom one.
DEMO_BUILDS ?= internal/sbom/testdata/switch-image.cdx.json.xz,sonic,SONiC,master,broadcom \
               internal/sbom/testdata/switch-image-mellanox.cdx.json.xz,sonic,SONiC,master,mellanox
# In the tree rather than under $HOME: a command that writes to somebody's home
# directory from a checkout is a surprise, and everything here is throwaway
# state that should be deleted by deleting the checkout. Git-ignored.
DEMO_DIR   ?= $(CURDIR)/.demo
DEMO_IMAGE ?= openpsirt:demo
DEMO_NET   ?= openpsirt-demo
# Fixed so the application can be told which addresses to trust the sign-in
# header from. A range docker hands out at random cannot be named in advance.
DEMO_SUBNET ?= 172.31.71.0/24
DEMO_URL    := http://$(DEMO_HOST):$(DEMO_PORT)

# The developer loop is a different thing and says so. It runs the binary and
# the interface's own dev server on this machine, for the fast edit-and-reload
# cycle; it needs Go, node and a scanner installed here, and it serves the
# interface from the dev server rather than from the binary — so it exercises a
# configuration nobody deploys.
DEV_HOST ?= localhost
DEV_PORT ?= 5173
DEV_API  ?= 127.0.0.1:8081
DEV_DIR  ?= $(DEMO_DIR)/dev
DEV_DB   ?= $(DEV_DIR)/dev.db
DEV_URL  := http://$(DEV_HOST):$(DEV_PORT)

.PHONY: unreachable unclaimed all build test test-all vet lint fmt openapi openapi-current run clean check check-packaging check-engines measure engines-up engines-down engines-status engines-check tools govulncheck licenses sbom web web-deps web-api web-check demo demo-image demo-up demo-down demo-seed demo-reset demo-status dev dev-up dev-down dev-seed dev-reset dev-status

all: check build

build:
	@mkdir -p bin
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/openpsirt

# The quick loop: SQLite only, packages in parallel, the build cache on, no
# race detector. Seconds, so it is run after every change. The four-engine,
# race-detected, uncached run is test-all, and the gate uses that.
test:
	OPENPSIRT_TEST_ENGINES=sqlite $(GO) test ./...

# Every configured engine, race detector on, nothing cached. Packages run in
# parallel: each test binary gets a database of its own on every engine
# (internal/dbtest), so they share nothing.
test-all:
	$(GO) test -race -count=1 ./...

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
check: build vet lint unreachable unclaimed test-all govulncheck licenses openapi-current sbom web-check
ifneq ($(ENGINES_MISSING),)
	@echo
	@echo "NOT TESTED ON: $(ENGINES_MISSING). Those engines were not configured,"
	@echo "so nothing here exercised them. Run 'make check-engines' before committing."
endif

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

# Decisions no design document names. The chain that makes this auditable runs
# code to design document to decision, and nothing checked that it was whole:
# 71 decisions in force were named nowhere, five of them cited by code that
# runs. A decision not built yet is not exempt — its design document says so.
unclaimed:
	$(GO) run ./internal/tools/unclaimed

# CI fails when the committed document has drifted, so check the same thing.
openapi-current: openapi
	@git diff --exit-code -- docs/reference/openapi.yaml \
	  || { echo "docs/reference/openapi.yaml is stale: commit the regenerated file"; exit 1; }

# Everything CI's test job asserts beyond the suite passing.
#
# A skipped test passes, so "go test" being green does not mean an engine ran —
# it means nothing failed, which is also what a skip looks like. CI greps its
# own output for each engine by name; without the same check locally, "the
# suite is green" and "the suite ran" are two different facts with one command
# behind them. This is the command to run before committing.
check-engines:
ifneq ($(ENGINES_MISSING),)
	@echo "Not configured: $(ENGINES_MISSING). SQLite alone tests none of the"
	@echo "portability traps, so this refuses rather than passing. See AGENTS.md."
	@exit 1
endif
	@out=$$(mktemp); trap 'rm -f "$$out"' EXIT; \
	  $(GO) test ./internal/schema/ -count=1 -v -run TestMigrationsApplyOnEveryEngine \
	    > "$$out" 2>&1 || { cat "$$out"; exit 1; }; \
	  for engine in sqlite postgres mysql mariadb; do \
	    grep -q "PASS: TestMigrationsApplyOnEveryEngine/$$engine" "$$out" \
	      || { echo "$$engine did not run"; exit 1; }; \
	  done
	@out=$$(mktemp); trap 'rm -f "$$out"' EXIT; \
	  $(GO) test ./internal/database/migrate/ -count=1 -v -run TestLockExcludes \
	    > "$$out" 2>&1 || { cat "$$out"; exit 1; }; \
	  for engine in postgres mysql mariadb; do \
	    grep -q "PASS: TestLockExcludesAnotherConnection/$$engine" "$$out" \
	      || { echo "the migration lock was not exercised on $$engine"; exit 1; }; \
	  done
	@out=$$(mktemp); trap 'rm -f "$$out"' EXIT; \
	  $(GO) test ./internal/dbtest/ -count=1 -v -run TestEachEngineIsTheEngineItSaysItIs \
	    > "$$out" 2>&1 || { cat "$$out"; exit 1; }; \
	  for engine in sqlite postgres mysql mariadb; do \
	    grep -q "PASS: TestEachEngineIsTheEngineItSaysItIs/$$engine" "$$out" \
	      || { echo "$$engine was not checked for being itself"; exit 1; }; \
	  done
ifneq ($(strip $(OPENPSIRT_TEST_TOO_OLD_URL)),)
	@out=$$(mktemp); trap 'rm -f "$$out"' EXIT; \
	  $(GO) test ./internal/database/ -count=1 -v -run TestOpenRefuses \
	    > "$$out" 2>&1 || { cat "$$out"; exit 1; }; \
	  grep -q "PASS: TestOpenRefusesAServerBelowTheFloor" "$$out" \
	    || { echo "the version floor test did not run"; exit 1; }
	@echo "every engine ran, and each was the engine it claimed to be"
else
	@echo "note: OPENPSIRT_TEST_TOO_OLD_URL unset, so the version floor refusal"
	@echo "      was not exercised here. CI runs it against postgres:13."
	@echo "every engine ran, and each was the engine it claimed to be,"
	@echo "except the version floor, which is noted above as not run."
endif

# The four servers the URLs above point at, started, waited for, and written
# into local.mk. Running against every engine is ordinary development here
# rather than something CI does afterwards (DAT-12), so a machine that can run
# the suite properly is one command rather than a setup document followed by
# hand — and a document followed by hand is how three of four engines end up
# unconfigured while the suite reports green.
#
# Idempotent: an engine already running is left alone, and a stopped container
# is started rather than replaced, so whatever is in it survives.
engines-up: engines-check
	@set -u; \
	up() { \
	  name="$$1"; image="$$2"; ports="$$3"; shift 3; \
	  if [ -n "$$($(DOCKER) ps -q -f name="^$$name$$")" ]; then \
	    echo "  $$name: already running"; return 0; \
	  fi; \
	  if [ -n "$$($(DOCKER) ps -aq -f name="^$$name$$")" ]; then \
	    $(DOCKER) start "$$name" >/dev/null; echo "  $$name: started again"; return 0; \
	  fi; \
	  $(DOCKER) run -d --name "$$name" -p "$$ports" "$$@" "$$image" >/dev/null; \
	  echo "  $$name: created from $$image"; \
	}; \
	up $(ENGINE_PREFIX)-pg16 $(ENGINE_PG_IMAGE) $(ENGINE_PG_PORT):5432 \
	   -e POSTGRES_PASSWORD=test -e POSTGRES_DB=openpsirt; \
	up $(ENGINE_PREFIX)-mysql $(ENGINE_MYSQL_IMAGE) $(ENGINE_MYSQL_PORT):3306 \
	   -e MYSQL_ROOT_PASSWORD=test -e MYSQL_DATABASE=openpsirt; \
	up $(ENGINE_PREFIX)-mariadb $(ENGINE_MARIADB_IMAGE) $(ENGINE_MARIADB_PORT):3306 \
	   -e MARIADB_ROOT_PASSWORD=test -e MARIADB_DATABASE=openpsirt; \
	up $(ENGINE_PREFIX)-floor $(ENGINE_FLOOR_IMAGE) $(ENGINE_FLOOR_PORT):5432 \
	   -e POSTGRES_PASSWORD=test -e POSTGRES_DB=openpsirt
	@# A container reported "Up" is not one that answers. Each server is asked
	@# with its own client, inside its own container, so nothing here depends on
	@# a client being installed on this machine. MariaDB renamed mysqladmin to
	@# mariadb-admin, so the two lines differ by more than the server they name.
	@set -u; \
	ready() { \
	  name="$$1"; shift; waited=0; \
	  while [ "$$waited" -lt $(ENGINE_WAIT) ]; do \
	    if $(DOCKER) exec "$$name" "$$@" >/dev/null 2>&1; then \
	      echo "  $$name: answering"; return 0; \
	    fi; \
	    sleep 1; waited=$$((waited + 1)); \
	  done; \
	  echo "  $$name: no answer in $(ENGINE_WAIT)s. '$(DOCKER) logs $$name' says why."; \
	  return 1; \
	}; \
	ready $(ENGINE_PREFIX)-pg16 pg_isready -U postgres -d openpsirt; \
	ready $(ENGINE_PREFIX)-floor pg_isready -U postgres -d openpsirt; \
	ready $(ENGINE_PREFIX)-mysql mysqladmin ping -uroot -ptest --silent; \
	ready $(ENGINE_PREFIX)-mariadb mariadb-admin ping -uroot -ptest --silent
	@# Never overwritten. The file is machine-local and somebody may be pointing
	@# at servers of their own; silently replacing that is how a run tests
	@# something other than what its author thinks it is testing.
	@if [ -e local.mk ]; then \
	  echo; echo "local.mk already exists, so it was left alone."; \
	else \
	  printf '%s\n' \
	    "# Written by 'make engines-up'. Git-ignored, so a checkout with one is" \
	    "# not different from a checkout without one." \
	    "#" \
	    "# '?=' rather than ':=', so setting a URL in the environment for a" \
	    "# single run still wins: a makefile assignment beats an environment" \
	    "# variable, and the conditional form does not." \
	    "OPENPSIRT_TEST_POSTGRES_URL ?= postgres://postgres:test@127.0.0.1:$(ENGINE_PG_PORT)/openpsirt?sslmode=disable" \
	    "OPENPSIRT_TEST_MYSQL_URL    ?= mysql://root:test@127.0.0.1:$(ENGINE_MYSQL_PORT)/openpsirt" \
	    "OPENPSIRT_TEST_MARIADB_URL  ?= mariadb://root:test@127.0.0.1:$(ENGINE_MARIADB_PORT)/openpsirt" \
	    "# A server below the supported floor, so the refusal to run against one" \
	    "# is exercised rather than skipped." \
	    "OPENPSIRT_TEST_TOO_OLD_URL  ?= postgres://postgres:test@127.0.0.1:$(ENGINE_FLOOR_PORT)/openpsirt?sslmode=disable" \
	    > local.mk; \
	  echo; echo "wrote local.mk"; \
	fi
	@# The URLs are read when make starts, so this run still has none of them.
	@echo "The URLs are read at startup, so they reach the next command, not this one:"
	@echo "    make check && make check-engines"

# Stops them. The containers are removed rather than stopped, because a
# half-migrated database left behind by an interrupted run is a confusing thing
# to come back to; local.mk is left alone, since it is yours.
engines-down:
	@$(DOCKER) rm -f $(ENGINE_PREFIX)-pg16 $(ENGINE_PREFIX)-mysql \
	  $(ENGINE_PREFIX)-mariadb $(ENGINE_PREFIX)-floor >/dev/null 2>&1 || true
	@echo "removed. local.mk was left alone; 'make engines-up' reuses it."

engines-status:
	@$(DOCKER) ps -a --filter "name=^$(ENGINE_PREFIX)-" \
	  --format '{{.Names}}: {{.Image}}, {{.Status}}' | sed 's/^/  /' || true
ifeq ($(ENGINES_MISSING),)
	@echo "  configured: postgres, mysql, mariadb"
else
	@echo "  NOT configured: $(ENGINES_MISSING). The suite would skip those, and a"
	@echo "  skipped engine passes. Run 'make engines-up'."
endif
ifeq ($(strip $(OPENPSIRT_TEST_TOO_OLD_URL)),)
	@echo "  NOT configured: the below-floor server, so the version refusal is skipped."
endif

# That the engines started here are the engines CI runs. Two pinned lists in
# two files drift, and the drift is invisible exactly when it matters: a suite
# green against MariaDB 11.4 says nothing about the 11.8 CI moved to. Checked
# in both directions, so an engine CI adds and this does not start is caught
# as well as the reverse.
engines-check:
	@ours="$(ENGINE_PG_IMAGE) $(ENGINE_MYSQL_IMAGE) $(ENGINE_MARIADB_IMAGE) $(ENGINE_FLOOR_IMAGE)"; \
	theirs=$$(grep -oE '^[[:space:]]+image: [^[:space:]]+' .github/workflows/ci.yml \
	  | awk '{print $$2}' | sort -u); \
	for image in $$ours; do \
	  echo "$$theirs" | grep -qxF "$$image" \
	    || { echo "$$image is pinned here, and CI runs no such server."; exit 1; }; \
	done; \
	for image in $$theirs; do \
	  printf '%s\n' $$ours | grep -qxF "$$image" \
	    || { echo "CI runs $$image, and nothing here starts one."; exit 1; }; \
	done

# Measurements, not gates.
#
# These take minutes, assert almost nothing, and produce numbers to write down.
# They are behind a build tag so that "check" never runs them and nobody has to
# decide whether a slow run is a failure — a decision here carries the
# measurement that forced it, and this is where those come from.
#
# Point it at a real server. SQLite answers a different question: one writer,
# one connection, and nothing a deployment runs on.
measure:
ifneq ($(ENGINES_MISSING),)
	@echo "Not configured: $(ENGINES_MISSING). A measurement taken on SQLite alone"
	@echo "describes one writer on one connection, which is not what a deployment"
	@echo "runs — so this refuses rather than producing a number that reads like"
	@echo "four engines and is one. Run 'make engines-up'."
	@exit 1
endif
	@# -run has to match something. A renamed test or a mistyped tag makes
	@# "no tests to run" a green exit, which is the same green as a
	@# measurement nobody took.
	@out=$$(mktemp); trap 'rm -f "$$out"' EXIT; \
	  $(GO) test -tags measure -count=1 -v -timeout 60m \
	    -run 'TestMeasure' ./internal/finding/ 2>&1 | tee "$$out"; \
	  grep -q "^=== RUN   TestMeasure" "$$out" \
	    || { echo "no measurement ran: -run matched nothing"; exit 1; }

# Requires docker and helm. Skipped by "check" so that a machine without them
# can still run everything else.
check-packaging:
	docker build -q -t openpsirt:check . >/dev/null
	docker run --rm openpsirt:check -version
	@test "$$(docker run --rm --entrypoint id openpsirt:check -u)" != "0" \
	  || { echo "image runs as root"; exit 1; }
	@docker run --rm --entrypoint /usr/local/bin/grype openpsirt:check version >/dev/null \
	  || { echo "image carries no working scanner, so it could ingest and never scan"; exit 1; }
	@# That the image serves the interface, not merely that it starts. The Go
	@# build embeds a git-ignored directory, so an image built from a clean
	@# checkout carried no interface at all and every other check here passed.
	@set -e; \
	  id=$$(docker run -d --rm -p 127.0.0.1:0:8080 \
	         --tmpfs /tmp \
	         -e OPENPSIRT_DATABASE_URL="sqlite:///tmp/check.db" \
	         -e OPENPSIRT_ADDR="0.0.0.0:8080" \
	         -e OPENPSIRT_PLAIN_HTTP=1 \
	         -e OPENPSIRT_BOOTSTRAP_ADMINS=check \
	         openpsirt:check); \
	  trap 'docker rm -f $$id >/dev/null 2>&1 || true' EXIT; \
	  port=$$(docker port $$id 8080/tcp | head -1 | sed 's/.*://'); \
	  waited=0; \
	  until curl -fsS --noproxy '*' "http://127.0.0.1:$$port/readyz" >/dev/null 2>&1; do \
	    waited=$$((waited + 1)); \
	    [ $$waited -gt 60 ] && { echo "the image never became ready"; exit 1; }; \
	    sleep 1; \
	  done; \
	  curl -fsS --noproxy '*' "http://127.0.0.1:$$port/" | grep -qi '<!doctype html' \
	    || { echo "the image serves no interface: it was built without one"; exit 1; }
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
# Somewhere to click around in, as the thing that actually ships.
#
# One command, and the only thing it needs on this machine is docker: the image
# builds the interface and the binary inside itself, and carries the scanner.
# Testing a change is ordinary development, so standing an instance up has to be
# something anybody can do anywhere rather than a page of prerequisites.
#
# It is the real image behind a real proxy, which is the shape a deployment
# actually has (ACC-19, ACC-20) — not a development server standing in for one.
demo: demo-image demo-up demo-seed demo-status

# Built from the working tree, so what comes up is the change being tested.
demo-image:
	@command -v $(DOCKER) >/dev/null 2>&1 \
	  || { echo "$(DOCKER) is needed to run the demo"; exit 1; }
	@echo "building $(DEMO_IMAGE) — the interface and the binary build inside it"
	@$(DOCKER) build -q -t $(DEMO_IMAGE) \
	  --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) \
	  --build-arg DATE=$(DATE) --build-arg CDXGOMOD_VERSION=$(CDXGOMOD_VERSION) . >/dev/null

demo-up: demo-down
	@mkdir -p $(DEMO_DIR)/data $(DEMO_DIR)/grype
	@$(DOCKER) network create --subnet $(DEMO_SUBNET) $(DEMO_NET) >/dev/null 2>&1 || true
	@# The vulnerability database is a gigabyte and does not change hourly, so
	@# it is kept between runs. Removing it is "make demo-reset --hard", which
	@# is deliberately not a thing: deleting .demo does it.
	@#
	@# Run as the invoking user rather than the image's own account, because
	@# the two mounts below come from the host and would otherwise be owned by
	@# somebody the container is not.
	@# The scanner fetches its vulnerability database on first use, so a
	@# machine that reaches the network through a proxy has to say so —
	@# a container inherits nothing of the shell that started it.
	@#
	@# And loopback is always excluded from it. Everything in the image that
	@# speaks HTTP honors these, including the health check, so passing a
	@# proxy through without this sends the container's own probe of itself to
	@# the proxy — which answers 403, and the container reports unhealthy while
	@# serving every request correctly.
	@$(DOCKER) run -d --name openpsirt-demo --network $(DEMO_NET) \
	  --user "$$(id -u):$$(id -g)" \
	  -e NO_PROXY="127.0.0.1,localhost,$${NO_PROXY}" \
	  -e no_proxy="127.0.0.1,localhost,$${no_proxy}" \
	  $${HTTP_PROXY:+-e HTTP_PROXY="$$HTTP_PROXY"} \
	  $${HTTPS_PROXY:+-e HTTPS_PROXY="$$HTTPS_PROXY"} \
	  $${http_proxy:+-e http_proxy="$$http_proxy"} \
	  $${https_proxy:+-e https_proxy="$$https_proxy"} \
	  -v "$(DEMO_DIR)/data:/data" \
	  -v "$(DEMO_DIR)/grype:/var/cache/openpsirt/grype" \
	  -e OPENPSIRT_DATABASE_URL="sqlite:///data/dev.db" \
	  -e OPENPSIRT_ADDR="0.0.0.0:8080" \
	  -e OPENPSIRT_PLAIN_HTTP=1 \
	  -e OPENPSIRT_BOOTSTRAP_ADMINS="proxy:$(DEMO_USER)" \
	  -e OPENPSIRT_TRUSTED_HEADER="X-User" \
	  -e OPENPSIRT_TRUSTED_SOURCES="$(DEMO_SUBNET)" \
	  $(DEMO_IMAGE) >/dev/null
	@# Deliberately no OPENPSIRT_BASE_URL. It is what a sign-in provider sends
	@# somebody back to, and the demo has no provider — but it is also what the
	@# forgery guard compares a browser's origin against, so setting it pins
	@# the demo to one hostname. Reaching it as anything else then reads
	@# perfectly and refuses every write with "not authorized", which is the
	@# guard working and looks nothing like it: somebody gets all the way
	@# through a decision and loses it at submit. Without it the guard compares
	@# against the request's own Host, which is sound for exactly this — a
	@# hostile page cannot make a browser send another site's Host.
	@# What authenticates. A deployment puts this application behind something
	@# that has already identified the caller and states who they are in a
	@# header; this is the smallest honest version of that, rather than a mode
	@# in the application that trusts anybody — which is a hole nobody should
	@# ship even switched off by default.
	@# One server block per person, each on its own port inside the proxy, so
	@# a second browser window is a second person. Written by a loop because
	@# the cast is configuration: adding somebody is adding a line to
	@# DEMO_CAST, not editing this three times.
	@: > $(DEMO_DIR)/proxy.conf
	@for entry in $(DEMO_USER):80 $(foreach c,$(DEMO_CAST),$(word 2,$(subst :, ,$(c))):$(word 1,$(subst :, ,$(c)))); do \
	    who=$${entry%%:*}; port=$${entry##*:}; \
	    printf '%s\n' \
	      'server {' \
	      "  listen $$port;" \
	      '  location / {' \
	      '    proxy_pass http://openpsirt-demo:8080;' \
	      "    proxy_set_header X-User $$who;" \
	      '    # $$http_host, not $$host: nginx strips the port from $$host, so a' \
	      '    # browser at name:8080 reaches an application that thinks it answers' \
	      '    # to name. Reads work and every write is refused by the forgery' \
	      '    # guard, which compares the origin the browser states against where' \
	      '    # this deployment believes it answers.' \
	      '    proxy_set_header Host $$http_host;' \
	      '    proxy_set_header X-Forwarded-For $$remote_addr;' \
	      '    client_max_body_size 256m;' \
	      '    proxy_read_timeout 300s;' \
	      '  }' \
	      '}' >> $(DEMO_DIR)/proxy.conf; \
	  done
	@# Said plainly before docker says it obscurely. A port already taken comes
	@# back as "driver failed programming external connectivity", which names
	@# neither the port nor what holds it — and the cast's ports are ordinary
	@# numbers somebody else's container may well be on.
	@# The "|| true" is load-bearing: recipes run under "-eo pipefail", so a
	@# grep that matches nothing — a free port, the good case — would otherwise
	@# end the recipe right here, before a word of any of this is printed.
	@for want in $(DEMO_PORT) $(foreach c,$(DEMO_CAST),$(word 1,$(subst :, ,$(c)))); do \
	  held=$$($(DOCKER) ps --format '{{.Names}} {{.Ports}}' | grep ":$$want->" | cut -d' ' -f1 || true); \
	  if [ -n "$$held" ]; then \
	    echo "  port $$want is already held by $$held."; \
	    echo "  Stop it, or set DEMO_PORT / DEMO_CAST to ports that are free."; \
	    exit 1; \
	  fi; \
	done
	@$(DOCKER) run -d --name openpsirt-demo-proxy --network $(DEMO_NET) \
	  -p $(DEMO_PORT):80 \
	  $(foreach c,$(DEMO_CAST),-p $(word 1,$(subst :, ,$(c))):$(word 1,$(subst :, ,$(c))) ) \
	  -v "$(DEMO_DIR)/proxy.conf:/etc/nginx/conf.d/default.conf:ro" \
	  nginx:1.29-alpine >/dev/null
	@# A container reported "Up" is not one that answers, the same trap the
	@# database servers have.
	@waited=0; \
	  until curl -fsS --noproxy '*' "$(DEMO_URL)/readyz" >/dev/null 2>&1; do \
	    waited=$$((waited + 1)); \
	    if [ $$waited -gt 60 ]; then \
	      echo "  it never answered. '$(DOCKER) logs openpsirt-demo' says why."; \
	      exit 1; \
	    fi; \
	    sleep 1; \
	  done
	@echo "  up at $(DEMO_URL)"

demo-down:
	@-$(DOCKER) rm -f openpsirt-demo openpsirt-demo-proxy >/dev/null 2>&1 || true
	@-$(DOCKER) network rm $(DEMO_NET) >/dev/null 2>&1 || true

# Declares somewhere to file scans against and sends the full-size fixture.
# Idempotent: declaring something that exists succeeds and changes nothing, so
# this can be run again without tearing anything down.
demo-seed:
	@command -v xz >/dev/null || { echo "xz is needed to read the fixtures"; exit 1; }
	@for entry in $(DEMO_BUILDS); do \
	  IFS=',' read -r file product display stream variant <<< "$$entry"; \
	  xz -dc "$$file" > "$(DEMO_DIR)/$$product-$$stream-$$variant.cdx.json"; \
	  for spec in \
	    "/v1/products|{\"name\":\"$$product\",\"display_name\":\"$$display\"}" \
	    "/v1/products/$$product/streams|{\"name\":\"$$stream\",\"kind\":\"branch\"}" \
	    "/v1/products/$$product/variants|{\"name\":\"$$variant\",\"customer_facing\":true}"; do \
	    path=$${spec%%|*}; body=$${spec#*|}; \
	    curl -sS --noproxy '*' -o /dev/null -w "  $$path %{http_code}\n" \
	      -X POST -H "Origin: $(DEMO_URL)" \
	      -H 'Content-Type: application/json' -d "$$body" \
	      "$(DEMO_URL)$$path"; \
	  done; \
	  curl -sS --noproxy '*' -o /dev/null -w "  upload $$product/$$stream/$$variant %{http_code}\n" \
	    -X POST -H "Origin: $(DEMO_URL)" \
	    -F "inventory=@$(DEMO_DIR)/$$product-$$stream-$$variant.cdx.json" \
	    "$(DEMO_URL)/v1/products/$$product/streams/$$stream/variants/$$variant/scans"; \
	done
	@# A second product: this deployment itself, from the two inventories the
	@# image carries. Two products is what makes the cross-product screens mean
	@# anything, and this one costs nobody a build pipeline.
	@#
	@# Two variants, and the difference between them is the point. "binary" is
	@# what the program was linked from — every Go module, and nothing else.
	@# "container" is what the image actually ships: musl, busybox, the
	@# certificate bundle, the scanner that rides along, and the modules of
	@# both binaries. A tool whose subject is knowing what is inside what you
	@# ship should be able to show somebody that those are not the same list,
	@# on itself, on the first screen they open.
	@$(DOCKER) cp openpsirt-demo:/usr/share/openpsirt/openpsirt.cdx.json $(DEMO_DIR)/openpsirt-binary.cdx.json
	@$(DOCKER) cp openpsirt-demo:/usr/share/openpsirt/image.cdx.json $(DEMO_DIR)/openpsirt-container.cdx.json
	@for spec in \
	  '/v1/products|{"name":"openpsirt","display_name":"OpenPSIRT"}' \
	  '/v1/products/openpsirt/streams|{"name":"main","kind":"branch"}' \
	  '/v1/products/openpsirt/variants|{"name":"binary","customer_facing":true}' \
	  '/v1/products/openpsirt/variants|{"name":"container","customer_facing":true}'; do \
	  path=$${spec%%|*}; body=$${spec#*|}; \
	  curl -sS --noproxy '*' -o /dev/null -w "  $$path %{http_code}\n" \
	    -X POST -H "Origin: $(DEMO_URL)" \
	    -H 'Content-Type: application/json' -d "$$body" \
	    "$(DEMO_URL)$$path"; \
	done
	@for variant in binary container; do \
	  curl -sS --noproxy '*' -o /dev/null -w "  upload openpsirt/main/$$variant %{http_code}\n" \
	    -X POST -H "Origin: $(DEMO_URL)" \
	    -F "inventory=@$(DEMO_DIR)/openpsirt-$$variant.cdx.json" \
	    "$(DEMO_URL)/v1/products/openpsirt/streams/main/variants/$$variant/scans"; \
	done
	@# The rest of the cast, with roles on every product just declared.
	@#
	@# Recorded rather than created on arrival: nobody appears here by having
	@# authenticated (ACC-21), so somebody who signs in through the proxy with
	@# no record is refused. The administrator records them, which is also the
	@# honest demonstration of how access works.
	@for entry in $(DEMO_CAST); do \
	  port=$${entry%%:*}; rest=$${entry#*:}; who=$${rest%%:*}; roles=$${rest#*:}; \
	  holds=""; \
	  products=$$(for b in $(DEMO_BUILDS); do IFS=',' read -r _ p _ <<< "$$b"; echo "$$p"; done | sort -u); \
	  for product in $$products openpsirt; do \
	    IFS=',' read -ra each <<< "$$roles"; \
	    for role in "$${each[@]}"; do \
	      holds="$$holds{\"product\":\"$$product\",\"role\":\"$$role\"},"; \
	    done; \
	  done; \
	  curl -sS --noproxy '*' -o /dev/null -w "  person $$who %{http_code}\n" \
	    -X POST -H "Origin: $(DEMO_URL)" -H 'Content-Type: application/json' \
	    -d "{\"identity\":\"$$who\",\"display_name\":\"$$who\",\"provider\":\"proxy\",\"username\":\"$$who\",\"holds\":[$${holds%,}]}" \
	    "$(DEMO_URL)/v1/people"; \
	done
	@echo "  the scans run in the background; make demo-status shows when they land"

demo-status:
	@builds=""; for entry in $(DEMO_BUILDS); do \
	  IFS=',' read -r file product display stream variant <<< "$$entry"; \
	  builds="$$builds $$product/streams/$$stream/variants/$$variant"; \
	done; \
	for build in $$builds \
	    openpsirt/streams/main/variants/binary \
	    openpsirt/streams/main/variants/container; do \
	  printf "  %-46s scan " "$$build"; curl -sS --noproxy '*' \
	    "$(DEMO_URL)/v1/products/$$build/scans" \
	    | sed -e 's/.*"state":"\([a-z]*\)".*/\1/' -e 's/^{.*/no scans yet/' | tr -d '\n'; \
	  printf " · open "; curl -sS --noproxy '*' \
	    "$(DEMO_URL)/v1/products/$$build/findings?limit=1" \
	    | sed -e 's/.*"total":\([0-9]*\).*/\1 findings/' -e 's/^{.*/unreadable/'; \
	done
	@echo "  open $(DEMO_URL) — you arrive as proxy:$(DEMO_USER), no sign-in"
	@# The rest of the cast, one door each. Two windows is two people, which is
	@# what it takes to show a claim being agreed to: approving your own is
	@# refused, so a single identity can propose a judgment and never finish
	@# one.
	@#
	@# Named without the provider, unlike the line above: the administrator is
	@# named in configuration, where an identity says which path it arrives by,
	@# and the cast is recorded through the API, where the identity is the bare
	@# name and the path is recorded beside it.
	@for entry in $(DEMO_CAST); do \
	  port=$${entry%%:*}; rest=$${entry#*:}; who=$${rest%%:*}; roles=$${rest#*:}; \
	  echo "       http://$(DEMO_HOST):$$port — as $$who ($$roles)"; \
	done


# Start over, keeping the vulnerability database: it is a gigabyte, it is not
# what anybody is resetting, and downloading it again is the slow part.
demo-reset: demo-down
	@rm -rf $(DEMO_DIR)/data $(DEMO_DIR)/image.cdx.json
	@echo "  removed the demo database. The scanner cache in $(DEMO_DIR)/grype was kept."

# The developer loop: this machine's binary and the interface's dev server, for
# editing the interface and seeing it reload. Needs Go, node and a scanner
# here. It does not exercise the embedded interface, so what it shows is not
# quite what ships — "make demo" is.
dev: build web dev-up dev-seed dev-status

dev-up: dev-down
	@mkdir -p $(DEV_DIR)
	@OPENPSIRT_DATABASE_URL="sqlite://$(DEV_DB)" \
	 OPENPSIRT_ADDR="$(DEV_API)" \
	 OPENPSIRT_PLAIN_HTTP=1 \
	 OPENPSIRT_BOOTSTRAP_ADMINS="proxy:$(DEMO_USER)" \
	 OPENPSIRT_TRUSTED_HEADER="X-User" \
	 OPENPSIRT_TRUSTED_SOURCES="127.0.0.0/8" \
	 OPENPSIRT_BASE_URL="$(DEV_URL)" \
	 nohup ./$(BIN) > $(DEV_DIR)/api.log 2>&1 & echo $$! > $(DEV_DIR)/api.pid
	@OPENPSIRT_DEV_USER="$(DEMO_USER)" \
	 OPENPSIRT_DEV_HOSTS="$(DEV_HOST)" \
	 OPENPSIRT_DEV_API="http://$(DEV_API)" \
	 nohup $(NPM) --prefix web run dev -- --host --port $(DEV_PORT) \
	   > $(DEV_DIR)/web.log 2>&1 & echo $$! > $(DEV_DIR)/web.pid
	@sleep 6
	@echo "api  $(DEV_API)   log $(DEV_DIR)/api.log"
	@echo "web  $(DEV_URL)   log $(DEV_DIR)/web.log"

dev-down:
	@-pkill -f "$(BIN)" 2>/dev/null || true
	@-pkill -f "vite.*--port $(DEV_PORT)" 2>/dev/null || true
	@rm -f $(DEV_DIR)/api.pid $(DEV_DIR)/web.pid
	@sleep 1

dev-seed:
	@command -v xz >/dev/null || { echo "xz is needed to read the fixtures"; exit 1; }
	@for entry in $(DEMO_BUILDS); do \
	  IFS=',' read -r file product display stream variant <<< "$$entry"; \
	  xz -dc "$$file" > "$(DEV_DIR)/$$product-$$stream-$$variant.cdx.json"; \
	  for spec in \
	    "/v1/products|{\"name\":\"$$product\",\"display_name\":\"$$display\"}" \
	    "/v1/products/$$product/streams|{\"name\":\"$$stream\",\"kind\":\"branch\"}" \
	    "/v1/products/$$product/variants|{\"name\":\"$$variant\",\"customer_facing\":true}"; do \
	    path=$${spec%%|*}; body=$${spec#*|}; \
	    curl -sS --noproxy '*' -o /dev/null -w "  $$path %{http_code}\n" \
	      -X POST -H "X-User: $(DEMO_USER)" -H "Origin: $(DEV_URL)" \
	      -H 'Content-Type: application/json' -d "$$body" \
	      "http://$(DEV_API)$$path"; \
	  done; \
	  curl -sS --noproxy '*' -o /dev/null -w "  upload $$product/$$stream/$$variant %{http_code}\n" \
	    -X POST -H "X-User: $(DEMO_USER)" -H "Origin: $(DEV_URL)" \
	    -F "inventory=@$(DEV_DIR)/$$product-$$stream-$$variant.cdx.json" \
	    "http://$(DEV_API)/v1/products/$$product/streams/$$stream/variants/$$variant/scans"; \
	done
	@# A second product: this deployment itself, from its own inventory.
	@#
	@# One variant here, not the two the container demo seeds. There is no
	@# container in this loop — it runs the binary on this machine — so the
	@# inventory of what an image ships does not exist to upload. Naming the
	@# one that does exist the same thing it is called there keeps the two
	@# loops describing one product rather than two that look alike.
	@$(MAKE) --no-print-directory sbom >/dev/null
	@for spec in \
	  '/v1/products|{"name":"openpsirt","display_name":"OpenPSIRT"}' \
	  '/v1/products/openpsirt/streams|{"name":"main","kind":"branch"}' \
	  '/v1/products/openpsirt/variants|{"name":"binary","customer_facing":true}'; do \
	  path=$${spec%%|*}; body=$${spec#*|}; \
	  curl -sS --noproxy '*' -o /dev/null -w "  $$path %{http_code}\n" \
	    -X POST -H "X-User: $(DEMO_USER)" -H "Origin: $(DEV_URL)" \
	    -H 'Content-Type: application/json' -d "$$body" \
	    "http://$(DEV_API)$$path"; \
	done
	@curl -sS --noproxy '*' -o /dev/null -w "  upload %{http_code}\n" \
	  -X POST -H "X-User: $(DEMO_USER)" -H "Origin: $(DEV_URL)" \
	  -F "inventory=@bin/openpsirt.cdx.json" \
	  "http://$(DEV_API)/v1/products/openpsirt/streams/main/variants/binary/scans"
	@echo "  the scans run in the background; make dev-status shows when they land"

dev-status:
	@builds=""; for entry in $(DEMO_BUILDS); do \
	  IFS=',' read -r file product display stream variant <<< "$$entry"; \
	  builds="$$builds $$product/streams/$$stream/variants/$$variant"; \
	done; \
	for build in $$builds openpsirt/streams/main/variants/binary; do \
	  printf "  %-46s scan " "$$build"; curl -sS --noproxy '*' -H "X-User: $(DEMO_USER)" \
	    "http://$(DEV_API)/v1/products/$$build/scans" \
	    | sed -e 's/.*"state":"\([a-z]*\)".*/\1/' -e 's/^{.*/no scans yet/' | tr -d '\n'; \
	  printf " · open "; curl -sS --noproxy '*' -H "X-User: $(DEMO_USER)" \
	    "http://$(DEV_API)/v1/products/$$build/findings?limit=1" \
	    | sed -e 's/.*"total":\([0-9]*\).*/\1 findings/' -e 's/^{.*/unreadable/'; \
	done
	@echo "  open $(DEV_URL) — you arrive as proxy:$(DEMO_USER), no sign-in"


dev-reset: dev-down
	@rm -f $(DEV_DB)
	@echo "  removed $(DEV_DB)"

clean-web:
	rm -rf web/node_modules web/dist internal/webui/dist/assets internal/webui/dist/index.html

clean:
	rm -rf bin
