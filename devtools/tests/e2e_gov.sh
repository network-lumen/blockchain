#!/usr/bin/env bash
set -euo pipefail

SOURCE_DIR="$(cd "$(dirname "$0")" && pwd)"
. "${SOURCE_DIR}/lib_pqc.sh"
. "${SOURCE_DIR}/lib_gov.sh"

pqc_require_bins
gov_require_bins

HOME_ROOT=$(mktemp -d -t lumen-e2e-gov-XXXXXX)
export HOME="$HOME_ROOT"

DIR=$(cd "${SOURCE_DIR}/../.." && pwd)
BIN="$DIR/build/lumend"
: "${LUMEN_BUILD_TAGS:=dev}"
HOME_DIR="${HOME}/.lumen"

# randomised base to avoid port collisions when running standalone
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

LOG_FILE="${LOG_FILE:-/tmp/lumen-gov.log}"
CHAIN_ID="${CHAIN_ID:-lumen-gov-1}"
KEYRING=${KEYRING:-test}
TX_FEES=${TX_FEES:-0ulmn}
NODE=${NODE:-$RPC_LADDR}
CASE_FILE="${CASE_FILE:-${SOURCE_DIR}/gov_param_cases.json}"
CASE_FILTER="${CASE_FILTER:-}"
GOV_ALLOW_PARAM_UPDATES="${GOV_ALLOW_PARAM_UPDATES:-0}"
DISABLE_PPROF="${DISABLE_PPROF:-1}"

GOV_DIRTY=0
DNS_DIRTY=0

keys_add_quiet() {
  local name="$1"
  printf '\n' | "$BIN" keys add "$name" --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null 2>&1 || true
}

step() { printf '\n==== %s\n' "$*"; }

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
  if [ "${SKIP_BUILD:-0}" = "1" ]; then
    return
  fi
  step "Build lumend"
  local cmd=(go build)
  if [ -n "$LUMEN_BUILD_TAGS" ]; then
    cmd+=(-tags "$LUMEN_BUILD_TAGS")
  fi
  cmd+=(-o "$BIN" ./cmd/lumend)
  (cd "$DIR" && "${cmd[@]}")
}

init_chain() {
  step "Init chain"
  rm -rf "$HOME_DIR"
  "$BIN" init gov-local --chain-id "$CHAIN_ID" --home "$HOME_DIR"

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
  if [ "${DISABLE_PPROF:-0}" = "1" ]; then
    args+=(--rpc.pprof_laddr "")
  fi
  ("${args[@]}" >"$LOG_FILE" 2>&1 &)
  sleep 1
  gov_wait_http "$RPC/status"
  gov_wait_http "$API/"
}

resolve_placeholder() {
  local value="$1"
  case "$value" in
    ""|"null") echo "$value" ;;
    "\$VALIDATOR"|"\$VAL_ADDR") echo "$VAL_ADDR" ;;
    "\$VOTER2"|"\$VOTER2_ADDR") echo "$VOTER2_ADDR" ;;
    "\$FEE_COLLECTOR") echo "$FEE_COLLECTOR_ADDR" ;;
    "\$GOV_AUTHORITY") echo "$GOV_AUTHORITY" ;;
    *) echo "$value" ;;
  esac
}

build_messages_from_spec() {
  local spec_json="$1"
  if [ -z "$spec_json" ] || [ "$spec_json" = "null" ]; then
    spec_json='[{"type":"dns_update_params","changes":{}}]'
  fi
  local acc='[]' dns_state
  dns_state=$(gov_query_dns_params)
  while IFS= read -r entry; do
    local type msg changes override
    type=$(echo "$entry" | jq -r '.type')
    case "$type" in
      dns_update_params)
        changes=$(echo "$entry" | jq -c '.changes // {}')
        override=$(echo "$entry" | jq -r '.authority_override // empty')
        override=$(resolve_placeholder "$override")
        if [ -z "$changes" ] || [ "$changes" = "null" ]; then
          changes="{}"
        fi
        dns_state=$(echo "$dns_state" | jq --argjson patch "$changes" 'reduce ($patch | keys_unsorted[]) as $k (. ; .[$k] = $patch[$k])')
        local authority="$GOV_AUTHORITY"
        if [ -n "$override" ]; then
          authority="$override"
        fi
        msg=$(jq -cn --arg auth "$authority" --argjson params "$dns_state" '{"@type":"/lumen.dns.v1.MsgUpdateParams", authority:$auth, params:$params}')
        acc=$(jq -n --argjson arr "$acc" --argjson msg "$msg" '$arr + [$msg]')
        ;;
      *)
        echo "unsupported message type: $type" >&2
        exit 1
        ;;
    esac
  done < <(echo "$spec_json" | jq -c '.[]')
  echo "$acc"
}

