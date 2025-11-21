#!/usr/bin/env bash
set -euo pipefail

SOURCE_DIR="$(cd "$(dirname "$0")" && pwd)"
. "${SOURCE_DIR}/lib_pqc.sh"
pqc_require_bins

HOME_E2E=$(mktemp -d -t lumen-e2e-tokenomics-XXXXXX)
export HOME="$HOME_E2E"
DIR=$(cd "${SOURCE_DIR}/../.." && pwd)
BIN="$DIR/build/lumend"
: "${LUMEN_BUILD_TAGS:=dev}"
HOME_DIR="$HOME/.lumen"

RANDOM_PORT_BASE=${E2E_BASE_PORT:-$(( (RANDOM % 1000) + 36000 ))}
: "${RPC_HOST:=127.0.0.1}"
: "${API_HOST:=127.0.0.1}"
: "${GRPC_HOST:=127.0.0.1}"
: "${P2P_HOST:=0.0.0.0}"
: "${RPC_PORT:=$RANDOM_PORT_BASE}"
: "${API_PORT:=$((RANDOM_PORT_BASE + 60))}"
: "${GRPC_PORT:=$((RANDOM_PORT_BASE + 120))}"
: "${P2P_PORT:=$((RANDOM_PORT_BASE + 180))}"
RPC_LADDR="tcp://${RPC_HOST}:${RPC_PORT}"
RPC="http://${RPC_HOST}:${RPC_PORT}"
API_ADDR="tcp://${API_HOST}:${API_PORT}"
API="http://${API_HOST}:${API_PORT}"
GRPC_ADDR="${GRPC_HOST}:${GRPC_PORT}"
P2P_LADDR="tcp://${P2P_HOST}:${P2P_PORT}"
LOG_FILE="${LOG_FILE:-/tmp/lumen-tokenomics.log}"
CHAIN_ID="${CHAIN_ID:-lumen-tokenomics}"
KEYRING=${KEYRING:-test}
TX_FEES=${TX_FEES:-0ulmn}
GRPC_WEB_ENABLE=${GRPC_WEB_ENABLE:-1}
DISABLE_PPROF=${DISABLE_PPROF:-1}
NODE=${NODE:-$RPC_LADDR}
export NODE

TX_ARGS=(--keyring-backend "$KEYRING" --home "$HOME_DIR" --chain-id "$CHAIN_ID" -y -o json --fees "$TX_FEES" --pqc-enable=true)
LAST_TX_CODE=""
PQC_ARGS=()

SKIP_BUILD=${SKIP_BUILD:-0}
while [[ $# -gt 0 ]]; do
  case "$1" in
    --skip-build) SKIP_BUILD=1 ;;
    *)
      echo "Unknown argument: $1" >&2
      exit 1
      ;;
  esac
  shift
done

pqc_args() {
  local signer="$1"
  if [ -z "$signer" ]; then
    PQC_ARGS=()
    return
  fi
  local addr="${ADDR[$signer]:-}"
  if [ -z "$addr" ]; then
    PQC_ARGS=()
    return
  fi
  PQC_ARGS=(--pqc-from "$addr" --pqc-key "pqc-$signer")
}

cleanup(){
  pkill -f "lumend start" >/dev/null 2>&1 || true
  if [ "${DEBUG_KEEP:-0}" != "1" ]; then
    rm -rf "$HOME_E2E" >/dev/null 2>&1 || true
  else
    echo "DEBUG_KEEP=1 -> keeping $HOME_E2E"
  fi
}
trap cleanup EXIT

PASS=0
FAIL=0
SUMMARY=()
ROUTED_ACCUM=0

add_result(){
  local name="$1" expected="$2" actual="$3" detail="$4"
  local status="OK"
  if [ "$expected" != "$actual" ]; then
    status="KO"
    FAIL=$((FAIL+1))
  else
    PASS=$((PASS+1))
  fi
  SUMMARY+=("$name|$expected|$actual|$status|$detail")
}

