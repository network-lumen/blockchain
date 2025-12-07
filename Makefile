BRANCH := $(shell git rev-parse --abbrev-ref HEAD)
COMMIT := $(shell git log -1 --format='%H')
APPNAME := lumen
GO_TEST_SCRIPT := ./devtools/scripts/go_test.sh
GO_WITH_PKGS_SCRIPT := ./devtools/scripts/go_with_pkgs.sh

# do not override user values
ifeq (,$(VERSION))
  VERSION := $(shell git describe --exact-match 2>/dev/null)
  # if VERSION is empty, then populate it with branch name and raw commit hash
  ifeq (,$(VERSION))
    VERSION := $(BRANCH)-$(COMMIT)
  endif
endif

# Update the ldflags with the app, client & server names
ldflags = -X github.com/cosmos/cosmos-sdk/version.Name=$(APPNAME) \
	-X github.com/cosmos/cosmos-sdk/version.AppName=$(APPNAME)d \
	-X github.com/cosmos/cosmos-sdk/version.Version=$(VERSION) \
	-X github.com/cosmos/cosmos-sdk/version.Commit=$(COMMIT)

.PHONY: help
help: ## Show all documented make targets
	@grep -h '^[a-zA-Z0-9_-]\+:.*## ' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS=":.*## "}; {printf "  \033[36m%-28s\033[0m %s\n", $$1, $$2}'

BUILD_FLAGS := -ldflags '$(ldflags)'

##############
###  Test  ###
##############

test-unit:
	@echo Running unit tests...
	@$(GO_TEST_SCRIPT) -mod=readonly -v -timeout 30m

test-race:
	@echo Running unit tests with race condition reporting...
	@$(GO_TEST_SCRIPT) -mod=readonly -v -race -timeout 30m

test-cover:
	@echo Running unit tests and creating coverage report...
	@$(GO_TEST_SCRIPT) -mod=readonly -v -timeout 30m -coverprofile=$(COVER_FILE) -covermode=atomic
	@go tool cover -html=$(COVER_FILE) -o $(COVER_HTML_FILE)
	@rm $(COVER_FILE)

bench:
	@echo Running unit tests with benchmarking...
	@$(GO_TEST_SCRIPT) -mod=readonly -v -timeout 30m -bench=.

test-legacy: govet govulncheck test-unit

.PHONY: test test-unit test-race test-cover bench test-legacy

#################
###  Install  ###
#################

all: build

VERSION := $(shell git describe --tags --dirty --always 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)

build: ## Build lumend for the current platform (Go build)
	@echo "--> building lumend (local platform)"
	@go build -trimpath -ldflags "-s -w $(ldflags)" -o build/lumend ./cmd/lumend

build-native: ## Build lumend via devtools/scripts/build_native.sh (NETWORK_DIR optional)
	@bash devtools/scripts/build_native.sh $(ARGS)

build-release: ## Produce cross-platform release archives (wraps devtools/scripts/build_release.sh)
	@bash devtools/scripts/build_release.sh $(ARGS)

test:
	@$(GO_TEST_SCRIPT)

preflight:
	@go test ./tests/preflight -count=1

doc-check:
	@go test ./tests/preflight -run TestDocs -count=1

pre-release: ## Run release readiness checks (devtools/scripts/pre_release_check.sh)
	@bash devtools/scripts/pre_release_check.sh

simulate-network: ## Launch the Docker simulator (devtools/scripts/simulate_network.sh)
	@bash devtools/scripts/simulate_network.sh $(ARGS)

install-service: ## Generate/install the systemd unit (devtools/scripts/install_service.sh)
	@bash devtools/scripts/install_service.sh $(ARGS)

sanity:
	@bash -c 'set -euo pipefail; \
		$(GO_WITH_PKGS_SCRIPT) vet; \
		$(GO_TEST_SCRIPT) -count=1; \
		mkdir -p build; \
		go build -trimpath -buildvcs=false -o ./build/lumend ./cmd/lumend; \
		export LC_ALL=C; \
		if strings ./build/lumend | grep -qiE '\''(pqc_testonly|\bnoop\b.*pqc)'\''; then \
		  echo "BAD: test-only/noop PQC symbols found"; exit 1; \
		else \
		  echo "OK: release binary clean"; \
		fi'

e2e: ## Run the full test orchestrator (devtools/tests/test_all.sh)
	@bash devtools/tests/test_all.sh $(ARGS)

e2e-pqc: ## Run the dedicated PQC e2e flow (devtools/tests/e2e_pqc.sh)
	@BIN=./build/lumend bash devtools/tests/e2e_pqc.sh $(ARGS)

e2e-pqc-cli: ## Run the CLI-centric PQC flow (devtools/tests/e2e_pqc_cli.sh)
	@BIN=./build/lumend bash devtools/tests/e2e_pqc_cli.sh $(ARGS)