prepare_proposal_from_entry() {
  local entry="$1"
  local title summary metadata messages_spec expedited
  title=$(echo "$entry" | jq -r '.proposal.title // "Governance proposal"')
  summary=$(echo "$entry" | jq -r '.proposal.summary // .description // "autogenerated"')
  metadata=$(echo "$entry" | jq -r '.proposal.metadata // ""')
  messages_spec=$(echo "$entry" | jq -c '.proposal.messages // []')
  expedited=$(echo "$entry" | jq -r '.proposal.expedited // false')
  local messages_json
  messages_json=$(build_messages_from_spec "$messages_spec")
  local file
  file=$(mktemp -t gov-proposal-XXXXXX)
  jq -n --arg title "$title" --arg summary "$summary" --arg metadata "$metadata" --argjson messages "$messages_json" \
    '{messages:$messages, title:$title, summary:$summary, metadata:$metadata}' >"$file"
  PROPOSAL_FILE="$file"
  PROPOSAL_EXPEDITED=$([ "$expedited" = "true" ] && echo 1 || echo 0)
}

normalize_param_value() {
  local type="$1" value="$2"
  case "$type" in
    ratio|string|address|uint) echo "$value" ;;
    duration)
      gov_canonical_duration "$value"
      ;;
    bool) echo "$value" ;;
    coins)
      echo "$value" | jq -c '[ .[] | capture("(?<amount>[0-9]+)(?<denom>[a-zA-Z0-9/]+)") | {denom: .denom, amount: .amount} ]'
      ;;
    json) echo "$value" ;;
    *) echo "$value" ;;
  esac
}

apply_requires() {
  local entry="$1"
  local requires
  requires=$(echo "$entry" | jq -c '.requires // []')
  if [ "$(echo "$requires" | jq 'length')" -eq 0 ]; then
    return
  fi
  local updated="$GOV_BASE_PARAMS"
  while IFS= read -r req; do
    local name type raw
    name=$(echo "$req" | jq -r '.name // .param')
    type=$(echo "$req" | jq -r '.value_type // "string"')
    if [[ "$type" == "bool" || "$type" == "json" || "$type" == "coins" || "$type" == "uint" ]]; then
      raw=$(echo "$req" | jq '.value')
    else
      raw=$(echo "$req" | jq -r '.value')
      raw=$(resolve_placeholder "$raw")
    fi
    updated=$(gov_override_param "$updated" "$name" "$type" "$raw")
  done < <(echo "$requires" | jq -c '.[]')
  if gov_apply_params "$updated" "Apply scenario prerequisites" "requires"; then
    CURRENT_GOV_PARAMS="$updated"
  else
    echo "failed to apply prereq params" >&2
    exit 1
  fi
}

restore_base_params() {
  if [ "$CURRENT_GOV_PARAMS" != "$GOV_BASE_PARAMS" ]; then
    gov_apply_params "$GOV_BASE_PARAMS" "Restore gov params" "restore"
  fi
  CURRENT_GOV_PARAMS="$GOV_BASE_PARAMS"
}

dns_mark_dirty() { DNS_DIRTY=1; }

dns_apply_params() {
  local params_json="$1"
  local title="${2:-"Restore DNS params"}"
  local summary="${3:-"restore-dns"}"
  local msg file pid
  msg=$(jq -cn --arg auth "$GOV_AUTHORITY" --argjson params "$params_json" '{"@type":"/lumen.dns.v1.MsgUpdateParams", authority:$auth, params:$params}')
  file=$(mktemp -t gov-dns-XXXXXX)
  jq -n --arg title "$title" --arg summary "$summary" --arg metadata "" --argjson msg "$msg" \
    '{messages:[$msg], title:$title, summary:$summary, metadata:$metadata}' >"$file"
  gov_submit_proposal_file "$file" "10000000ulmn" || return 1
  pid="$GOV_LAST_PROPOSAL_ID"
  gov_cast_vote "$pid" validator yes
  gov_wait_status "$pid" "PROPOSAL_STATUS_PASSED"
  GOV_LAST_PROPOSAL_ID="$pid"
}

