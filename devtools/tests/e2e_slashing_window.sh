#!/usr/bin/env bash
set -euo pipefail

SOURCE_DIR="$(cd "$(dirname "$0")" && pwd)"
. "${SOURCE_DIR}/lib_gov.sh"
. "${SOURCE_DIR}/lib_pqc.sh"

gov_require_bins
pqc_require_bins

HOME_E2E=$(mktemp -d -t lumen-e2e-slashing-window-XXXXXX)
export HOME="$HOME_E2E"

DIR=$(cd "${SOURCE_DIR}/../.." && pwd)
BIN="${BIN:-"$DIR/build/lumend"}"
: "${LUMEN_BUILD_TAGS:=dev}"

CHAIN_ID="${CHAIN_ID:-lumen-slashwin-1}"
KEYRING="${KEYRING:-test}"
TX_FEES="${TX_FEES:-0ulmn}"

RANDOM_PORT_BASE=${E2E_BASE_PORT:-$(( (RANDOM % 1000) + 38000 ))}
RPC_HOST="${RPC_HOST:-127.0.0.1}"

RPC0_PORT="${RPC0_PORT:-$RANDOM_PORT_BASE}"
RPC1_PORT="${RPC1_PORT:-$((RANDOM_PORT_BASE + 10))}"
RPC2_PORT="${RPC2_PORT:-$((RANDOM_PORT_BASE + 20))}"
P2P0_PORT="${P2P0_PORT:-$((RANDOM_PORT_BASE + 180))}"
P2P1_PORT="${P2P1_PORT:-$((RANDOM_PORT_BASE + 190))}"
P2P2_PORT="${P2P2_PORT:-$((RANDOM_PORT_BASE + 200))}"

RPC0="http://${RPC_HOST}:${RPC0_PORT}"
NODE0="tcp://${RPC_HOST}:${RPC0_PORT}"

DISABLE_PPROF="${DISABLE_PPROF:-1}"

TESTNET_DIR="$HOME_E2E/.testnets"
HOME0="$TESTNET_DIR/validator0"
HOME1="$TESTNET_DIR/validator1"
HOME2="$TESTNET_DIR/validator2"

LOG0="${LOG0:-/tmp/lumen-slashwin-0.log}"
LOG1="${LOG1:-/tmp/lumen-slashwin-1.log}"
LOG2="${LOG2:-/tmp/lumen-slashwin-2.log}"

SKIP_BUILD=${SKIP_BUILD:-0}
if [ "${1:-}" = "--skip-build" ]; then
  SKIP_BUILD=1
  shift
fi

step() { printf '\n==== %s\n' "$*"; }

kill_nodes() { pkill -f "lumend start.*$HOME_E2E" >/dev/null 2>&1 || true; }

