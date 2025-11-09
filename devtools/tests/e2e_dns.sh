#!/usr/bin/env bash
set -euo pipefail

# Environment variables:
#   SKIP_BUILD       Skip rebuilding the binary (default 0)
#   GRACE_DAYS       Override grace period in genesis (default 0)
#   AUCTION_DAYS     Override auction period in genesis (default 0)
#   RPC_HOST/PORT    RPC bind host/port (default 127.0.0.1:26657)
#   API_HOST/PORT    REST bind host/port (default 127.0.0.1:1317)
#   GRPC_HOST/PORT   gRPC bind host/port (default 127.0.0.1:9090)
#   GRPC_WEB_ENABLE  Enable gRPC-Web (default 1)
#   LOG_FILE         Node log destination (default /tmp/lumen.log)
#   DEBUG_KEEP       Set to 1 to keep the temporary HOME directory on exit

HOME_LUMEN=$(mktemp -d -t lumen-e2e-XXXXXX)
trap '[[ "${DEBUG_KEEP:-0}" = "1" ]] || rm -rf "$HOME_LUMEN"' EXIT
export HOME="$HOME_LUMEN"

DIR=$(cd "$(dirname "$0")/../.." && pwd)
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
GRACE_DAYS=${GRACE_DAYS:-0}
AUCTION_DAYS=${AUCTION_DAYS:-0}
CHAIN_ID="lumen"
FARMER_NAME=farmer

require() { command -v "$1" >/dev/null || { echo "Missing dependency: $1" >&2; exit 1; }; }
require jq
require curl

keys_add_quiet() {
  local name="$1"
  printf '\n' | "$BIN" keys add "$name" --keyring-backend test --home "$HOME_DIR" >/dev/null 2>&1 || true
}

step() { echo; echo "==== $*"; }
wait_http() {
  local url="$1"; local tries=60
  for i in $(seq 1 $tries); do
    if curl -sSf "$url" >/dev/null; then return 0; fi; sleep 0.5
  done
  echo "Timeout waiting for $url" >&2; return 1
}

wait_tx_commit() {
  local hash="$1"; local tries=60
  for i in $(seq 1 $tries); do
    local code
    code=$(curl -s "$RPC/tx?hash=0x$hash" | jq -r .result.tx_result.code)
    if [ "$code" != "null" ]; then echo "tx_code=$code"; return 0; fi
    sleep 0.5
  done
  echo "Timeout waiting for tx $hash" >&2
  return 1
}

kill_node() {
  pkill -f "lumend start" >/dev/null 2>&1 || true
}

build() {
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
  ADDR_FARMER=$("$BIN" keys show "$FARMER_NAME" -a --keyring-backend test --home "$HOME_DIR")
  "$BIN" genesis add-genesis-account "$ADDR_FARMER" 100000000000ulmn --keyring-backend test --home "$HOME_DIR"
  "$BIN" genesis gentx "$FARMER_NAME" 1000000ulmn \
    --chain-id "$CHAIN_ID" --keyring-backend test --home "$HOME_DIR" \
    --moniker "local-farmer" --commission-rate 0.10 \
    --commission-max-rate 0.20 --commission-max-change-rate 0.01 \
    --min-self-delegation 1 >/dev/null
  "$BIN" genesis collect-gentxs --home "$HOME_DIR" >/dev/null

  tmp=$(mktemp)
  jq \
    ".app_state.dns.params.grace_days=\"$GRACE_DAYS\" \
     | .app_state.dns.params.auction_days=\"$AUCTION_DAYS\" \
     | .app_state.dns.params.update_rate_limit_seconds=\"0\" \
     | .app_state.dns.params.update_pow_difficulty=\"0\"" \
    "$HOME_DIR/config/genesis.json" > "$tmp" && mv "$tmp" "$HOME_DIR/config/genesis.json"

  "$BIN" genesis validate --home "$HOME_DIR"
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
  ("${cmd[@]}" >"$LOG_FILE" 2>&1 &)
  sleep 1
  wait_http "$RPC/status"
  wait_http "$API/"
  for i in $(seq 1 80); do
    H=$(curl -s "$RPC/status" | jq -r .result.sync_info.latest_block_height)
    [ "$H" != "0" ] && break || sleep 0.5
  done
  for i in $(seq 1 20); do
    H2=$(curl -s "$RPC/status" | jq -r .result.sync_info.latest_block_height)
    [ "$H2" -ge 2 ] && break || sleep 0.5
  done
  ADDR_FARMER=$("$BIN" keys show "$FARMER_NAME" -a --keyring-backend test --home "$HOME_DIR")
  for i in $(seq 1 80); do
    CODE=$(curl -s "$API/cosmos/auth/v1beta1/accounts/$ADDR_FARMER" | jq -r .code 2>/dev/null || echo "")
    if [ -z "$CODE" ] || [ "$CODE" = "null" ]; then break; fi
    sleep 0.5
  done
  for i in $(seq 1 80); do
    OK=$(curl -s "$API/cosmos/bank/v1beta1/balances/$ADDR_FARMER" | jq -r .balances | wc -c || true)
    [ -n "$OK" ] && [ "$OK" -gt 0 ] && break || sleep 0.5
  done
}



register_cli(){
  local name="$1"; local ext="$2"; local fromName="$3"
  "$BIN" tx dns register "$name" "$ext" "[]" 0 \
    --from "$fromName" --keyring-backend test --home "$HOME_DIR" \
    --chain-id "$CHAIN_ID" -y -o json
}

