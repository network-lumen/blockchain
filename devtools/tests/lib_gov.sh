#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

# Helper functions for governance-driven e2e tests.
# Expects the following variables to be defined by the caller:
# BIN, NODE, RPC, API, KEYRING, HOME_DIR, CHAIN_ID, TX_FEES.

GOV_LAST_TX_HASH=""
GOV_LAST_TX_CODE=""
GOV_LAST_PROPOSAL_ID=""
GOV_LAST_TX_JSON=""

GOV_LAST_TX_FILE="/tmp/gov_last_tx.json"
GOV_LAST_PROP_FILE="/tmp/gov_last_proposal.json"

_gov_require_bin() {
  local bin="$1"
  if ! command -v "$bin" >/dev/null 2>&1; then
    echo "error: missing dependency '$bin'" >&2
    exit 1
  fi
}

gov_require_bins() {
  for bin in jq curl python3; do
    _gov_require_bin "$bin"
  done
}

gov_wait_http() {
  local url="$1"; shift
  local tries="${1:-120}"
  for _ in $(seq 1 "$tries"); do
    if curl -sSf "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  echo "timeout waiting for $url" >&2
  return 1
}

gov_wait_height() {
  local target="$1"
  for _ in $(seq 1 120); do
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

gov_wait_balance() {
  local addr="$1"; local min="$2"
  local last="0"
  for _ in $(seq 1 120); do
    last=$(curl -s "$API/cosmos/bank/v1beta1/balances/$addr" \
      | jq -r '((.balances // [])[] | select(.denom=="ulmn") | .amount) // "0"' 2>/dev/null || echo "0")
    if [ -n "$last" ] && [ "$last" != "null" ]; then
      if [ "$last" -ge "$min" ]; then
        return 0
      fi
    fi
    sleep 0.5
  done
  echo "timeout waiting for $addr to hold at least ${min}ulmn (last balance: ${last})" >&2
  return 1
}

gov_query_params() {
  "$BIN" query gov params --node "$NODE" -o json | jq '.params'
}

gov_resolve_authority() {
  "$BIN" query auth module-account gov --node "$NODE" -o json \
    | jq -r '.account.base_account.address // .account.value.address // .account.address // .account.base_vesting_account.base_account.address // empty'
}

gov_query_dns_params() {
  "$BIN" query dns params --node "$NODE" -o json | jq '.params'
}

gov_canonical_duration() {
  local value="$1"
  python3 - "$value" <<'PY'
import re, sys
value = sys.argv[1].strip()
if not value:
    print("0s")
    raise SystemExit(0)
pattern = re.compile(r'(\d+)([hms])')
pos = 0
total = 0
factors = {'h': 3600, 'm': 60, 's': 1}
for match in pattern.finditer(value):
    if match.start() != pos:
        raise SystemExit(f"invalid duration literal: {value}")
    total += int(match.group(1)) * factors[match.group(2)]
    pos = match.end()
if pos != len(value):
    raise SystemExit(f"invalid duration literal: {value}")
hours, rem = divmod(total, 3600)
minutes, seconds = divmod(rem, 60)
parts = []
if hours:
    parts.append(f"{hours}h")
if hours or minutes:
    parts.append(f"{minutes}m")
parts.append(f"{seconds}s")
print(''.join(parts))
PY
}

gov_set_last_tx() {
  GOV_LAST_TX_HASH="$1"
  GOV_LAST_TX_CODE="$2"
  GOV_LAST_TX_JSON="$3"
  echo "$GOV_LAST_TX_JSON" > "$GOV_LAST_TX_FILE"
}

gov_extract_proposal_id() {
  local tx_json="$1"
  echo "$tx_json" | jq -r '.result.tx_result.events[]? | select(.type=="submit_proposal") | .attributes[]? | select(.key=="proposal_id") | .value' | tail -n1
}

gov_submit_proposal_file() {
  local file="$1"
  local deposit="$2"
  local expedited="${3:-0}"
  local tmp
  tmp=$(mktemp -t gov-proposal-XXXXXX)
  if [ "$expedited" = "1" ]; then
    jq --arg deposit "$deposit" '.deposit = $deposit | .expedited = true' "$file" >"$tmp"
  else
    jq --arg deposit "$deposit" '.deposit = $deposit | del(.expedited)' "$file" >"$tmp"
  fi
  local res hash code tx_json
  res=$("$BIN" tx gov submit-proposal "$tmp" \
    --from validator \
    --chain-id "$CHAIN_ID" \
    --node "$NODE" \
    --keyring-backend "$KEYRING" \
    --home "$HOME_DIR" \
    --gas 400000 \
    --fees "$TX_FEES" \
    --yes \
    -o json)
  hash=$(echo "$res" | jq -r '.txhash // empty')
  if [ -z "$hash" ] || [ "$hash" = "null" ]; then
    GOV_LAST_TX_HASH=""
    GOV_LAST_TX_CODE=""
    GOV_LAST_PROPOSAL_ID=""
    echo "$res" > "$GOV_LAST_TX_FILE"
    echo "$res" > "$GOV_LAST_PROP_FILE"
    return 1
  fi
  code=$(gov_wait_tx "$hash") || code="1"
  tx_json=$(curl -s "$RPC/tx?hash=0x$hash")
  gov_set_last_tx "$hash" "$code" "$tx_json"
  if [ "$code" != "0" ]; then
    GOV_LAST_PROPOSAL_ID=""
    return 1
  fi
  local pid
  pid=$(gov_extract_proposal_id "$tx_json")
  if [ -z "$pid" ] || [ "$pid" = "null" ]; then
    pid=$("$BIN" query gov proposals --proposal-status unspecified --node "$NODE" -o json \
      | jq -r '.proposals[]?.id' | sort -n | tail -n1)
  fi
  GOV_LAST_PROPOSAL_ID="$pid"
  echo "$tx_json" > "$GOV_LAST_PROP_FILE"
  return 0
}

gov_wait_tx() {
  local hash="$1"
  for _ in $(seq 1 120); do
    local resp code
    resp=$(curl -s "$RPC/tx?hash=0x$hash" 2>/dev/null || true)
    code=$(echo "$resp" | jq -r '.result.tx_result.code // empty' 2>/dev/null || echo "")
    if [ -n "$code" ] && [ "$code" != "null" ]; then
      echo "$code"
      return 0
    fi
    sleep 0.5
  done
  echo "timeout waiting for tx $hash" >&2
  return 1
}

gov_cast_vote() {
  local proposal_id="$1"; local voter="$2"; local option="$3"
  local res hash opt
  opt=$(echo "$option" | tr '[:upper:]' '[:lower:]')
  res=$("$BIN" tx gov vote "$proposal_id" "$opt" \
    --from "$voter" \
    --chain-id "$CHAIN_ID" \
    --node "$NODE" \
    --keyring-backend "$KEYRING" \
    --home "$HOME_DIR" \
    --fees "$TX_FEES" \
    --yes \
    -o json)
  hash=$(echo "$res" | jq -r '.txhash // empty')
  if [ -n "$hash" ] && [ "$hash" != "null" ]; then
    gov_wait_tx "$hash" >/dev/null || true
  fi
}

gov_cast_weighted_vote() {
  local proposal_id="$1"; local voter="$2"; local options="$3"
  local res hash opts
  opts=$(echo "$options" | tr '[:upper:]' '[:lower:]')
  res=$("$BIN" tx gov weighted-vote "$proposal_id" "$opts" \
    --from "$voter" \
    --chain-id "$CHAIN_ID" \
    --node "$NODE" \
    --keyring-backend "$KEYRING" \
    --home "$HOME_DIR" \
    --fees "$TX_FEES" \
    --yes \
    -o json)
  hash=$(echo "$res" | jq -r '.txhash // empty')
  if [ -n "$hash" ] && [ "$hash" != "null" ]; then
    gov_wait_tx "$hash" >/dev/null || true
  fi
}

gov_deposit() {
  local proposal_id="$1"; local amount="$2"; local from="$3"
  local res hash
  res=$("$BIN" tx gov deposit "$proposal_id" "$amount" \
    --from "$from" \
    --chain-id "$CHAIN_ID" \
    --node "$NODE" \
    --keyring-backend "$KEYRING" \
    --home "$HOME_DIR" \
    --fees "$TX_FEES" \
    --yes \
    -o json)
  hash=$(echo "$res" | jq -r '.txhash // empty')
  if [ -n "$hash" ] && [ "$hash" != "null" ]; then
    gov_wait_tx "$hash" >/dev/null || true
  fi
}

gov_wait_status() {
  local proposal_id="$1"; local target="$2"; local tries="${3:-180}"
  for _ in $(seq 1 "$tries"); do
    local resp status
    if ! resp=$("$BIN" query gov proposal "$proposal_id" --node "$NODE" -o json 2>&1); then
      if echo "$resp" | grep -qi "not found"; then
        echo "$resp" >&2
        return 2
      fi
      echo "$resp" >&2
      return 1
    fi
    status=$(printf '%s' "$resp" | jq -r '.proposal.status')
    if [ "$status" = "$target" ]; then
      return 0
    fi
    sleep 1
  done
  echo "proposal $proposal_id did not reach $target" >&2
  return 1
}

gov_balance_ulmn() {
  local addr="$1"
  "$BIN" query bank balances "$addr" --node "$NODE" -o json \
    | jq -r '((.balances // [])[] | select(.denom=="ulmn") | .amount) // "0"'
}

gov_expect_balance_delta() {
  local before="$1"; local after="$2"; local cmp="$3"; local amount="$4"; local label="$5"
  python3 - "$before" "$after" "$cmp" "$amount" "$label" <<'PY'
import sys
before = int(sys.argv[1])
after = int(sys.argv[2])
cmp = sys.argv[3]
amount = int(sys.argv[4])
label = sys.argv[5]
delta = after - before
ok = False
if cmp == '>=':
    ok = delta >= amount
elif cmp == '<=':
    ok = delta <= amount
elif cmp == '==':
    ok = delta == amount
elif cmp == '>':
    ok = delta > amount
elif cmp == '<':
    ok = delta < amount
else:
    sys.exit(f"invalid comparator {cmp} for {label}")
if not ok:
    sys.exit(f"unexpected balance delta for {label}: {delta} (expected {cmp} {amount})")
PY
}

gov_override_param() {
  local base_json="$1"; local field="$2"; local vtype="$3"; local raw_value="$4"
  case "$vtype" in
    ratio|duration|string|address)
      echo "$base_json" | jq --arg f "$field" --arg v "$raw_value" '.[$f] = $v'
      ;;
    bool)
      echo "$base_json" | jq --arg f "$field" --argjson v "$raw_value" '.[$f] = $v'
      ;;
    uint)
      echo "$base_json" | jq --arg f "$field" --argjson v "$raw_value" '.[$f] = $v'
      ;;
    coins)
      local coins
      coins=$(echo "$raw_value" | jq -c '[ .[] | capture("(?<amount>[0-9]+)(?<denom>[a-zA-Z0-9/]+)") | {denom: .denom, amount: .amount} ]')
      echo "$base_json" | jq --arg f "$field" --argjson coins "$coins" '.[$f] = $coins'
      ;;
    json)
      echo "$base_json" | jq --arg f "$field" --argjson v "$raw_value" '.[$f] = $v'
      ;;
    *)
      echo "unsupported value type $vtype" >&2
      return 1
      ;;
  esac
}