dns_restore_if_dirty() {
  if [ "${DNS_DIRTY:-0}" = "1" ]; then
    dns_apply_params "$DNS_BASE_PARAMS" "Restore DNS params" "restore-dns"
    DNS_DIRTY=0
  fi
}

submit_case_proposal() {
  local entry="$1"
  prepare_proposal_from_entry "$entry"
  local deposit
  deposit=$(echo "$entry" | jq -r '.proposal.initial_deposit // "10000000ulmn"')
  gov_submit_proposal_file "$PROPOSAL_FILE" "$deposit" "$PROPOSAL_EXPEDITED"
}

coin_amount() {
  python3 - "$1" <<'PY'
import re, sys
coin = sys.argv[1]
m = re.match(r'^(\d+)', coin)
if not m:
    raise SystemExit("invalid coin literal: %s" % coin)
print(m.group(1))
PY
}

ratio_mul_coin() {
  python3 - "$1" "$2" <<'PY'
from decimal import Decimal, ROUND_FLOOR, getcontext
import sys
getcontext().prec = 50
amount = Decimal(sys.argv[1])
ratio = Decimal(sys.argv[2])
print(int((amount * ratio).to_integral_value(rounding=ROUND_FLOOR)))
PY
}

wait_for_status_or_sleep() {
  local pid="$1" target="$2" sleep_s="${3:-0}"
  if [ "$sleep_s" -gt 0 ]; then
    sleep "$sleep_s"
  fi
  gov_wait_status "$pid" "$target"
}

scenario_deposit_topup() {
  local entry="$1"
  apply_requires "$entry"
  local before_validator before_voter2
  before_validator=$(gov_balance_ulmn "$VAL_ADDR")
  before_voter2=$(gov_balance_ulmn "$VOTER2_ADDR")
  submit_case_proposal "$entry"
  local pid="$GOV_LAST_PROPOSAL_ID"
  gov_wait_status "$pid" "PROPOSAL_STATUS_DEPOSIT_PERIOD"
  local top_amount top_from
  top_amount=$(echo "$entry" | jq -r '.topup.amount')
  top_from=$(echo "$entry" | jq -r '.topup.from')
  gov_deposit "$pid" "$top_amount" "$top_from"
  gov_wait_status "$pid" "PROPOSAL_STATUS_VOTING_PERIOD"
  gov_cast_vote "$pid" validator yes
  gov_wait_status "$pid" "PROPOSAL_STATUS_PASSED"
  dns_mark_dirty
  local after_validator after_voter2
  after_validator=$(gov_balance_ulmn "$VAL_ADDR")
  after_voter2=$(gov_balance_ulmn "$VOTER2_ADDR")
  gov_expect_balance_delta "$before_validator" "$after_validator" ">=" "-100000" "validator deposit refund"
  gov_expect_balance_delta "$before_voter2" "$after_voter2" ">=" "-100000" "topup deposit refund"
  restore_base_params
  dns_restore_if_dirty
}

scenario_deposit_expiry() {
  local entry="$1"
  apply_requires "$entry"
  submit_case_proposal "$entry"
  local pid="$GOV_LAST_PROPOSAL_ID"
  local sleep_seconds
  sleep_seconds=$(echo "$entry" | jq -r '.sleep_seconds // 12')
  if wait_for_status_or_sleep "$pid" "PROPOSAL_STATUS_FAILED" "$sleep_seconds"; then
    :
  else
    local rc=$?
    if [ "$rc" -eq 2 ]; then
      echo "→ proposal $pid pruned after deposit expiry (expected)"
    else
      echo "deposit expiry proposal $pid still queryable after timeout" >&2
      exit 1
    fi
  fi
  restore_base_params
}

scenario_initial_deposit_guard() {
  local entry="$1"
  apply_requires "$entry"
  if submit_case_proposal "$entry"; then
    echo "expected insufficient initial deposit to fail" >&2
    exit 1
  fi
  gov_expect_tx_failure "minimum deposit"
  restore_base_params
}

scenario_quorum_burn() {
  local entry="$1"
  apply_requires "$entry"
  local before
  before=$(gov_balance_ulmn "$VAL_ADDR")
  submit_case_proposal "$entry"
  local pid="$GOV_LAST_PROPOSAL_ID"
  local sleep_seconds
  sleep_seconds=$(echo "$entry" | jq -r '.sleep_seconds // 12')
  wait_for_status_or_sleep "$pid" "PROPOSAL_STATUS_REJECTED" "$sleep_seconds"
  local after
  after=$(gov_balance_ulmn "$VAL_ADDR")
  gov_expect_balance_delta "$before" "$after" "<=" "-8000000" "validator quorum burn"
  restore_base_params
}