make_unsigned_update_json() {
  local owner="$1"; local name="$2"; local ext="$3"; local memo="$4"
  jq -nc --arg own "$owner" --arg d "$name" --arg e "$ext" --arg m "$memo" '{
    body:{messages:[{"@type":"/lumen.dns.v1.MsgUpdate","creator":$own,"domain":$d,"ext":$e,"records":[]}], memo:$m, timeout_height:"0", extension_options:[], non_critical_extension_options:[]},
    auth_info:{signer_infos:[], fee:{amount:[], gas_limit:"200000", payer:"", granter:""}},
    signatures:[]
  }'
}

sign_and_broadcast() {
  local unsigned_json="$1"; local from_name="$2"; local accnum="$3"; local seq="$4"; local out_json="$5"
  echo "$unsigned_json" > /tmp/tx_unsigned.json
  "$BIN" tx sign /tmp/tx_unsigned.json --from "$from_name" --keyring-backend test --home "$HOME_DIR" -o json --chain-id "$CHAIN_ID" --account-number "$accnum" --sequence "$seq" > /tmp/tx_signed.json
  "$BIN" tx broadcast /tmp/tx_signed.json -o json > "$out_json"
}

fund_owner() {
  local owner="$1"; local amt="$2"
  local res hash
  res=$("$BIN" tx bank send "$FARMER_NAME" "$owner" "$amt" --keyring-backend test --home "$HOME_DIR" --chain-id "$CHAIN_ID" -y -o json)
  echo "$res" | jq
  hash=$(echo "$res" | jq -r .txhash)
  [ -n "$hash" ] && [ "$hash" != "null" ] && wait_tx_commit "$hash"
}

update_domain_expire() {
  local index="$1"; local name="$2"; local owner="$3"; local expire_at="$4"
  local res hash
  res=$("$BIN" tx dns update-domain "$index" "$name" "$owner" "{}" "$expire_at" \
    --from "$FARMER_NAME" --keyring-backend test --home "$HOME_DIR" --chain-id "$CHAIN_ID" -y -o json)
  echo "$res" | jq
  hash=$(echo "$res" | jq -r .txhash)
  [ -n "$hash" ] && [ "$hash" != "null" ] && wait_tx_commit "$hash"
}


build
init_chain
start_node

step "Create owner1 (unfunded)"
keys_add_quiet owner1
OWNER1=$("$BIN" keys show owner1 -a --keyring-backend test --home "$HOME_DIR")
echo "OWNER1=$OWNER1"

NAME="codex-e2e"
EXT="lumen"

step "Register via CLI (owner1)"
REG=$(register_cli "$NAME" "$EXT" owner1)
echo "$REG" | jq
HASH=$(echo "$REG" | jq -r .txhash)
[[ -n "$HASH" && "$HASH" != "null" ]] || { echo "register failed" >&2; exit 1; }

step "Wait for register commit and verify account"
for i in $(seq 1 30); do
  CODE=$(curl -s "$RPC/tx?hash=0x$HASH" | jq -r .result.tx_result.code); [ "$CODE" != "null" ] && break; sleep 0.5
done
echo "tx_code=${CODE:-?}"
curl -s "$API/cosmos/auth/v1beta1/accounts/$OWNER1" | jq

step "Verify domain listed"
"$BIN" query dns list-domain -o json --home "$HOME_DIR" | jq

step "Update via CLI (owner1)"
UPD=$("$BIN" tx dns update "$NAME" "$EXT" "[]" \
  --from owner1 --keyring-backend test --home "$HOME_DIR" --chain-id "$CHAIN_ID" -y -o json)
echo "$UPD" | jq
HASH=$(echo "$UPD" | jq -r .txhash)
wait_tx_commit "$HASH"

step "Verify domain updated"
"$BIN" query dns list-domain -o json --home "$HOME_DIR" | jq

step "Fund owner1 for paid update"
fund_owner "$OWNER1" 100000ulmn

step "Paid update (owner pays fees)"
ACCNUM=$(curl -s "$API/cosmos/auth/v1beta1/accounts/$OWNER1" | jq -r '.account.account_number // .account.base_account.account_number')
SEQ=$(curl -s "$API/cosmos/auth/v1beta1/accounts/$OWNER1" | jq -r '.account.sequence // .account.base_account.sequence')
UNSIGNED=$(make_unsigned_update_json "$OWNER1" "$NAME" "$EXT" "paid")
sign_and_broadcast "$UNSIGNED" owner1 "$ACCNUM" "$SEQ" /tmp/update_paid.json
cat /tmp/update_paid.json | jq
HASH=$(jq -r .txhash /tmp/update_paid.json)
wait_tx_commit "$HASH"

step "Verify domain updated"
"$BIN" query dns list-domain -o json --home "$HOME_DIR" | jq

step "Simulate expiration by setting expire_at in the past"
NOW=$(date +%s)
PAST=$((NOW-10))
INDEX="$NAME.$EXT"
update_domain_expire "$INDEX" "$INDEX" "$OWNER1" "$PAST"
"$BIN" query dns list-domain -o json --home "$HOME_DIR" | jq

step "Create owner2 and re-register same name after expiration (CLI)"
keys_add_quiet owner2
REG2=$(register_cli "$NAME" "$EXT" owner2)
echo "$REG2" | jq
HASH=$(echo "$REG2" | jq -r .txhash)
wait_tx_commit "$HASH"

step "Verify ownership transferred to owner2 "
"$BIN" query dns list-domain -o json --home "$HOME_DIR" | jq

echo "\nAll steps completed."



