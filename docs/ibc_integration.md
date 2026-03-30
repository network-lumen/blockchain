# Integrating Lumen over IBC

This guide documents the IBC integration that is implemented and validated in Lumen today.

It is intentionally narrow: it only describes behaviour that is present in the codebase and covered by the current test suite. It does not promise support for other Cosmos interoperability features that are not wired or not exercised.

## Scope

Lumen currently exposes a minimal IBC surface aimed at token transfers and DEX listing:

- IBC core
- Tendermint light client (`07-tendermint`)
- ICS-20 token transfer (`x/transfer`)

This is the supported path for moving `ulmn` onto another Cosmos chain or bringing ICS-20 vouchers back to Lumen.

The chain does not currently document or guarantee support for:

- ICA
- ICQ
- packet-forward middleware
- custom application channels beyond `transfer`

## Network Version

IBC support is introduced by the on-chain upgrade named `v1.5.0`.

Integrators should confirm that the target Lumen network has already passed that upgrade before attempting to connect a relayer or open a transfer channel.

## What Is Confirmed Working

The current automated two-chain IBC end-to-end test covers the following operational flow:

- `MsgCreateClient`
- `MsgUpdateClient`
- `MsgConnectionOpenInit`
- `MsgConnectionOpenTry`
- `MsgConnectionOpenAck`
- `MsgConnectionOpenConfirm`
- `MsgChannelOpenInit`
- `MsgChannelOpenTry`
- `MsgChannelOpenAck`
- `MsgChannelOpenConfirm`
- user-side `MsgTransfer`
- relayer-side `MsgRecvPacket`
- relayer-side `MsgAcknowledgement`
- relayer-side `MsgTimeout`
- return transfer of an ICS-20 voucher back to the source chain

The same test suite also confirms the following rejection paths:

- IBC relayer tx rejected when the relayer address is not allowlisted for PQC bypass
- IBC relayer tx rejected when it tries to pay zero fee
- `MsgTransfer` rejected when it pays zero fee
- `MsgTransfer` rejected when the user omits the Lumen PQC signature
- `MsgChannelCloseInit` rejected on the ICS-20 transfer channel by application logic

The same end-to-end coverage also validates the exposed CLI query surface against live chains:

- `lumend query ibc client ...`
- `lumend query ibc connection ...`
- `lumend query ibc channel ...`
- `lumend query ibc-transfer params`
- `lumend query ibc-transfer escrow-address`
- `lumend query ibc-transfer denom-hash`
- `lumend query ibc-transfer denom`

The same suite also smoke-tests the standard IBC client transaction CLI in generate-only mode:

- `lumend tx ibc client ...`

The test entry point is:

```bash
make e2e-ibc
```

The script lives at [`devtools/tests/e2e_ibc.sh`](../devtools/tests/e2e_ibc.sh).

## Economic Model: Gas vs Fees

Lumen keeps the validator-level minimum gas price at zero:

- validators must run with `--minimum-gas-prices 0ulmn`
- the node refuses non-zero `minimum-gas-prices`

This does **not** mean gas execution is disabled.

Gas is still measured by the Cosmos SDK execution path. The important distinction is how transaction fees are handled:

- most native Lumen application transactions are still fee-free at the application level
- fee-bearing IBC transactions must include a positive `ulmn` fee
- `staking.MsgEditValidator` must include exactly `1000000ulmn`

In other words:

- IBC tx: gas is metered, and a positive `ulmn` fee is mandatory
- `staking.MsgEditValidator`: gas is metered internally, and the application fee is fixed at `1000000ulmn`
- most other native non-IBC Lumen tx: gas is still metered internally, but the required application fee is zero

This is why integrators should think in terms of "fee-free" vs "fee-bearing", not "gas on" vs "gas off".

## Which Transactions Must Pay Fees

The Lumen ante policy treats the following as fee-bearing:

- `/ibc.applications.transfer.v1.MsgTransfer`
- IBC relayer/core messages such as client creation/update, connection handshake, channel handshake, `RecvPacket`, `Acknowledgement`, and `Timeout`
- `/cosmos.staking.v1beta1.MsgEditValidator`

Those transactions must pay:

- IBC txs: exactly one positive `ulmn` fee coin
- `MsgEditValidator`: exactly `1000000ulmn`

Other native Lumen application transactions remain gasless from the user point of view and must keep a zero fee.

Mixing fee-bearing IBC messages and gasless native Lumen application messages is rejected. Mixing fee-bearing IBC
messages and `MsgEditValidator` in the same transaction is also rejected, as is mixing `MsgEditValidator` with gasless
messages.

## PQC Rules

Lumen uses mandatory Dilithium dual-signing for user transactions.

For IBC, the rule is intentionally split:

### User-initiated ICS-20 transfers

`MsgTransfer` does **not** bypass PQC.

That means a Lumen user sending an IBC transfer must:

- sign the normal Cosmos transaction
- attach the required Lumen PQC signature
- pay a positive `ulmn` fee

The built-in CLI wrapper is:

```bash
lumend tx ibc-transfer transfer [source-channel] [receiver] [amount]
```

This command follows the standard Lumen PQC signing path.

The binary also exposes the standard read-only IBC query groups:

```bash
lumend query ibc ...
lumend query ibc-transfer ...
```

For operator and admin workflows, the binary also exposes the standard IBC client transaction group:

```bash
lumend tx ibc client ...
```

This is available for Cosmos-standard client/admin flows and is smoke-tested in generate-only mode. The currently validated live handshake path in Lumen still uses `rly`.

### Relayer core messages

Relayer-sent core IBC messages may bypass the PQC requirement, but only under strict conditions:

