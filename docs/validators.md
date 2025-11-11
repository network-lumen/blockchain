# Validator & Operator Guide

This guide covers the essentials for running a Lumen node in production or staging environments.

## Building the Binary

```bash
# Local build (produces build/lumend)
make

# Cross-platform release artifacts (dist/<version>/…)
make build-release
```

Release artifacts contain Linux amd64/arm64 and macOS arm64 binaries plus `SHA256SUMS`.

## Systemd Service

Generate or install the unit file using the helper script:

```bash
# Dry run (prints unit to stdout)
make install-service ARGS="--print-unit"

# Install under /etc/systemd/system/lumend.service (requires sudo)
sudo make install-service
sudo systemctl enable --now lumend
```

The default unit starts the node with:

- `--minimum-gas-prices 0ulmn` (gasless operation)
- REST (`:2327`), gRPC (`:9190`), and gRPC-Web enabled on localhost
- Rate-limit env vars exported to the process (see below)

## Rate-Limit Environment Variables

Set these before launching `lumend` (or edit the systemd unit environment block):

| Variable | Default | Meaning |
|----------|---------|---------|
| `LUMEN_RL_PER_BLOCK` | `5` | Max gasless transactions per block per sender |
| `LUMEN_RL_PER_WINDOW` | `20` | Max gasless transactions within the sliding window |
| `LUMEN_RL_WINDOW_SEC` | `10` | Sliding-window length in seconds |

Example overrides:

```bash
export LUMEN_RL_PER_BLOCK=10
export LUMEN_RL_PER_WINDOW=40
export LUMEN_RL_WINDOW_SEC=30
lumend start --minimum-gas-prices 0ulmn
```

## Useful REST Queries

```bash
API=http://127.0.0.1:2327

# DNS
curl -s "$API/lumen/dns/v1/params" | jq
curl -s "$API/lumen/dns/v1/domain/example.lumen" | jq

# Gateways
curl -s "$API/lumen/gateway/v1/params" | jq
curl -s "$API/lumen/gateway/v1/gateways" | jq '.gateways[] | {id, operator, active}'

# Release metadata
curl -s "$API/lumen/release/params" | jq
curl -s "$API/lumen/release/latest?channel=stable&platform=linux-amd64&kind=daemon" | jq
```

## Networking & Security

- Run validators with `--minimum-gas-prices 0ulmn`; the rate-limit decorator supplies DoS protection.
- Place a TLS-enabled reverse proxy (nginx, Caddy, envoy, …) with request rate limiting in front of `:2327` if the REST API is exposed publicly.
- Keep keys in an OS keyring, KMS, or HSM when possible. Avoid the `--keyring-backend test` setting outside of development.
- Monitor module accounts via `lumend q bank balances <module-address>` to confirm fee/tax flows.

## Upgrades & Releases

- Use `make build-release` to produce reproducible artifacts.
- Tag releases (`git tag vX.Y.Z && git push origin vX.Y.Z`) after running the validation checklist in [`docs/releases.md`](releases.md).
- Update operators with parameter changes from governance proposals (see [`docs/params.md`](params.md)).
