# Devtools

Everything needed to build, test, and package the chain lives here. Use this guide as the entry point when hacking on automation, CI, or local release tooling.

## Layout

```
devtools/
├── docker/
│   ├── .dockerignore
│   └── builder/
│       └── Dockerfile          # Multi-stage builder / dev shell
├── scripts/
│   ├── build_native.sh         # Local developer build with optional artifact copy
│   ├── build_release.sh        # Cross-platform release binaries + checksums
│   ├── install_service.sh      # Systemd unit generator (with --print-unit)
│   ├── pre_release_check.sh    # Guardrails before tagging/publishing
│   └── simulate_network.sh     # Docker-based network simulator (seed + validators + runner)
└── tests/
    ├── e2e_dns.sh              # Legacy DNS flow (needs CLI JSON fix)
    ├── e2e_dns_auction.sh      # Auction lifecycle (`--mode prod|dev`)
    ├── e2e_gateways.sh         # Gateways module happy path
    ├── e2e_release.sh          # Release publisher workflows
    ├── e2e_send_tax.sh         # Send-tax ante/post handler behaviour
    ├── e2e_pqc.sh              # Dilithium dual-signing flow
    ├── smoke_rest.sh           # Lightweight REST/RPC health check
    ├── test_all.sh             # Orchestrates unit + E2E suites
    └── test_gasless_enforcement.sh # Manual gateway ante handler probe
```

## Tools & Scripts

### `scripts/simulate_network.sh` — Docker network simulator

Spins up a seed node, configurable validator/full-node set, and a runner container that drives end-to-end traffic (PQC link/negative path, bank tax assertion, DNS register→auction→settle, gateway contracts, release publish, per-account and global rate-limit bursts). Requirements: Docker, Go, and `jq`. Artifacts (logs, genesis, node data, snapshots, PQC keys) land in `artifacts/sim/{logs,nodes,genesis,keys,snapshots}`.

Flags: `--validators N`, `--fullnodes N`, `--fast`, `--clean`, `--keep`, `--timeout SEC`, `--image <tag>`.

Common invocations:

```bash
make simulate-network              # defaults: 2 validators, 1 full node
LUMEN_RL_GLOBAL_MAX=50 make simulate-network ARGS="--clean --timeout 600"
```

The script honours `LUMEN_RL_*`, `FAST`, `CLEAN`, `KEEP`, `TIMEOUT`, and `IMAGE_TAG` overrides. See also [`docs/simulation.md`](../docs/simulation.md).

### `tests/test_all.sh` — unit + E2E orchestrator

Builds `./build/lumend` once, exports `SKIP_BUILD=1`, and runs:

- `go test ./...`
- `devtools/tests/e2e_*.sh` (send-tax, DNS auction, release, gateways, PQC)
- `devtools/tests/smoke_rest.sh`

Logs land in `artifacts/test-logs/*.log` and a JSON summary in `artifacts/test-report.json`.

### `scripts/build_release.sh` — cross-platform release artifacts

Produces trimmed binaries for `linux/amd64`, `linux/arm64`, and `darwin/arm64`, places them under `dist/<git-describe>/<GOOS>-<GOARCH>/lumend`, and emits `SHA256SUMS`. Each build embeds the git version/commit via `-ldflags`.

### `docker/builder/Dockerfile` — multi-stage builder & dev shell

- **Builder target** (outputs Linux + Windows CLIs without touching the host):

  ```bash
  docker build \
    --ignorefile devtools/docker/.dockerignore \
    -t lumen-builder \
    --target builder \
    -f devtools/docker/builder/Dockerfile .
  cid=$(docker create lumen-builder)
  mkdir -p artifacts/bin
  docker cp "$cid":/out/. artifacts/bin/
  docker rm "$cid"
  ```

- **Developer shell** (mounted source, cached Go modules):

  ```bash
  docker build --ignorefile devtools/docker/.dockerignore \
    -t lumen-dev \
    --target dev \
    -f devtools/docker/builder/Dockerfile .
  docker run --rm -it \
    -v "$(pwd)":/src \
    -v lumen_gocache:/go/pkg/mod \
    -w /src lumen-dev bash
  # inside the container
  make build
  GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o build/lumend.exe ./cmd/lumend
  ```

Override the Go toolchain via `--build-arg GO_VERSION=<1.2x>` when needed. The build context must be the repo root.

## Prerequisites

