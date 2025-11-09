#!/usr/bin/env bash
set -euo pipefail

# Environment variables:
#   SKIP_BUILD       Skip rebuilding the binary (default 0 / --skip-build)
#   MONTH_SECONDS    Duration of one billing month in seconds (default 5 for tests)
#   RPC_HOST/PORT    RPC bind host/port (default 127.0.0.1:26657)
#   API_HOST/PORT    REST bind host/port (default 127.0.0.1:1317)
#   GRPC_HOST/PORT   gRPC bind host/port (default 127.0.0.1:9090)
#   GRPC_WEB_ENABLE  Enable gRPC-Web (default 1)
#   LOG_FILE         Node log destination (default /tmp/lumen_gateways.log)
#   DEBUG_KEEP       Set to 1 to keep the temporary HOME directory on exit
#   LUMEN_RL_PER_BLOCK / PER_WINDOW / WINDOW_SEC / GLOBAL_MAX  Override rate-limit clamps for ante tests

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
LOG_FILE="${LOG_FILE:-/tmp/lumen_gateways.log}"
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
CHAIN_ID="lumen"
MONTH_SECONDS=${MONTH_SECONDS:-5}
KEYRING=${KEYRING:-test}
TX_FEES=${TX_FEES:-0ulmn}
NODE=${NODE:-$RPC_LADDR}
LUMEN_RL_PER_BLOCK="${LUMEN_RL_PER_BLOCK:-50}"
LUMEN_RL_PER_WINDOW="${LUMEN_RL_PER_WINDOW:-50}"
LUMEN_RL_WINDOW_SEC="${LUMEN_RL_WINDOW_SEC:-10}"
LUMEN_RL_GLOBAL_MAX="${LUMEN_RL_GLOBAL_MAX:-500}"

SKIP_BUILD=${SKIP_BUILD:-0}
while [[ $# -gt 0 ]]; do
  case "$1" in
    --skip-build) SKIP_BUILD=1 ;;
    *) echo "Unknown argument: $1" >&2; exit 1 ;;
  esac
  shift
done

require() { command -v "$1" >/dev/null || { echo "Missing dependency: $1" >&2; exit 1; }; }
require jq
require curl

keys_add_quiet() {
  local name="$1"
  printf '\n' | "$BIN" keys add "$name" --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null 2>&1 || true
}

step() { echo; echo "==== $*"; }

wait_http() {
  local url="$1"
  for _ in $(seq 1 120); do
    curl -sSf "$url" >/dev/null && return 0
    sleep 0.3
  done
  echo "Timeout waiting for $url" >&2
  return 1
}

wait_first_block() {
  for _ in $(seq 1 800); do
    local h
    h=$(curl -s "$RPC/status" | jq -r '.result.sync_info.latest_block_height')
    if [[ "$h" != "null" && "$h" =~ ^[0-9]+$ && "$h" -ge 1 ]]; then
      return 0
    fi
    sleep 0.25
  done
  echo "Timeout waiting for first block" >&2
  return 1
}

wait_tx_commit() {
  local hash="$1" raw code status=0
  for _ in $(seq 1 200); do
    set +e
    raw=$(curl -s "$RPC/tx?hash=0x$hash")
    status=$?
    set -e
    if [[ $status -eq 0 ]]; then
      code=$(echo "$raw" | jq -r '.result.tx_result.code' 2>/dev/null)
      if [[ "$code" != "" && "$code" != "null" ]]; then
        echo "$code"
        return 0
      fi
    fi
    sleep 0.25
  done
  echo "timeout" >&2
  return 1
}

kill_node() { pkill -f "lumend start" >/dev/null 2>&1 || true; }
cleanup() { kill_node; }
trap cleanup EXIT

ensure_code_zero() {
  local json="$1"
  local code
  code=$(echo "$json" | jq -r '.code // .Code // "0"')
  if [[ "$code" != "0" ]]; then
    echo "$json" >&2
    exit 1
  fi
}