gov_apply_params() {
  local params_json="$1"; local title="$2"; local summary="$3"
  local metadata="${4:-""}"
  local msg
  msg=$(jq -cn --arg auth "$GOV_AUTHORITY" --argjson params "$params_json" '{"@type":"/cosmos.gov.v1.MsgUpdateParams", authority:$auth, params:$params}')
  local proposal_file
  proposal_file=$(mktemp -t gov-proposal-XXXXXX)
  jq -n --arg title "$title" --arg summary "$summary" --arg metadata "$metadata" --argjson msg "$msg" \
    '{messages: [$msg], title: $title, summary: $summary, metadata: $metadata}' >"$proposal_file"
  if ! gov_submit_proposal_file "$proposal_file" "10000000ulmn"; then
    return 1
  fi
  local pid="$GOV_LAST_PROPOSAL_ID"
  gov_cast_vote "$pid" validator yes
  gov_wait_status "$pid" "PROPOSAL_STATUS_PASSED"
  GOV_LAST_PROPOSAL_ID="$pid"
  return 0
}

gov_try_params_update() {
  local params_json="$1"; local title="$2"; local summary="$3"
  local msg
  msg=$(jq -cn --arg auth "$GOV_AUTHORITY" --argjson params "$params_json" '{"@type":"/cosmos.gov.v1.MsgUpdateParams", authority:$auth, params:$params}')
  local proposal_file
  proposal_file=$(mktemp -t gov-proposal-XXXXXX)
  jq -n --arg title "$title" --arg summary "$summary" --arg metadata "" --argjson msg "$msg" \
    '{messages: [$msg], title: $title, summary: $summary, metadata: $metadata}' >"$proposal_file"
  gov_submit_proposal_file "$proposal_file" "10000000ulmn"
  return $?
}

