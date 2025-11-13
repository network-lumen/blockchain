# Governance Notes

Lumen now wires the upstream Cosmos SDK `x/gov` module directly into the application. All module `MsgUpdateParams`
entry-points share the same authority: the `gov` module account (`lmn1...` derived from `gov`). Governance proposals
are therefore able to execute arbitrary messages (parameter changes, treasury spends, emergency toggles) once quorum and
threshold requirements are met.

## Module wiring & defaults

- **Authority:** every module keeper receives `authtypes.NewModuleAddress("gov")` as its authority unless it is
  explicitly overridden in the module configuration. When a `MsgUpdateParams` request specifies a different `authority`,
  it is rejected.
- **Module account:** `gov` owns a burn-enabled module account so that failed/withdrawn deposits can be slashed when
  the relevant burn flags are set.
- **Metadata cap:** the runtime configuration sets `max_metadata_len = 4096` bytes for proposal metadata.
- **Default parameters:** the SDK defaults are reused (min deposit `10,000,000 ulmn`, expedited deposit
  `50,000,000 ulmn`, deposit period `172800s`, voting period `172800s`, expedited voting period `86400s`,
  quorum `0.334`, threshold `0.5`, veto `0.334`, min initial deposit ratio `0.25`, expedited threshold `0.67`,
  `burn_*` flags disabled). Duration fields always use Go/protobuf literals such as `"60s"` or `"172800s"`.
  All values live under `app_state.gov.params` in genesis and are queryable via `lumend q gov params`.

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
- **`x/tokenomics`:** control emission schedule (`initial_reward_per_block_lumn`, `halving_interval_blocks`), tax rate,
  and distribution cadence.

## Operational Guidance

- Document every parameter change in governance proposals to keep operators aligned.
- Prefer params over ad-hoc environment variables; the chain only honours `LUMEN_RL_*` at runtime.
- When adjusting rates or fees, consider the impact on existing contracts (gateways) and pending auctions (DNS).
- Use `make e2e-gov` to run the dedicated governance E2E suite that submits, votes, passes, and validates a
  multi-message proposal (including negative deposit coverage).
- Treat the more destructive knobs (`min_initial_deposit_ratio`, `min_deposit_ratio`, `proposal_cancel_ratio`,
  `proposal_cancel_dest`, and the `burn_*` flags) as advanced settings—only flip them when a governance proposal
  clearly documents the operational impact and rollback plan.
