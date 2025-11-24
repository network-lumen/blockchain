#!/usr/bin/env bash
set -euo pipefail

SOURCE_DIR="$(cd "$(dirname "$0")" && pwd)"
. "${SOURCE_DIR}/lib_pqc.sh"
. "${SOURCE_DIR}/lib_gov.sh"

pqc_require_bins
gov_require_bins

HOME_ROOT=$(mktemp -d -t lumen-e2e-upgrade-XXXXXX)
export HOME="$HOME_ROOT"

DIR=$(cd "${SOURCE_DIR}/../.." && pwd)
BIN="${BIN:-"$DIR/build/lumend"}"
: "${LUMEN_BUILD_TAGS:=dev}"
HOME_DIR="${HOME}/.lumen"

RANDOM_PORT_BASE=${E2E_BASE_PORT:-$(( (RANDOM % 1000) + 36000 ))}

RPC_HOST="${RPC_HOST:-127.0.0.1}"
RPC_PORT="${RPC_PORT:-$RANDOM_PORT_BASE}"
RPC_LADDR="${RPC_LADDR:-tcp://${RPC_HOST}:${RPC_PORT}}"
RPC="${RPC:-http://${RPC_HOST}:${RPC_PORT}}"

API_HOST="${API_HOST:-127.0.0.1}"
API_PORT="${API_PORT:-$((RANDOM_PORT_BASE + 60))}"
API_ADDR="${API_ADDR:-tcp://${API_HOST}:${API_PORT}}"
API="${API:-http://${API_HOST}:${API_PORT}}"

GRPC_HOST="${GRPC_HOST:-127.0.0.1}"
GRPC_PORT="${GRPC_PORT:-$((RANDOM_PORT_BASE + 120))}"
GRPC_ADDR="${GRPC_ADDR:-${GRPC_HOST}:${GRPC_PORT}}"

P2P_HOST="${P2P_HOST:-0.0.0.0}"
P2P_PORT="${P2P_PORT:-$((RANDOM_PORT_BASE + 180))}"
P2P_LADDR="${P2P_LADDR:-tcp://${P2P_HOST}:${P2P_PORT}}"

LOG_FILE="${LOG_FILE:-/tmp/lumen-upgrade.log}"
CHAIN_ID="${CHAIN_ID:-lumen-upgrade-1}"
KEYRING=${KEYRING:-test}
TX_FEES=${TX_FEES:-0ulmn}
NODE=${NODE:-$RPC_LADDR}
DISABLE_PPROF="${DISABLE_PPROF:-1}"
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

kill_node() { pkill -f "lumend start" >/dev/null 2>&1 || true; }

cleanup() {
  kill_node
  if [ "${DEBUG_KEEP:-0}" != "1" ]; then
    rm -rf "$HOME_ROOT" >/dev/null 2>&1 || true
  else
    echo "DEBUG_KEEP=1 -> preserving $HOME_ROOT"
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
  step "Init chain with short gov windows"
  rm -rf "$HOME_DIR"
  "$BIN" init gov-upgrade --chain-id "$CHAIN_ID" --home "$HOME_DIR"

  keys_add_quiet validator
  keys_add_quiet voter2

  VAL_ADDR=$("$BIN" keys show validator -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
  VOTER2_ADDR=$("$BIN" keys show voter2 -a --keyring-backend "$KEYRING" --home "$HOME_DIR")

  "$BIN" genesis add-genesis-account "$VAL_ADDR" 200000000ulmn --keyring-backend "$KEYRING" --home "$HOME_DIR"
  "$BIN" genesis add-genesis-account "$VOTER2_ADDR" 80000000ulmn --keyring-backend "$KEYRING" --home "$HOME_DIR"

  "$BIN" genesis gentx validator 50000000ulmn --chain-id "$CHAIN_ID" --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null
  "$BIN" genesis collect-gentxs --home "$HOME_DIR" >/dev/null

  local tmp
  tmp=$(mktemp)
  jq '.app_state.gov.params = {
      min_deposit:[{denom:"ulmn",amount:"10000000"}],
      expedited_min_deposit:[{denom:"ulmn",amount:"50000000"}],
      max_deposit_period:"8s",
      voting_period:"8s",
      expedited_voting_period:"4s",
      quorum:"0.670000000000000000",
      threshold:"0.750000000000000000",
      expedited_threshold:"0.850000000000000000",
      veto_threshold:"0.334000000000000000",
      min_initial_deposit_ratio:"0.000000000000000000",
      proposal_cancel_ratio:"0.000000000000000000",
      proposal_cancel_dest:"",
      burn_proposal_deposit_prevote:false,
      burn_vote_quorum:false,
      burn_vote_veto:false,
      min_deposit_ratio:"0.010000000000000000"
    }
    | .app_state.gov.constitution = "Lumen DAO stewards DNS and gateway policies."
    ' "$HOME_DIR/config/genesis.json" >"$tmp"
  mv "$tmp" "$HOME_DIR/config/genesis.json"

  tmp=$(mktemp)
  jq '.app_state.dns.params |= (.update_rate_limit_seconds = "2" | .update_pow_difficulty = 4)' \
    "$HOME_DIR/config/genesis.json" >"$tmp"
  mv "$tmp" "$HOME_DIR/config/genesis.json"

  tmp=$(mktemp)
  jq '.app_state.distribution.params.community_tax = "0.0"' \
    "$HOME_DIR/config/genesis.json" >"$tmp"
  mv "$tmp" "$HOME_DIR/config/genesis.json"

  jq -r '.app_state.gov.params.min_initial_deposit_ratio' "$HOME_DIR/config/genesis.json" | grep -qx '0.000000000000000000'
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
  gov_wait_http "$RPC/status"
  gov_wait_http "$API/"
}