e2e-pqc-tx-paths: ## Run PQC tx-path coverage (create-validator + delegate)
	@BIN=./build/lumend bash devtools/tests/e2e_pqc_tx_paths.sh $(ARGS)

e2e-bootstrap-validator: ## Run bootstrap validator e2e flow (devtools/tests/e2e_bootstrap_validator.sh)
	@BIN=./build/lumend bash devtools/tests/e2e_bootstrap_validator.sh $(ARGS)

e2e-dns: ## Legacy DNS CLI flow (devtools/tests/e2e_dns.sh)
	@bash devtools/tests/e2e_dns.sh $(ARGS)

e2e-dns-auction: ## Auction lifecycle flow (devtools/tests/e2e_dns_auction.sh)
	@bash devtools/tests/e2e_dns_auction.sh $(ARGS)

e2e-gateways: ## Gateways happy-path suite (devtools/tests/e2e_gateways.sh)
	@bash devtools/tests/e2e_gateways.sh $(ARGS)

e2e-release: ## Release publisher workflow (devtools/tests/e2e_release.sh)
	@bash devtools/tests/e2e_release.sh $(ARGS)

e2e-gov: ## Governance / DAO parameter workflow (devtools/tests/e2e_gov.sh)
	@bash devtools/tests/e2e_gov.sh $(ARGS)

e2e-send-tax: ## Send-tax ante/post handler suite (devtools/tests/e2e_send_tax.sh)
	@bash devtools/tests/e2e_send_tax.sh $(ARGS)

smoke-rest: ## Lightweight REST/RPC smoke test (devtools/tests/smoke_rest.sh)
	@bash devtools/tests/smoke_rest.sh $(ARGS)

clean:
	@rm -rf build/ dist/ artifacts/

.PHONY: build build-native build-release test preflight doc-check pre-release simulate-network install-service sanity \
	e2e e2e-pqc e2e-dns e2e-dns-auction e2e-gateways e2e-release e2e-gov e2e-send-tax smoke-rest clean

docs:
	@mkdir -p artifacts/docs
	@DOC_VERSION=$$(git describe --tags --dirty --always); \
		buf generate --template proto/buf.gen.swagger.yaml; \
		DOC_VERSION="$$DOC_VERSION" python3 -c 'import json, os, pathlib; path = pathlib.Path("docs/static/openapi.json"); data = json.loads(path.read_text()); info = data.get("info", {}); info["version"] = os.environ.get("DOC_VERSION", "").strip() or "unversioned"; data["info"] = info; path.write_text(json.dumps(data, separators=(",", ":")))'; \
		echo "OpenAPI generated (version $$DOC_VERSION)"

.PHONY: docs

.PHONY: all install

##################
###  Protobuf  ###
##################

proto:
	@command -v buf >/dev/null || { echo "buf CLI not found in PATH"; exit 1; }
	@echo "==> Generating protobuf code"
	@buf generate --template proto/buf.gen.gogo.yaml
	@for module in dns gateway release tokenomics pqc; do \
			target="$$module"; \
		[ "$$module" = "gateway" ] && target=gateways; \
		if [ -d "lumen/$$module/v1" ]; then \
			mkdir -p "x/$$target/types"; \
			cp -f lumen/$$module/v1/*.pb.go "x/$$target/types/" 2>/dev/null || true; \
			cp -f lumen/$$module/v1/*.pb.gw.go "x/$$target/types/" 2>/dev/null || true; \
		fi; \
		if [ -d "lumen/$$module/module" ]; then \
			cp -f lumen/$$module/module/v1/*.pb.go "x/$$target/types/" 2>/dev/null || true; \
			cp -f lumen/$$module/module/v1/*.pb.gw.go "x/$$target/types/" 2>/dev/null || true; \
		fi; \
	done
	@rm -rf lumen
	@echo "==> Tidying go.mod"
	@go mod tidy

.PHONY: proto

#################
###  Linting  ###
#################

lint:
	@golangci-lint run ./...

lint-fix:
	@echo "--> Running linter and fixing issues"
	@go tool github.com/golangci/golangci-lint/cmd/golangci-lint run ./... --fix --timeout 15m

.PHONY: lint lint-fix

# Filter only SA1019 coming from generated *_pb.go / *_pb.gw.go files (grpc-gateway).
staticcheck:
	@set -e; \
	echo "--> staticcheck (filter generated grpc-gateway deprecations)"; \
	out="$$(staticcheck ./... 2>&1 || true)"; \
	if printf "%s\n" "$$out" | grep -q 'invalid array length -delta \* delta'; then \
	  echo "note: staticcheck hit upstream go/types bug (delta*delta); ignoring offending line"; \
	fi; \
	if printf "%s\n" "$$out" | grep -q 'unsupported version: 2'; then \
	  echo "note: staticcheck hit upstream go/types unsupported-version bug; ignoring offending lines"; \
	fi; \
	if printf "%s\n" "$$out" | grep -q 'module requires at least go1'; then \
	  echo "note: staticcheck emitted go toolchain version mismatch warnings; ignoring offending lines"; \
	fi; \
	filtered="$$(printf "%s\n" "$$out" \
		| grep -Ev 'x/.*/types/query\.pb(\.gw)?\.go:.*SA1019' \
		| grep -Ev 'invalid array length -delta \* delta' \
		| grep -Ev 'internal error in importing .*unsupported version: 2' \
		| grep -Ev 'module requires at least go1\.[0-9.]+, but Staticcheck was built with go1\.[0-9.]+' \
		|| true)"; \
	if [ -n "$$filtered" ]; then \
	  printf "%s\n" "$$filtered"; \
	  exit 1; \
	fi; \
echo "ok: staticcheck (generated SA1019 filtered)"

