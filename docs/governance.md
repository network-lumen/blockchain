# Governance Notes

Lumen wires the upstream Cosmos SDK `x/gov` module directly into the application. DAO authority is deliberately scoped:
only the DNS, gateways, release, and non-fundamental tokenomics knobs accept `MsgUpdateParams`. Core SDK modules
(`x/auth`, `x/bank`, `x/staking`, `x/distribution`, `x/consensus`, `x/gov`, and `x/pqc`) reject governance attempts and
require a binary upgrade instead.

## Module wiring & defaults

- **Authority:** DNS, gateways, release, and tokenomics keepers still receive `authtypes.NewModuleAddress("gov")`.
  All SDK keepers (auth/bank/staking/distribution/consensus/gov) are wired to an internal `gov-immutable` module
  address so that `MsgUpdateParams` from the DAO are rejected with `ErrInvalidSigner`.
- **Module account:** `gov` owns a burn-enabled module account so that failed/withdrawn deposits can be slashed when
  the relevant burn flags are set.
- **Metadata cap:** the runtime configuration sets `max_metadata_len = 4096` bytes for proposal metadata.
- **Distribution params:** the Cosmos distribution module is initialised with `community_tax = 0` and keeps the proposer
  reward fields untouched. Because `x/distribution` is wired to the immutable authority, governance proposals cannot
  raise the community tax; only module-specific flows (e.g. DNS/Gateways) fund the community pool.
- **Default parameters (fixed):** quorum `0.67`, threshold `0.75`, veto `0.334`, expedited threshold `0.85`,
  `max_deposit_period/voting_period = 172800s`, `expedited_voting_period = 86400s`, and deposits of
  `10,000,000 ulmn` (regular) / `50,000,000 ulmn` (expedited). These values are hard-coded; proposals attempting to
  change them will fail because `x/gov` no longer accepts `MsgUpdateParams`. Duration fields always use Go/protobuf
  literals such as `"60s"` or `"172800s"`. The current params live under `app_state.gov.params` and are queryable via
  `lumend q gov params`.

## Immutable vs governable surface

- ✅ Governable: `x/dns`, `x/gateways`, `x/release`, and tokenomics' soft knobs (`tx_tax_rate`, `min_send_ulmn`,
  `distribution_interval_blocks`).
- ❌ Immutable: `x/auth`, `x/bank`, `x/staking`, `x/distribution`, `x/consensus`, `x/gov`, `x/pqc`, and the tokenomics
  supply/currency fields (`denom`, `decimals`, `supply_cap_lumn`, `halving_interval_blocks`,
  `initial_reward_per_block_lumn`). Governance proposals that touch these areas will be rejected.

## Submitting proposals

Use the standard Cosmos CLI flow. The messages array can contain any combination of SDK or custom module messages.
Example (update the DNS `update_fee_ulmn`):

```bash
AUTH=$(lumend q auth module-account gov -o json | jq -r '.account.base_account.address')
lumend q dns params -o json | jq '.params' > /tmp/dns_params.json
jq '.update_fee_ulmn="250000"' /tmp/dns_params.json > /tmp/dns_params_updated.json
jq -n \
  --arg auth "$AUTH" \
  --argjson params "$(cat /tmp/dns_params_updated.json)" \
  '{ "@type": "/lumen.dns.v1.MsgUpdateParams", authority: $auth, params: $params }' \
  > /tmp/dns_msg.json
jq -n \
  --arg title "Set DNS update fee" \
  --arg summary "Charge 0.25 LMN per MsgUpdate" \
  --arg metadata "ipfs://dao/dns-update-fee" \
  --argjson dns "$(cat /tmp/dns_msg.json)" \
  '{messages: [$dns], title: $title, summary: $summary, metadata: $metadata}' \
  > /tmp/proposal.json

lumend tx gov submit-proposal /tmp/proposal.json \
  --from validator \
  --deposit 10000000ulmn \
  --chain-id lumen-local-1 \
  --yes
lumend tx gov vote 1 yes --from validator --yes
```

## Parameter Examples

- **`x/dns`:** adjust the dynamic multiplier (`base_fee_dns`), lifecycle windows (`grace_days`, `auction_days`),
  DAO floors (`min_price_ulmn_per_month`), the new `update_fee_ulmn`, or the tier tables that control surcharges
  for short names/extensions.
- **`x/gateways`:** set `platform_commission_bps`, minimum contract price, action fee, or the delay before contracts
  can be finalised.
- **`x/release`:** maintain publisher allowlists, add/remove channels, tweak anti-spam fees, or enforce validation for
  stable releases.
- **`x/tokenomics`:** adjust the tax rate and distribution cadence. Emission schedule fields are genesis-locked.

## Operational Guidance

- Document every parameter change in governance proposals to keep operators aligned.
- Prefer params over ad-hoc environment variables; the chain only honours `LUMEN_RL_*` at runtime.
- When adjusting rates or fees, consider the impact on existing contracts (gateways) and pending auctions (DNS).
- Use `make e2e-gov` to run the dedicated governance E2E suite that submits, votes, passes, and validates a
  multi-message proposal (including negative deposit coverage).
- Treat the more destructive knobs (`min_initial_deposit_ratio`, `min_deposit_ratio`, `proposal_cancel_ratio`,
  `proposal_cancel_dest`, and the `burn_*` flags) as advanced settings—only flip them when a governance proposal
  clearly documents the operational impact and rollback plan. All burn flags default to `false`; enabling them must be
  an explicit governance decision.