scenario_tally_veto() {
  local entry="$1"
  apply_requires "$entry"
  local before
  before=$(gov_balance_ulmn "$VAL_ADDR")
  submit_case_proposal "$entry"
  local pid="$GOV_LAST_PROPOSAL_ID"
  gov_cast_vote "$pid" validator NO_WITH_VETO
  gov_cast_vote "$pid" voter2 YES
  gov_wait_status "$pid" "PROPOSAL_STATUS_REJECTED"
  local after
  after=$(gov_balance_ulmn "$VAL_ADDR")
  gov_expect_balance_delta "$before" "$after" "<=" "-8000000" "validator veto burn"
  restore_base_params
}

scenario_expedited() {
  local entry="$1"
  apply_requires "$entry"
  submit_case_proposal "$entry"
  local pid="$GOV_LAST_PROPOSAL_ID"
  gov_cast_vote "$pid" validator YES
  gov_wait_status "$pid" "PROPOSAL_STATUS_PASSED"
  local voting_start voting_end
  voting_start=$("$BIN" query gov proposal "$pid" --node "$NODE" -o json | jq -r '.proposal.voting_start_time')
  voting_end=$("$BIN" query gov proposal "$pid" --node "$NODE" -o json | jq -r '.proposal.voting_end_time')
  local voting_window
  voting_window=$(python3 - "$voting_start" "$voting_end" <<'PY'
from datetime import datetime
import sys

def normalize(ts: str) -> str:
    if ts.endswith('Z'):
        ts = ts[:-1] + '+00:00'
    tz_idx = None
    for idx in range(19, len(ts)):
        if ts[idx] in '+-':
            tz_idx = idx
            break
    if tz_idx is not None:
        main, tz = ts[:tz_idx], ts[tz_idx:]
    else:
        main, tz = ts, ''
    if '.' in main:
        head, frac = main.split('.', 1)
        frac = (frac + '000000')[:6]
        main = f"{head}.{frac}"
    return main + tz

start = datetime.fromisoformat(normalize(sys.argv[1]))
end = datetime.fromisoformat(normalize(sys.argv[2]))
delta = int((end - start).total_seconds())
print(delta)
PY
 )
  if [ "$voting_window" -gt 6 ]; then
    echo "expedited proposal voting window too long (${voting_window}s)" >&2
    exit 1
  fi
  dns_mark_dirty
  restore_base_params
  dns_restore_if_dirty
}

scenario_cancel() {
  local entry="$1"
  apply_requires "$entry"
  local before_validator before_dest deposit ratio
  before_validator=$(gov_balance_ulmn "$VAL_ADDR")
  before_dest=$(gov_balance_ulmn "$VOTER2_ADDR")
  deposit=$(echo "$entry" | jq -r '.proposal.initial_deposit // "10000000ulmn"')
  ratio=$(gov_query_params | jq -r '.proposal_cancel_ratio')
  submit_case_proposal "$entry"
  local pid="$GOV_LAST_PROPOSAL_ID"
  local res hash
  res=$("$BIN" tx gov cancel-proposal "$pid" \
    --from validator \
    --chain-id "$CHAIN_ID" \
    --node "$NODE" \
    --keyring-backend "$KEYRING" \
    --home "$HOME_DIR" \
    --fees "$TX_FEES" \
    --yes -o json)
  hash=$(echo "$res" | jq -r '.txhash // empty')
  if [ -n "$hash" ] && [ "$hash" != "null" ]; then
    gov_wait_tx "$hash" >/dev/null || true
  fi
  if wait_for_status_or_sleep "$pid" "PROPOSAL_STATUS_CANCELLED"; then
    :
  else
    local rc=$?
    if [ "$rc" -ne 2 ]; then
      echo "proposal $pid did not cancel cleanly" >&2
      exit 1
    fi
  fi
  local after_validator after_dest
  after_validator=$(gov_balance_ulmn "$VAL_ADDR")
  after_dest=$(gov_balance_ulmn "$VOTER2_ADDR")
  local deposit_amount share
  deposit_amount=$(coin_amount "$deposit")
  share=$(ratio_mul_coin "$deposit_amount" "$ratio")
  gov_expect_balance_delta "$before_dest" "$after_dest" ">=" "$share" "cancel dest credit"
  gov_expect_balance_delta "$before_validator" "$after_validator" "<=" "-$share" "validator cancel payout"
  restore_base_params
}

