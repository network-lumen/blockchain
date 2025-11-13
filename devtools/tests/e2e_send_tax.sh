#!/usr/bin/env bash
set -euo pipefail

# Environment variables (overridable):
#   SKIP_BUILD       Skip rebuilding the binary (default 0)
#   DEBUG            Enable bash tracing before starting the node (default 0)
#   RPC_HOST/PORT    RPC bind host/port (default 127.0.0.1:26657)
#   API_HOST/PORT    REST bind host/port (default 127.0.0.1:1317)
#   GRPC_HOST/PORT   gRPC bind host/port (default 127.0.0.1:9090)
#   GRPC_WEB_ENABLE  Enable gRPC-Web (default 1)
#   NODE             Tendermint RPC URL (defaults to rpc laddr)
#   LOG_FILE         Node log destination (default /tmp/lumen.log)
#   DEBUG_KEEP       Set to 1 to keep the temporary HOME directory on exit

SOURCE_DIR="$(cd "$(dirname "$0")" && pwd)"
. "${SOURCE_DIR}/lib_pqc.sh"
pqc_require_bins

HOME_LUMEN=$(mktemp -d -t lumen-e2e-XXXXXX)
trap '[[ "${DEBUG_KEEP:-0}" = "1" ]] || rm -rf "$HOME_LUMEN" >/dev/null 2>&1 || true' EXIT
export HOME="$HOME_LUMEN"

DIR=$(cd "${SOURCE_DIR}/../.." && pwd)
BIN="$DIR/build/lumend"
: "${LUMEN_BUILD_TAGS:=dev}"
HOME_DIR="${HOME}/.lumen"
RPC_HOST="${RPC_HOST:-127.0.0.1}"
RPC_PORT="${RPC_PORT:-26657}"
RPC_LADDR="${RPC_LADDR:-tcp://${RPC_HOST}:${RPC_PORT}}"
RPC="${RPC:-http://${RPC_HOST}:${RPC_PORT}}"
API_HOST="${API_HOST:-127.0.0.1}"
API_PORT="${API_PORT:-1317}"
API_ADDR="${API_ADDR:-tcp://${API_HOST}:${API_PORT}}"
API="${API:-http://${API_HOST}:${API_PORT}}"
GRPC_HOST="${GRPC_HOST:-127.0.0.1}"
GRPC_PORT="${GRPC_PORT:-9090}"
GRPC_ADDR="${GRPC_ADDR:-${GRPC_HOST}:${GRPC_PORT}}"
GRPC_WEB_ENABLE="${GRPC_WEB_ENABLE:-1}"
NODE=${NODE:-$RPC_LADDR}
LOG_FILE="${LOG_FILE:-/tmp/lumen.log}"
CHAIN_ID="lumen"
FARMER_NAME=farmer
FEE_COLLECTOR_NAME=fee_collector
KEYRING=${KEYRING:-test}
TX_FEES=${TX_FEES:-0ulmn}

keys_add_quiet() {
  local name="$1"
  printf '\n' | "$BIN" keys add "$name" --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null 2>&1 || true
}

step() { echo; echo "==== $*"; }
wait_http() {
  local url="$1"; local tries=60
  for i in $(seq 1 $tries); do
    if curl -sSf "$url" >/dev/null; then return 0; fi; sleep 0.5
  done
  echo "Timeout waiting for $url" >&2
  echo "---- ${LOG_FILE} (tail) ----" >&2
  tail -n 120 "$LOG_FILE" 2>/dev/null >&2 || true
  return 1
}
wait_tx_commit() {
  local hash="$1"; local tries=100
  for i in $(seq 1 $tries); do
    local code
    code=$(curl -s "$RPC/tx?hash=0x$hash" | jq -r .result.tx_result.code)
    if [ "$code" != "null" ]; then echo "tx_code=$code"; return 0; fi
    sleep 0.3
  done
  echo "Timeout waiting for tx $hash" >&2
  return 1
}

kill_node() { pkill -f "lumend start" >/dev/null 2>&1 || true; }

