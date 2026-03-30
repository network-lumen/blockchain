# Parameters & Environment

This page lists the configurable knobs exposed by the chain. Governance can only mutate the DNS, gateways, release, and **non-fundamental** tokenomics parameters; all other modules require a binary upgrade or manual intervention.

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
- `update_fee_ulmn` – fixed fee charged on every `MsgUpdate` (defaults to `0`)
- `update_rate_limit_seconds`, `update_pow_difficulty` – `MsgUpdate` guards (durations use standard Go/protobuf literals such as `"2s"` or `"60s"`)
- `min_price_ulmn_per_month` – DAO floor applied before multipliers
- `domain_tiers`, `ext_tiers` – ordered `{max_len, multiplier_bps}` tables controlling surcharges for short names/extensions (last tier uses `max_len = 0` to denote infinity)

> Advanced knobs: `alpha`, `t`, the tier tables, and the `update_pow_difficulty` guard are primarily for economists / protocol engineers. Adjust them only when you fully understand how they feed into DNS pricing and spam resistance.

### `x/gateways`

`GET /lumen/gateway/v1/params`

- `platform_commission_bps` – share retained by the chain on payouts
- `month_seconds` – base duration of a billing month
- `finalize_delay_months` – cooldown before a contract can be finalised
- `finalizer_reward_bps` – reward paid from leftover escrow to the finaliser
- `min_price_ulmn_per_month` – floor price for new contracts
- `max_active_contracts_per_gateway` – concurrency limit per gateway
- `action_fee_ulmn` – flat fee charged on gateway actions (register/update/etc.)

> Advanced knobs: `month_seconds`, `finalize_delay_months`, and `max_active_contracts_per_gateway` should only be tweaked when redesigning the entire gateway lifecycle. Small DAO/operator tweaks should focus on pricing and commissions.

### `x/release`

`GET /lumen/release/params`

- `allowed_publishers` – addresses authorised to publish releases
- `dao_publishers` – addresses allowed to toggle emergency flags
- `channels` – permitted release channels (e.g. `stable`, `beta`)
- `max_artifacts`, `max_urls_per_art`, `max_sigs_per_art`, `max_notes_len`
- `publish_fee_ulmn` – escrowed fee required to publish
- `max_pending_ttl` – lifetime of a pending release before auto-expiry
- `reject_refund_bps` – refund percentage (0–10,000 bps) on rejection
- `require_validation_for_stable` – enforce validation before `stable` installs

> Advanced knobs: `dao_publishers`, the various `max_*` limits, and `reject_refund_bps` directly impact anti-spam safeguards. Treat them as expert-level settings.

### `x/tokenomics`

`GET /lumen/tokenomics/v1/params`

- `tx_tax_rate` – decimal tax applied to eligible transactions
- `min_send_ulmn` – minimum transferable amount per recipient (default `1000`)
- `distribution_interval_blocks` – cadence for validator distribution
- `initial_reward_per_block_lumn` – first-block emission (LMN) *(genesis-locked)*
- `halving_interval_blocks` – blocks between halving events *(genesis-locked)*
- `supply_cap_lumn` – maximum cumulative supply (LMN) *(genesis-locked)*
- `decimals` – chain-wide denom precision (default `6`) *(genesis-locked)*
- `denom` – base staking/fee denom (default `ulmn`) *(genesis-locked)*

> The REST path for tokenomics queries follows the usual gRPC-Gateway convention once the service is registered; use `lumend q tokenomics params` for AutoCLI output.

> Immutable fields: `initial_reward_per_block_lumn`, `halving_interval_blocks`, `supply_cap_lumn`, `decimals`, and `denom` are fixed at genesis. Any proposal trying to modify them will be rejected by the chain.

### `x/pqc`

`GET /lumen/pqc/v1/params`

- `policy` – enforced as `REQUIRED`
- `min_scheme` – minimum accepted Dilithium variant
- `min_balance_for_link` – spendable ULUMEN threshold required before linking
- `pow_difficulty_bits` – difficulty target for `sha256(pubkey || nonce)`
- `ibc_relayer_allowlist` – relayer addresses allowed to bypass PQC for relayer/core IBC messages

> Advanced knobs: `pow_difficulty_bits` and `min_balance_for_link` control security-critical flows. Leave them at defaults unless the PQC module owners propose a coordinated change. Account rotation is permanently disabled; proposals cannot re-enable it. The `policy`/`min_scheme` pair should remain `REQUIRED` / `dilithium3` for the foreseeable future. Governance only touches the dedicated IBC relayer allowlist messages; the rest of `x/pqc` stays immutable.

### `x/gov`

`GET /cosmos/gov/v1/params`

- `min_deposit`, `expedited_min_deposit` – escrow requirements for regular / expedited proposals (defaults: `10,000,000 ulmn` and `50,000,000 ulmn`).
- `max_deposit_period`, `voting_period`, `expedited_voting_period` – Go/protobuf duration strings (e.g. `"60s"`, `"172800s"`) controlling each phase.
- `quorum`, `threshold`, `veto_threshold`, `expedited_threshold` – decimal strings (`0.67`, `0.75`, `0.334`, `0.85` in Lumen).
- `min_initial_deposit_ratio`, `min_deposit_ratio` – ratios enforcing how much of the deposit must be supplied at submit time and in follow-up deposits.
- `proposal_cancel_ratio`, `proposal_cancel_dest` – controls how much of the escrow is burned and where it is redirected on cancellation (defaults to zero / empty).
- `burn_proposal_deposit_prevote`, `burn_vote_quorum`, `burn_vote_veto` – boolean burn toggles; all default to `false` in Lumen.

> Advanced knobs: `min_initial_deposit_ratio`, `min_deposit_ratio`, the cancellation fields, and the burn toggles can destroy deposits or destabilise proposal flow when misused. Reserve them for tightly scoped governance changes with clear operator consensus. Quorum/threshold defaults are hard-coded (67% participation, 75% YES threshold) and cannot be changed via DAO.

`MsgSubmitProposal` is still used to drive DNS/gateways/release updates and the soft tokenomics knobs, but the `x/gov` parameters above are fixed in the binary/genesis and cannot be changed via DAO votes.

## Environment Variables

The application consults a small set of process-level variables at startup:

| Variable | Default | Purpose |
|----------|---------|---------|
| `LUMEN_RL_PER_BLOCK` | `5` | Rate-limit ante: max tx per block per sender |
| `LUMEN_RL_PER_WINDOW` | `20` | Rate-limit ante: max tx in the sliding window |
| `LUMEN_RL_WINDOW_SEC` | `10` | Rate-limit ante: sliding window length in seconds |

Scripts in `devtools/` accept additional environment overrides (e.g., bind addresses, logging paths), but those do **not** alter chain behaviour.
