# CHANGELOG

## [Unreleased]

---

## 🧪 Development versions (pre-1.0)

## [0.12.1] - 2025-11-14

### ⚖️ Governance polish
- Clarified [docs/params.md](docs/params.md) and [docs/governance.md](docs/governance.md) so the burn toggles (`burn_proposal_deposit_prevote`, `burn_vote_quorum`, `burn_vote_veto`) are documented as disabled by default and treated as hazardous knobs that require explicit proposals.
- `devtools/scripts/simulate_network.sh` now forces those burn flags to `false` when bootstrapping the Docker simulator, keeping sandbox runs aligned with mainnet defaults.

## [0.12.0] - 2025-11-14

### 🔒 Governance hardening
- Locked the Cosmos SDK keepers (`x/auth`, `x/bank`, `x/staking`, `x/distribution`, `x/consensus`, `x/gov`) to an internal `gov-immutable` authority so DAO proposals can no longer mutate core chain invariants. Only DNS, gateways, release, and the soft tokenomics knobs remain governable.
- Overrode the `x/gov` defaults at init to enforce quorum `0.67`, threshold `0.75`, veto `0.334`, and expedited threshold `0.85`; proposals attempting to change those values are now rejected. Updated docs to reflect the tighter requirements.

### 🪙 Tokenomics guards
- `denom`, `decimals`, `supply_cap_lumn`, `halving_interval_blocks`, and `initial_reward_per_block_lumn` are now genesis-only. `MsgUpdateParams` compares the stored values and aborts with `tokenomics: <field> is immutable` if a proposal tries to change them.
- Added regression tests for the mutable/immutable split.

### 🧠 PQC registry
- Removed the unused `allow_account_rotate` parameter and associated proto field (Go + TypeScript) so the module surface matches reality—rotation attempts now consistently fail unless they replay the exact same key. Generated code via `make proto` and `npm run gen:proto`.
- `MsgLinkAccountPQC` short-circuits as a no-op when the hash already matches, but refuses to overwrite existing hashes. Docs (`modules/pqc.md`, `params.md`, `params_introspection.md`) now describe the PoW/balance guards and permanent rotation ban.

### 📚 Docs
- Updated [docs/governance.md](docs/governance.md) and [docs/params.md](docs/params.md) to document the carved-out governance surface and the hardened vote thresholds.
- Refreshed [docs/params_introspection.md](docs/params_introspection.md) with DAO/❌ annotations for every module and clarified which tokenomics/PQC fields are immutable.

## [0.11.0] - 2025-11-13

### 🗳 Governance integration
- Wired the Cosmos SDK `x/gov` module into the app wiring (module account with burner permissions, begin/end blockers, keeper injection, param authority).
- Added the governance CLI wiring so `lumend tx gov ...` commands are available alongside the existing DNS/gateways/release flows.

### 📚 Docs & tests
- Expanded [docs/governance.md](docs/governance.md) with a full CLI walkthrough and called out advanced knobs/caution notes in [docs/params.md](docs/params.md).
- Documented the governance tooling in the root README and `devtools/README.md`, including usage of `make e2e-gov`.
- Grew the table-driven governance E2E suite to 25 cases covering ratios, durations, deposits, burn/cancel flags, PQC-negative paths, and DNS parameter mutations.

## [0.10.1] - 2025-11-12

### 🧭 DNS module
- **New parameter `update_fee_ulmn`**: a flat fee (in `ulmn`) charged on every `MsgUpdate`.
  - **Defaults to `0`** → updates remain gasless unless configured.
  - The fee is **deducted from the creator** and **credited to the fee collector** module account (`auth/fee_collector`).
  - **Event enrichment:** `dns_update` now includes a `fee_ulmn` attribute for transparency.
- **Params / Genesis / Query:** the field is fully integrated into the `Params` structure, persists through genesis, and is exposed via the `/params` endpoint.
- **Events doc:** `dns_update` event attributes (`name`, `fee_ulmn`) are now documented for explorers/tooling.
- **Compatibility:** non-breaking addition (new proto field, no state migration required). PoW and rate-limit enforcement remain unchanged and compatible.

### 🧪 Tests
- **Covered cases:**
  - `update_fee_ulmn > 0` → fee is deducted from creator and credited to fee collector; event `fee_ulmn` matches the charged amount.
  - **Insufficient funds** → transaction fails cleanly, no state changes, no event emitted.
  - `update_fee_ulmn = 0` → no fee deducted; event shows `fee_ulmn="0"`.
  - **Genesis round-trip** confirms parameter persistence across init/export cycles.

---

## [0.10.0] - 2025-11-11

Major dev milestone introducing PQC hardening, on-chain footprint reduction, and unified Make-based workflows.

### 🧩 Core chain & PQC
- **Hash-only storage:** the `x/pqc` module now stores only the SHA-256 hash of the public key (`pub_key_hash`), removing full PQC keys from state.
- **PoW requirement:** `MsgLinkAccountPQC` now requires a proof-of-work (`pow_nonce`) computed client-side; difficulty adjustable via the new param `pow_difficulty_bits`.
- **Economic guard:** a new param `min_balance_for_link` ensures an account must hold a minimal spendable balance before linking its PQC key.
- **Ante verification:** PQC ante-handler now validates `sha256(pubkey)` equality, key size, and signature correctness against the provided PQC key.
- **CLI improvements:** automatic PoW mining when linking, clear errors for invalid PoW or insufficient balance.
- **Events & params:** enriched events (`pow_difficulty`, `min_balance`) and extended param validation.
- **Breaking proto change:**
  - `AccountPQC`: field `pub_key` replaced by `pub_key_hash` (bytes, 32 B).
  - `PQCSignatureEntry`: new field `pub_key` (bytes).
  - `Params`: added `min_balance_for_link (Coin)` and `pow_difficulty_bits (uint32)`.

### ⚙️ Devtools & E2E
- **Unified ports:** defaults switched to RPC `27657`, API `2327`, gRPC `9190`.
- **Simulator:** `devtools/scripts/simulate_network.sh` now exposes host ports via `SIM_HOST_*` and waits for node readiness through `HOST_RPC_URL`.
- **E2E tests:** 
  - Negative tests added for PQC link failure (no balance / invalid PoW).
  - All E2E suites (`dns`, `auction`, `gateways`, `release`, `send-tax`) updated with `--node` propagation and environment consistency.
- **Minor fix:** added default `NODE=${NODE:-$RPC_LADDR}` in `e2e_gateways.sh` to prevent undefined variable errors.

### 🧰 Makefile & DX
- **Make-first workflow:** all `.sh` invocations replaced by `make <target>` equivalents across docs.
- **New/updated targets:**  
  `simulate-network`, `e2e-dns`, `e2e-dns-auction`, `e2e-gateways`, `e2e-pqc`, `e2e-release`, `e2e-send-tax`, `smoke-rest`, and `help`.
- **Aliases:** optional `test-e2e-*` aliases available for CI/dev ergonomics.
- **Docs updated:** full replacement of `.sh` examples with `make`, plus unified table of environment variables.

### 📘 Documentation
- `pqc.md`: updated to describe hash-based linking, PoW workflow, and minimum balance enforcement.
- `params.md`: documents new PQC parameters.
- `devtools/README.md` and root `README.md`: reflect new Make targets, port defaults, and environment overrides.
- Added quickstart section using only `make` commands for local dev.

---

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
