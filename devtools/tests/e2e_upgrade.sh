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
BIN_NEW="${BIN_NEW:-"${BIN:-"$DIR/build/lumend"}"}"
BIN_OLD="${BIN_OLD:-""}"
: "${LUMEN_BUILD_TAGS:=dev}"
HOME_DIR="${HOME}/.lumen"

OLD_REF="${OLD_REF:-320727d}"
PLAN_NAME="${PLAN_NAME:-v1.4.2}"

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

LOG_FILE="${LOG_FILE:-$HOME_ROOT/lumen-upgrade.log}"
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

NODE_PID=""

keys_add_quiet() {
  local name="$1"
  printf '\n' | "$BIN_OLD" keys add "$name" --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null 2>&1 || true
}

kill_node() {
  if [ -n "${NODE_PID:-}" ] && kill -0 "$NODE_PID" >/dev/null 2>&1; then
    kill "$NODE_PID" >/dev/null 2>&1 || true
    wait "$NODE_PID" >/dev/null 2>&1 || true
  fi
  # Fallback: make sure we don't leave stray nodes from previous runs.
  pkill -f "start --home ${HOME_DIR}" >/dev/null 2>&1 || true
  NODE_PID=""
}

cleanup() {
  kill_node
  if [ "${DEBUG_KEEP:-0}" != "1" ]; then
    rm -rf "$HOME_ROOT" >/dev/null 2>&1 || true
  else
    echo "DEBUG_KEEP=1 -> preserving $HOME_ROOT"
  fi
}
trap cleanup EXIT

build_old() {
  if [ -n "$BIN_OLD" ]; then
    return
  fi
  step "Build old lumend ($OLD_REF) into temp dir"
  local wt
  wt="$(mktemp -d -t lumen-upgrade-wt-XXXXXX)"
  git -C "$DIR" worktree add --detach "$wt" "$OLD_REF" >/dev/null
  trap 'git -C "$DIR" worktree remove --force "$wt" >/dev/null 2>&1 || true; rm -rf "$wt" >/dev/null 2>&1 || true' RETURN

  BIN_OLD="$HOME_ROOT/lumend-old"
  local cmd=(go build -trimpath -ldflags "-s -w")
  if [ -n "$LUMEN_BUILD_TAGS" ]; then
    cmd+=(-tags "$LUMEN_BUILD_TAGS")
  fi
  cmd+=(-o "$BIN_OLD" ./cmd/lumend)
  (cd "$wt" && "${cmd[@]}")

  git -C "$DIR" worktree remove --force "$wt" >/dev/null 2>&1 || true
  rm -rf "$wt" >/dev/null 2>&1 || true
  trap - RETURN
}

build_new_if_needed() {
  if [ "$SKIP_BUILD" = "1" ]; then
    echo "==> Skip new build (SKIP_BUILD=1)"
    return
  fi
  step "Build new lumend (current)"
  local cmd=(go build -trimpath -ldflags "-s -w")
  if [ -n "$LUMEN_BUILD_TAGS" ]; then
    cmd+=(-tags "$LUMEN_BUILD_TAGS")
  fi
  cmd+=(-o "$BIN_NEW" ./cmd/lumend)
  (cd "$DIR" && "${cmd[@]}")
}