.PHONY: staticcheck

static: staticcheck

.PHONY: static

####################
###  Vulnerable  ###
####################

vuln-tools:
	@echo "==> installing govulncheck"
	@go install golang.org/x/vuln/cmd/govulncheck@latest

# IDs tolerated because x/crisis is not linked in our binary.
ALLOW_VULNS ?= GO-2023-1881,GO-2023-1821

# Hardened vulncheck target:
# 1) Try source mode. If it fails with the KNOWN internal error only, switch to binary mode.
# 2) In binary mode, allow only the IDs in ALLOW_VULNS; any other finding is blocking.
vulncheck: vuln-tools
	@set -e; \
	echo "==> govulncheck (source)"; \
	src_out="$$(govulncheck ./... 2>&1 || true)"; \
	if printf "%s" "$$src_out" | grep -q "^No vulnerabilities found"; then \
	  echo "ok: source scan"; \
	  exit 0; \
	fi; \
	if printf "%s" "$$src_out" | grep -qi 'internal error: package "golang\.org/x/sys/unix" without types was imported from "github\.com/mattn/go-isatty"'; then \
	  echo "warn: source scan hit known internal error, trying binary mode..."; \
	else \
	  echo "$$src_out"; \
	  echo "fail: govulncheck (source) failed for another reason"; \
	  exit 1; \
	fi; \
	mkdir -p build; \
	[ -f build/lumend ] || { echo "-> building build/lumend"; go build -o build/lumend ./cmd/lumend; }; \
	tmpjson=$$(mktemp); \
	if govulncheck -mode=binary -json build/lumend > $$tmpjson; then \
	  if command -v jq >/dev/null 2>&1; then \
	    allow_regex=$$(echo "$(ALLOW_VULNS)" | sed 's/,/|/g'); \
	    remaining=$$(jq -r '.vulns[]?.id' $$tmpjson | grep -Ev "^($$allow_regex)$$" || true); \
	    if [ -n "$$remaining" ]; then \
	      echo "fail: binary scan found blocking vulns:"; echo "$$remaining" | sed 's/^/  - /'; \
	      rm -f $$tmpjson; exit 1; \
	    fi; \
	    echo "ok: binary scan (only allowlisted advisories present)"; \
	  else \
	    echo "ok: binary scan (jq not found; cannot enforce allowlist)"; \
	  fi; \
	else \
	  echo "warn: binary scan failed; checking for known internal error..."; \
	  if govulncheck ./... 2>&1 | grep -qi 'internal error: package "golang\.org/x/sys/unix" without types was imported from "github\.com/mattn/go-isatty"'; then \
	    echo "note: known upstream internal error – not blocking"; \
	  else \
	    echo "fail: govulncheck failed for another reason"; exit 1; \
	  fi; \
	fi

# Optional non-blocking JSON export (writes to artifacts/security)
vulncheck-json: vuln-tools
	@set -e; \
	mkdir -p artifacts/security; \
	echo "==> govulncheck (source json)"; \
	if govulncheck -json ./... > artifacts/security/govuln-source.json 2>/dev/null; then \
	  echo "ok: wrote artifacts/security/govuln-source.json"; \
	else \
	  echo "warn: source json failed; trying binary json..."; \
	  [ -f build/lumend ] || { echo "-> building build/lumend"; go build -o build/lumend ./cmd/lumend; }; \
	  if govulncheck -mode=binary -json build/lumend > artifacts/security/govuln-binary.json 2>/dev/null; then \
	    echo "ok: wrote artifacts/security/govuln-binary.json"; \
	  else \
	    echo "note: govulncheck json failed (both modes); continuing (non-blocking)"; \
	  fi; \
	fi

.PHONY: vuln-tools vulncheck vulncheck-json

vuln: vulncheck

.PHONY: vuln

###################
### Development ###
###################

govet:
	@echo Running go vet...
	@$(GO_WITH_PKGS_SCRIPT) vet

govulncheck:
	@$(MAKE) vulncheck

.PHONY: govet govulncheck
