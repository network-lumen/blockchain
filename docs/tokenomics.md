# Tokenomics Overview

Lumen is a gasless chain: validators only accept transactions whose fee is exactly zero, and ante decorators enforce
per-sender quotas (`LUMEN_RL_*`) plus a dust guard (`min_send_ulmn`). Monetary supply growth and redistribution are
handled by the `x/tokenomics` module, while individual application modules (DNS, Gateways, Release, etc.) levy their own
fixed fees or escrow requirements.

## Monetary Base

- **Initial supply:** the chain mints `1 LMN` per block starting at height 1.
- **Halving schedule:** the block reward halves every `31,536,000` blocks (~4 years at 4-second blocks). The cumulative
  supply is capped at `63,072,000 LMN`.
- **Denomination:** LMN has 6 decimal places; `ulmn` is the canonical base denom for staking, transfers, and module fees.

## Emission & Tax

- **Block rewards:** minted into the tokenomics module account, then forwarded to the fee collector before distribution.
- **Tax rate:** `tx_tax_rate = 1%` by default. Ante handlers (e.g., send-tax) split eligible transfers according to this
  rate, depositing proceeds into the fee collector account.
- **Distribution interval:** every 10 blocks the fee collector balance is distributed to validators via the Cosmos
  distribution module.

Governance can adjust these parameters via `MsgUpdateParams` to slow/accelerate inflation, tweak dust guards
(`min_send_ulmn`), or alter the tax rate.

## Gasless Policy & Rate Limits

Even though transactions carry no fees, each node enforces:

- `--minimum-gas-prices 0ulmn`
- `LUMEN_RL_PER_BLOCK` (default 5 tx per block per sender)
- `LUMEN_RL_PER_WINDOW` / `LUMEN_RL_WINDOW_SEC` (20 tx over a 10-second sliding window)
- `LUMEN_RL_GLOBAL_MAX` (global spam guard used in simulator/testing flows)

Every transaction must set its fee to `0`; the `ZeroFeeDecorator` rejects any non-zero fee long before execution. Any
`MsgSend`/`MsgMultiSend` output carrying less than `min_send_ulmn` is rejected before reaching application logic.  
> Note: `min_send_ulmn` s’applique au montant brut envoyé. La taxe de 1 % est prélevée après coup, donc le destinataire
> reçoit légèrement moins que le seuil minimum quand ce dernier est utilisé exactement.

## Treasury Accounts

When the tax or module fees are collected, funds flow into canonical module accounts:

- `FeeCollector` (Cosmos SDK default) receives the global tax, then distributes to validators.
- Module-specific accounts (e.g., `GatewaysEscrow`, `GatewaysTreasury`) hold client escrow and platform commission.

Operators should monitor these accounts via `lumend q bank balances <account-address>` to track inflows/outflows.

## Long-Term Sustainability

- With 4-second blocks and a 1 LMN starting reward, the supply tapers towards 63M LMN over successive halvings.
- The protocol tax (default 1%) captures on-chain economic activity to fund validator rewards even when transaction fees
  are zero.
- Additional module-level fees (DNS registration, gateway action fees, release publish fees) can be tuned via their
  respective params without touching the base tokenomics.

See `docs/modules/tokenomics.md` for implementation details (messages, queries, params) of the `x/tokenomics` module.