scenario_authority_invalid() {
  local entry="$1"
  prepare_proposal_from_entry "$entry"
  if gov_submit_proposal_file "$PROPOSAL_FILE" "10000000ulmn" "$PROPOSAL_EXPEDITED"; then
    echo "authority invalid case unexpectedly succeeded" >&2
    exit 1
  fi
  gov_expect_tx_failure "expected gov account"
  restore_base_params
}

scenario_multi_message_atomic() {
  local entry="$1"
  local before_fee
  before_fee=$(gov_query_dns_params | jq -c '.update_fee_ulmn')
  submit_case_proposal "$entry"
  local pid="$GOV_LAST_PROPOSAL_ID"
  gov_cast_vote "$pid" validator YES
  gov_wait_status "$pid" "PROPOSAL_STATUS_FAILED"
  local after_fee
  after_fee=$(gov_query_dns_params | jq -c '.update_fee_ulmn')
  if [ "$before_fee" != "$after_fee" ]; then
    echo "dns params changed despite failed batch" >&2
    exit 1
  fi
  restore_base_params
}

scenario_multi_message_success() {
  local entry="$1"
  dns_mark_dirty
  submit_case_proposal "$entry"
  local pid="$GOV_LAST_PROPOSAL_ID"
  gov_cast_vote "$pid" validator YES
  gov_wait_status "$pid" "PROPOSAL_STATUS_PASSED"
  local expected_params
  expected_params=$(echo "$entry" | jq -c '.expect.params // {}')
  if [ "$expected_params" != "{}" ]; then
    local dns_params
    dns_params=$(gov_query_dns_params)
    while IFS= read -r key; do
      local expected actual
      expected=$(echo "$expected_params" | jq -r --arg k "$key" '.[$k]')
      actual=$(echo "$dns_params" | jq -r --arg k "$key" '.[$k]')
      if [ "$actual" != "$expected" ]; then
        echo "multi message success expected $key=$expected got $actual" >&2
        exit 1
      fi
    done < <(echo "$expected_params" | jq -r 'keys[]')
  fi
  restore_base_params
  dns_restore_if_dirty
}

register_domain_if_needed() {
  local label="$1" ext="$2" owner_key="$3" owner_addr="$4"
  if "$BIN" query dns domain "$label" "$ext" --node "$NODE" -o json >/dev/null 2>&1; then
    return 0
  fi
  local tx hash code
  tx=$("$BIN" tx dns register "$label" "$ext" \
    --records '[]' \
    --duration-days 30 \
    --owner "$owner_addr" \
    --from "$owner_key" \
    --node "$NODE" \
    --keyring-backend "$KEYRING" \
    --home "$HOME_DIR" \
    --chain-id "$CHAIN_ID" \
    --fees "$TX_FEES" \
    -y -o json)
  hash=$(echo "$tx" | jq -r '.txhash // empty')
  if [ -n "$hash" ] && [ "$hash" != "null" ]; then
    code=$(gov_wait_tx "$hash" || true)
    if [ -z "$code" ] || [ "$code" != "0" ]; then
      echo "dns register $label.$ext failed (code=${code:-?})" >&2
      curl -s "$RPC/tx?hash=0x$hash" | tee /tmp/gov_dns_register_failed.json >&2 || true
      exit 1
    fi
  fi
}

dns_pow_nonce() {
  python3 - "$1" "$2" "$3" <<'PY'
import hashlib, sys
identifier, creator, difficulty = sys.argv[1], sys.argv[2], int(sys.argv[3])
nonce = 0
while True:
    payload = f"{identifier}|{creator}|{nonce}".encode()
    digest = hashlib.sha256(payload).digest()
    bits = 0
    for byte in digest:
        if byte == 0:
            bits += 8
            continue
        for i in range(7, -1, -1):
            if (byte >> i) & 1 == 0:
                bits += 1
            else:
                break
        break
    if bits >= difficulty:
        print(nonce)
        sys.exit(0)
    nonce += 1
PY
}