big_add(){
  python3 - "$1" "$2" <<'PY'
import sys
print(int(sys.argv[1]) + int(sys.argv[2]))
PY
}

big_sub(){
  python3 - "$1" "$2" <<'PY'
import sys
print(int(sys.argv[2]) - int(sys.argv[1]))
PY
}

big_mul(){
  python3 - "$1" "$2" <<'PY'
import sys
print(int(sys.argv[1]) * int(sys.argv[2]))
PY
}

dec_to_int(){
  python3 - "$1" <<'PY'
from decimal import Decimal
import sys
print(int(Decimal(sys.argv[1] or "0")))
PY
}

debug_log(){
  if [ "${DEBUG_TOKENOMICS:-0}" = "1" ]; then
    echo "DEBUG_TOKENOMICS: $*" >&2
  fi
}

wait_http(){
  local url="$1"
  for _ in $(seq 1 120); do
    if curl -sSf "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  echo "timeout waiting for $url" >&2
  exit 1
}

wait_tx(){
  local hash="$1"
  for _ in $(seq 1 120); do
    local code
    code=$(curl -s "$RPC/tx?hash=0x$hash" | jq -r '.result.tx_result.code' 2>/dev/null || echo null)
    if [ "$code" != "null" ]; then
      LAST_TX_CODE="$code"
      echo "tx_code=$code"
      if [ "$code" != "0" ]; then
        echo "tx $hash failed:" >&2
        curl -s "$RPC/tx?hash=0x$hash" | jq >&2
        exit 1
      fi
      return 0
    fi
    sleep 0.5
  done
  echo "timeout waiting for tx $hash" >&2
  exit 1
}

check_tx_code_zero(){
  local payload="$1"
  local label="$2"
  local code
  code=$(printf '%s' "$payload" | jq -r '.code // 0')
  if [ "$code" = "null" ] || [ -z "$code" ]; then
    code=0
  fi
  if [ "$code" != "0" ]; then
    printf '%s failed: %s\n' "$label" "$payload" >&2
    exit 1
  fi
}

current_height(){
  curl -s "$RPC/status" | jq -r '.result.sync_info.latest_block_height' 2>/dev/null || echo 0
}

wait_height(){
  local target="$1"
  for _ in $(seq 1 180); do
    local h
    h=$(current_height)
    if [ "${h:-0}" -ge "$target" ]; then
      return 0
    fi
    sleep 1
  done
  echo "timeout waiting height $target" >&2
  exit 1
}

bank_balance(){
  local addr="$1"
  curl -s "$API/cosmos/bank/v1beta1/balances/$addr" | jq -r '
    def to_array(x):
      if x == null then []
      elif (x | type) == "array" then x
      elif (x | type) == "object" then [x]
      else []
      end;
    (to_array(.balances)[]? | select(.denom=="ulmn") | .amount) // "0"
  '
}

community_pool(){
  local amt
  amt=$(
    curl -s "$API/cosmos/distribution/v1beta1/community_pool" | jq -r '
      def to_array(x):
        if x == null then []
        elif (x | type) == "array" then x
        elif (x | type) == "object" then [x]
        else []
        end;
      (to_array(.pool)[]? | select(.denom=="ulmn") | .amount) // "0"
    '
  )
  dec_to_int "$amt"
}

module_account(){
  local name="$1"
  "$BIN" q auth module-account "$name" --home "$HOME_DIR" --node "$RPC_LADDR" -o json | jq -r '.account.base_account.address'
}

module_balance_ulmn(){
  local name="$1"
  local addr
  addr=$(module_account "$name" 2>/dev/null || echo "")
  if [ -z "$addr" ] || [ "$addr" = "null" ]; then
    echo "0"
    return
  fi
  bank_balance "$addr"
}

