# Network Simulation (Docker)

This document explains how to spin up a small Lumen network (seed + validators + full nodes), exercise PQC, DNS, Gateways, Release flows, and stress the rate-limit logic using Docker.

The orchestrator is exposed via `make simulate-network` (wrapping `devtools/scripts/simulate_network.sh`).

## Requirements
- Docker (with permission to access `/var/run/docker.sock`)
- `jq`, `bash`, `go`

## What it does
- Builds `lumend` locally and packs a slim runtime image.
- Creates a Docker network, a seed node, N validators, and optional full nodes.
- Generates deterministic test mnemonics, funds accounts, and links PQC keys.
- Drives these flows end-to-end:
  - **PQC:** link-account, negative path (disabled signing), optional rotate (auto-skipped if CLI verb missing).
  - **Bank + Tax:** verifies the fixed 1% model via on-chain params (sender/recipient deltas).
  - **DNS:** register → auction → settle, and reads back owner state.
  - **Gateways:** create/claim/finalize a contract; optional endpoint verb auto-detected.
  - **Release:** publishes to the `beta` channel with a synthetic artifact.
  - **Rate-limit:** per-account and optional global windows with hints to tune ENV caps.

## CLI

quick run, cleans artifacts, shorter spam tests

```
make simulate-network ARGS="--fast --clean"
```

larger run with more validators/fullnodes

```
make simulate-network ARGS="--validators 3 --fullnodes 2 --timeout 900 --clean"
```

preserve containers for inspection

```
make simulate-network ARGS="--keep"
```

## Environment knobs (rate-limit)
The script passes these ENV variables into nodes (sane defaults provided):
- `LUMEN_RL_PER_BLOCK` (default 5)
- `LUMEN_RL_PER_WINDOW` (default 20)
- `LUMEN_RL_WINDOW_SEC` (default 10)
- `LUMEN_RL_GLOBAL_MAX` (default 300)

Example:

```
LUMEN_RL_GLOBAL_MAX=50 make simulate-network ARGS="--clean"
```

## Artifacts & Logs
- Artifacts: `artifacts/sim/` (nodes data, genesis, logs, snapshots)
- Logs: `artifacts/sim/logs/seed-0.log`, `val-*.log`, `full-*.log`

## CI (optional)
A GitHub workflow can invoke:

```
make simulate-network ARGS="--fast --timeout 300 --clean"
```

This provides smoke coverage of the full runtime without exposing ports publicly.