community_pool_amount_ulmn() {
  "$BIN" q distribution community-pool --node "$NODE" -o json 2>/dev/null | jq -r '
    def parse_coin($v):
      if ($v | type) == "object" then
        { denom: ($v.denom // ""), amount: ($v.amount // "0") }
      elif ($v | type) == "string" then
        ($v | capture("(?<amount>[-0-9.]+)(?<denom>[a-zA-Z0-9/]+)") // {denom:"", amount:$v})
      else
        { denom: "", amount: "0" }
      end;
    def pull_amount($x):
      if ($x | type) == "array" then
        ([ $x[]? | parse_coin(.) | select(.denom=="ulmn") | .amount ] | first? // "0")
      elif ($x | type) == "object" then
        (parse_coin($x) | select(.denom=="ulmn") | .amount // "0")
      elif ($x | type) == "string" then
        (parse_coin($x) | select(.denom=="ulmn") | .amount // "0")
      else
        "0"
      end;
    pull_amount(.pool)
  '
}

scenario_dns_fee_effect() {
  local entry="$1"
  apply_requires "$entry"
  submit_case_proposal "$entry"
  local pid="$GOV_LAST_PROPOSAL_ID"
  gov_cast_vote "$pid" validator YES
  gov_wait_status "$pid" "PROPOSAL_STATUS_PASSED"
  dns_mark_dirty

  local domain ext fqdn records_json to_addr
  domain=$(echo "$entry" | jq -r '.post_tx.dns_transfer.domain // empty')
  if [ -z "$domain" ] || [ "$domain" = "null" ]; then
    domain="govcase$(date +%s)"
  fi
  ext=$(echo "$entry" | jq -r '.post_tx.dns_transfer.ext // "test"')
  fqdn="${domain}.${ext}"
  records_json=$(echo "$entry" | jq -c '.post_tx.dns_transfer.records // [{"key":"txt","value":"governance"}]')
  register_domain_if_needed "$domain" "$ext" validator "$VAL_ADDR"
  to_addr=$(echo "$entry" | jq -r '.post_tx.dns_transfer.to // "$VOTER2"')
  to_addr=$(resolve_placeholder "$to_addr")

  local tx hash
  tx=$("$BIN" tx dns transfer "$domain" "$ext" "$to_addr" \
    --from validator \
    --node "$NODE" \
    --keyring-backend "$KEYRING" \
    --home "$HOME_DIR" \
    --chain-id "$CHAIN_ID" \
    --fees "$TX_FEES" \
    -y -o json)
  hash=$(echo "$tx" | jq -r '.txhash // empty')
  if [ -z "$hash" ] || [ "$hash" = "null" ]; then
    echo "dns transfer tx missing hash" >&2
    exit 1
  fi
  gov_wait_tx "$hash" >/dev/null || true
  local tx_json expected fee_attr
  tx_json=$(curl -s "$RPC/tx?hash=0x$hash")
  echo "$tx_json" > /tmp/dns_update_fee_tx.json
  expected=$(echo "$entry" | jq -r '.post_tx.expect_fee_ulmn')
  fee_attr=$(echo "$tx_json" | jq -r '.result.tx_result.events[]? | select(.type=="dns_transfer") | .attributes[]? | select(.key=="fee_ulmn") | .value' | tail -n1)
  if [ "$fee_attr" != "$expected" ]; then
    echo "expected dns_transfer fee $expected got $fee_attr" >&2
    exit 1
  fi
  local owner_spent collector_recv
  owner_spent=$(echo "$tx_json" | jq -r --arg addr "$VAL_ADDR" '
    [ .result.tx_result.events[]? 
      | select(.type=="coin_spent") 
      | select(any(.attributes[]?; .key=="spender" and .value==$addr))
      | .attributes[]? 
      | select(.key=="amount") 
      | .value 
    ] 
    | map(select(endswith("ulmn"))) 
    | map(sub("ulmn$";"")) 
    | map(tonumber) 
    | add // 0
  ')
  if [ "$owner_spent" != "$expected" ]; then
    echo "validator spent $owner_spent ulmn but expected $expected during dns transfer" >&2
    exit 1
  fi
  restore_base_params
  dns_restore_if_dirty
}

scenario_pqc_disabled() {
  local entry="$1"
  prepare_proposal_from_entry "$entry"
  local deposit tmp
  deposit=$(echo "$entry" | jq -r '.proposal.initial_deposit // "10000000ulmn"')
  tmp=$(mktemp -t gov-pqc-XXXXXX)
  jq --arg deposit "$deposit" '.deposit = $deposit' "$PROPOSAL_FILE" >"$tmp"
  if "$BIN" tx gov submit-proposal "$tmp" \
      --from validator \
      --chain-id "$CHAIN_ID" \
      --node "$NODE" \
      --keyring-backend "$KEYRING" \
      --home "$HOME_DIR" \
      --gas 400000 \
      --fees "$TX_FEES" \
      --pqc-enable=false \
      --yes -o json >/tmp/gov_pqc_disabled.log 2>&1; then
    echo "PQC-disabled submission unexpectedly succeeded" >&2
    exit 1
  fi
  if ! grep -qi "pqc" /tmp/gov_pqc_disabled.log; then
    echo "PQC disabled error did not mention pqc" >&2
    exit 1
  fi
  restore_base_params
}

scenario_weighted_split() {
  local entry="$1"
  apply_requires "$entry"
  local before
  before=$(gov_balance_ulmn "$VAL_ADDR")
  submit_case_proposal "$entry"
  local pid="$GOV_LAST_PROPOSAL_ID"
  while IFS= read -r vote; do
    local voter option weighted
    voter=$(echo "$vote" | jq -r '.voter // "validator"')
    option=$(echo "$vote" | jq -r '.option // empty')
    weighted=$(echo "$vote" | jq -r '.weighted // false')
    if [ "$weighted" = "true" ]; then
      local opts
      opts=$(echo "$vote" | jq -r '.options')
      gov_cast_weighted_vote "$pid" "$voter" "$opts"
    else
      gov_cast_vote "$pid" "$voter" "$option"
    fi
  done < <(echo "$entry" | jq -c '.votes[]?')
  local expect_status
  expect_status=$(echo "$entry" | jq -r '.expect.final_status // "PROPOSAL_STATUS_REJECTED"')
  gov_wait_status "$pid" "$expect_status"
  local after
  after=$(gov_balance_ulmn "$VAL_ADDR")
  gov_expect_balance_delta "$before" "$after" ">=" "-100000" "weighted refund"
  restore_base_params
  dns_restore_if_dirty
}

scenario_gov_param_mutation_blocked() {
  local entry="$1"
  local value
  value=$(echo "$entry" | jq -r '.value // "0.750000000000000000"')
  local mutated
  mutated=$(gov_override_param "$GOV_BASE_PARAMS" "quorum" "ratio" "$value")
  if gov_apply_params "$mutated" "illegal gov quorum change" "gov-immutable"; then
    echo "expected gov_apply_params to reject quorum mutation" >&2
    exit 1
  fi
  gov_assert_param_equals "quorum" "0.670000000000000000" 1
}

run_param_case() {
  local entry="$1"
  local name type
  name=$(echo "$entry" | jq -r '.name')
  type=$(echo "$entry" | jq -r '.value_type')
  step "CASE param:$name ($type)"

  local valid_cmd invalid_cmd
  if [[ "$type" == "bool" || "$type" == "json" || "$type" == "coins" || "$type" == "uint" ]]; then
    valid_cmd='jq -c ".valid[]?"'
    invalid_cmd='jq -c ".invalid[]?"'
  else
    valid_cmd='jq -r ".valid[]?"'
    invalid_cmd='jq -r ".invalid[]?"'
  fi

  while IFS= read -r val; do
    local updated actual expected
    updated=$(gov_override_param "$GOV_BASE_PARAMS" "$name" "$type" "$val")
    if ! gov_apply_params "$updated" "param $name=$val" "param-valid"; then
      echo "unexpected failure applying valid $name=$val" >&2
      exit 1
    fi
    expected=$(normalize_param_value "$type" "$val")
    if [ "$type" = "coins" ]; then
      actual=$(gov_query_params | jq -c --arg f "$name" '.[$f]')
    else
      actual=$(gov_query_params | jq -r --arg f "$name" '.[$f]')
    fi
    if [ "$actual" != "$expected" ]; then
      echo "param $name expected $expected got $actual" >&2
      exit 1
    fi
    CURRENT_GOV_PARAMS="$updated"
    restore_base_params
  done < <(eval "$valid_cmd" <<<"$entry")

  while IFS= read -r val; do
    local updated
    updated=$(gov_override_param "$GOV_BASE_PARAMS" "$name" "$type" "$val") || continue
    if gov_try_params_update "$updated" "param invalid" "invalid"; then
      local pid="$GOV_LAST_PROPOSAL_ID"
      gov_cast_vote "$pid" validator YES
      gov_wait_status "$pid" "PROPOSAL_STATUS_FAILED"
      local current base
      if [ "$type" = "coins" ]; then
        current=$(gov_query_params | jq -c --arg f "$name" '.[$f]')
        base=$(echo "$GOV_BASE_PARAMS" | jq -c --arg f "$name" '.[$f]')
      else
        current=$(gov_query_params | jq -r --arg f "$name" '.[$f]')
        base=$(echo "$GOV_BASE_PARAMS" | jq -r --arg f "$name" '.[$f]')
      fi
      if [ "$current" != "$base" ]; then
        echo "invalid $name=$val unexpectedly changed state" >&2
        exit 1
      fi
    else
      echo "→ invalid $name=$val rejected prior to broadcast (CLI validation)"
    fi
  done < <(eval "$invalid_cmd" <<<"$entry")
}

run_scenario_case() {
  local entry="$1"
  local scenario
  scenario=$(echo "$entry" | jq -r '.scenario')
  step "CASE scenario:$scenario"
  case "$scenario" in
    deposit_topup) scenario_deposit_topup "$entry" ;;
    deposit_expiry) scenario_deposit_expiry "$entry" ;;
    insufficient_initial_deposit) scenario_initial_deposit_guard "$entry" ;;
    gov_param_mutation_blocked) scenario_gov_param_mutation_blocked "$entry" ;;
    quorum_burn) scenario_quorum_burn "$entry" ;;
    tally_veto_burn) scenario_tally_veto "$entry" ;;
    expedited_threshold) scenario_expedited "$entry" ;;
    cancel_refund) scenario_cancel "$entry" ;;
    authority_invalid) scenario_authority_invalid "$entry" ;;
    multi_msg_atomic) scenario_multi_message_atomic "$entry" ;;
    multi_msg_success) scenario_multi_message_success "$entry" ;;
    dns_update_fee_effect) scenario_dns_fee_effect "$entry" ;;
    pqc_disabled) scenario_pqc_disabled "$entry" ;;
    weighted_split) scenario_weighted_split "$entry" ;;
    *)
      echo "unsupported scenario $scenario" >&2
      exit 1
      ;;
  esac
}