- Go 1.23+ (tested with 1.24 and 1.25.2)
- `jq`, `curl`, `sed`, `awk`, `find`, `tar`
- `make` (optional, used by build scripts when present)
- `git`
- `sudo` (only for `install_service.sh` when not using `--print-unit`)
- Docker ≥ 24.0 (for `--ignorefile` in the builder workflow)

E2E scripts spin up a temporary HOME (`mktemp -d -t lumen-e2e-XXXXXX`) and remove it on exit; set `DEBUG_KEEP=1` to retain the directory for debugging.

> **Toolchain note:** `go.mod` uses Go 1.24 features (tool declarations). When invoking commands with Go 1.23, enable toolchain forwarding (`GOTOOLCHAIN=go1.23.0+auto`) or use Go 1.24+ directly.

## Environment Knobs

| Variable | Default | Affecting | Notes |
|----------|---------|-----------|-------|
| `SKIP_BUILD` | `0` | E2E suites | Reuse an existing `./build/lumend`. |
| `MODE` | `prod` | `tests/e2e_dns_auction.sh` | `prod` (default) mirrors production fee routing; `dev` enables the historical proposer-direct env. |
| `FAST`, `CLEAN`, `KEEP`, `TIMEOUT`, `IMAGE_TAG` | `0`, `0`, `0`, `600`, `lumen-node:sim` | `scripts/simulate_network.sh` | Skip heavy spam, force cleanup, retain containers, change timeout/image tag. |
| `DEBUG` | `0` | `tests/e2e_send_tax.sh` | Enables `set -x` before boot. |
| `RPC_HOST`, `RPC_PORT`, `API_HOST`, `API_PORT`, `GRPC_HOST`, `GRPC_PORT` | Localhost defaults | All E2E + service installer | Override bind addresses without editing scripts. |
| `RPC_LADDR`, `API_ADDR`, `GRPC_ADDR` | Derived | All E2E + service installer | Use when you need full endpoint control. |
| `GRPC_WEB_ENABLE` | `1` | Node start scripts + service installer | Set to `0` to disable gRPC-Web. |
| `LOG_FILE` | `/tmp/lumen.log` | E2E suites | Node stdout/stderr destination. |
| `NETWORK_DIR` | unset | `scripts/build_native.sh` | Copies build artifacts to this path when set. |
| `LUMEN_TAX_DIRECT_TO_PROPOSER`, `LUMEN_AUCTION_DIRECT_TO_PROPOSER` | unset | Relevant E2E suites | On/off switches consumed by the app at boot. |
| `LUMEN_RL_PER_BLOCK`, `LUMEN_RL_PER_WINDOW`, `LUMEN_RL_WINDOW_SEC`, `LUMEN_RL_GLOBAL_MAX` | `5`, `20`, `10`, `300` | `scripts/install_service.sh`, `scripts/simulate_network.sh`, `tests/e2e_gateways.sh` | Clamp the ante decorators; scripts only allow tightening (lower numbers). |
| `DEBUG_KEEP` | `0` | E2E suites | Keep the temporary `$HOME` for debugging. |

## Common Workflows

### Build (local developer)

```bash
bash devtools/scripts/build_native.sh
# optionally copy to another folder
NETWORK_DIR=artifacts/bin bash devtools/scripts/build_native.sh
```

### Build (release artifacts)

```bash
bash devtools/scripts/build_release.sh
ls dist/$(git describe --tags --dirty --always)
```

### End-to-end suites

```bash
# Full run: unit tests + all E2E flows + smoke test
HOME=$(mktemp -d) bash devtools/tests/test_all.sh

# Individual flows (skip rebuild after the first run)
bash devtools/tests/e2e_send_tax.sh --skip-build
bash devtools/tests/e2e_dns_auction.sh --skip-build --mode prod
```

### Documentation / OpenAPI

```bash
make docs
```

This regenerates Swagger assets under `artifacts/docs/` and stamps `docs/static/openapi.json` with `git describe --tags --dirty --always`.

### Systemd service

```bash
# dry run (no sudo required)
devtools/scripts/install_service.sh --print-unit

# install to /etc/systemd/system/lumend.service (requires sudo)
sudo devtools/scripts/install_service.sh
```

## Notes & Caveats

- `tests/e2e_dns.sh` still uses CLI JSON that fails in current builds; keep it around for reference or rewrite using the unsigned-message helpers.
- `tests/test_gasless_enforcement.sh` assumes a running node and pre-seeded keyring; we do not exercise it in CI.
- Artifacts land under `artifacts/test-logs/` and `artifacts/test-report.json` when running the orchestrator.
- `private/init.sh` (outside this folder) stays untracked and should not be committed.