- every message in the transaction must be an allowed IBC core/relayer message
- every signer in the transaction must be in the `x/pqc` parameter `ibc_relayer_allowlist`

This is what lets a standard Cosmos relayer operate against Lumen without implementing the custom Lumen PQC extension format for relay packets and handshakes.

The relayer still needs:

- a normal Cosmos key on Lumen
- positive `ulmn` fees

The PQC bypass is only for the extra Lumen PQC signature requirement. It is not a bypass of normal Cosmos transaction signing.

## Relayer Requirements

The integration tested in Lumen uses the Go relayer (`rly`) with standard Cosmos direct signing.

Minimum requirements on the Lumen side:

- the relayer address on Lumen must hold `ulmn`
- the same address must be present in `x/pqc.params.ibc_relayer_allowlist`
- relayer transactions must pay a positive `ulmn` fee
- `minimum-gas-prices` must still remain `0ulmn` on the node

The supported on-chain management path for that allowlist is governance via the dedicated PQC messages:

- `/lumen.pqc.v1.MsgAddIBCRelayer`
- `/lumen.pqc.v1.MsgRemoveIBCRelayer`

Practical consequence:

- do not try to enable validator min gas prices for IBC
- do not expect relayer transactions with `0ulmn` fee to pass

## Transfer Channel Behaviour

The currently validated application path is the standard ICS-20 transfer port:

- source port: `transfer`
- channel opened by the relayer after client and connection setup

Observed runtime behaviour:

- native `ulmn` sent from Lumen to another chain is escrowed and represented on the destination as an ICS-20 voucher (`ibc/...`)
- returning that voucher to Lumen over the reverse path unescrows native `ulmn`
- timeout refunds work
- transfer-channel close is rejected by the ICS-20 application logic and should not be part of normal operator workflows

Treat the transfer channel as a long-lived channel.

## Rate Limiting

Native gasless Lumen application transactions are subject to the chain's rate-limit decorators.

Fee-bearing IBC transactions are intentionally excluded from those rate limits so that relayers are not throttled by the gasless UX policy.

## CLI Examples

User-side ICS-20 transfer from Lumen:

```bash
lumend tx ibc-transfer transfer channel-0 osmo1... 1000000ulmn \
  --from alice \
  --fees 1000ulmn \
  --node tcp://127.0.0.1:26657 \
  --chain-id lumen-1
```

Optional flags:

- `--source-port` default: `transfer`
- `--packet-timeout-height`
- `--packet-timeout-seconds`
- `--packet-memo`

The command still uses the normal Lumen PQC client flow. If your integration signs transactions outside the Lumen CLI, it must reproduce the same Lumen PQC extension behaviour for user-side `MsgTransfer`.

Read-only operational queries:

```bash
lumend query ibc client states --node tcp://127.0.0.1:26657
lumend query ibc connection connections --node tcp://127.0.0.1:26657
lumend query ibc channel channels --node tcp://127.0.0.1:26657
lumend query ibc-transfer params --node tcp://127.0.0.1:26657
lumend query ibc-transfer escrow-address transfer channel-0
lumend query ibc-transfer denom-hash transfer/channel-0/ulmn --node tcp://127.0.0.1:26657
```

Generate-only admin smoke test:

```bash
lumend tx ibc client delete-client-creator 07-tendermint-0 \
  --from relayer \
  --fees 1000ulmn \
  --generate-only \
  --output json
```

## Integration Checklist for a DEX or Counterparty Chain

1. Verify that the target Lumen network already passed upgrade `v1.5.0`.
2. Fund the relayer address on Lumen with `ulmn`.
3. Add that address to `x/pqc.params.ibc_relayer_allowlist` on Lumen through a governance proposal carrying `/lumen.pqc.v1.MsgAddIBCRelayer`.
4. Keep node `minimum-gas-prices` at `0ulmn`.
5. Configure the relayer to pay positive `ulmn` fees on Lumen.
6. Create clients, connections, and a transfer channel.
7. Test an outbound `MsgTransfer` from Lumen.
8. Test the reverse path back to Lumen.
9. Test a timeout/refund path before treating the route as production-ready.

Example governance message payload:

```json
{
  "@type": "/lumen.pqc.v1.MsgAddIBCRelayer",
  "authority": "<gov-module-address>",
  "relayer": "lmn1..."
}
```

## Current Limits

The current end-to-end test covers the practical ICS-20 path needed for listings and transfers.

It does not currently exercise more exotic or operationally rarer flows such as:

- client upgrade
- client recovery
- `MsgTimeoutOnClose`
- IBC v2 operational flows

Those message types may still be recognized by policy code, but they are not part of the validated minimal DEX-transfer path documented here.

## Reference Files

- App wiring: [`app/ibc.go`](../app/ibc.go)
- Fee policy: [`app/ante_zero_fee.go`](../app/ante_zero_fee.go)
- IBC tx classification: [`app/ibc_tx_policy.go`](../app/ibc_tx_policy.go)
- PQC bypass logic: [`app/ante_pqc_dualsign.go`](../app/ante_pqc_dualsign.go)
- Query CLI wiring: [`cmd/lumend/cmd/commands.go`](../cmd/lumend/cmd/commands.go)
- Root/client CLI wiring: [`cmd/lumend/cmd/root.go`](../cmd/lumend/cmd/root.go)
- User transfer CLI: [`cmd/lumend/cmd/tx_ibc.go`](../cmd/lumend/cmd/tx_ibc.go)
- End-to-end test: [`devtools/tests/e2e_ibc.sh`](../devtools/tests/e2e_ibc.sh)
