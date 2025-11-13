#!/usr/bin/env bash
set -euo pipefail

# Environment variables:
#   SKIP_BUILD         Skip rebuilding the binary (default 0 / --skip-build)
#   MODE               'prod' (default) or 'dev'; can also be set via --mode
#   SKIP_PQC_NEGATIVE  Set to 1 to skip the PQC negative test
#   RPC_HOST/PORT      RPC bind host/port (default 127.0.0.1:26657)
#   API_HOST/PORT      REST bind host/port (default 127.0.0.1:1317)
#   GRPC_HOST/PORT     gRPC bind host/port (default 127.0.0.1:9090)
#   GRPC_WEB_ENABLE    Enable gRPC-Web (default 1)
#   LOG_FILE           Node log destination (default /tmp/lumen.log)
#   DEBUG_KEEP         Set to 1 to keep the temporary HOME directory on exit

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
LOG_FILE="${LOG_FILE:-/tmp/lumen.log}"
CHAIN_ID="lumen"
FARMER_NAME=farmer
KEYRING=${KEYRING:-test}
TX_FEES=${TX_FEES:-0ulmn}
NODE=${NODE:-$RPC_LADDR}
MODE="${MODE:-prod}"
DNS_MIN_BID_ULMN=""
DNS_HIGH_BID_ULMN=""
DNS_AFTER_BID_ULMN=""
NAME="codex-auc"
EXT="lumen"
LOWER_BID=""

keys_add_quiet(){
  local name="$1"
  printf '\n' | "$BIN" keys add "$name" --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null 2>&1 || true
}

step() { echo; echo "==== $*"; }
wait_http() { local url="$1"; for i in $(seq 1 80); do curl -sSf "$url" >/dev/null && return 0; sleep 0.3; done; echo "Timeout $url" >&2; return 1; }
wait_tx_commit() {
  local h="$1"
  LAST_TX_CODE=""
  for i in $(seq 1 100); do
    local c
    c=$(curl -s "$RPC/tx?hash=0x$h" | jq -r .result.tx_result.code)
    if [ "$c" != "null" ]; then
      LAST_TX_CODE="$c"
      echo "tx_code=$c"
      return 0
    fi
    sleep 0.3
  done
  echo "Timeout tx $h" >&2
  return 1
}

