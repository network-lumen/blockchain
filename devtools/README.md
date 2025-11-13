# Devtools

Everything needed to build, test, and package the chain lives here. Use this guide as the entry point when hacking on automation, CI, or local release tooling.

> ℹ️ **Recommended entrypoint:** All examples now invoke `make <target>` wrappers (e.g. `make simulate-network`).
> The underlying `.sh` scripts still live in `devtools/scripts` and `devtools/tests` for direct use when needed.

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
    ├── e2e_gov.sh              # Table-driven governance suite (driven by gov_param_cases.json)
    ├── gov_param_cases.json    # Declarative parameter/scenario cases for the governance suite
    ├── lib_gov.sh              # Shared governance helpers (waiters, proposal/vote helpers, assertions)
    ├── smoke_rest.sh           # Lightweight REST/RPC health check
    ├── test_all.sh             # Orchestrates unit + E2E suites
    └── test_gasless_enforcement.sh # Manual gateway ante handler probe
```

## Tools & Scripts

### `make simulate-network` (wraps `scripts/simulate_network.sh`) — Docker network simulator

Spins up a seed node, configurable validator/full-node set, and a runner container that drives end-to-end traffic (PQC link/negative path, bank tax assertion, DNS register→auction→settle, gateway contracts, release publish, per-account and global rate-limit bursts). Requirements: Docker, Go, and `jq`. Artifacts (logs, genesis, node data, snapshots, PQC keys) land in `artifacts/sim/{logs,nodes,genesis,keys,snapshots}`.

Flags: `--validators N`, `--fullnodes N`, `--fast`, `--clean`, `--keep`, `--timeout SEC`, `--image <tag>`.

Common invocations:

```bash
make simulate-network              # defaults: 2 validators, 1 full node
LUMEN_RL_GLOBAL_MAX=50 make simulate-network ARGS="--clean --timeout 600"
```

The simulator binds the seed’s RPC/API/P2P endpoints to `localhost:27657/2327/27656` so it can run alongside the Docker
devnet. Override these with `SIM_HOST_RPC_PORT`, `SIM_HOST_API_PORT`, or `SIM_HOST_P2P_PORT` if you need different host
ports.

The target honours `LUMEN_RL_*`, `FAST`, `CLEAN`, `KEEP`, `TIMEOUT`, and `IMAGE_TAG` overrides. See also [`docs/simulation.md`](../docs/simulation.md).

### `make e2e` (wraps `tests/test_all.sh`) — unit + E2E orchestrator

Builds `./build/lumend` once, exports `SKIP_BUILD=1`, and runs:

- `go test ./...`
- `devtools/tests/e2e_*.sh` (send-tax, DNS auction, release, gateways, gov, PQC)
- `devtools/tests/smoke_rest.sh`

Logs land in `artifacts/test-logs/*.log` and a JSON summary in `artifacts/test-report.json`.

### `make build-release` (wraps `scripts/build_release.sh`) — cross-platform release artifacts

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
- `sudo` (only for `make install-service` when not using `ARGS="--print-unit"`)
- Docker ≥ 24.0 (for `--ignorefile` in the builder workflow)

E2E scripts spin up a temporary HOME (`mktemp -d -t lumen-e2e-XXXXXX`) and remove it on exit; set `DEBUG_KEEP=1` to retain the directory for debugging.

> **Toolchain note:** `go.mod` uses Go 1.24 features (tool declarations). When invoking commands with Go 1.23, enable toolchain forwarding (`GOTOOLCHAIN=go1.23.0+auto`) or use Go 1.24+ directly.

## Environment Knobs

| Variable | Default | Affecting | Notes |
|----------|---------|-----------|-------|
| `SKIP_BUILD` | `0` | E2E make targets | Reuse an existing `./build/lumend`. |
| `MODE` | `prod` | `make e2e-dns-auction` | `prod` (default) mirrors production fee routing; `dev` enables the historical proposer-direct env. |
| `FAST`, `CLEAN`, `KEEP`, `TIMEOUT`, `IMAGE_TAG` | `0`, `0`, `0`, `600`, `lumen-node:sim` | `make simulate-network` | Skip heavy spam, force cleanup, retain containers, change timeout/image tag. |
| `SIM_HOST_RPC_PORT`, `SIM_HOST_API_PORT`, `SIM_HOST_P2P_PORT` | `27657`, `2327`, `27656` | `make simulate-network` | Host bindings for the simulator seed; adjust when 27657/2327/27656 are already in use. |
| `DEBUG` | `0` | `make e2e-send-tax` | Enables `set -x` before boot. |
| `RPC_HOST`, `RPC_PORT`, `API_HOST`, `API_PORT`, `GRPC_HOST`, `GRPC_PORT` | `127.0.0.1` / `27657`, `2327`, `9190` | All E2E make targets + `make install-service` | Override bind addresses without editing scripts. |
| `RPC_LADDR`, `API_ADDR`, `GRPC_ADDR` | Derived | All E2E make targets + `make install-service` | Use when you need full endpoint control. |
| `GRPC_WEB_ENABLE` | `1` | Node start scripts + service installer | Set to `0` to disable gRPC-Web. |
| `LOG_FILE` | `/tmp/lumen.log` | E2E suites | Node stdout/stderr destination. |
| `NETWORK_DIR` | unset | `make build-native` | Copies build artifacts to this path when set. |
| `LUMEN_TAX_DIRECT_TO_PROPOSER`, `LUMEN_AUCTION_DIRECT_TO_PROPOSER` | unset | Relevant E2E suites | On/off switches consumed by the app at boot. |
| `LUMEN_RL_PER_BLOCK`, `LUMEN_RL_PER_WINDOW`, `LUMEN_RL_WINDOW_SEC`, `LUMEN_RL_GLOBAL_MAX` | `5`, `20`, `10`, `300` | `make install-service`, `make simulate-network`, `make e2e-gateways` | Clamp the ante decorators; targets only allow tightening (lower numbers). |
| `DEBUG_KEEP` | `0` | E2E suites | Keep the temporary `$HOME` for debugging. |

## Common Workflows

### Build (local developer)

```bash
make build-native
# optionally copy to another folder
NETWORK_DIR=artifacts/bin make build-native
```

### Build (release artifacts)

```bash
make build-release
ls dist/$(git describe --tags --dirty --always)
```

### End-to-end suites

```bash
# Full run: unit tests + all E2E flows + smoke test
HOME=$(mktemp -d) make e2e

# Individual flows (skip rebuild after the first run)
make e2e-send-tax ARGS="--skip-build"
make e2e-dns-auction ARGS="--skip-build --mode prod"
make e2e-gov ARGS="--skip-build"
```

The governance suite (`devtools/tests/e2e_gov.sh`) is fully table-driven. Cases live in
`devtools/tests/gov_param_cases.json` and are executed in order—each entry can either tweak a
single parameter (`"type": "param"`) or describe a richer scenario (`"scenario": ...`) with
messages, votes, deposits, PQC expectations, DNS parameter effects, authority overrides, and
accounting assertions. Shared shell helpers for proposals, voting, waiters, and DNS round-trips
live in `devtools/tests/lib_gov.sh`. The full matrix currently runs ~12–13 minutes and covers
25 cases (ratios/durations/coins + lifecycle/burn/PQC/DNS scenarios).

Helpful knobs when iterating on the governance suite:

- `CASE_FILTER="substring"` limits execution to cases whose `name`/`scenario`/`description`
  contains the given substring (e.g. `CASE_FILTER=dns make e2e-gov ARGS="--skip-build"`).
- `CASE_FILE=/path/to/custom.json` swaps in an alternate case file without touching
  `gov_param_cases.json`.
- `DEBUG_KEEP=1` preserves the temporary HOME directory (`/tmp/lumen-e2e-gov-XXXXXX`) so you
  can inspect logs, node state, or keyrings after a failure.

To extend coverage, add new cases to the JSON file (no code changes needed unless a brand-new
message type is introduced), then run `make e2e-gov ARGS="--skip-build"` to verify both the
individual case and the whole matrix.

### Documentation / OpenAPI

```bash
make docs
```

This regenerates Swagger assets under `artifacts/docs/` and stamps `docs/static/openapi.json` with `git describe --tags --dirty --always`.

### Systemd service

```bash
# dry run (no sudo required)
make install-service ARGS="--print-unit"

# install to /etc/systemd/system/lumend.service (requires sudo)
sudo make install-service
```

## Notes & Caveats

- `tests/e2e_dns.sh` still uses CLI JSON that fails in current builds; keep it around for reference or rewrite using the unsigned-message helpers.
- `tests/test_gasless_enforcement.sh` assumes a running node and pre-seeded keyring; we do not exercise it in CI.
- Artifacts land under `artifacts/test-logs/` and `artifacts/test-report.json` when running the orchestrator.
- `private/init.sh` (outside this folder) stays untracked and should not be committed.
