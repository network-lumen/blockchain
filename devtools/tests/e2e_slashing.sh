#!/usr/bin/env bash
set -euo pipefail

SOURCE_DIR="$(cd "$(dirname "$0")" && pwd)"
. "${SOURCE_DIR}/lib_pqc.sh"
. "${SOURCE_DIR}/lib_gov.sh"
pqc_require_bins
gov_require_bins

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
  # Speed up governance so we can exercise proposals in this e2e.
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
    }' "$HOME_DIR/config/genesis.json" >"$tmp"
  mv "$tmp" "$HOME_DIR/config/genesis.json"
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

slashing_query_params() {
  "$BIN" query slashing params \
    --node "$NODE" \
    --home "$HOME_DIR" \
    -o json | jq '.params'
}

slashing_test_downtime_params_update() {
  step "Resolve gov authority for slashing params"
  GOV_AUTHORITY=$(gov_resolve_authority)
  if [ -z "$GOV_AUTHORITY" ] || [ "$GOV_AUTHORITY" = "null" ]; then
    echo "failed to resolve gov authority address" >&2
    exit 1
  fi

  step "Query initial slashing params"
  local before_json before_frac before_jail
  before_json=$(slashing_query_params)
  before_frac=$(echo "$before_json" | jq -r '.slash_fraction_downtime')
  before_jail=$(echo "$before_json" | jq -r '.downtime_jail_duration')

  step "Gov: propose invalid downtime fraction (>5%)"
  local bad_msg bad_pid bad_json
  bad_msg=$(jq -cn --arg auth "$GOV_AUTHORITY" \
    '{"@type":"/lumen.tokenomics.v1.MsgUpdateSlashingDowntimeParams", "authority":$auth, "slash_fraction_downtime":"0.5", "downtime_jail_duration":"600s"}')
  gov_submit_single_msg_proposal "$bad_msg" "Bad slashing downtime params" "should fail" "10000000ulmn" || true
  bad_pid="$GOV_LAST_PROPOSAL_ID"
  if [ -n "$bad_pid" ]; then
    gov_cast_vote "$bad_pid" validator yes
    gov_wait_status "$bad_pid" "PROPOSAL_STATUS_FAILED"
  fi
  bad_json=$(slashing_query_params)
  if [ "$(echo "$bad_json" | jq -r '.slash_fraction_downtime')" != "$before_frac" ]; then
    echo "slash_fraction_downtime changed after invalid proposal" >&2
    exit 1
  fi
  if [ "$(echo "$bad_json" | jq -r '.downtime_jail_duration')" != "$before_jail" ]; then
    echo "downtime_jail_duration changed after invalid proposal" >&2
    exit 1
  fi

  step "Gov: propose update with wrong authority"
  local wrong_msg wrong_pid wrong_json
  wrong_msg=$(jq -cn --arg auth "$VAL_ADDR" \
    '{"@type":"/lumen.tokenomics.v1.MsgUpdateSlashingDowntimeParams", "authority":$auth, "slash_fraction_downtime":"0.02", "downtime_jail_duration":"600s"}')
  gov_submit_single_msg_proposal "$wrong_msg" "Wrong authority slashing params" "should fail" "10000000ulmn" || true
  wrong_pid="$GOV_LAST_PROPOSAL_ID"
  if [ -n "$wrong_pid" ]; then
    gov_cast_vote "$wrong_pid" validator yes
    gov_wait_status "$wrong_pid" "PROPOSAL_STATUS_FAILED"
  fi
  wrong_json=$(slashing_query_params)
  if [ "$(echo "$wrong_json" | jq -r '.slash_fraction_downtime')" != "$before_frac" ]; then
    echo "slash_fraction_downtime changed after wrong-authority proposal" >&2
    exit 1
  fi
  if [ "$(echo "$wrong_json" | jq -r '.downtime_jail_duration')" != "$before_jail" ]; then
    echo "downtime_jail_duration changed after wrong-authority proposal" >&2
    exit 1
  fi

  step "Gov: propose valid downtime params update (2% / 120s)"
  local good_msg good_pid final_json final_frac final_jail
  good_msg=$(jq -cn --arg auth "$GOV_AUTHORITY" \
    '{"@type":"/lumen.tokenomics.v1.MsgUpdateSlashingDowntimeParams", "authority":$auth, "slash_fraction_downtime":"0.02", "downtime_jail_duration":"120s"}')
  gov_submit_single_msg_proposal "$good_msg" "Update slashing downtime params" "set fraction=0.02 and jail=120s" "10000000ulmn"
  good_pid="$GOV_LAST_PROPOSAL_ID"
  gov_cast_vote "$good_pid" validator yes
  gov_wait_status "$good_pid" "PROPOSAL_STATUS_PASSED"

  final_json=$(slashing_query_params)
  final_frac=$(echo "$final_json" | jq -r '.slash_fraction_downtime')
  final_jail=$(echo "$final_json" | jq -r '.downtime_jail_duration')
  if [ "$final_frac" = "$before_frac" ] || [ "$final_jail" = "$before_jail" ]; then
    echo "expected downtime params to change after valid proposal" >&2
    echo "before: $before_frac / $before_jail" >&2
    echo "after : $final_frac / $final_jail" >&2
    exit 1
  fi
}