gateway_id_by_operator(){
  local addr="$1"
  curl -s "$API/lumen/gateway/v1/gateways" | jq -r --arg op "$addr" '.gateways[]? | select(.operator==$op) | .id' | head -n1
}

sum_block_rewards(){
  local from="$1" to="$2" total="0"
  if [ -z "$from" ] || [ -z "$to" ] || [ "$to" -lt "$from" ]; then
    echo "0"
    return
  fi
  local h reward
  for ((h=from; h<=to; h++)); do
    reward=$(curl -s "$RPC/block_results?height=$h" | jq -r '
      .result.finalize_block_events[]? 
      | select(.type=="block_reward") 
      | .attributes[]? 
      | select(.key=="minted_ulmn") 
      | .value' | tail -n1)
    if [ -n "$reward" ] && [ "$reward" != "null" ]; then
      total=$(big_add "$total" "$reward")
    fi
  done
  echo "$total"
}

outstanding_rewards(){
  local amt
  amt=$(
    curl -s "$API/cosmos/distribution/v1beta1/validators/$VALOPER/outstanding_rewards" | jq -r '
      def to_array(x):
        if x == null then []
        elif (x | type) == "array" then x
        elif (x | type) == "object" then [x]
        else []
        end;
      (to_array(.rewards)[]? | select(.denom=="ulmn") | .amount) // "0"
    '
  )
  dec_to_int "$amt"
}

validator_commission(){
  local amt
  amt=$(
    curl -s "$API/cosmos/distribution/v1beta1/validators/$VALOPER/commission" | jq -r '
      def to_array(x):
        if x == null then []
        elif (x | type) == "array" then x
        elif (x | type) == "object" then [x]
        else []
        end;
      (to_array(.commission)[]? | select(.denom=="ulmn") | .amount) // "0"
    '
  )
  dec_to_int "$amt"
}

ensure_pqc_required(){
  for _ in $(seq 1 30); do
    local params policy
    params=$("$BIN" q pqc params --node "$NODE" -o json 2>/dev/null || true)
    policy=$(echo "$params" | jq -r '.params.policy // .policy // empty')
    if [ "$policy" = "PQC_POLICY_REQUIRED" ]; then
      return 0
    fi
    sleep 1
  done
  echo "PQC policy is not REQUIRED" >&2
  exit 1
}

calc_dns_price(){
  local path="$1" domain="$2" ext="$3" days="$4"
  python3 - "$path" "$domain" "$ext" "$days" <<'PY'
import json, sys
from decimal import Decimal, ROUND_CEILING, getcontext
getcontext().prec = 64
path, domain, ext, days = sys.argv[1:5]
days = int(days)
with open(path) as f:
    params = json.load(f)["app_state"]["dns"]["params"]
def pick(tiers, length):
    if not tiers:
        return 10000
    for tier in tiers:
        max_len = int(tier.get("max_len", 0) or 0)
        if max_len == 0 or length <= max_len:
            return int(tier["multiplier_bps"])
    return int(tiers[-1]["multiplier_bps"])
def apply(amount, bps):
    return (amount * bps + 9999) // 10000
months = max(1, (days + 29) // 30)
amount = int(params["min_price_ulmn_per_month"]) * months
amount = apply(amount, pick(params.get("domain_tiers") or [], len(domain)))
amount = apply(amount, pick(params.get("ext_tiers") or [], len(ext)))
price = (Decimal(amount) * Decimal(params.get("base_fee_dns", "1"))).to_integral_value(rounding=ROUND_CEILING)
print(int(price))
PY
}

build(){
  if [ "$SKIP_BUILD" = "1" ]; then
    echo "==> Skip build (SKIP_BUILD=1)"
    return
  fi
  echo "==> Building lumend"
  local cmd=(go build -trimpath -ldflags "-s -w")
  if [ -n "$LUMEN_BUILD_TAGS" ]; then
    cmd+=(-tags "$LUMEN_BUILD_TAGS")
  fi
  cmd+=(-o "$BIN" ./cmd/lumend)
  (cd "$DIR" && "${cmd[@]}")
}

keys_add(){
  printf '\n' | "$BIN" keys add "$1" --home "$HOME_DIR" --keyring-backend "$KEYRING" >/dev/null 2>&1 || true
}

init_chain(){
  echo "==> Init chain"
  rm -rf "$HOME_DIR"
  "$BIN" init tokenomics --chain-id "$CHAIN_ID" --home "$HOME_DIR"
  local actors=(validator funder sender receiver dns_owner dns_bidder gateway)
  for a in "${actors[@]}"; do
    keys_add "$a"
  done
  local funds=5000000000000ulmn
  for a in "${actors[@]}"; do
    addr=$("$BIN" keys show "$a" -a --home "$HOME_DIR" --keyring-backend "$KEYRING")
    "$BIN" genesis add-genesis-account "$addr" "$funds" --home "$HOME_DIR" --keyring-backend "$KEYRING"
  done
  "$BIN" genesis gentx validator 1000000000ulmn --home "$HOME_DIR" --keyring-backend "$KEYRING" --chain-id "$CHAIN_ID" --commission-rate 0 --commission-max-rate 0 --commission-max-change-rate 0 --min-self-delegation 1 >/dev/null
  "$BIN" genesis collect-gentxs --home "$HOME_DIR" >/dev/null
  local g="$HOME_DIR/config/genesis.json"
  tmp=$(mktemp)
  jq ' .app_state.distribution.params.community_tax="0.0"
    | .app_state.dns.params.grace_days="0"
    | .app_state.dns.params.auction_days="1"
    | .app_state.dns.params.update_rate_limit_seconds="0"
    | .app_state.dns.params.update_pow_difficulty=0
    | .app_state.gateways.gateway_count="1"
  ' "$g" >"$tmp" && mv "$tmp" "$g"
  "$BIN" genesis validate --home "$HOME_DIR" >/dev/null
  pqc_set_client_config "$HOME_DIR" "$RPC_LADDR" "$CHAIN_ID"
}

start_node(){
  echo "==> Start node"
  pkill -f "lumend start" >/dev/null 2>&1 || true
  local cmd=("$BIN" start --home "$HOME_DIR" --rpc.laddr "$RPC_LADDR" --p2p.laddr "$P2P_LADDR" --api.enable --api.address "$API_ADDR" --grpc.address "$GRPC_ADDR" --minimum-gas-prices 0ulmn)
  [ "$GRPC_WEB_ENABLE" = "1" ] && cmd+=(--grpc-web.enable)
  if [ "$DISABLE_PPROF" = "1" ]; then
    cmd+=(--rpc.pprof_laddr "")
  fi
  ("${cmd[@]}" >"$LOG_FILE" 2>&1 &)
  sleep 1
  wait_http "$RPC/status"
  wait_http "$API/"
}

run_pool_delta(){
  local label="$1" expected="$2" signer="$3"
  shift 3
  local hash json before after delta
  before=$(community_pool)
  pqc_args "$signer"
  json=$("$@" "${PQC_ARGS[@]}" "${TX_ARGS[@]}" | tee /tmp/${label}_tx.json)
  check_tx_code_zero "$json" "$label"
  hash=$(echo "$json" | jq -r '.txhash // empty')
  [ -z "$hash" ] && { echo "$label missing txhash" >&2; exit 1; }
  wait_tx "$hash"
  echo "$hash" >"/tmp/${label}_txhash"
  local h
  h=$(current_height)
  wait_height $((h + 1))
  if [ "${DEBUG_TOKENOMICS:-0}" = "1" ]; then
    curl -s "$RPC/tx?hash=0x$hash" >"/tmp/${label}_tx_rpc.json"
    echo "tx_events_$label saved to /tmp/${label}_tx_rpc.json" >&2
  fi
  after=$(community_pool)
  delta=$(big_sub "$before" "$after")
  if [ "${DEBUG_TOKENOMICS:-0}" = "1" ]; then
    echo "pool_delta_$label: before=$before after=$after delta=$delta" >&2
  fi
  ROUTED_ACCUM=$(big_add "$ROUTED_ACCUM" "$delta")
  add_result "$label" "$expected" "$delta" "community_pool"
}

build
init_chain
start_node
pqc_wait_ready "$RPC" "$API"
ensure_pqc_required
wait_height 2

for acct in validator sender dns_owner dns_bidder gateway; do
  setup_pqc_signer "$acct"
done

declare -A ADDR
for acct in validator funder sender receiver dns_owner dns_bidder gateway; do
  ADDR[$acct]=$("$BIN" keys show "$acct" -a --home "$HOME_DIR" --keyring-backend "$KEYRING")
done
VALOPER=$("$BIN" keys show validator --bech val -a --home "$HOME_DIR" --keyring-backend "$KEYRING")
GENESIS_JSON="$HOME_DIR/config/genesis.json"
FEE_COLLECTOR_ADDR=$(module_account fee_collector)
DISTR_ADDR=$(module_account distribution)

START_POOL=$(community_pool)
START_SENDER=$(bank_balance "${ADDR[sender]}")
START_RECEIVER=$(bank_balance "${ADDR[receiver]}")
START_OUTSTANDING=$(outstanding_rewards)
START_COMMISSION=$(validator_commission)
START_HEIGHT=$(current_height)
debug_log "start_pool=$START_POOL start_sender=$START_SENDER start_receiver=$START_RECEIVER"
debug_log "start_outstanding=$START_OUTSTANDING commission=$START_COMMISSION start_height=$START_HEIGHT"
add_result "commission_initial" "0" "$START_COMMISSION" "validator_commission"

TOK_PARAMS=$(curl -s "$API/lumen/tokenomics/v1/params")
BLOCK_REWARD_LMN=$(echo "$TOK_PARAMS" | jq -r '.params.initial_reward_per_block_lumn')
MINT_PER_BLOCK=$(
python3 - "$BLOCK_REWARD_LMN" <<'PY'
import sys
print(int(sys.argv[1]) * 1000000)
PY
)
TX_TAX_RATE=$(echo "$TOK_PARAMS" | jq -r '.params.tx_tax_rate')
DIST_INTERVAL=$(echo "$TOK_PARAMS" | jq -r '.params.distribution_interval_blocks // 1' 2>/dev/null)
if [ -z "$DIST_INTERVAL" ] || [ "$DIST_INTERVAL" = "null" ]; then
  DIST_INTERVAL=1
fi

calc_tax(){
  python3 - "$1" "$2" <<'PY'
from decimal import Decimal, ROUND_FLOOR
import sys
amt=int(sys.argv[1])
rate=Decimal(sys.argv[2])
print(int((rate*amt).to_integral_value(rounding=ROUND_FLOOR)))
PY
}

send_amount=50000000
pqc_args sender
SEND_JSON=$("$BIN" tx bank send sender "${ADDR[receiver]}" "${send_amount}ulmn" "${PQC_ARGS[@]}" "${TX_ARGS[@]}" | tee /tmp/send_tx.json)
check_tx_code_zero "$SEND_JSON" "bank send sender->receiver"
SEND_HASH=$(echo "$SEND_JSON" | jq -r '.txhash // empty')
wait_tx "$SEND_HASH"

sender_after=$(bank_balance "${ADDR[sender]}")
receiver_after=$(bank_balance "${ADDR[receiver]}")
tax_expected=$(calc_tax "$send_amount" "$TX_TAX_RATE")

sender_delta=$(big_sub "$START_SENDER" "$sender_after")
receiver_delta=$(big_sub "$START_RECEIVER" "$receiver_after")

add_result "send_sender_balance" "-$send_amount" "$sender_delta" "ulmn"
recv_expected=$(big_add "$send_amount" "-$tax_expected")
add_result "send_receiver_balance" "$recv_expected" "$receiver_delta" "ulmn"
EXPECTED_POOL_TOTAL=0
EXPECTED_POOL_TOTAL=$(big_add "$EXPECTED_POOL_TOTAL" 0)

wait_height $((START_HEIGHT + DIST_INTERVAL + 1))
debug_log "waited_for_height=$((START_HEIGHT + DIST_INTERVAL + 1))"
BEFORE_WITHDRAW=$(outstanding_rewards)
AFTER_HEIGHT=$(current_height)
debug_log "before_withdraw_outstanding=$BEFORE_WITHDRAW after_height=$AFTER_HEIGHT"
REWARD_SUM_BEFORE=$(sum_block_rewards 1 "$START_HEIGHT")
REWARD_SUM_AFTER=$(sum_block_rewards 1 "$AFTER_HEIGHT")
REWARD_SUM=$(big_sub "$REWARD_SUM_BEFORE" "$REWARD_SUM_AFTER")
debug_log "reward_sum_before=$REWARD_SUM_BEFORE reward_sum_after=$REWARD_SUM_AFTER reward_sum_interval=$REWARD_SUM"
block_span=$((AFTER_HEIGHT - START_HEIGHT))
if [ "$block_span" -lt 0 ]; then
  block_span=0
fi
expected_interval_reward=$(big_mul "$MINT_PER_BLOCK" "$block_span")
debug_log "block_span=$block_span expected_interval_reward=$expected_interval_reward"
add_result "block_reward_interval" "$expected_interval_reward" "$REWARD_SUM" "block_reward_events"

pqc_args validator
WITHDRAW_JSON=$("$BIN" tx distribution withdraw-rewards "$VALOPER" --from validator "${PQC_ARGS[@]}" "${TX_ARGS[@]}" | tee /tmp/withdraw_tx.json)
check_tx_code_zero "$WITHDRAW_JSON" "withdraw rewards"
WITHDRAW_HASH=$(echo "$WITHDRAW_JSON" | jq -r '.txhash // empty')
wait_tx "$WITHDRAW_HASH"
WITHDRAW_RPC=$(curl -s "$RPC/tx?hash=0x$WITHDRAW_HASH")
WITHDRAW_AMOUNT=$(echo "$WITHDRAW_RPC" | jq -r '.result.tx_result.events[]? | select(.type=="withdraw_rewards") | .attributes[]? | select(.key=="amount") | .value' | tail -n1)
WITHDRAW_INT=$(echo "$WITHDRAW_AMOUNT" | sed 's/ulmn$//')
expected_withdraw_total=$(big_add "$REWARD_SUM_AFTER" "$tax_expected")
debug_log "expected_withdraw_total=$expected_withdraw_total"
add_result "withdraw_amount" "$expected_withdraw_total" "$WITHDRAW_INT" "delegator_reward"
POST_OUT=$(outstanding_rewards)
add_result "outstanding_reset" "0" "$POST_OUT" "validator_outstanding"
POST_COMMISSION=$(validator_commission)
add_result "commission_final" "$START_COMMISSION" "$POST_COMMISSION" "validator_commission"

DNS_MIN_PRICE=$(calc_dns_price "$GENESIS_JSON" "eco" "lmn" 365)
DNS_RENEW_PRICE=$(calc_dns_price "$GENESIS_JSON" "eco" "lmn" 180)
DNS_HIGH_BID=$((DNS_MIN_PRICE + DNS_MIN_PRICE / 2))
DNS_PARAMS=$(jq '.app_state.dns.params' "$GENESIS_JSON")
DNS_TRANSFER_FEE=$(echo "$DNS_PARAMS" | jq -r '.transfer_fee_ulmn // 0')
DNS_BID_FEE=$(echo "$DNS_PARAMS" | jq -r '.bid_fee_ulmn // 0')
DNS_UPDATE_FEE=$(echo "$DNS_PARAMS" | jq -r '.update_fee_ulmn // 0')
DNS_AUCTION_DAYS=$(echo "$DNS_PARAMS" | jq -r '.auction_days | tonumber')

run_pool_delta "dns_register" "$DNS_MIN_PRICE" dns_owner "$BIN" tx dns register eco lmn --records '[]' --duration-days 365 --owner "${ADDR[dns_owner]}" --from dns_owner
run_pool_delta "dns_renew" "$DNS_RENEW_PRICE" dns_owner "$BIN" tx dns renew eco lmn 180 --from dns_owner
run_pool_delta "dns_update" "$DNS_UPDATE_FEE" dns_owner "$BIN" tx dns update eco lmn '[{"key":"txt","value":"economy"}]' --pow-nonce 0 --from dns_owner
run_pool_delta "dns_transfer" "$DNS_TRANSFER_FEE" dns_owner "$BIN" tx dns transfer eco lmn "${ADDR[gateway]}" --from dns_owner
EXPECTED_POOL_TOTAL=$(big_add "$EXPECTED_POOL_TOTAL" "$DNS_MIN_PRICE")
EXPECTED_POOL_TOTAL=$(big_add "$EXPECTED_POOL_TOTAL" "$DNS_RENEW_PRICE")
EXPECTED_POOL_TOTAL=$(big_add "$EXPECTED_POOL_TOTAL" "$DNS_UPDATE_FEE")
EXPECTED_POOL_TOTAL=$(big_add "$EXPECTED_POOL_TOTAL" "$DNS_TRANSFER_FEE")

INDEX="eco.lmn"
NOW_TS=$(date +%s)
PAST=$((NOW_TS - 30))
update_domain_expire(){
  local signer="$1"
  local owner="$2"
  local expire="$3"
  pqc_args "$signer"
  local res hash
  res=$("$BIN" tx dns update-domain "$INDEX" "$INDEX" "$owner" "{}" "$expire" --from "$signer" "${PQC_ARGS[@]}" "${TX_ARGS[@]}")
  check_tx_code_zero "$res" "update-domain"
  hash=$(echo "$res" | jq -r '.txhash // empty')
  [ -n "$hash" ] && wait_tx "$hash"
}
update_domain_expire dns_owner "${ADDR[gateway]}" "$PAST"

run_pool_delta "dns_bid1" "$DNS_BID_FEE" dns_owner "$BIN" tx dns bid eco lmn "$DNS_MIN_PRICE" --from dns_owner
run_pool_delta "dns_bid2" "$DNS_BID_FEE" dns_bidder "$BIN" tx dns bid eco lmn "$DNS_HIGH_BID" --from dns_bidder
bid_fee_total=$(big_mul "$DNS_BID_FEE" 2)
EXPECTED_POOL_TOTAL=$(big_add "$EXPECTED_POOL_TOTAL" "$bid_fee_total")

PAST_DONE=$((NOW_TS - (DNS_AUCTION_DAYS * 86400 + 10)))
if [ "$PAST_DONE" -ge "$PAST" ]; then
  PAST_DONE=$((PAST - DNS_AUCTION_DAYS * 86400 - 10))
fi
update_domain_expire dns_owner "${ADDR[gateway]}" "$PAST_DONE"
run_pool_delta "dns_settle" "$DNS_HIGH_BID" dns_owner "$BIN" tx dns settle eco lmn --from dns_owner
EXPECTED_POOL_TOTAL=$(big_add "$EXPECTED_POOL_TOTAL" "$DNS_HIGH_BID")
DNS_FINAL_OWNER=$("$BIN" q dns list-domain -o json --home "$HOME_DIR" | jq -r --arg n "$INDEX" '.domain[]? | select(.name==$n) | .owner')
add_result "dns_owner_post_settle" "${ADDR[dns_bidder]}" "$DNS_FINAL_OWNER" "dns_settle"

GW_PARAMS=$(jq '.app_state.gateways.params' "$GENESIS_JSON")
GW_REGISTER_FEE=$(echo "$GW_PARAMS" | jq -r '.register_gateway_fee_ulmn | tonumber')
GW_ACTION_FEE=$(echo "$GW_PARAMS" | jq -r '.action_fee_ulmn | tonumber')
run_pool_delta "gateway_register" "$GW_REGISTER_FEE" gateway "$BIN" tx gateways register-gateway "${ADDR[gateway]}" --metadata "eco" --from gateway
GW_ID=""
REG_TX_HASH=$(cat /tmp/gateway_register_txhash 2>/dev/null || echo "")
REG_RPC_FILE=/tmp/gateway_register_tx_rpc.json
if [ -n "$REG_TX_HASH" ]; then
  curl -s "$RPC/tx?hash=0x$REG_TX_HASH" > "$REG_RPC_FILE"
fi
if [[ -s "$REG_RPC_FILE" ]]; then
  GW_ID=$(jq -r '
    .result.tx_result.events[]? 
    | select(.type=="gateway_register") 
    | .attributes[]? 
    | select(.key=="id") 
    | .value' "$REG_RPC_FILE" 2>/dev/null | head -n1)
fi
if [[ ! "$GW_ID" =~ ^[0-9]+$ ]] && [ -n "$REG_TX_HASH" ]; then
  for _ in $(seq 1 30); do
    GW_ID=$(curl -s "$RPC/tx?hash=0x$REG_TX_HASH" | jq -r '
      .result.tx_result.events[]? 
      | select(.type=="gateway_register") 
      | .attributes[]? 
      | select(.key=="id") 
      | .value' 2>/dev/null | head -n1)
    if [[ "$GW_ID" =~ ^[0-9]+$ ]]; then
      break
    fi
    sleep 0.5
  done
fi
if [[ ! "$GW_ID" =~ ^[0-9]+$ ]]; then
  for _ in $(seq 1 20); do
    GW_ID=$(gateway_id_by_operator "${ADDR[gateway]}")
    if [[ "$GW_ID" =~ ^[0-9]+$ ]]; then
      break
    fi
    sleep 0.5
  done
fi
if [[ ! "$GW_ID" =~ ^[0-9]+$ ]]; then
  echo "failed to fetch gateway id" >&2
  exit 1
fi
echo "Resolved gateway id: $GW_ID"
run_pool_delta "gateway_update" "$GW_ACTION_FEE" gateway "$BIN" tx gateways update-gateway "$GW_ID" --metadata "updated" --from gateway
EXPECTED_POOL_TOTAL=$(big_add "$EXPECTED_POOL_TOTAL" "$GW_REGISTER_FEE")
EXPECTED_POOL_TOTAL=$(big_add "$EXPECTED_POOL_TOTAL" "$GW_ACTION_FEE")

FINAL_POOL=$(community_pool)
TOTAL_POOL_DELTA=$(big_sub "$START_POOL" "$FINAL_POOL")
add_result "pool_total" "$EXPECTED_POOL_TOTAL" "$TOTAL_POOL_DELTA" "community_pool"

printf '\n%-30s | %-12s | %-12s | %-4s | %s\n' "Metric" "Expected" "Actual" "Stat" "Detail"
printf '%s\n' "--------------------------------------------------------------------------------"
for row in "${SUMMARY[@]}"; do
  IFS='|' read -r name exp act status detail <<<"$row"
  printf '%-30s | %-12s | %-12s | %-4s | %s\n' "$name" "$exp" "$act" "$status" "$detail"
done

if [ "$FAIL" -gt 0 ]; then
  echo "\nTokenomics e2e failed ($FAIL mismatches)." >&2
  exit 1
else
  echo "\nTokenomics e2e passed ($PASS checks)."
fi