calc_dns_price_from_file() {
  local json_path="$1"
  local domain="$2"
  local ext="$3"
  local days="${4:-365}"
  python3 - "$json_path" "$domain" "$ext" "$days" <<'PY'
import json, sys
from decimal import Decimal, ROUND_CEILING, getcontext

getcontext().prec = 64
json_path, domain, ext, days = sys.argv[1:5]
days = int(days)
with open(json_path) as f:
    data = json.load(f)
if "app_state" in data:
    params = data["app_state"]["dns"]["params"]
else:
    params = data["params"]

def pick(tiers, length):
    if not tiers:
        return 10000
    for tier in tiers:
        max_len = int(tier.get("max_len", 0) or 0)
        if max_len == 0 or length <= max_len:
            return int(tier["multiplier_bps"])
    return int(tiers[-1]["multiplier_bps"])

def apply_bps(amount, bps):
    return (amount * bps + 9999) // 10000

min_price = int(params.get("min_price_ulmn_per_month", 0))
if min_price <= 0:
    raise SystemExit("0")

months = max(1, (days + 29) // 30)
amount = min_price * months
amount = apply_bps(amount, pick(params.get("domain_tiers") or [], len(domain)))
amount = apply_bps(amount, pick(params.get("ext_tiers") or [], len(ext)))
price = (Decimal(amount) * Decimal(params.get("base_fee_dns", "1"))).to_integral_value(rounding=ROUND_CEILING)
print(int(price))
PY
}

kill_node(){ pkill -f "lumend start" >/dev/null 2>&1 || true; }

SKIP_BUILD=${SKIP_BUILD:-0}
build() {
  if [ "$SKIP_BUILD" = "1" ]; then
    return 0
  fi
  step "Build lumend"
  build_cmd=(go build)
  if [ -n "$LUMEN_BUILD_TAGS" ]; then
    build_cmd+=(-tags "$LUMEN_BUILD_TAGS")
  fi
  build_cmd+=(-o "$BIN" ./cmd/lumend)
  (cd "$DIR" && "${build_cmd[@]}")
}

init_chain(){
  step "Init chain"
  rm -rf "$HOME_DIR"
  "$BIN" init local --chain-id "$CHAIN_ID" --home "$HOME_DIR"
  keys_add_quiet "$FARMER_NAME"
  local acct_addr
  for acct in owner1 owner2 owner3; do
    keys_add_quiet "$acct"
    acct_addr=$("$BIN" keys show "$acct" -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
    "$BIN" genesis add-genesis-account "$acct_addr" 1000000ulmn --keyring-backend "$KEYRING" --home "$HOME_DIR"
  done
  ADDR_FARMER=$("$BIN" keys show "$FARMER_NAME" -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
  "$BIN" genesis add-genesis-account "$ADDR_FARMER" 100000000000ulmn --keyring-backend "$KEYRING" --home "$HOME_DIR"
  "$BIN" genesis gentx "$FARMER_NAME" 1000000ulmn \
    --chain-id "$CHAIN_ID" --keyring-backend "$KEYRING" --home "$HOME_DIR" \
    --moniker "local-farmer" --commission-rate 0.10 --commission-max-rate 0.20 --commission-max-change-rate 0.01 \
    --min-self-delegation 1 >/dev/null
  "$BIN" genesis collect-gentxs --home "$HOME_DIR" >/dev/null
  tmp=$(mktemp)
  jq ' .app_state.distribution.params |= (.community_tax="0.0" | .base_proposer_reward="1.0" | .bonus_proposer_reward="0.0")' \
    "$HOME_DIR/config/genesis.json" > "$tmp" && mv "$tmp" "$HOME_DIR/config/genesis.json"
  tmp=$(mktemp)
  jq '
    .app_state.dns.params.grace_days="0"
    | .app_state.dns.params.auction_days="2"
    | .app_state.dns.params.update_rate_limit_seconds="0"
    | .app_state.dns.params.update_pow_difficulty="0"
  ' "$HOME_DIR/config/genesis.json" > "$tmp" && mv "$tmp" "$HOME_DIR/config/genesis.json"
  "$BIN" genesis validate --home "$HOME_DIR"
}

set_dns_bid_amounts() {
  DNS_MIN_BID_ULMN=$(calc_dns_price_from_file "$HOME_DIR/config/genesis.json" "$NAME" "$EXT" 365)
  if [[ -z "$DNS_MIN_BID_ULMN" || "$DNS_MIN_BID_ULMN" -le 0 ]]; then
    DNS_MIN_BID_ULMN=1000000
  fi
  local delta=$(( DNS_MIN_BID_ULMN / 2 ))
  if (( delta == 0 )); then
    delta=1
  fi
  DNS_HIGH_BID_ULMN=$(( DNS_MIN_BID_ULMN + delta ))
  DNS_AFTER_BID_ULMN=$(( DNS_HIGH_BID_ULMN + delta ))
  LOWER_BID=$(( DNS_MIN_BID_ULMN - 1 ))
  if (( LOWER_BID <= 0 )); then
    LOWER_BID=1
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --skip-build) SKIP_BUILD=1 ;;
    --mode)
      MODE="$2"
      shift
      ;;
    --prod) MODE="prod" ;;
    --dev) MODE="dev" ;;
    *) echo "Unknown argument: $1" >&2; exit 1 ;;
  esac
  shift
done

case "$MODE" in
  prod|dev) ;;
  *) echo "Unknown mode: $MODE" >&2; exit 1 ;;
esac

