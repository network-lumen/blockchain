# Lumen Documentation

This folder is the landing zone for everything related to Lumen: architecture notes, module guides, operator runbooks,
and API references. All pages are plain Markdown so you can read them on GitHub or ship them with your own tooling.

## Quick Start

- **Build:** `make` (or `bash devtools/scripts/build_native.sh`) → `build/lumend`
- **Tests:** `go test ./...` or `bash devtools/tests/test_all.sh` (unit + E2E)
- **OpenAPI:** `make docs` → inspect `artifacts/docs/openapi.swagger.json`

## Architecture Highlights

- **Runtime:** Cosmos SDK v0.53; application modules live under `x/`
- **Block cadence:** CometBFT tuned for ~4s (4s propose + 4s commit)
- **Binary:** single entry point `cmd/lumend`
- **Storage:** Cosmos `collections` for typed key/value access
- **APIs:** gRPC `:9090`, REST gRPC-Gateway `:1317`, OpenAPI served via `docs.RegisterOpenAPIService`
- **Gasless UX:** ante decorators enforce per-sender quotas driven by `LUMEN_RL_*`

### Core Modules

| Module | Path | Purpose |
|--------|------|---------|
| DNS | `x/dns` | Domain registration, renewals, transfers, and expiration-driven auctions (tiered pricing, gasless). |
| Gateways | `x/gateways` | Fixed-price monthly contracts between clients and gateways with escrow, payouts, and finalization logic. |
| Release | `x/release` | On-chain release metadata (channels, artifacts, mirrors, validation) with publisher allowlists. |
| Tokenomics | `x/tokenomics` | Block rewards, halving cadence, tax rate, dust guard, and reward distribution cadence. |
| PQC | `x/pqc` | Mandatory Dilithium registry + dual-sign ante enforcement for all EOAs. |

### Module Snapshots

#### DNS
- Messages: `MsgRegister`, `MsgRenew`, `MsgUpdate`, `MsgTransfer`, `MsgBid`, `MsgSettle`
- Pricing: `min_price_ulmn_per_month × domain_tier × ext_tier × base_fee_dns × months`
- Limits: 64 records / 16 KiB payload, lifecycle = active → grace → auction → free
- Queries: `/lumen/dns/v1/params`, `/domain/{name.ext}`, `/resolve/{name}/{ext}`, `/auction/{id}`

#### Gateways
- Entities: gateways (`id`, `operator`, `payout`, `metadata`) and contracts (`price`, `months_total`, `claimed_months`, status)
- Fees: minimum monthly price, platform commission (bps), finalizer reward, action fee, registration fee
- Messages: `register-gateway`, `update-gateway`, `create-contract`, `claim-payment`, `cancel-contract`, `finalize-contract`, `update-params`
- Queries: `/lumen/gateway/v1/params`, `/gateways`, `/gateways/{id}`, `/contracts`, `/contracts/{id}`, `/module_accounts`

#### Release
- Flow: `MsgPublish` (semver + artifacts) → optional `MsgMirror`/`MsgYank` → authority `MsgValidateRelease` / `MsgRejectRelease`
- Params: `allowed_publishers`, `channels`, `publish_fee_ulmn`, `max_artifacts`, `reject_refund_bps`, `require_validation_for_stable`
- Queries: `/lumen/release/params`, `/releases`, `/latest`, `/by_version/{semver}`, `/release/{id}`

#### Tokenomics
- Parameters: `tx_tax_rate`, `initial_reward_per_block_lumn`, `halving_interval_blocks`, `supply_cap_lumn`, `min_send_ulmn`, `distribution_interval_blocks`, `denom`
- Defaults: 1% tax, 1 LMN block reward, ~4-year halving cadence, 63,072,000 LMN cap, 6 decimals
- Usage: ante decorators apply the tax; gateways and other modules consult the same rate before seeding escrow

#### PQC
- Every EOA must link a Dilithium key and include a PQ signature (no exceptions)
- CLI helpers (`lumend keys pqc-*`) are available in `dev`/`test` builds
- Ante decorator fetches the registry entry and verifies signatures embedded in `PQCSignatures` extensions

### Tooling & Ops

- **AutoCLI:** `lumend tx --help`, `lumend q --help`
- **Devtools:** build/test/sim scripts live in `devtools/`
- **Protobuf:** `buf generate --template proto/buf.gen.gogo.yaml`
- **Security:** run validators with `--minimum-gas-prices 0ulmn`, respect `LUMEN_RL_*`, and front REST with a rate-limited proxy
- **Useful Queries:** see [`docs/modules/*`](modules) for per-module cheat sheets

## Reference Map

| Topic | File |
|-------|------|
| Validator / operator guide | [`docs/validators.md`](validators.md) |
| Module deep dives | [`docs/modules/*.md`](modules) |
| Governance & parameters | [`docs/governance.md`](governance.md), [`docs/params.md`](params.md) |
| Tokenomics (macro) | [`docs/tokenomics.md`](tokenomics.md) |
| Release checklist | [`docs/releases.md`](releases.md) |
| Security posture | [`docs/security.md`](security.md) |
| Changelog | [`CHANGELOG.md`](../CHANGELOG.md) |

Static API assets live under [`docs/static/`](static) and are served by `docs.RegisterOpenAPIService`. The Go modules
themselves reside in `x/`, with supporting scripts in `devtools/`.
