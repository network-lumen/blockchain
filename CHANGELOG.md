# CHANGELOG

## [Unreleased]

## [0.9.0] - 2025-11-09

First public milestone for the Lumen blockchain. Highlights:

- **Core chain & PQC**
  - Dual-sign ante decorator enforcing Dilithium3 signatures for every EOA (`x/pqc`), client-side CLI helpers, and keystore UX.
  - Gasless execution with hardened rate-limit knobs (`LUMEN_RL_*`) and zero-fee enforcement throughout the ante pipeline.
- **DNS module (`x/dns`)**
  - Domain lifecycle (register → update → renew → transfer → auction → settle) with dynamic pricing tiers, PoW throttling, and REST search endpoints.
- **Gateways module (`x/gateways`)**
  - Operator registry, prepaid contracts, escrow + claim/cancel/finalize flows, and treasury commission hooks.
- **Release module (`x/release`)**
  - Artifact publishing, mirroring, emergency toggles, validation/rejection flows, and REST queries for channels/latest/by-version.
- **Tokenomics module (`x/tokenomics`)**
  - Chain-wide tax parameters, halving cadence, dust guards, and governance-controlled knobs surfaced via gRPC/REST.
- **Tooling & docs**
  - Docker-based simulator (`devtools/scripts/simulate_network.sh`), comprehensive e2e suites, and authored docs covering modules, PQC policy, and operator guidance.

This tag serves as the compatibility anchor for downstream SDKs (e.g., `@lumen-chain/sdk`); breaking changes will be announced in future changelog entries.
