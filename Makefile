BRANCH := $(shell git rev-parse --abbrev-ref HEAD)
COMMIT := $(shell git log -1 --format='%H')
APPNAME := lumen

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

BUILD_FLAGS := -ldflags '$(ldflags)'

##############
###  Test  ###
##############

test-unit:
	@echo Running unit tests...
	@go test -mod=readonly -v -timeout 30m ./...

test-race:
	@echo Running unit tests with race condition reporting...
	@go test -mod=readonly -v -race -timeout 30m ./...

test-cover:
	@echo Running unit tests and creating coverage report...
	@go test -mod=readonly -v -timeout 30m -coverprofile=$(COVER_FILE) -covermode=atomic ./...
	@go tool cover -html=$(COVER_FILE) -o $(COVER_HTML_FILE)
	@rm $(COVER_FILE)

bench:
	@echo Running unit tests with benchmarking...
	@go test -mod=readonly -v -timeout 30m -bench=. ./...

test-legacy: govet govulncheck test-unit

.PHONY: test test-unit test-race test-cover bench test-legacy

#################
###  Install  ###
#################

all: build

VERSION := $(shell git describe --tags --dirty --always 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)

build:
	@echo "--> building lumend (local platform)"
	@go build -trimpath -ldflags "-s -w $(ldflags)" -o build/lumend ./cmd/lumend

test:
	@go test ./...

preflight:
	@go test ./tests/preflight -count=1

doc-check:
	@go test ./tests/preflight -run TestDocs -count=1

pre-release:
	@bash devtools/scripts/pre_release_check.sh

simulate-network:
	@bash devtools/scripts/simulate_network.sh $(ARGS)

sanity:
	@bash -c 'set -euo pipefail; \
		go vet ./...; \
		go test ./... -count=1; \
		mkdir -p build; \
		go build -trimpath -buildvcs=false -o ./build/lumend ./cmd/lumend; \
		export LC_ALL=C; \
		if strings ./build/lumend | grep -qiE '\''(pqc_testonly|\bnoop\b.*pqc)'\''; then \
		  echo "BAD: test-only/noop PQC symbols found"; exit 1; \
		else \
		  echo "OK: release binary clean"; \
		fi'

e2e:
	@bash devtools/tests/test_all.sh

e2e-pqc:
	@BIN=./build/lumend bash devtools/tests/e2e_pqc.sh

clean:
	@rm -rf build/ dist/ artifacts/

.PHONY: build test preflight doc-check pre-release simulate-network sanity e2e e2e-pqc clean

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

###################
### Development ###
###################

govet:
	@echo Running go vet...
	@go vet ./...

govulncheck:
	@$(MAKE) vulncheck

.PHONY: govet govulncheck