cleanup() {
  kill_nodes
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

wait_http() {
  local url="$1"
  for _ in $(seq 1 180); do
    if curl -sSf "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  echo "timeout waiting for $url" >&2
  return 1
}

latest_height() {
  curl -s "$RPC0/status" | jq -r '.result.sync_info.latest_block_height' 2>/dev/null || echo "0"
}

wait_height() {
  local target="$1"
  for _ in $(seq 1 240); do
    local h
    h="$(latest_height)"
    if [ -n "$h" ] && [ "$h" != "null" ] && [ "$h" -ge "$target" ] 2>/dev/null; then
      return 0
    fi
    sleep 0.5
  done
  echo "timeout waiting for height >= $target" >&2
  return 1
}

tx_wait() {
  local hash="$1"
  local code
  code=$(gov_wait_tx "$hash")
  if [ "$code" != "0" ]; then
    echo "tx $hash failed with code=$code" >&2
    curl -s "$RPC0/tx?hash=0x$hash" | jq >&2 || true
    exit 1
  fi
}

tx_submit_proposal() {
  local proposal_file="$1"
  local res hash pid tx_json
  res=$("$BIN" tx gov submit-proposal "$proposal_file" \
    --from validator0 \
    --chain-id "$CHAIN_ID" \
    --node "$NODE0" \
    --keyring-backend "$KEYRING" \
    --home "$HOME0" \
    --gas 400000 \
    --fees "$TX_FEES" \
    --yes \
    -o json)
  hash=$(echo "$res" | jq -r '.txhash // empty')
  [ -n "$hash" ] && [ "$hash" != "null" ]
  tx_wait "$hash"
  tx_json=$(curl -s "$RPC0/tx?hash=0x$hash")
  pid=$(gov_extract_proposal_id "$tx_json")
  if [ -z "$pid" ] || [ "$pid" = "null" ]; then
    pid=$("$BIN" query gov proposals --node "$NODE0" -o json \
      | jq -r '.proposals[]?.id' | sort -n | tail -n1)
  fi
  echo "$pid"
}

tx_gov_vote_yes() {
  local pid="$1"
  local voter="$2"
  local voter_home="$3"
  local res hash
  res=$("$BIN" tx gov vote "$pid" yes \
    --from "$voter" \
    --chain-id "$CHAIN_ID" \
    --node "$NODE0" \
    --keyring-backend "$KEYRING" \
    --home "$voter_home" \
    --fees "$TX_FEES" \
    --yes \
    -o json)
  hash=$(echo "$res" | jq -r '.txhash // empty')
  [ -n "$hash" ] && [ "$hash" != "null" ]
  tx_wait "$hash"
}

gov_wait_passed() {
  local pid="$1"
  for _ in $(seq 1 180); do
    local status
    status=$("$BIN" query gov proposal "$pid" --node "$NODE0" -o json | jq -r '.proposal.status // .status // empty')
    if [ "$status" = "PROPOSAL_STATUS_PASSED" ]; then
      return 0
    fi
    if [ "$status" = "PROPOSAL_STATUS_REJECTED" ] || [ "$status" = "PROPOSAL_STATUS_FAILED" ]; then
      echo "proposal $pid ended with status=$status" >&2
      "$BIN" query gov proposal "$pid" --node "$NODE0" -o json | jq >&2 || true
      return 1
    fi
    sleep 0.5
  done
  echo "timeout waiting for proposal $pid to pass" >&2
  "$BIN" query gov proposal "$pid" --node "$NODE0" -o json | jq >&2 || true
  return 1
}

init_multinode() {
  step "Init 3-validator local testnet"
  rm -rf "$TESTNET_DIR"

  # Ensure validator1 can go offline while the chain continues (>2/3 power remains online).
  # validator0: 100, validator1: 5, validator2: 100 (units: ulmn)
  "$BIN" multi-node \
    --v 3 \
    --output-dir "$TESTNET_DIR" \
    --chain-id "$CHAIN_ID" \
    --validators-stake-amount "100000000,5000000,100000000" \
    --keyring-backend "$KEYRING" >/dev/null

  for h in "$HOME0" "$HOME1" "$HOME2"; do
    local g="$h/config/genesis.json"
    local tmp
    tmp=$(mktemp)
    jq '
      .app_state.gov.params.min_deposit = [{denom:"ulmn",amount:"10000000"}]
      | .app_state.gov.params.expedited_min_deposit = [{denom:"ulmn",amount:"50000000"}]
      | .app_state.gov.params.max_deposit_period = "8s"
      | .app_state.gov.params.voting_period = "8s"
      | .app_state.gov.params.expedited_voting_period = "4s"
      | .app_state.gov.params.min_initial_deposit_ratio = "0.000000000000000000"
      | .app_state.gov.deposit_params = {
          min_deposit:[{denom:"ulmn",amount:"10000000"}],
          max_deposit_period:"8s"
        }
      | .app_state.gov.voting_params = { voting_period:"8s" }
      | .app_state.gov.tally_params = {
          quorum:"0.670000000000000000",
          threshold:"0.750000000000000000",
          veto_threshold:"0.334000000000000000"
        }
    ' "$g" >"$tmp"
    mv "$tmp" "$g"
  done

  jq -r '.app_state.gov.params.voting_period' "$HOME0/config/genesis.json" | grep -qx '8s'
}

node_id() {
  local home="$1"
  "$BIN" comet show-node-id --home "$home" 2>/dev/null | tail -n1
}

start_nodes() {
  step "Start nodes"
  kill_nodes

  local id0 id1 id2 peers
  id0="$(node_id "$HOME0")"
  id1="$(node_id "$HOME1")"
  id2="$(node_id "$HOME2")"
  [ -n "$id0" ] && [ -n "$id1" ] && [ -n "$id2" ]
  peers="${id0}@${RPC_HOST}:${P2P0_PORT},${id1}@${RPC_HOST}:${P2P1_PORT},${id2}@${RPC_HOST}:${P2P2_PORT}"

  local args_common=(
    --p2p.persistent_peers "$peers"
    --grpc.enable=false
    --api.enable=false
    --minimum-gas-prices 0ulmn
  )
  if [ "$DISABLE_PPROF" = "1" ]; then
    args_common+=(--rpc.pprof_laddr "")
  fi

  "$BIN" start --home "$HOME0" --rpc.laddr "tcp://${RPC_HOST}:${RPC0_PORT}" --p2p.laddr "tcp://${RPC_HOST}:${P2P0_PORT}" "${args_common[@]}" >"$LOG0" 2>&1 &
  PID0=$!
  "$BIN" start --home "$HOME1" --rpc.laddr "tcp://${RPC_HOST}:${RPC1_PORT}" --p2p.laddr "tcp://${RPC_HOST}:${P2P1_PORT}" "${args_common[@]}" >"$LOG1" 2>&1 &
  PID1=$!
  "$BIN" start --home "$HOME2" --rpc.laddr "tcp://${RPC_HOST}:${RPC2_PORT}" --p2p.laddr "tcp://${RPC_HOST}:${P2P2_PORT}" "${args_common[@]}" >"$LOG2" 2>&1 &
  PID2=$!

  export RPC="$RPC0" NODE="$NODE0" HOME_DIR="$HOME0"
  wait_http "$RPC0/status"
  wait_height 1
}

setup_pqc_accounts() {
  step "Setup PQC-enabled validator accounts"
  export BIN
  export CHAIN_ID KEYRING TX_FEES
  export RPC="$RPC0" NODE="$NODE0"

  HOME_DIR="$HOME0"
  setup_pqc_signer validator0

  HOME_DIR="$HOME2"
  setup_pqc_signer validator2

  HOME_DIR="$HOME0"
}

gov_update_liveness_params() {
  step "Gov: update slashing liveness params"
  local authority msg proposal tmp pid
  authority=$("$BIN" query auth module-account gov --node "$NODE0" -o json \
    | jq -r '.account.base_account.address // .account.value.address // .account.address // .account.base_vesting_account.base_account.address // empty' \
    | tail -n1)
  [ -n "$authority" ] && [ "$authority" != "null" ]

  msg=$(jq -cn --arg auth "$authority" \
    '{"@type":"/lumen.tokenomics.v1.MsgUpdateSlashingLivenessParams","authority":$auth,"signed_blocks_window":"20","min_signed_per_window":"0.90"}')

  tmp=$(mktemp -t slashwin-prop-XXXXXX.json)
  jq -n --argjson m "$msg" '{
      messages: [$m],
      deposit: "10000000ulmn",
      title: "Slashing liveness params",
      summary: "Tighten signed blocks window for e2e",
      metadata: ""
    }' >"$tmp"

  pid="$(tx_submit_proposal "$tmp")"
  [ -n "$pid" ] && [ "$pid" != "null" ]
  tx_gov_vote_yes "$pid" validator0 "$HOME0"
  tx_gov_vote_yes "$pid" validator2 "$HOME2"
  gov_wait_passed "$pid"

  local after
  after=$("$BIN" query slashing params --node "$NODE0" -o json | jq '.params')
  echo "$after" | jq -r '.signed_blocks_window' | grep -qx '20'
  echo "$after" | jq -r '.min_signed_per_window' | grep -qx '0.900000000000000000'
}