await_tx() {
  local json="$1" hash code
  hash=$(echo "$json" | jq -r '.txhash // ""')
  if [[ -z "$hash" || "$hash" == "null" ]]; then
    echo "missing txhash in response: $json" >&2
    exit 1
  fi
  code=$(wait_tx_commit "$hash") || { echo "timeout waiting for $hash" >&2; exit 1; }
  if [[ "$code" != "0" ]]; then
    echo "tx $hash failed with code $code" >&2
    curl -s "$RPC/tx?hash=0x$hash" | jq >&2
    exit 1
  fi
}

expect_eq() {
  local got="$1" expect="$2" msg="$3"
  if [[ "$got" != "$expect" ]]; then
    echo "Assertion failed: $msg (expected $expect, got $got)" >&2
    exit 1
  fi
}

query_contract() {
  local id="$1" json status
  for _ in $(seq 1 40); do
    set +e
    json=$("$BIN" q gateways contract "$id" --node "$RPC_LADDR" --output json 2>/dev/null)
    status=$?
    set -e
    if [[ $status -eq 0 ]]; then
      echo "$json"
      return 0
    fi
    sleep 0.5
  done
  echo "failed to fetch contract $id" >&2
  exit 1
}

build() {
  if [[ "$SKIP_BUILD" == "1" ]]; then
    step "Skip build (SKIP_BUILD=1)"
    return
  fi
  step "Build lumend"
  build_cmd=(go build -trimpath -ldflags "-s -w")
  if [[ -n "$LUMEN_BUILD_TAGS" ]]; then
    build_cmd+=(-tags "$LUMEN_BUILD_TAGS")
  fi
  build_cmd+=(-o "$BIN" ./cmd/lumend)
  (cd "$DIR" && "${build_cmd[@]}")
}

ADDR_CLIENT=""
ADDR_GATEWAY=""
ADDR_FINALIZER=""