main() {
  if [ "${1:-}" = "--skip-build" ]; then
    SKIP_BUILD=1
    shift
  fi

  build
  init_chain
  start_node

  gov_wait_height 2
  local net
  net=$(curl -s "$RPC/status" | jq -r '.result.node_info.network' 2>/dev/null || echo "")
  if [ "$net" != "$CHAIN_ID" ]; then
    echo "wrong chain: $net" >&2
    exit 1
  fi

  pqc_wait_ready "$RPC" "$API"
  pqc_policy_must_be_required "$RPC"
  gov_wait_balance "$VAL_ADDR" 1000
  gov_wait_balance "$VOTER2_ADDR" 1000

  setup_pqc_signer validator
  setup_pqc_signer voter2

  GOV_AUTHORITY=$(gov_resolve_authority)
  if [ -z "$GOV_AUTHORITY" ]; then
    echo "failed to resolve governance authority" >&2
    exit 1
  fi
  export GOV_AUTHORITY

  FEE_COLLECTOR_ADDR=$("$BIN" query auth module-account fee_collector --node "$NODE" -o json | jq -r '.. | .address? // empty' | head -n1)
  if [ -z "$FEE_COLLECTOR_ADDR" ]; then
    echo "failed to resolve fee collector address" >&2
    exit 1
  fi

  GOV_BASE_PARAMS=$(gov_query_params)
  CURRENT_GOV_PARAMS="$GOV_BASE_PARAMS"
  DNS_BASE_PARAMS=$(gov_query_dns_params)
  DNS_DIRTY=0

  local jq_expr='.[]'
  if [ -n "$CASE_FILTER" ]; then
    jq_expr='.[] | select(((.name // .scenario // .description // "") | tostring) | contains($filter))'
    echo "CASE_FILTER='$CASE_FILTER' → running filtered subset"
  fi

  local cases=0
  while IFS= read -r entry; do
    cases=$((cases+1))
    local scenario_type
    scenario_type=$(echo "$entry" | jq -r '.scenario // empty')
    if [ -n "$scenario_type" ]; then
      run_scenario_case "$entry"
    else
      run_param_case "$entry"
    fi
  done < <(jq -c --arg filter "$CASE_FILTER" "$jq_expr" "$CASE_FILE")

  if [ "$cases" -eq 0 ]; then
    echo "no governance cases matched CASE_FILTER='${CASE_FILTER}'" >&2
    exit 1
  fi

  restore_base_params
  dns_restore_if_dirty
  printf '\nDAO governance suite completed (%d cases).\n' "$cases"
  kill_node
}

main "$@"