assert_validator_jailed() {
  local valoper="$1"
  local jailed
  jailed=$("$BIN" query staking validator "$valoper" --node "$NODE0" -o json | jq -r '.validator.jailed // .jailed // "false"')
  if [ "$jailed" != "true" ]; then
    echo "expected validator $valoper to be jailed, got jailed=$jailed" >&2
    "$BIN" query staking validator "$valoper" --node "$NODE0" -o json | jq >&2 || true
    return 1
  fi
}

wait_validator_jailed() {
  local valoper="$1"
  local start_height="$2"
  local max_wait_blocks="${3:-40}"
  local deadline=$((start_height + max_wait_blocks))
  for _ in $(seq 1 240); do
    local h jailed
    h="$(latest_height)"
    jailed=$("$BIN" query staking validator "$valoper" --node "$NODE0" -o json | jq -r '.validator.jailed // .jailed // "false"')
    if [ "$jailed" = "true" ]; then
      return 0
    fi
    if [ -n "$h" ] && [ "$h" != "null" ] && [ "$h" -ge "$deadline" ] 2>/dev/null; then
      echo "validator $valoper not jailed by height $deadline (current=$h)" >&2
      "$BIN" query staking validator "$valoper" --node "$NODE0" -o json | jq >&2 || true
      return 1
    fi
    sleep 0.5
  done
  echo "timeout waiting for validator $valoper to be jailed" >&2
  "$BIN" query staking validator "$valoper" --node "$NODE0" -o json | jq >&2 || true
  return 1
}

test_liveness_window_affects_consensus() {
  step "Stop validator1; wait for slashing/jailing under new liveness window"
  local h0 target val1oper
  val1oper=$("$BIN" keys show validator1 --bech val -a --keyring-backend "$KEYRING" --home "$HOME1")
  [ -n "$val1oper" ] && [ "$val1oper" != "null" ]

  h0="$(latest_height)"
  [ -n "$h0" ] && [ "$h0" != "null" ]

  kill "$PID1" >/dev/null 2>&1 || true
  sleep 1

  # With window=20 and min=0.90 (max missed=2), validator1 should be jailed quickly.
  wait_validator_jailed "$val1oper" "$h0" 50
}

main() {
  build
  init_multinode
  start_nodes
  setup_pqc_accounts
  gov_update_liveness_params
  test_liveness_window_affects_consensus
  echo "e2e_slashing_window passed: liveness params applied + validator jailed after downtime"
}

main "$@"
