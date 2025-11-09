# Module `x/dns`

Lumen’s DNS module manages domains (`name.ext`) with paid registrations, renewals, transfers, and an expiration/auction lifecycle. Pricing and behaviour are driven entirely by module parameters. Fees are computed as:

```
monthly_min_price (ulmn) *
domain_tier_multiplier *
extension_tier_multiplier *
base_fee_dns (dimensionless) *
months_requested
```

`min_price_ulmn_per_month` sets the DAO-controlled floor, the tier tables express how much short names/extensions are surcharged (multipliers are stored in basis points), and `base_fee_dns` acts as the dynamic congestion multiplier constrained by `[floor, ceiling]`.

## Transactions

Use AutoCLI (`lumend tx dns --help`) or any Cosmos SDK client to broadcast the following messages:

- `MsgRegister domain ext --records ... --duration-days N --owner <bech32?>`
- `MsgRenew domain ext --duration-days N`
- `MsgUpdate domain ext --records ...`
- `MsgTransfer domain ext --new-owner <bech32>`
- `MsgBid domain ext --amount <ulmn>`
- `MsgSettle domain ext`

Notes:

- `duration_days` defaults to 365 when omitted and cannot exceed 365 days for either register or renew.
- Domain names and extensions are ASCII-only (lowercase `[a-z0-9-]`, no leading/trailing hyphen); internationalized domains (IDN/punycode) are not supported today.
- Up to 64 records per domain, with a combined key/value payload ≤ 16 KiB.
- Transfers move ownership immediately after the fixed `transfer_fee_ulmn` is paid.
- Auctions begin automatically once `grace_days` elapse; `MsgSettle` finalises the highest bid at the end of the auction window.
- `MsgUpdate` enforces a per-domain cooldown (`update_rate_limit_seconds`) and a lightweight proof-of-work: the client must supply a `pow_nonce` such that `sha256(fqdn|creator|nonce)` contains at least `update_pow_difficulty` leading zero bits. Set the difficulty to `0` to disable PoW.

## Parameters

`GET /lumen/dns/v1/params` returns:

- `base_fee_dns`: unitless multiplier applied after tiers.
- `alpha`: adjustment aggressiveness for dynamic pricing.
- `floor`, `ceiling`: lower/upper bounds for the base fee.
- `t`: nominal DNS ops per block target (internal metric).
- `grace_days`, `auction_days`: lifecycle windows after expiration.
- `transfer_fee_ulmn`: fixed fee charged on ownership transfers.
- `bid_fee_ulmn`: flat fee charged on every auction bid.
- `update_rate_limit_seconds`: minimum spacing between two `MsgUpdate` calls on the same domain.
- `update_pow_difficulty`: number of leading zero bits required in the update PoW (0 disables it).
- `min_price_ulmn_per_month`: DAO floor before tiers are applied.
- `domain_tiers`, `ext_tiers`: ordered lists of `{max_len, multiplier_bps}` entries describing how short names/extensions are surcharged (the last tier uses `max_len = 0` to denote “infinite”).

Governance can update these via `MsgUpdateParams`.

## Queries

```sh
# Parameters
curl -s http://127.0.0.1:1317/lumen/dns/v1/params | jq

# Domain details (owner, records, expiry, status)
curl -s http://127.0.0.1:1317/lumen/dns/v1/domain/example.lumen | jq

# Resolver-friendly path (ignored segments kept for compatibility)
curl -s http://127.0.0.1:1317/lumen/dns/v1/resolve/example/lumen/--/--/--/0/active | jq

# Auction status
curl -s http://127.0.0.1:1317/lumen/dns/v1/auction/<name.ext> | jq
```

## Lifecycle Reference

| Phase   | Condition                                              |
|---------|--------------------------------------------------------|
| active  | `now < expire_at`                                      |
| grace   | `expire_at ≤ now < expire_at + grace_days`             |
| auction | `expire_at + grace_days ≤ now < expire_at + grace_days + auction_days` |
| free    | otherwise                                              |

## Operational Tips

- Run nodes with `--minimum-gas-prices 0ulmn` to honour the gasless flow; the ante decorator enforces rate limits via `LUMEN_RL_*`.
- Use a rate-limiting proxy in front of REST if you expose it publicly.
- AutoCLI exposes JSON schema hints: `lumend tx dns register --help` prints accepted flags and formats.
