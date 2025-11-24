#!/usr/bin/env bash
set -euo pipefail

SOURCE_DIR="$(cd "$(dirname "$0")" && pwd)"
. "${SOURCE_DIR}/lib_pqc.sh"
pqc_require_bins

HOME_E2E=$(mktemp -d -t lumen-e2e-slashing-XXXXXX)
export HOME="$HOME_E2E"

DIR=$(cd "${SOURCE_DIR}/../.." && pwd)
BIN="${BIN:-"$DIR/build/lumend"}"
: "${LUMEN_BUILD_TAGS:=dev}"
HOME_DIR="${HOME}/.lumen"

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
LOG_FILE="${LOG_FILE:-/tmp/lumen-slashing.log}"
CHAIN_ID="${CHAIN_ID:-lumen-slashing-1}"
KEYRING=${KEYRING:-test}
TX_FEES=${TX_FEES:-0ulmn}
DISABLE_PPROF="${DISABLE_PPROF:-1}"
NODE=${NODE:-$RPC_LADDR}
export NODE

SKIP_BUILD=${SKIP_BUILD:-0}
if [ "${1:-}" = "--skip-build" ]; then
  SKIP_BUILD=1
  shift
fi

step() { printf '\n==== %s\n' "$*"; }

keys_add_quiet() {
  local name="$1"
  printf '\n' | "$BIN" keys add "$name" --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null 2>&1 || true
}

wait_http() {
  local url="$1"
  for _ in $(seq 1 120); do
    if curl -sSf "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  echo "timeout waiting for $url" >&2
  tail -n 80 "$LOG_FILE" 2>/dev/null >&2 || true
  return 1
}

wait_height() {
  local target=${1:-1}
  for _ in $(seq 1 240); do
    local height
    height=$(curl -s "$RPC/status" | jq -r '.result.sync_info.latest_block_height' 2>/dev/null || echo "")
    if [ -n "$height" ] && [ "$height" != "null" ]; then
      if [ "$height" -ge "$target" ] 2>/dev/null; then
        return 0
      fi
    fi
    sleep 0.5
  done
  echo "timeout waiting for block height >= $target" >&2
  tail -n 80 "$LOG_FILE" 2>/dev/null >&2 || true
  return 1
}

kill_node() { pkill -f "lumend start" >/dev/null 2>&1 || true; }

cleanup() {
  kill_node
  if [ "${DEBUG_KEEP:-0}" != "1" ]; then
    rm -rf "$HOME_E2E" >/dev/null 2>&1 || true
  else
    echo "DEBUG_KEEP=1: keeping $HOME_E2E"
  fi
}
trap cleanup EXIT

build() {
  if [ "$SKIP_BUILD" = "1" ]; then
    echo "==> Skip build (SKIP_BUILD=1)"
    return
  fi
  step "Build lumend"
  local cmd=(go build -trimpath -ldflags "-s -w")
  if [ -n "$LUMEN_BUILD_TAGS" ]; then
    cmd+=(-tags "$LUMEN_BUILD_TAGS")
  fi
  cmd+=(-o "$BIN" ./cmd/lumend)
  (cd "$DIR" && "${cmd[@]}")
}

init_chain() {
  step "Init chain with slashing"
  rm -rf "$HOME_DIR"
  "$BIN" init slashing-local --chain-id "$CHAIN_ID" --home "$HOME_DIR" >/dev/null

  keys_add_quiet validator
  keys_add_quiet signer

  VAL_ADDR=$("$BIN" keys show validator -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
  SIGNER_ADDR=$("$BIN" keys show signer -a --keyring-backend "$KEYRING" --home "$HOME_DIR")

  "$BIN" genesis add-genesis-account "$VAL_ADDR" 200000000ulmn --keyring-backend "$KEYRING" --home "$HOME_DIR"
  "$BIN" genesis add-genesis-account "$SIGNER_ADDR" 100000000ulmn --keyring-backend "$KEYRING" --home "$HOME_DIR"

  "$BIN" genesis gentx validator 1000000ulmn \
    --chain-id "$CHAIN_ID" \
    --keyring-backend "$KEYRING" \
    --home "$HOME_DIR" >/dev/null
  "$BIN" genesis collect-gentxs --home "$HOME_DIR" >/dev/null
  "$BIN" genesis validate --home "$HOME_DIR" >/dev/null
  pqc_set_client_config "$HOME_DIR" "$RPC_LADDR" "$CHAIN_ID"
}

start_node() {
  step "Start node"
  kill_node
  local args=(
    "$BIN" start
    --home "$HOME_DIR"
    --rpc.laddr "$RPC_LADDR"
    --p2p.laddr "$P2P_LADDR"
    --api.enable
    --api.address "$API_ADDR"
    --grpc.address "$GRPC_ADDR"
    --minimum-gas-prices 0ulmn
  )
  if [ "$DISABLE_PPROF" = "1" ]; then
    args+=(--rpc.pprof_laddr "")
  fi
  ("${args[@]}" >"$LOG_FILE" 2>&1 &)
  sleep 1
  wait_http "$RPC/status"
  wait_http "$API/"
}

send_pqc_tx() {
  local to_addr="$1"
  local amount="${2:-1000ulmn}"
  local res hash code
  res=$("$BIN" tx bank send signer "$to_addr" "$amount" \
    --from signer \
    --chain-id "$CHAIN_ID" \
    --keyring-backend "$KEYRING" \
    --home "$HOME_DIR" \
    --fees "$TX_FEES" \
    --yes \
    --broadcast-mode sync \
    --pqc-enable=true \
    --pqc-from "$SIGNER_ADDR" \
    --pqc-key "pqc-signer" \
    -o json)
  echo "$res" | jq
  hash=$(echo "$res" | jq -r '.txhash // empty')
  if [ -z "$hash" ] || [ "$hash" = "null" ]; then
    echo "missing txhash for bank send" >&2
    exit 1
  fi
  code=$(pqc_wait_tx "$hash" "$RPC") || exit 1
  if [ "$code" != "0" ]; then
    echo "bank send failed with code=$code" >&2
    exit 1
  fi
}

main() {
  build
  init_chain
  start_node

  step "Setup PQC signer accounts"
  setup_pqc_signer validator
  setup_pqc_signer signer

  step "Wait for initial blocks"
  wait_height 3

  step "Send PQC-protected tx before downtime"
  send_pqc_tx "$VAL_ADDR" "1000ulmn"

  local before_height
  before_height=$(curl -s "$RPC/status" | jq -r '.result.sync_info.latest_block_height' 2>/dev/null || echo "1")
  if ! [[ "$before_height" =~ ^[0-9]+$ ]]; then
    before_height=1
  fi

  step "Simulate validator downtime (stop node)"
  kill_node
  sleep 8

  step "Restart node after downtime"
  start_node
  wait_height $((before_height + 3))

  step "Send PQC-protected tx after downtime"
  send_pqc_tx "$VAL_ADDR" "2000ulmn"

  echo "e2e_slashing passed: downtime did not halt chain and PQC txs succeed"
}

main "$@"

