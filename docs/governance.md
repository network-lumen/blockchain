# Governance Notes

Lumen follows the Cosmos SDK governance model: each module exposes a `MsgUpdateParams` entry point guarded by the on-chain authority (typically the governance module). Use these levers to tune behaviour without modifying binaries.

## Parameter Examples

- **`x/dns`:** adjust the dynamic multiplier (`base_fee_dns`), lifecycle windows (`grace_days`, `auction_days`), DAO floors (`min_price_ulmn_per_month`), or the tier tables that control surcharges for short names/extensions.
- **`x/gateways`:** set `platform_commission_bps`, minimum contract price, action fee, or the delay before contracts can be finalised.
- **`x/release`:** maintain publisher allowlists, add/remove channels, tweak anti-spam fees, or enforce validation for stable releases.
- **`x/tokenomics`:** control emission schedule (`initial_reward_per_block_lumn`, `halving_interval_blocks`), tax rate, and distribution cadence.

## Operational Guidance

- Document every parameter change in governance proposals to keep operators aligned.
- Prefer params over ad-hoc environment variables; the chain only honours `LUMEN_RL_*` at runtime.
- When adjusting rates or fees, consider the impact on existing contracts (gateways) and pending auctions (DNS).
