# Module `x/tokenomics`

## Overview

`x/tokenomics` controls Lumen’s monetary policy (block rewards, halving cadence, supply cap) and the network-wide tax
rate used by ante decorators (e.g., send-tax). It also enforces dust guards (`min_send_ulmn`) and distributes tax revenue
to validators. All transactions are gasless—module fees and taxes fund validators.

## Transactions (`lumend tx tokenomics …`)

- `update-params` – Governance-only; sets the parameter set described below. Authority is the governance module address.

The module has no user-facing transactions beyond parameter updates.

## Parameters (`GET /lumen/tokenomics/v1/params`)

| Parameter | Description |
|-----------|-------------|
| `tx_tax_rate` | Decimal string (e.g., `"0.01"`) representing the proportional tax rate applied to eligible transfers |
| `initial_reward_per_block_lumn` | Block reward (LMN) minted at genesis height |
| `halving_interval_blocks` | Number of blocks between reward halvings |
| `supply_cap_lumn` | Hard cap on cumulative LMN supply |
| `decimals` | Denom precision (default 6) |
| `min_send_ulmn` | Minimum `ulmn` amount allowed in `MsgSend` (dust guard) |
| `denom` | Base staking/fee denom (e.g., `ulmn`) |
| `distribution_interval_blocks` | How often (in blocks) the fee collector redistributes rewards |

Governance adjusts these via `MsgUpdateParams`. All fields are required; validation rejects inconsistent values
(e.g., `decimals=0` or `tx_tax_rate < 0`).

## Queries

```sh
API=http://127.0.0.1:1317
curl -s "$API/lumen/tokenomics/v1/params" | jq
```

The module also exposes `TotalMinted` through the keeper (queried indirectly by other modules).

## Operational Notes

- Validators must keep `x/tokenomics` parameters in sync with off-chain monitoring to ensure the halving cadence and tax
  rate match expectations.
- Ante decorators (send-tax) call `GetTxTaxRateBps()` to apply the current tax rate to `MsgSend` transactions; the same
  rate is used when modules seed escrow (e.g., gateways).
- The keeper mints LMN into the module account and immediately forwards them to `FeeCollector` before distribution.
- `min_send_ulmn` prevents dust spam by rejecting microsends before they hit module logic.