init_chain() {
  step "Init chain"
  rm -rf "$HOME_DIR"
  export LUMEN_PQC_DISABLE=1
  "$BIN" init local --chain-id "$CHAIN_ID" --home "$HOME_DIR"

  for name in client gateway finalizer; do
    keys_add_quiet "$name"
  done

  ADDR_CLIENT=$("$BIN" keys show client -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
  ADDR_GATEWAY=$("$BIN" keys show gateway -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
  ADDR_FINALIZER=$("$BIN" keys show finalizer -a --keyring-backend "$KEYRING" --home "$HOME_DIR")

  for addr in "$ADDR_CLIENT" "$ADDR_GATEWAY" "$ADDR_FINALIZER"; do
    "$BIN" genesis add-genesis-account "$addr" 200000000ulmn --keyring-backend "$KEYRING" --home "$HOME_DIR"
  done

  local tmp; tmp=$(mktemp)
  jq --arg month "$MONTH_SECONDS" --arg gw "$ADDR_GATEWAY" '
    .app_state.gateways.params.platform_commission_bps = 150
    | .app_state.gateways.params.month_seconds = $month
    | .app_state.gateways.params.finalize_delay_months = 0
    | .app_state.gateways.params.finalizer_reward_bps = 500
    | .app_state.gateways.params.min_price_ulmn_per_month = "100"
    | .app_state.gateways.params.max_active_contracts_per_gateway = 10
    | .app_state.gateways.gateways = [
        {
          id:"1",
          operator:$gw,
          payout:$gw,
          active:true,
          metadata:"genesis",
          created_at:"1",
          active_clients:"0",
          cancellations:"0"
        }
      ]
    | .app_state.gateways.gateway_count = "1"
  ' "$HOME_DIR/config/genesis.json" > "$tmp"
  mv "$tmp" "$HOME_DIR/config/genesis.json"

  "$BIN" genesis gentx client 1000000ulmn --chain-id "$CHAIN_ID" --keyring-backend "$KEYRING" --home "$HOME_DIR" --moniker local --commission-rate 0.10 --commission-max-rate 0.20 --commission-max-change-rate 0.01 --min-self-delegation 1 >/dev/null
  "$BIN" genesis collect-gentxs --home "$HOME_DIR" >/dev/null
  "$BIN" genesis validate --home "$HOME_DIR" >/dev/null
  unset LUMEN_PQC_DISABLE
}

start_node() {
  step "Start node"
  kill_node
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
  (
    LUMEN_RL_PER_BLOCK="$LUMEN_RL_PER_BLOCK" \
    LUMEN_RL_PER_WINDOW="$LUMEN_RL_PER_WINDOW" \
    LUMEN_RL_WINDOW_SEC="$LUMEN_RL_WINDOW_SEC" \
    LUMEN_RL_GLOBAL_MAX="$LUMEN_RL_GLOBAL_MAX" \
    "${cmd[@]}" >"$LOG_FILE" 2>&1 &
  )
  sleep 1
  wait_http "$RPC/status"
  wait_http "$API/"
  wait_first_block
}

sleep_one_period() { sleep $((MONTH_SECONDS + 1)); }

build
init_chain
start_node
pqc_wait_ready "$RPC" "$API"
pqc_policy_must_be_required "$RPC"
for signer in client gateway finalizer; do
  setup_pqc_signer "$signer"
done

GATEWAY_ID="1"
step "Verify genesis gateway"
GW_JSON=$("$BIN" q gateways gateway "$GATEWAY_ID" --node "$RPC_LADDR" --output json)
expect_eq "$(echo "$GW_JSON" | jq -r '.gateway.operator')" "$ADDR_GATEWAY" "gateway operator"

step "Create contract (single month term)"
CREATE_JSON=$("$BIN" tx gateways create-contract "$GATEWAY_ID" 1000 50 20 1 --metadata "payout-flow" --from client --chain-id "$CHAIN_ID" --home "$HOME_DIR" --keyring-backend test --pqc-from "$ADDR_CLIENT" --pqc-key "pqc-client" --yes --broadcast-mode sync --output json)
ensure_code_zero "$CREATE_JSON"
await_tx "$CREATE_JSON"
if [ "${SKIP_PQC_NEGATIVE:-0}" != "1" ]; then
  if "$BIN" tx bank send client "$ADDR_GATEWAY" "1ulmn" \
     --pqc-enable=false --fees "$TX_FEES" --chain-id "$CHAIN_ID" \
     --keyring-backend "$KEYRING" --home "$HOME_DIR" \
     --broadcast-mode sync --yes >/tmp/pqc_neg.out 2>&1; then
    echo "error: PQC-disabled TX unexpectedly succeeded" >&2
    exit 1
  fi
  grep -qiE "pqc.*(missing|required|signature)" /tmp/pqc_neg.out || \
    echo "warning: PQC-disabled TX failed but no explicit PQC error found" >&2
fi
CONTRACT_CLAIM_ID="0"
sleep 1

sleep_one_period
step "Claim payment"
CLAIM1=$("$BIN" tx gateways claim-payment "$CONTRACT_CLAIM_ID" --from gateway --chain-id "$CHAIN_ID" --home "$HOME_DIR" --keyring-backend test --pqc-from "$ADDR_GATEWAY" --pqc-key "pqc-gateway" --yes --broadcast-mode sync --output json)
ensure_code_zero "$CLAIM1"
await_tx "$CLAIM1"

CLAIM_CONTRACT_JSON=$(query_contract "$CONTRACT_CLAIM_ID")
expect_eq "$(echo "$CLAIM_CONTRACT_JSON" | jq -r '.contract.claimed_months // "0"')" "1" "single month claimed"
expect_eq "$(echo "$CLAIM_CONTRACT_JSON" | jq -r '.contract.status')" "CONTRACT_STATUS_COMPLETED" "contract completed after payout"

step "Create contract for cancellation"
CREATE_CANCEL=$("$BIN" tx gateways create-contract "$GATEWAY_ID" 1500 40 15 3 --metadata "cancel-flow" --from client --chain-id "$CHAIN_ID" --home "$HOME_DIR" --keyring-backend test --pqc-from "$ADDR_CLIENT" --pqc-key "pqc-client" --yes --broadcast-mode sync --output json)
ensure_code_zero "$CREATE_CANCEL"
await_tx "$CREATE_CANCEL"
CONTRACT_CANCEL_ID="1"
sleep 1

step "Cancel contract before completion"
CANCEL_JSON=$("$BIN" tx gateways cancel-contract "$CONTRACT_CANCEL_ID" --from client --chain-id "$CHAIN_ID" --home "$HOME_DIR" --keyring-backend test --pqc-from "$ADDR_CLIENT" --pqc-key "pqc-client" --yes --broadcast-mode sync --output json)
ensure_code_zero "$CANCEL_JSON"
await_tx "$CANCEL_JSON"

CANCEL_CONTRACT_JSON=$(query_contract "$CONTRACT_CANCEL_ID")
expect_eq "$(echo "$CANCEL_CONTRACT_JSON" | jq -r '.contract.status')" "CONTRACT_STATUS_CANCELED" "contract canceled"

GW_JSON=$("$BIN" q gateways gateway "$GATEWAY_ID" --node "$RPC_LADDR" --output json)
expect_eq "$(echo "$GW_JSON" | jq -r '.gateway.cancellations')" "1" "gateway cancellation counter incremented"

step "Create contract for finalization"
CREATE_FINAL=$("$BIN" tx gateways create-contract "$GATEWAY_ID" 2000 25 10 1 --metadata "finalize-flow" --from client --chain-id "$CHAIN_ID" --home "$HOME_DIR" --keyring-backend test --pqc-from "$ADDR_CLIENT" --pqc-key "pqc-client" --yes --broadcast-mode sync --output json)
ensure_code_zero "$CREATE_FINAL"
await_tx "$CREATE_FINAL"
CONTRACT_FINAL_ID="2"
sleep 1

sleep_one_period
FINAL_CLAIM=$("$BIN" tx gateways claim-payment "$CONTRACT_FINAL_ID" --from gateway --chain-id "$CHAIN_ID" --home "$HOME_DIR" --keyring-backend test --pqc-from "$ADDR_GATEWAY" --pqc-key "pqc-gateway" --yes --broadcast-mode sync --output json)
ensure_code_zero "$FINAL_CLAIM"
await_tx "$FINAL_CLAIM"

sleep 1
FINALIZE_JSON=$("$BIN" tx gateways finalize-contract "$CONTRACT_FINAL_ID" --from finalizer --chain-id "$CHAIN_ID" --home "$HOME_DIR" --keyring-backend test --pqc-from "$ADDR_FINALIZER" --pqc-key "pqc-finalizer" --yes --broadcast-mode sync --output json)
ensure_code_zero "$FINALIZE_JSON"
await_tx "$FINALIZE_JSON"

FINAL_CONTRACT_JSON=$(query_contract "$CONTRACT_FINAL_ID")
expect_eq "$(echo "$FINAL_CONTRACT_JSON" | jq -r '.contract.status')" "CONTRACT_STATUS_FINALIZED" "contract finalized"

MOD_ACCOUNTS=$("$BIN" q gateways module-accounts --node "$RPC_LADDR" --output json)
if [[ "$(echo "$MOD_ACCOUNTS" | jq -r '.escrow')" == "" || "$(echo "$MOD_ACCOUNTS" | jq -r '.treasury')" == "" ]]; then
  echo "Module accounts query returned empty addresses" >&2
  exit 1
fi

step "Gateway E2E checks passed"
echo "Gateway E2E log: $LOG_FILE"