gov_restore_params() {
  local params_json="$1"
  gov_apply_params "$params_json" "Restore gov params" "restore"
}

gov_assert_param_equals() {
  local field="$1"; local expected="$2"; local strip_quotes="${3:-0}"
  local val
  if [ "$strip_quotes" = "1" ]; then
    val=$(gov_query_params | jq -r --arg f "$field" '.[$f]')
  else
    val=$(gov_query_params | jq --arg f "$field" '.[$f]')
  fi
  if [ "$strip_quotes" = "1" ]; then
    if [ "$val" != "$expected" ]; then
      echo "param $field expected $expected got $val" >&2
      return 1
    fi
  else
    if [ "$val" != "$expected" ]; then
      echo "param $field expected $expected got $val" >&2
      return 1
    fi
  fi
}

gov_expect_tx_failure() {
  local contains="${1:-}"
  if [ -z "${GOV_LAST_TX_CODE:-}" ]; then
    if [ -n "$contains" ]; then
      if ! grep -qi "$contains" "$GOV_LAST_TX_FILE" 2>/dev/null && ! grep -qi "$contains" "$GOV_LAST_PROP_FILE" 2>/dev/null; then
        echo "tx failure did not mention '$contains'" >&2
        return 1
      fi
    fi
    return 0
  fi
  if [ "${GOV_LAST_TX_CODE}" = "0" ]; then
    echo "expected tx failure but last code=${GOV_LAST_TX_CODE}" >&2
    return 1
  fi
  if [ -n "$contains" ]; then
    if ! grep -qi "$contains" "$GOV_LAST_TX_FILE" 2>/dev/null && ! grep -qi "$contains" "$GOV_LAST_PROP_FILE" 2>/dev/null; then
      echo "tx failure did not mention '$contains'" >&2
      return 1
    fi
  fi
  return 0
}