slashing_test_liveness_params_update() {
  step "Resolve gov authority for slashing liveness params"
  GOV_AUTHORITY=$(gov_resolve_authority)
  if [ -z "$GOV_AUTHORITY" ] || [ "$GOV_AUTHORITY" = "null" ]; then
    echo "failed to resolve gov authority address" >&2
    exit 1
  fi

  step "Query initial slashing liveness params"
  local before_json before_window before_min
  before_json=$(slashing_query_params)
  before_window=$(echo "$before_json" | jq -r '.signed_blocks_window')
  before_min=$(echo "$before_json" | jq -r '.min_signed_per_window')

  step "Gov: propose invalid liveness params (window=0)"
  local bad_msg bad_pid bad_json
  bad_msg=$(jq -cn --arg auth "$GOV_AUTHORITY" \
    '{"@type":"/lumen.tokenomics.v1.MsgUpdateSlashingLivenessParams", "authority":$auth, "signed_blocks_window":"0", "min_signed_per_window":"0.9"}')
  gov_submit_single_msg_proposal "$bad_msg" "Bad slashing liveness params" "should fail" "10000000ulmn" || true
  bad_pid="$GOV_LAST_PROPOSAL_ID"
  if [ -n "$bad_pid" ]; then
    gov_cast_vote "$bad_pid" validator yes
    gov_wait_status "$bad_pid" "PROPOSAL_STATUS_FAILED"
  fi
  bad_json=$(slashing_query_params)
  if [ "$(echo "$bad_json" | jq -r '.signed_blocks_window')" != "$before_window" ]; then
    echo "signed_blocks_window changed after invalid proposal" >&2
    exit 1
  fi
  if [ "$(echo "$bad_json" | jq -r '.min_signed_per_window')" != "$before_min" ]; then
    echo "min_signed_per_window changed after invalid proposal" >&2
    exit 1
  fi

  step "Gov: propose liveness params update with wrong authority"
  local wrong_msg wrong_pid wrong_json
  wrong_msg=$(jq -cn --arg auth "$VAL_ADDR" \
    '{"@type":"/lumen.tokenomics.v1.MsgUpdateSlashingLivenessParams", "authority":$auth, "signed_blocks_window":"200", "min_signed_per_window":"0.9"}')
  gov_submit_single_msg_proposal "$wrong_msg" "Wrong authority slashing liveness" "should fail" "10000000ulmn" || true
  wrong_pid="$GOV_LAST_PROPOSAL_ID"
  if [ -n "$wrong_pid" ]; then
    gov_cast_vote "$wrong_pid" validator yes
    gov_wait_status "$wrong_pid" "PROPOSAL_STATUS_FAILED"
  fi
  wrong_json=$(slashing_query_params)
  if [ "$(echo "$wrong_json" | jq -r '.signed_blocks_window')" != "$before_window" ]; then
    echo "signed_blocks_window changed after wrong-authority proposal" >&2
    exit 1
  fi
  if [ "$(echo "$wrong_json" | jq -r '.min_signed_per_window')" != "$before_min" ]; then
    echo "min_signed_per_window changed after wrong-authority proposal" >&2
    exit 1
  fi

  step "Gov: propose valid liveness params update (window=200 / min=0.9)"
  local good_msg good_pid final_json final_window final_min
  good_msg=$(jq -cn --arg auth "$GOV_AUTHORITY" \
    '{"@type":"/lumen.tokenomics.v1.MsgUpdateSlashingLivenessParams", "authority":$auth, "signed_blocks_window":"200", "min_signed_per_window":"0.9"}')
  gov_submit_single_msg_proposal "$good_msg" "Update slashing liveness params" "set window=200 and min_signed=0.9" "10000000ulmn"
  good_pid="$GOV_LAST_PROPOSAL_ID"
  gov_cast_vote "$good_pid" validator yes
  gov_wait_status "$good_pid" "PROPOSAL_STATUS_PASSED"

  final_json=$(slashing_query_params)
  final_window=$(echo "$final_json" | jq -r '.signed_blocks_window')
  final_min=$(echo "$final_json" | jq -r '.min_signed_per_window')
  if [ "$final_window" != "200" ]; then
    echo "expected signed_blocks_window=200, got $final_window" >&2
    exit 1
  fi
  if [ "$final_min" != "0.900000000000000000" ]; then
    echo "expected min_signed_per_window=0.900000000000000000, got $final_min" >&2
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

  slashing_test_downtime_params_update
  slashing_test_liveness_params_update

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

  echo "e2e_slashing passed: governance slashing params updated; downtime did not halt chain and PQC txs succeed"
}

main "$@"
