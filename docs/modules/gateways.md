# Module `x/gateways`

## Overview

The gateways module matches Lumen clients with registered gateway operators. It manages fixed-price monthly contracts,
escrows client funds in module accounts, settles payouts (minus the chain’s commission), and enforces finalization /
refund flows. All transactions remain gasless—the ante rate limits (`LUMEN_RL_*`) still apply, but no fees are charged.

## Core Entities

- **Gateway** – `{id, operator, payout, metadata, active, created_at, active_clients, cancellations}`
- **Contract** – `{id, client, gateway_id, price_ulmn_per_month, storage_gb_per_month, network_gb_per_month,
  months_total, start_time, escrow_ulmn, claimed_months, status, metadata, next_payout_time}`
- **Statuses** – `PENDING → ACTIVE → COMPLETED → FINALIZED` (or `CANCELED`)
- **Module accounts** – `GatewaysEscrow` (holds client deposits) and `GatewaysTreasury` (platform commission)

## Transactions (AutoCLI: `lumend tx gateways …`)

- `register-gateway [payout]` – Signer becomes the operator for a new gateway (pays `register_gateway_fee_ulmn`)
- `update-gateway [gateway_id]` – Toggle active flag, payout account, or metadata blob (≤512 bytes)
- `create-contract [gateway_id] [price_ulmn] [storage_gb] [network_gb] [months_total]` – Client deposits
  `price * months_total` into escrow; checks `min_price_ulmn_per_month`
- `claim-payment [contract_id]` – Gateway operator withdraws the next scheduled payout
- `cancel-contract [contract_id]` – Client cancels an active contract; remaining escrow (minus current month) is refunded
- `finalize-contract [contract_id]` – Anyone finalizes a completed contract after `finalize_delay_months`, distributing
  rewards and leftover escrow
- `update-params` – Governance-only; adjusts the parameter set below

## Parameters (`GET /lumen/gateway/v1/params`)

- `platform_commission_bps` – Portion of each payout kept by the chain
- `month_seconds` – Base interval used to schedule payouts
- `finalize_delay_months` – Cooldown before a completed contract may be finalized
- `finalizer_reward_bps` – Reward paid (from leftovers) to the caller who finalizes
- `min_price_ulmn_per_month` – Floor applied when clients create contracts
- `max_active_contracts_per_gateway` – Cap on simultaneous active contracts per gateway
- `action_fee_ulmn` – Flat fee charged for gateway-side actions (create/update/claim/finalize)
- `register_gateway_fee_ulmn` – One-time fee charged on gateway registration

All parameters are governable via `MsgUpdateParams`.

## Queries (REST/gRPC-gateway)

- `GET /lumen/gateway/v1/params`
- `GET /lumen/gateway/v1/authority`
- `GET /lumen/gateway/v1/module_accounts`
- `GET /lumen/gateway/v1/gateways?limit=…` (default 50, capped at 200)
- `GET /lumen/gateway/v1/gateways/{id}`
- `GET /lumen/gateway/v1/contracts?status=&client=&gateway_id=&offset=&limit=…`
- `GET /lumen/gateway/v1/contracts/{id}`

```sh
curl -s localhost:1317/lumen/gateway/v1/params | jq
curl -s localhost:1317/lumen/gateway/v1/gateways | jq '.gateways[] | {id, operator, active}'
curl -s localhost:1317/lumen/gateway/v1/contracts?status=ACTIVE | jq '.contracts[] | {id, client, gateway_id}'
```

## Operational Notes

- Gateway metadata is opaque JSON/bytes; use it for contact details or discovery hints.
- Rate-limits: only `MsgRegisterGateway` bypasses the per-sender rate limiter; day-to-day ops (update/claim/cancel/finalize) are throttled via `LUMEN_RL_*`, so space automation accordingly.
- `CreateContract` charges the send-tax up front, so clients must hold slightly more than the escrow amount.
- `ClaimPayment` moves the monthly payout to the operator, sends the commission to `GatewaysTreasury`, and bumps
  `claimed_months`.
- `CancelContract` retains the current month’s payment (plus commission) and refunds the rest of the escrow to the client.
- `FinalizeContract` honors `finalize_delay_months`, pays the caller the configured reward, refunds leftovers, and frees
  the gateway’s active slot.
- All denomination handling goes through `app/denom.BaseDenom`; changing the chain denom only requires updating that file.