SKIP_BUILD=${SKIP_BUILD:-0}
build() {
  if [ "$SKIP_BUILD" = "1" ] || [ "${1:-}" = "--skip-build" ]; then return 0; fi
  step "Build lumend"
  build_cmd=(go build)
  if [ -n "$LUMEN_BUILD_TAGS" ]; then
    build_cmd+=(-tags "$LUMEN_BUILD_TAGS")
  fi
  build_cmd+=(-o "$BIN" ./cmd/lumend)
  (cd "$DIR" && "${build_cmd[@]}")
}

init_chain() {
  step "Init single-node chain"
  rm -rf "$HOME_DIR"
  "$BIN" init local --chain-id "$CHAIN_ID" --home "$HOME_DIR"
  keys_add_quiet "$FARMER_NAME"
  ADDR_FARMER=$("$BIN" keys show "$FARMER_NAME" -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
  "$BIN" genesis add-genesis-account "$ADDR_FARMER" 100000000000ulmn --keyring-backend "$KEYRING" --home "$HOME_DIR"
  for acct in alice bob carol dave eric frank; do
    keys_add_quiet "$acct"
    local acct_addr
    acct_addr=$("$BIN" keys show "$acct" -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
    "$BIN" genesis add-genesis-account "$acct_addr" 1000000ulmn --keyring-backend "$KEYRING" --home "$HOME_DIR"
  done
  "$BIN" genesis gentx "$FARMER_NAME" 1000000ulmn \
    --chain-id "$CHAIN_ID" --keyring-backend "$KEYRING" --home "$HOME_DIR" \
    --moniker "local-farmer" --commission-rate 0.10 \
    --commission-max-rate 0.20 --commission-max-change-rate 0.01 \
    --min-self-delegation 1 >/dev/null
  "$BIN" genesis collect-gentxs --home "$HOME_DIR" >/dev/null
  "$BIN" genesis validate --home "$HOME_DIR"
}

start_node() {
  [ "${DEBUG:-0}" = "1" ] && set -x
  step "Start node"
  kill_node
  local grpc_web_flag=()
  local extra_flags=()
  [ "$GRPC_WEB_ENABLE" = "1" ] && grpc_web_flag+=(--grpc-web.enable)
  if [ "${DISABLE_PPROF:-0}" = "1" ]; then
    extra_flags+=(--pprof.laddr "")
  fi
  get_addr() {
    local name="$1"; local addr=""; local tries=15
    while [ $tries -gt 0 ]; do
      addr=$("$BIN" keys show "$name" -a --keyring-backend "$KEYRING" --home "$HOME_DIR" 2>/dev/null | tr -d '\n')
      [ -n "$addr" ] && { echo "$addr"; return 0; }
      tries=$((tries-1)); sleep 0.2
    done
    echo ""; return 1
  }
  echo "Resolving farmer key '$FARMER_NAME'..."
  "$BIN" keys list --keyring-backend "$KEYRING" --home "$HOME_DIR" --list-names 2>/dev/null || true
  ADDR_FARMER=$(get_addr "$FARMER_NAME")
  [ -n "$ADDR_FARMER" ] || { echo "failed to resolve farmer address" >&2; exit 1; }
  echo "ADDR_FARMER=$ADDR_FARMER"
  echo "Binary: $BIN"; ls -l "$BIN" || true
  (LUMEN_TAX_DIRECT_TO_PROPOSER=1 "$BIN" start \
    --home "$HOME_DIR" \
    --rpc.laddr "$RPC_LADDR" \
    --api.enable --api.address "$API_ADDR" \
    --grpc.address "$GRPC_ADDR" "${grpc_web_flag[@]}" \
    --minimum-gas-prices 0ulmn \
    "${extra_flags[@]}" >"$LOG_FILE" 2>&1 &)
  sleep 1
  echo "Node log (head):"; head -n 40 "$LOG_FILE" 2>/dev/null || true
  wait_http "$RPC/status"
  echo "RPC ready"
  wait_http "$API/"
  echo "API ready"
  for i in $(seq 1 80); do
    H=$(curl -s "$RPC/status" | jq -r .result.sync_info.latest_block_height)
    if [ "$H" != "0" ] && [ "$H" != "1" ]; then break; fi; sleep 0.3
  done
  sleep 1
  for i in $(seq 1 80); do
    OK=$(curl -s "$API/cosmos/bank/v1beta1/balances/$ADDR_FARMER" | jq -r '.balances | length' 2>/dev/null || echo "")
    [ -n "$OK" ] && [ "$OK" != "null" ] && break || sleep 0.3
  done
  echo "Balances endpoint OK for farmer"
}

q_balance_ulmn() {
  local addr="$1"
  curl -s "$API/cosmos/bank/v1beta1/balances/$addr" | jq -r '((.balances // [])[] | select(.denom=="ulmn") | .amount) // "0"'
}

send() {
  local from_name="$1" to_addr="$2" amount="$3" memo="${4:-}"
  local res hash code tries=15 tx_json total_b64 rate_b64
  while [ $tries -gt 0 ]; do
    if [ -n "$memo" ]; then
      res=$("$BIN" tx bank send "$from_name" "$to_addr" "$amount" --note "$memo" --keyring-backend "$KEYRING" --home "$HOME_DIR" --chain-id "$CHAIN_ID" --fees "$TX_FEES" -y -o json 2>/dev/null || true)
    else
      res=$("$BIN" tx bank send "$from_name" "$to_addr" "$amount" --keyring-backend "$KEYRING" --home "$HOME_DIR" --chain-id "$CHAIN_ID" --fees "$TX_FEES" -y -o json 2>/dev/null || true)
    fi
    echo "$res" | jq
    hash=$(echo "$res" | jq -r .txhash 2>/dev/null)
    if [ -n "$hash" ] && [ "$hash" != "null" ]; then
      wait_tx_commit "$hash" >/dev/null || true
      tx_json=$(curl -s "$RPC/tx?hash=0x$hash")
      code=$(echo "$tx_json" | jq -r .result.tx_result.code)
      total_val=$(echo "$tx_json" | jq -r '.result.tx_result.events[]? | select(.type=="send_tax") | .attributes[]? | select(.key=="total") | .value' | head -n1)
      rate_val=$(echo "$tx_json" | jq -r '.result.tx_result.events[]? | select(.type=="send_tax") | .attributes[]? | select(.key=="rate") | .value' | head -n1)
      LAST_TX_TOTAL=0
      LAST_TX_RATE=0
      if [ -n "$total_val" ] && [ "$total_val" != "null" ]; then
        LAST_TX_TOTAL=$total_val
        LAST_TX_TOTAL=${LAST_TX_TOTAL%ulmn}
      fi
      if [ -n "$rate_val" ] && [ "$rate_val" != "null" ]; then
        LAST_TX_RATE=$rate_val
      fi
      LAST_TX_HASH="$hash"
      echo "$code"
      return 0
    fi
    tries=$((tries-1))
    sleep 0.5
  done
  echo 1
}

if [ "${1:-}" = "--skip-build" ]; then SKIP_BUILD=1; fi
build ${1:-}
init_chain
start_node

pqc_wait_ready "$RPC" "$API"
pqc_policy_must_be_required "$RPC"

for name in alice bob carol dave eric frank; do
  keys_add_quiet "$name"
done

ADDR_ALICE=$("$BIN" keys show alice -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
ADDR_BOB=$("$BIN" keys show bob -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
ADDR_CAROL=$("$BIN" keys show carol -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
ADDR_DAVE=$("$BIN" keys show dave -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
ADDR_ERIC=$("$BIN" keys show eric -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
ADDR_FRANK=$("$BIN" keys show frank -a --keyring-backend "$KEYRING" --home "$HOME_DIR")

FARMER=$("$BIN" keys show "$FARMER_NAME" -a --keyring-backend "$KEYRING" --home "$HOME_DIR")

setup_pqc_signer "$FARMER_NAME"

ADDR_FEE_COLLECTOR=$(curl -s "$API/cosmos/auth/v1beta1/module_accounts/$FEE_COLLECTOR_NAME" | \
  jq -r '.. | .address? // empty' | head -n1)
[ -n "$ADDR_FEE_COLLECTOR" ] || { echo "failed to resolve fee collector address" >&2; exit 1; }
step "Fund users from farmer"
send "$FARMER_NAME" "$ADDR_ALICE" 100000ulmn >/dev/null || true
send "$FARMER_NAME" "$ADDR_CAROL"  50000ulmn  >/dev/null || true
send "$FARMER_NAME" "$ADDR_ERIC"   20000ulmn  >/dev/null || true

for signer in alice carol eric; do
  setup_pqc_signer "$signer"
done

step "Default deduct: Alice sends 20000 to Bob (Bob receives 19800; Fee collector +200)"
BAL_ALICE_BEFORE=$(q_balance_ulmn "$ADDR_ALICE")
BAL_BOB_BEFORE=$(q_balance_ulmn "$ADDR_BOB")

CODE=$(send alice "$ADDR_BOB" 20000ulmn)
echo "code=$CODE"
echo "send_tax total=$LAST_TX_TOTAL rate=$LAST_TX_RATE"

BAL_ALICE_AFTER=$(q_balance_ulmn "$ADDR_ALICE")
BAL_BOB_AFTER=$(q_balance_ulmn "$ADDR_BOB")

echo "fee_collector balance: $(q_balance_ulmn "$ADDR_FEE_COLLECTOR")"
echo "alice : $BAL_ALICE_BEFORE -> $BAL_ALICE_AFTER"
echo "bob   : $BAL_BOB_BEFORE -> $BAL_BOB_AFTER"

test "$LAST_TX_TOTAL" = "200"
test "$LAST_TX_RATE" = "0.010000000000000000"

if [ "${SKIP_PQC_NEGATIVE:-0}" != "1" ]; then
  if "$BIN" tx bank send alice "$ADDR_BOB" "1ulmn" \
     --pqc-enable=false --fees "$TX_FEES" --chain-id "$CHAIN_ID" \
     --keyring-backend "$KEYRING" --home "$HOME_DIR" \
     --broadcast-mode sync --yes >/tmp/pqc_neg.out 2>&1; then
    echo "error: PQC-disabled TX unexpectedly succeeded" >&2
    exit 1
  fi
  grep -qiE "pqc.*(missing|required|signature)" /tmp/pqc_neg.out || \
    echo "warning: PQC-disabled TX failed but no explicit PQC error found" >&2
fi

step "Surcharge: Carol sends 20000 to Dave (Carol pays +200; Dave +20000; Fee collector +200)"
BAL_CAROL_BEFORE=$(q_balance_ulmn "$ADDR_CAROL")
BAL_DAVE_BEFORE=$(q_balance_ulmn "$ADDR_DAVE")

CODE=$(send carol "$ADDR_DAVE" 20000ulmn "sendtax=surcharge")
echo "code=$CODE"
echo "send_tax total=$LAST_TX_TOTAL rate=$LAST_TX_RATE"

BAL_CAROL_AFTER=$(q_balance_ulmn "$ADDR_CAROL")
BAL_DAVE_AFTER=$(q_balance_ulmn "$ADDR_DAVE")

echo "carol : $BAL_CAROL_BEFORE -> $BAL_CAROL_AFTER"
echo "dave  : $BAL_DAVE_BEFORE -> $BAL_DAVE_AFTER"
test "$LAST_TX_TOTAL" = "200"
test "$LAST_TX_RATE" = "0.010000000000000000"

step "Surcharge with insufficient funds should fail (Eric has exactly 20000)"
CODE=$(send eric "$ADDR_FRANK" 20000ulmn "sendtax=surcharge")
echo "code=$CODE"
test "$CODE" != "0"

echo "\nAll send-tax checks passed."
kill_node