start_node(){
  step "Start node"
  kill_node
  ADDR_FARMER=$("$BIN" keys show "$FARMER_NAME" -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
  local cmd=(
    "$BIN" start
    --home "$HOME_DIR"
    --rpc.laddr "$RPC_LADDR"
    --api.enable
    --api.address "$API_ADDR"
    --grpc.address "$GRPC_ADDR"
    --minimum-gas-prices 0ulmn
  )
  [ "$GRPC_WEB_ENABLE" = "1" ] && cmd+=(--grpc-web.enable)
  if [ "${DISABLE_PPROF:-0}" = "1" ]; then
    cmd+=(--pprof.laddr "")
  fi
  if [ "$MODE" = "prod" ]; then
    ("${cmd[@]}" >"$LOG_FILE" 2>&1 &)
  else
    (LUMEN_AUCTION_DIRECT_TO_PROPOSER=1 "${cmd[@]}" >"$LOG_FILE" 2>&1 &)
  fi
  sleep 1
  wait_http "$RPC/status"; wait_http "$API/"
  for i in $(seq 1 80); do H=$(curl -s "$RPC/status" | jq -r .result.sync_info.latest_block_height); [ "$H" != "0" ] && [ "$H" != "1" ] && break || sleep 0.3; done
}

pool_amount(){
  curl -s "$API/cosmos/distribution/v1beta1/community_pool" | jq -r '((.pool // [])[] | select(.denom=="ulmn") | .amount) // "0"'
}

update_domain_expire(){
  local index="$1" name="$2" owner="$3" exp="$4"
  local res
  res=$("$BIN" tx dns update-domain "$index" "$name" "$owner" "{}" "$exp" \
    --from "$owner" --keyring-backend "$KEYRING" --home "$HOME_DIR" \
    --chain-id "$CHAIN_ID" --fees "$TX_FEES" -y -o json)
  echo "$res" | jq
  local h
  h=$(echo "$res" | jq -r .txhash)
  if [ -n "$h" ] && [ "$h" != "null" ]; then
    wait_tx_commit "$h"
  fi
}

q_balance_ulmn(){ local a="$1"; curl -s "$API/cosmos/bank/v1beta1/balances/$a" | jq -r '((.balances // [])[] | select(.denom=="ulmn") | .amount) // "0"'; }

build
init_chain
set_dns_bid_amounts
start_node
pqc_wait_ready "$RPC" "$API"
pqc_policy_must_be_required "$RPC"

for name in owner1 owner2 owner3; do
  keys_add_quiet "$name"
done

OWNER1=$("$BIN" keys show owner1 -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
OWNER2=$("$BIN" keys show owner2 -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
OWNER3=$("$BIN" keys show owner3 -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
FARMER=$("$BIN" keys show "$FARMER_NAME" -a --keyring-backend "$KEYRING" --home "$HOME_DIR")

setup_pqc_signer "$FARMER_NAME"

step "Fund bidders"
H=$("$BIN" tx bank send "$FARMER_NAME" "$OWNER1" 2000000000ulmn --keyring-backend "$KEYRING" --home "$HOME_DIR" --chain-id "$CHAIN_ID" --fees "$TX_FEES" -y -o json | jq -r .txhash); [ -n "$H" ] && [ "$H" != "null" ] && wait_tx_commit "$H"
H=$("$BIN" tx bank send "$FARMER_NAME" "$OWNER2" 100000ulmn --keyring-backend "$KEYRING" --home "$HOME_DIR" --chain-id "$CHAIN_ID" --fees "$TX_FEES" -y -o json | jq -r .txhash); [ -n "$H" ] && [ "$H" != "null" ] && wait_tx_commit "$H"
H=$("$BIN" tx bank send "$FARMER_NAME" "$OWNER3" 100000ulmn --keyring-backend "$KEYRING" --home "$HOME_DIR" --chain-id "$CHAIN_ID" --fees "$TX_FEES" -y -o json | jq -r .txhash); [ -n "$H" ] && [ "$H" != "null" ] && wait_tx_commit "$H"

for signer in owner1 owner2 owner3; do
  setup_pqc_signer "$signer"
done

TX_GASLESS_ARGS=(--keyring-backend "$KEYRING" --home "$HOME_DIR" --chain-id "$CHAIN_ID" -y -o json)

step "Register $NAME.$EXT for owner1"
RES=$("$BIN" tx dns register "$NAME" "$EXT" \
  --records '[]' --duration-days 0 --owner "$OWNER1" \
  --from owner1 "${TX_GASLESS_ARGS[@]}")
echo "$RES" | jq
HASH=$(echo "$RES" | jq -r .txhash)
if [ -n "$HASH" ] && [ "$HASH" != "null" ]; then
  wait_tx_commit "$HASH"
  CODE=${LAST_TX_CODE:-0}
else
  CODE=$(echo "$RES" | jq -r .code)
fi
echo "code=$CODE"

if [ "${SKIP_PQC_NEGATIVE:-0}" != "1" ]; then
  if "$BIN" tx bank send owner1 "$OWNER2" "1ulmn" \
     --pqc-enable=false --fees "$TX_FEES" --chain-id "$CHAIN_ID" \
     --keyring-backend "$KEYRING" --home "$HOME_DIR" \
     --broadcast-mode sync --yes >/tmp/pqc_neg.out 2>&1; then
    echo "error: PQC-disabled TX unexpectedly succeeded" >&2
    exit 1
  fi
  grep -qiE "pqc.*(missing|required|signature)" /tmp/pqc_neg.out || \
    echo "warning: PQC-disabled TX failed but no explicit PQC error found" >&2
fi

step "Force expiration and enter auction window"
NOW=$(date +%s)
PAST=$((NOW-30))
INDEX="$NAME.$EXT"
update_domain_expire "$INDEX" "$INDEX" "$OWNER1" "$PAST"


step "Bid1 owner2 = ${DNS_MIN_BID_ULMN}"
RES=$("$BIN" tx dns bid "$NAME" "$EXT" "$DNS_MIN_BID_ULMN" \
  --from owner2 "${TX_GASLESS_ARGS[@]}")
echo "$RES" | jq
HASH=$(echo "$RES" | jq -r .txhash)
if [ -n "$HASH" ] && [ "$HASH" != "null" ]; then
  wait_tx_commit "$HASH"
  CODE=${LAST_TX_CODE:-0}
else
  CODE=$(echo "$RES" | jq -r .code)
fi
echo "code=$CODE"

step "Bid2 owner3 = ${DNS_HIGH_BID_ULMN} (higher)"
RES=$("$BIN" tx dns bid "$NAME" "$EXT" "$DNS_HIGH_BID_ULMN" \
  --from owner3 "${TX_GASLESS_ARGS[@]}")
echo "$RES" | jq
HASH=$(echo "$RES" | jq -r .txhash)
if [ -n "$HASH" ] && [ "$HASH" != "null" ]; then
  wait_tx_commit "$HASH"
  CODE=${LAST_TX_CODE:-0}
else
  CODE=$(echo "$RES" | jq -r .code)
fi
echo "code=$CODE"
step "Settle auction"
PAST_DONE=$((NOW-172810))
update_domain_expire "$INDEX" "$INDEX" "$OWNER1" "$PAST_DONE"
RES=$("$BIN" tx dns settle "$NAME" "$EXT" \
  --from "$FARMER_NAME" "${TX_GASLESS_ARGS[@]}")
echo "$RES" | jq
HASH=$(echo "$RES" | jq -r .txhash)
if [ -n "$HASH" ] && [ "$HASH" != "null" ]; then
  wait_tx_commit "$HASH"
  CODE=${LAST_TX_CODE:-0}
else
  CODE=$(echo "$RES" | jq -r .code)
fi
echo "code=$CODE"

step "Verify ownership transferred to owner3"
"$BIN" query dns list-domain -o json --home "$HOME_DIR" | jq -r --arg n "$INDEX" '.domain[] | select(.name==$n) | .owner' | grep -q "$OWNER3"

step "Negative: lower bid rejected"
RES=$("$BIN" tx dns bid "$NAME" "$EXT" "$LOWER_BID" \
  --from owner2 "${TX_GASLESS_ARGS[@]}") || true
echo "$RES" | jq
HASH=$(echo "$RES" | jq -r .txhash)
if [ -n "$HASH" ] && [ "$HASH" != "null" ]; then
  wait_tx_commit "$HASH"
  CODE=${LAST_TX_CODE:-0}
else
  CODE=$(echo "$RES" | jq -r .code)
fi
test "$CODE" != "0"

step "Negative: bid after auction end rejected"
RES=$("$BIN" tx dns bid "$NAME" "$EXT" "$DNS_AFTER_BID_ULMN" \
  --from owner2 "${TX_GASLESS_ARGS[@]}") || true
echo "$RES" | jq
HASH=$(echo "$RES" | jq -r .txhash)
if [ -n "$HASH" ] && [ "$HASH" != "null" ]; then
  wait_tx_commit "$HASH"
  CODE=${LAST_TX_CODE:-0}
else
  CODE=$(echo "$RES" | jq -r .code)
fi
test "$CODE" != "0"

echo
echo "All auction tests passed (mode=$MODE)."
