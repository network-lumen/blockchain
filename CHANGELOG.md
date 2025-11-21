# CHANGELOG

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
