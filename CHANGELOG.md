# CHANGELOG

## [1.0.1] - 2025-11-18

First stable release of the Lumen blockchain.

- **Post-quantum by default** – Dilithium3 dual-signing is enforced for every EOA, with CLI helpers, encrypted keystore, and bootstrap scripts so validators can come online safely.
- **Full module set** – DNS lifecycle, gateway contracts, release publisher, and tokenomics/tax knobs are all wired, documented, and governable through hardened Cosmos SDK governance defaults.
- **Gasless UX with safeguards** – Chain-wide rate limits and PoW throttles keep DNS/PQC operations free while defending validators.
- **Operator tooling & tests** – Make-based workflows, Docker simulator, validator bootstrap script, and comprehensive e2e suites (send-tax, DNS, gateways, release, governance, PQC, bootstrap validator) ship alongside the chain.

This tag is the compatibility anchor for downstream SDKs; subsequent breaking changes will be documented in future entries.
