#!/usr/bin/env bash
set -euo pipefail

# Environment variables:
# - CHAIN_ID (default lumen-local): target chain ID.
# - NODE (default http://127.0.0.1:26657): RPC endpoint used for tx submission.
# - DENOM (default ulmn): fee denom for MsgCreateContract.
# - KEYRING (default test): keyring backend for CLI commands.
# - FROM / FROM_ADDR: gateway account name/address (address auto-derived when unset).
# - GATEWAY_ID: gateway identifier for MsgCreateContract (default 0).
# - CLIENT_ADDR: client bech32 address (defaults to FROM address).
# - PRICE, MONTHS, GAS: price per contract, total months, and gas limit used in broadcasts.
# - CLI (default lumend): binary that sends transactions.
# - LCD (default http://127.0.0.1:1317): optional REST endpoint for helper queries.

CHAIN_ID="${CHAIN_ID:-lumen-local}"
NODE="${NODE:-http://127.0.0.1:26657}"
DENOM="${DENOM:-ulmn}"
KEYRING="${KEYRING:-test}"
FROM="${FROM:-alice}"
FROM_ADDR="${FROM_ADDR:-}"
GATEWAY_ID="${GATEWAY_ID:-0}"
CLIENT_ADDR="${CLIENT_ADDR:-}"
PRICE="${PRICE:-5000000}"
MONTHS="${MONTHS:-1}"
GAS="${GAS:-250000}"
MEMO="gateway:plan:subscribe"

CLI="${CLI:-lumend}"
LCD="${LCD:-http://127.0.0.1:1317}"

if [[ -z "$FROM_ADDR" ]]; then
  FROM_ADDR="$($CLI keys show "$FROM" -a --keyring-backend "$KEYRING")"
fi
if [[ -z "$CLIENT_ADDR" ]]; then
  CLIENT_ADDR="$FROM_ADDR"
fi

echo "Using FROM=$FROM ($FROM_ADDR)  CHAIN_ID=$CHAIN_ID  NODE=$NODE"

send_create_contract() {
  local fee_amount="$1"
  local fee_flag
  if [[ "$fee_amount" == "0" ]]; then
    fee_flag="--fees '' --gas $GAS"
  else
    fee_flag="--fees ${fee_amount}${DENOM} --gas $GAS"
  fi

  echo "==> Broadcasting MsgCreateContract with fee=$fee_amount$DENOM"
  set +e
  TX_OUT=$($CLI tx gateways create-contract \
    --from "$FROM" \
    --keyring-backend "$KEYRING" \
    --chain-id "$CHAIN_ID" \
    --node "$NODE" \
    --yes \
    --broadcast-mode=block \
    --memo "$MEMO" \
    --client "$CLIENT_ADDR" \
    --gateway-id "$GATEWAY_ID" \
    --price-ulmn "$PRICE" \
    --storage-gb-per-month 100 \
    --network-gb-per-month 1000 \
    --months-total "$MONTHS" \
    $fee_flag 2>&1)
  RC=$?
  set -e

  echo "$TX_OUT"

  if [[ "$fee_amount" != "0" ]]; then
    if echo "$TX_OUT" | grep -qiE "invalid fee|gasless tx must have zero fee|ErrInvalidFee|code: [^0]"; then
      echo "OK: fee=$fee_amount rejected as expected."
    else
      echo "BUG: fee=$fee_amount was NOT rejected. Ante is not enforcing."
      return 2
    fi
  else
    if echo "$TX_OUT" | grep -qi "code: 0"; then
      echo "OK: fee=0 accepted as expected."
    else
      echo "BUG: fee=0 was rejected unexpectedly."
      return 2
    fi
  fi
  echo
}

send_create_contract 0
send_create_contract 1
send_create_contract 7

echo "All tests completed."
