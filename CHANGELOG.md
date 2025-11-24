# CHANGELOG

## v1.2.0 — 2025-11-24

### Added
- Store-level immutability for PQC link-account records in `x/pqc`, so each Cosmos account can register a Dilithium key exactly once.
- PQC-aware staking transaction wiring: `lumend tx staking delegate`, `redelegate`, `unbond`, and `cancel-unbond` now route through `pqctxext.GenerateOrBroadcastTxCLI`, ensuring PQC signatures are injected for all staking flows.

### Changed
- `MsgLinkAccountPQC` semantics: re-linking an already-linked account (even with the same key material) now returns `ErrAccountAlreadyLinked` instead of being treated as a no-op.
- PQC CLI `tx pqc link-account` performs a best-effort registry lookup before broadcasting, surfacing a clear “already linked” PQC error when an account is already registered.
- No proto definitions or CLI flags were changed; behavior changes are scoped strictly to PQC linking and staking transaction wiring.

### Fixed
- Prevented accidental PQC key “rotation” by rejecting all attempts to overwrite an existing PQC registry entry, eliminating inconsistencies between on-chain state and local PQC key management.

### Tests
- `x/pqc/keeper` unit tests updated to assert that:
  - The first `LinkAccountPQC` call succeeds and the second fails with `ErrAccountAlreadyLinked`.
  - “Rotation” attempts using a different Dilithium key also fail with `ErrAccountAlreadyLinked`.
- `devtools/tests/e2e_pqc.sh` extended to cover:
  - Successful PQC link on the first `tx pqc link-account` and failure on the second link for the same address, with an explicit PQC immutability/“already linked” error.
  - Successful PQC-enabled staking delegation (`tx staking delegate`) when PQC is required.
  - Rejection of staking delegation when `--pqc-enable=false` under PQC-required policy, with a PQC-related error.
- `make e2e` remains fully green, validating the new PQC behavior alongside existing DNS, gateways, release, governance, tokenomics, and bootstrap suites.

### Breaking Changes
- **PQC account linking is now immutable.** Once a PQC record exists for an account, any subsequent `MsgLinkAccountPQC` for that address is rejected with `ErrAccountAlreadyLinked` (no rotations, no idempotent relink).


## [1.1.0] - 2025-11-21

- **Community pool routing** – All fixed DNS/Gateway fees now fund the community pool via `DistrKeeper.FundCommunityPool`, with fee-collector used only as a fallback.
- **Tokenomics defaults** – Distribution genesis forces `community_tax = 0` and the new `e2e_tokenomics` suite validates tax, rewards, and fee routing end-to-end.
- **DNS economics** – Floor price cut to 2 LMN/month, domain transfer fee raised to 1 LMN, update fee removed; CLI gains `dns renew`/`dns transfer` and update records optional argument.
- **Gateways economics** – Register fee raised to 50 LMN, action fee lowered to 0.001 LMN; fees flow to the community pool; CLI adds `register-gateway` and `update-gateway`.
- **Tooling & tests** – `go_test.sh`/`go_with_pkgs.sh` replace raw `go test/vet` in Makefile/docs; e2e scripts randomize ports, clean traps, and set client config; governance cases align with DNS transfer fee.
- **Resilience** – Tokenomics BeginBlocker tolerates missing commission state; PQC helper updates client config automatically; doc updates clarify community tax immutability.

## [1.0.1] - 2025-11-18

First stable release of the Lumen blockchain.

- **Post-quantum by default** – Dilithium3 dual-signing is enforced for every EOA, with CLI helpers, encrypted keystore, and bootstrap scripts so validators can come online safely.
- **Full module set** – DNS lifecycle, gateway contracts, release publisher, and tokenomics/tax knobs are all wired, documented, and governable through hardened Cosmos SDK governance defaults.
- **Gasless UX with safeguards** – Chain-wide rate limits and PoW throttles keep DNS/PQC operations free while defending validators.
- **Operator tooling & tests** – Make-based workflows, Docker simulator, validator bootstrap script, and comprehensive e2e suites (send-tax, DNS, gateways, release, governance, PQC, bootstrap validator) ship alongside the chain.

This tag is the compatibility anchor for downstream SDKs; subsequent breaking changes will be documented in future entries.
