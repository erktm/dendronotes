# dendronotes: build and development tasks. `make help` lists all targets.

VERSION           ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
SOURCE_DATE_EPOCH ?= $(shell git log -1 --format=%ct 2>/dev/null || echo 0)
IMAGE             ?= dendronotes:$(VERSION)
PLATFORM          ?= linux/$(shell uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')
LDFLAGS           := -s -w -X main.version=$(VERSION)

DEV_SOMA     ?= $(CURDIR)/dev/soma
DEV_SOMAS    ?= $(CURDIR)/dev/somas
DEV_DATA     ?= $(CURDIR)/dev/data
DEV_PASSWORD := $(CURDIR)/dev/password
# dev/soma (the single soma of earlier versions) becomes the admin's first soma.
DEV_ENV       = DENDRONOTES_SOMAS_DIR=$(DEV_SOMAS) DENDRONOTES_SOMA_DIR=$(DEV_SOMA) DENDRONOTES_DATA_DIR=$(DEV_DATA) \
                DENDRONOTES_ADMIN_PASSWORD_FILE=$(DEV_PASSWORD) DENDRONOTES_LOG_FORMAT=text DENDRONOTES_LOG_LEVEL=debug

NPM_DEPS := web/node_modules/.package-lock.json

.PHONY: help dev dev-ports dev-setup dev-api dev-web web build test test-go test-web fuzz \
        vendor vendor-go vendor-npm image image-offline run-image audit vulncheck sbom clean

help: ## Show this help
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*## "}{printf "  \033[1m%-14s\033[0m %s\n", $$1, $$2}'

$(NPM_DEPS): web/package-lock.json
	cd web && npm ci

# --- Development -----------------------------------------------------------------

# A random dev password is created once in dev/password (git-ignored).
$(DEV_PASSWORD):
	@mkdir -p $(dir $@)
	@head -c 18 /dev/urandom | base64 | tr -d '/+=' > $@
	@chmod 600 $@

dev-setup: $(DEV_PASSWORD)
	@mkdir -p $(DEV_SOMA) $(DEV_SOMAS) $(DEV_DATA)
	@if [ -z "$$(ls -A $(DEV_SOMA))" ]; then \
	  printf '# Welcome to dendronotes\n\nThis note is a plain Markdown file in your dev soma (dev/soma).\n' > $(DEV_SOMA)/Welcome.md; \
	fi
	@echo "Dev login: admin / $$(cat $(DEV_PASSWORD))   (password stored in dev/password)"

# Fail early with a clear hint if an earlier `make dev` still holds the ports.
dev-ports:
	@command -v lsof >/dev/null || exit 0; \
	for p in 8080 5173; do \
	  pid=$$(lsof -ti tcp:$$p -sTCP:LISTEN 2>/dev/null | head -1); \
	  if [ -n "$$pid" ]; then \
	    pgid=$$(ps -o pgid= -p $$pid | tr -d ' '); \
	    echo "Port $$p is already in use by PID $$pid ($$(ps -o comm= -p $$pid | xargs basename))."; \
	    echo "Stop the other 'make dev' first (Ctrl+C in its terminal), or stop its whole"; \
	    echo "process group (Go server and Vite together) with: kill -- -$$pgid"; \
	    exit 1; \
	  fi; \
	done

dev: $(NPM_DEPS) dev-ports dev-setup ## Run Go server (:8080) and Vite dev server (:5173) together
	@trap 'kill 0' INT TERM EXIT; \
	  $(DEV_ENV) go run ./cmd/dendronotes & \
	  (cd web && npm run dev) & \
	  wait

dev-api: dev-setup ## Run only the Go server with the dev soma
	$(DEV_ENV) go run ./cmd/dendronotes

dev-web: $(NPM_DEPS) ## Run only the Vite dev server
	cd web && npm run dev

# --- Build & test ------------------------------------------------------------------

web: $(NPM_DEPS) ## Build the frontend (web/dist)
	cd web && npm run build

build: web ## Build bin/dendronotes with the embedded frontend
	CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags "$(LDFLAGS)" -o bin/dendronotes ./cmd/dendronotes

test: test-go test-web ## Run all tests and checks

test-go: ## go vet + Go tests with the race detector
	go vet ./...
	go test -race -count=1 ./...

test-web: $(NPM_DEPS) ## Type-check the frontend and run its unit tests
	cd web && npm run check && npm test

fuzz: ## Fuzz the soma path validation for 60 seconds
	go test -run='^$$' -fuzz=FuzzCleanPath -fuzztime=60s ./internal/soma/

audit: $(NPM_DEPS) ## Check npm dependencies for known vulnerabilities (needs network)
	cd web && npm audit --audit-level=moderate

GOVULNCHECK ?= go run golang.org/x/vuln/cmd/govulncheck@v1.9.0

vulncheck: $(NPM_DEPS) ## Scan Go and npm dependencies; fail on findings without a VEX statement (needs network)
	@mkdir -p bin
	$(GOVULNCHECK) -format json ./... > bin/govulncheck.json
	cd web && npm audit --json > ../bin/npm-audit.json || true
	node scripts/vex-check.mjs bin/govulncheck.json bin/npm-audit.json

sbom: ## Write the software bill of materials (security/sbom.cdx.json)
	node scripts/sbom.mjs $(VERSION) > security/sbom.cdx.json

# --- Air gap / vendoring -------------------------------------------------------------

vendor: vendor-go vendor-npm ## Provide all dependencies for offline builds

vendor-go: ## Vendor Go modules into vendor/ (checked in)
	go mod tidy
	go mod vendor

# Downloads all npm packages, including the Linux binaries (musl, amd64 +
# arm64) needed by the build container, into .offline/npm-cache. The
# directory is not checked in; it travels with the source into the air gap.
vendor-npm: ## Create the npm offline cache for the container build
	rm -rf .offline/npm-cache
	@tmp=$$(mktemp -d) && trap 'rm -rf "$$tmp"' EXIT && \
	cp web/package.json web/package-lock.json web/.npmrc "$$tmp"/ && \
	for cpu in x64 arm64; do \
	  echo "npm cache: linux/$$cpu (musl)"; \
	  (cd "$$tmp" && rm -rf node_modules && \
	   npm ci --cache "$(CURDIR)/.offline/npm-cache" --os=linux --cpu=$$cpu --libc=musl) || exit 1; \
	done
	@rm -rf .offline/npm-cache/_logs
	@du -sh .offline/npm-cache

# --- Container -----------------------------------------------------------------------

# The image is exported as an OCI archive (bin/dendronotes-<version>-<platform>.oci.tar)
# and then loaded into Docker. The archive can be carried straight into an
# air gap (`docker load` or `skopeo copy oci-archive:…`).
# rewrite-timestamp + SOURCE_DATE_EPOCH + no provenance attestation (it
# contains build timestamps) yield the same digest for the same source.
OCI_TAR    = bin/dendronotes-$(VERSION)-$(subst /,-,$(PLATFORM)).oci.tar
BUILD_ARGS = --platform $(PLATFORM) --provenance=false \
             --build-arg VERSION=$(VERSION) \
             --build-arg SOURCE_DATE_EPOCH=$(SOURCE_DATE_EPOCH) \
             --output type=oci,dest=$(OCI_TAR),name=$(IMAGE),rewrite-timestamp=true

image: ## Build the container image for the local platform (reproducible) and load it
	@mkdir -p bin
	docker buildx build $(BUILD_ARGS) .
	docker load -i $(OCI_TAR)

image-offline: ## Rebuild without cache and network (checks air-gap readiness; needs `make vendor`)
	@test -d .offline/npm-cache || { echo "Run 'make vendor' first."; exit 1; }
	@mkdir -p bin
	docker buildx build --no-cache --network=none $(BUILD_ARGS) .
	docker load -i $(OCI_TAR)

run-image: dev-setup ## Run the image hardened (read-only, no capabilities)
	@mkdir -p $(CURDIR)/dev/data-image
	docker run --rm -p 8080:8080 \
	  --read-only --cap-drop ALL --security-opt no-new-privileges \
	  -v $(DEV_SOMAS):/somas -v $(CURDIR)/dev/data-image:/data \
	  -v $(DEV_PASSWORD):/run/secrets/password:ro -e DENDRONOTES_ADMIN_PASSWORD_FILE=/run/secrets/password \
	  $(IMAGE)

clean: ## Remove build artifacts
	rm -rf bin web/dist/assets web/dist/index.html web/dist/favicon.svg
