# Parameters & Environment

This page lists the configurable knobs exposed by the chain. Unless stated otherwise all parameters are managed on-chain through governance via the respective `MsgUpdateParams` messages.

## Module Parameters

### `x/dns`

`GET /lumen/dns/v1/params`

- `base_fee_dns` – base LMN price per day (decimal string)
- `alpha` – adjustment aggressiveness for dynamic pricing
- `floor`, `ceiling` – minimum/maximum bounds for `base_fee_dns`
- `t` – nominal DNS operations per block (internal metric)
- `grace_days`, `auction_days` – lifecycle windows post-expiration
- `transfer_fee_ulmn` – fixed fee charged on `MsgTransfer`
- `bid_fee_ulmn` – flat fee charged on each `MsgBid`
- `update_rate_limit_seconds`, `update_pow_difficulty` – `MsgUpdate` guards
- `min_price_ulmn_per_month` – DAO floor applied before multipliers
- `domain_tiers`, `ext_tiers` – ordered `{max_len, multiplier_bps}` tables controlling surcharges for short names/extensions (last tier uses `max_len = 0` to denote infinity)

### `x/gateways`

`GET /lumen/gateway/v1/params`

- `platform_commission_bps` – share retained by the chain on payouts
- `month_seconds` – base duration of a billing month
- `finalize_delay_months` – cooldown before a contract can be finalised
- `finalizer_reward_bps` – reward paid from leftover escrow to the finaliser
- `min_price_ulmn_per_month` – floor price for new contracts
- `max_active_contracts_per_gateway` – concurrency limit per gateway
- `action_fee_ulmn` – flat fee charged on gateway actions (register/update/etc.)

### `x/release`

`GET /lumen/release/params`

- `allowed_publishers` – addresses authorised to publish/mirror/yank
- `dao_publishers` – addresses allowed to toggle emergency flags
- `channels` – permitted release channels (e.g. `stable`, `beta`)
- `max_artifacts`, `max_urls_per_art`, `max_sigs_per_art`, `max_notes_len`
- `publish_fee_ulmn` – escrowed fee required to publish
- `max_pending_ttl` – lifetime of a pending release before auto-expiry
- `reject_refund_bps` – refund percentage (0–10,000 bps) on rejection
- `require_validation_for_stable` – enforce validation before `stable` installs

### `x/tokenomics`

`GET /lumen/tokenomics/v1/params`

- `tx_tax_rate` – decimal tax applied to eligible transactions
- `initial_reward_per_block_lumn` – first-block emission (LMN)
- `halving_interval_blocks` – blocks between halving events
- `supply_cap_lumn` – maximum cumulative supply (LMN)
- `decimals` – chain-wide denom precision (default `6`)
- `min_send_ulmn` – minimum transferable amount per recipient (default `1000`)
- `denom` – base staking/fee denom (default `ulmn`)
- `distribution_interval_blocks` – cadence for validator distribution

> The REST path for tokenomics queries follows the usual gRPC-Gateway convention once the service is registered; use `lumend q tokenomics params` for AutoCLI output.

## Environment Variables

The application consults a small set of process-level variables at startup:

| Variable | Default | Purpose |
|----------|---------|---------|
| `LUMEN_RL_PER_BLOCK` | `5` | Rate-limit ante: max tx per block per sender |
| `LUMEN_RL_PER_WINDOW` | `20` | Rate-limit ante: max tx in the sliding window |
| `LUMEN_RL_WINDOW_SEC` | `10` | Rate-limit ante: sliding window length in seconds |

Scripts in `devtools/` accept additional environment overrides (e.g., bind addresses, logging paths), but those do **not** alter chain behaviour.