submit_upgrade_proposal() {
  local current_height
  current_height=$(curl -s "$RPC/status" | jq -r '.result.sync_info.latest_block_height' 2>/dev/null || echo "1")
  if ! [[ "$current_height" =~ ^[0-9]+$ ]]; then
    current_height=1
  fi
  local upgrade_height=$((current_height + 6))

  GOV_AUTHORITY=$(gov_resolve_authority)
  if [ -z "$GOV_AUTHORITY" ] || [ "$GOV_AUTHORITY" = "null" ]; then
    echo "failed to resolve gov authority address" >&2
    exit 1
  fi

  local proposal_file
  proposal_file=$(mktemp -t gov-upgrade-v1-XXXXXX.json)
  jq -n \
    --arg auth "$GOV_AUTHORITY" \
    --arg name "v1" \
    --arg height_str "$upgrade_height" \
    '{
      messages: [
        {
          "@type": "/cosmos.upgrade.v1.MsgSoftwareUpgrade",
          authority: $auth,
          plan: {
            name: $name,
            height: $height_str,
            info: "e2e-upgrade-test"
          }
        }
      ],
      title: "Upgrade v1",
      summary: "E2E upgrade test for v1",
      metadata: ""
    }' >"$proposal_file"

  if ! gov_submit_proposal_file "$proposal_file" "10000000ulmn"; then
    echo "failed to submit upgrade proposal" >&2
    exit 1
  fi

  PROPOSAL_ID="$GOV_LAST_PROPOSAL_ID"
  UPGRADE_HEIGHT="$upgrade_height"
}

main() {
  build
  init_chain
  start_node

  step "Setup PQC-enabled validator accounts"
  setup_pqc_signer validator
  setup_pqc_signer voter2

  step "Submit software upgrade proposal v1"
  submit_upgrade_proposal

  step "Deposit and vote YES on proposal $PROPOSAL_ID"
  gov_deposit "$PROPOSAL_ID" "10000000ulmn" validator
  gov_cast_vote "$PROPOSAL_ID" validator YES
  gov_cast_vote "$PROPOSAL_ID" voter2 YES
  gov_wait_status "$PROPOSAL_ID" "PROPOSAL_STATUS_PASSED"

  step "Wait for upgrade height $UPGRADE_HEIGHT"
  gov_wait_height "$UPGRADE_HEIGHT"
  gov_wait_height $((UPGRADE_HEIGHT + 2))

  step "Verify node still serving RPC after upgrade"
  gov_wait_http "$RPC/status" 20

  echo "e2e_upgrade passed: v1 upgrade handler executed and chain continued producing blocks"
}

main "$@"