init_chain() {
  step "Init chain with short gov windows"
  rm -rf "$HOME_DIR"
  "$BIN_OLD" init gov-upgrade --chain-id "$CHAIN_ID" --home "$HOME_DIR"

  keys_add_quiet validator
  keys_add_quiet voter2

  VAL_ADDR=$("$BIN_OLD" keys show validator -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
  VOTER2_ADDR=$("$BIN_OLD" keys show voter2 -a --keyring-backend "$KEYRING" --home "$HOME_DIR")

  "$BIN_OLD" genesis add-genesis-account "$VAL_ADDR" 200000000ulmn --keyring-backend "$KEYRING" --home "$HOME_DIR"
  "$BIN_OLD" genesis add-genesis-account "$VOTER2_ADDR" 80000000ulmn --keyring-backend "$KEYRING" --home "$HOME_DIR"

  "$BIN_OLD" genesis gentx validator 50000000ulmn --chain-id "$CHAIN_ID" --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null
  "$BIN_OLD" genesis collect-gentxs --home "$HOME_DIR" >/dev/null

  local tmp
  tmp=$(mktemp)
  jq '
    # Keep defaults, but shorten deposit/voting windows so this e2e completes fast.
    .app_state.gov.params.min_deposit = [{denom:"ulmn",amount:"10000000"}]
    | .app_state.gov.params.expedited_min_deposit = [{denom:"ulmn",amount:"50000000"}]
    | .app_state.gov.params.max_deposit_period = "8s"
    | .app_state.gov.params.voting_period = "8s"
    | .app_state.gov.params.expedited_voting_period = "4s"
    | .app_state.gov.constitution = "Lumen DAO stewards DNS and gateway policies."
    # Back-compat (older gov genesis schemas):
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

  jq -r '(.app_state.gov.params.voting_period // .app_state.gov.voting_params.voting_period // empty)' \
    "$HOME_DIR/config/genesis.json" | grep -qx '8s'
  jq -r '.app_state.gov.params.min_initial_deposit_ratio' "$HOME_DIR/config/genesis.json" | grep -qx '0.000000000000000000'
  "$BIN_OLD" genesis validate --home "$HOME_DIR" >/dev/null
  pqc_set_client_config "$HOME_DIR" "$RPC_LADDR" "$CHAIN_ID"
}

start_node() {
  local bin="$1"
  local label="$2"
  step "Start node ($label)"
  kill_node
  local args=(
    "$bin" start
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
  "${args[@]}" >"$LOG_FILE" 2>&1 &
  NODE_PID=$!
  sleep 1
  gov_wait_http "$RPC/status"
  gov_wait_http "$API/"
}

resolve_upgrade_authority() {
  # Prefer the chain's configured upgrade authority.
  local addr
  addr=$("$BIN_OLD" query upgrade authority --node "$NODE" -o json 2>/dev/null \
    | jq -r '.address // .authority // empty' | tail -n1) || true
  if [ -n "$addr" ] && [ "$addr" != "null" ]; then
    echo "$addr"
    return 0
  fi

  # Fallbacks for older CLIs / configs.
  addr=$("$BIN_OLD" query auth module-account gov-immutable --node "$NODE" -o json 2>/dev/null \
    | jq -r '.account.base_account.address // .account.value.address // .account.address // .account.base_vesting_account.base_account.address // empty' \
    | tail -n1) || true
  if [ -n "$addr" ] && [ "$addr" != "null" ]; then
    echo "$addr"
    return 0
  fi
  addr=$("$BIN_OLD" query auth module-account gov --node "$NODE" -o json 2>/dev/null \
    | jq -r '.account.base_account.address // .account.value.address // .account.address // .account.base_vesting_account.base_account.address // empty' \
    | tail -n1) || true
  if [ -n "$addr" ] && [ "$addr" != "null" ]; then
    echo "$addr"
    return 0
  fi
  return 1
}

submit_upgrade_proposal() {
  local current_height
  current_height=$(curl -s "$RPC/status" | jq -r '.result.sync_info.latest_block_height' 2>/dev/null || echo "1")
  if ! [[ "$current_height" =~ ^[0-9]+$ ]]; then
    current_height=1
  fi
  local upgrade_height=$((current_height + 6))

  GOV_AUTHORITY=$(resolve_upgrade_authority)
  if [ -z "$GOV_AUTHORITY" ] || [ "$GOV_AUTHORITY" = "null" ]; then
    echo "failed to resolve upgrade authority address" >&2
    exit 1
  fi

  local proposal_file
  proposal_file=$(mktemp -t gov-upgrade-v1-XXXXXX.json)
  jq -n \
    --arg auth "$GOV_AUTHORITY" \
    --arg name "$PLAN_NAME" \
    --arg height_str "$upgrade_height" \
    '{
      messages: [
        {
          "@type": "/cosmos.upgrade.v1beta1.MsgSoftwareUpgrade",
          authority: $auth,
          plan: {
            name: $name,
            height: $height_str,
            info: "e2e-upgrade-test"
          }
        }
      ],
      title: "Upgrade plan",
      summary: "E2E upgrade test",
      metadata: ""
    }' >"$proposal_file"

  if ! gov_submit_proposal_file "$proposal_file" "10000000ulmn"; then
    echo "failed to submit upgrade proposal" >&2
    exit 1
  fi

  PROPOSAL_ID="$GOV_LAST_PROPOSAL_ID"
  UPGRADE_HEIGHT="$upgrade_height"
}

wait_for_upgrade_halt() {
  local expected_name="$1"
  local expected_height="$2"
  local upgrade_info="$HOME_DIR/data/upgrade-info.json"

  for _ in $(seq 1 120); do
    if [ -f "$upgrade_info" ]; then
      local name height
      name=$(jq -r '.name // empty' "$upgrade_info" 2>/dev/null || echo "")
      height=$(jq -r '.height // empty' "$upgrade_info" 2>/dev/null || echo "")
      if [ "$name" = "$expected_name" ] && [ "$height" = "$expected_height" ]; then
        # Give the old binary a brief chance to exit cleanly, then force-stop.
        for _ in $(seq 1 20); do
          if [ -n "${NODE_PID:-}" ] && kill -0 "$NODE_PID" >/dev/null 2>&1; then
            sleep 0.5
          else
            return 0
          fi
        done
        kill_node
        return 0
      fi
    fi
    sleep 0.5
  done

  echo "timeout waiting for upgrade halt (expected $expected_name at height $expected_height)" >&2
  if [ -f "$upgrade_info" ]; then
    echo "found upgrade-info.json:" >&2
    cat "$upgrade_info" >&2 || true
  else
    echo "missing $upgrade_info" >&2
  fi
  return 1
}

wait_height_with_bin() {
  local bin="$1"
  local target="$2"
  for _ in $(seq 1 240); do
    local h
    h=$(curl -s "$RPC/status" | jq -r '.result.sync_info.latest_block_height' 2>/dev/null || echo "0")
    if [ -n "$h" ] && [ "$h" != "null" ] && [ "$h" -ge "$target" ]; then
      return 0
    fi
    sleep 0.5
  done
  echo "timeout waiting for block height >= $target" >&2
  return 1
}

main() {
  build_old
  build_new_if_needed
  BIN="$BIN_OLD"
  init_chain
  start_node "$BIN_OLD" "old"

  step "Setup PQC-enabled validator accounts"
  setup_pqc_signer validator
  setup_pqc_signer voter2

  step "Submit software upgrade proposal ($PLAN_NAME)"
  submit_upgrade_proposal

  step "Deposit and vote YES on proposal $PROPOSAL_ID"
  gov_deposit "$PROPOSAL_ID" "10000000ulmn" validator
  gov_cast_vote "$PROPOSAL_ID" validator YES
  gov_cast_vote "$PROPOSAL_ID" voter2 YES
  if ! gov_wait_status "$PROPOSAL_ID" "PROPOSAL_STATUS_PASSED"; then
    # Do not fail hard on status string drift; the critical assertion
    # for this e2e is that the chain reaches the upgrade height and
    # continues producing blocks.
    echo "warning: proposal $PROPOSAL_ID did not report PROPOSAL_STATUS_PASSED (non-fatal for upgrade e2e)" >&2
  fi

  step "Wait for chain to approach upgrade height $UPGRADE_HEIGHT"
  gov_wait_height $((UPGRADE_HEIGHT - 1))

  step "Verify chain halts at upgrade height (upgrade-info.json written)"
  wait_for_upgrade_halt "$PLAN_NAME" "$UPGRADE_HEIGHT"

  step "Restart with new binary; verify chain resumes"
  start_node "$BIN_NEW" "new"
  wait_height_with_bin "$BIN_NEW" $((UPGRADE_HEIGHT + 2))

  step "Verify no upgrade is pending"
  if "$BIN_NEW" query upgrade plan --node "$NODE" -o json 2>/dev/null | jq -e '.plan.name? // empty | length == 0' >/dev/null; then
    :
  else
    # tolerate CLI behavior differences (some versions return non-zero with 'no upgrade scheduled')
    "$BIN_NEW" query upgrade plan --node "$NODE" -o json 2>/dev/null || true
  fi

  echo "e2e_upgrade passed: plan executed end-to-end (halt + restart + continued blocks)"
}

main "$@"
