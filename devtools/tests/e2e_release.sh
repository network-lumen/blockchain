#!/usr/bin/env bash
set -euo pipefail

# Environment variables:
#   SKIP_BUILD       Skip rebuilding the binary (default 0)
#   RPC_HOST/PORT    RPC bind host/port (default 127.0.0.1:26657)
#   API_HOST/PORT    REST bind host/port (default 127.0.0.1:1317)
#   GRPC_HOST/PORT   gRPC bind host/port (default 127.0.0.1:9090)
#   GRPC_WEB_ENABLE  Enable gRPC-Web (default 1)
#   LOG_FILE         Node log destination (default /tmp/lumen.log)
#   DEBUG_KEEP       Set to 1 to keep the temporary HOME directory on exit

SOURCE_DIR="$(cd "$(dirname "$0")" && pwd)"
. "${SOURCE_DIR}/lib_pqc.sh"
pqc_require_bins

HOME_LUMEN=$(mktemp -d -t lumen-e2e-XXXXXX)
export HOME="$HOME_LUMEN"

DIR=$(cd "${SOURCE_DIR}/../.." && pwd)
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
P2P_HOST="${P2P_HOST:-0.0.0.0}"
P2P_PORT="${P2P_PORT:-26656}"
P2P_LADDR="${P2P_LADDR:-tcp://${P2P_HOST}:${P2P_PORT}}"
LOG_FILE="${LOG_FILE:-/tmp/lumen.log}"
CHAIN_ID="lumen"
KEYRING=${KEYRING:-test}
TX_FEES=${TX_FEES:-0ulmn}
NODE=${NODE:-$RPC_LADDR}

keys_add_quiet() {
  local name="$1"
  printf '\n' | "$BIN" keys add "$name" --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null 2>&1 || true
}

step() { echo; echo "==== $*"; }
wait_http() { local url="$1"; for i in $(seq 1 80); do curl -sSf "$url" >/dev/null && return 0; sleep 0.3; done; echo "Timeout $url" >&2; return 1; }
wait_tx_commit() { local h="$1"; for i in $(seq 1 100); do local c; c=$(curl -s "$RPC/tx?hash=0x$h" | jq -r .result.tx_result.code); [ "$c" != "null" ] && return 0; sleep 0.3; done; return 1; }

kill_node(){ pkill -f "lumend start" >/dev/null 2>&1 || true; }
cleanup(){
  kill_node
  if [ "${DEBUG_KEEP:-0}" != "1" ]; then
    rm -rf "$HOME_LUMEN" >/dev/null 2>&1 || true
  else
    echo "DEBUG_KEEP=1: keeping $HOME_LUMEN"
  fi
}
trap cleanup EXIT

SKIP_BUILD=${SKIP_BUILD:-0}
build(){
  if [ "$SKIP_BUILD" = "1" ] || [ "${1:-}" = "--skip-build" ]; then return 0; fi
  step "Build lumend"
  build_cmd=(go build)
  if [ -n "$LUMEN_BUILD_TAGS" ]; then
    build_cmd+=(-tags "$LUMEN_BUILD_TAGS")
  fi
  build_cmd+=(-o "$BIN" ./cmd/lumend)
  (cd "$DIR" && "${build_cmd[@]}")
}

init_chain(){
  step "Init chain"
  rm -rf "$HOME_DIR"
  "$BIN" init local --chain-id "$CHAIN_ID" --home "$HOME_DIR"
  keys_add_quiet publisher1
  keys_add_quiet publisher2
  keys_add_quiet intruder
  ADDR_P1=$("$BIN" keys show publisher1 -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
  ADDR_P2=$("$BIN" keys show publisher2 -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
  ADDR_INTRUDER=$("$BIN" keys show intruder -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
  "$BIN" genesis add-genesis-account "$ADDR_P1" 100000000ulmn --keyring-backend "$KEYRING" --home "$HOME_DIR"
  "$BIN" genesis add-genesis-account "$ADDR_P2" 100000000ulmn --keyring-backend "$KEYRING" --home "$HOME_DIR"
  "$BIN" genesis add-genesis-account "$ADDR_INTRUDER" 1000000ulmn --keyring-backend "$KEYRING" --home "$HOME_DIR"
  "$BIN" genesis gentx publisher1 1000000ulmn --chain-id "$CHAIN_ID" --keyring-backend "$KEYRING" --home "$HOME_DIR" --moniker local --commission-rate 0.10 --commission-max-rate 0.20 --commission-max-change-rate 0.01 --min-self-delegation 1 >/dev/null
  "$BIN" genesis collect-gentxs --home "$HOME_DIR" >/dev/null
  tmp=$(mktemp)
  jq --arg p1 "$ADDR_P1" --arg p2 "$ADDR_P2" '
    .app_state.release.params = {
      allowed_publishers:[$p1,$p2],
      channels:["stable","beta","nightly"],
      max_artifacts:"4",
      max_urls_per_art:"8",
      max_sigs_per_art:"4",
      max_notes_len:"512"
    } | .' "$HOME_DIR/config/genesis.json" > "$tmp" && mv "$tmp" "$HOME_DIR/config/genesis.json"
  "$BIN" genesis validate --home "$HOME_DIR" >/dev/null
  pqc_set_client_config "$HOME_DIR" "$RPC_LADDR" "$CHAIN_ID"
}

start_node(){
  step "Start node"
  kill_node
  local cmd=(
    "$BIN" start
    --home "$HOME_DIR"
    --rpc.laddr "$RPC_LADDR"
    --p2p.laddr "$P2P_LADDR"
    --api.enable
    --api.address "$API_ADDR"
    --grpc.address "$GRPC_ADDR"
    --minimum-gas-prices 0ulmn
  )
  [ "$GRPC_WEB_ENABLE" = "1" ] && cmd+=(--grpc-web.enable)
  if [ "${DISABLE_PPROF:-0}" = "1" ]; then
    cmd+=(--rpc.pprof_laddr "")
  fi
  ("${cmd[@]}" >"$LOG_FILE" 2>&1 &)
  sleep 1
  wait_http "$RPC/status"; wait_http "$API/"
  for i in $(seq 1 80); do H=$(curl -s "$RPC/status" | jq -r .result.sync_info.latest_block_height); [ "$H" != "0" ] && [ "$H" != "1" ] && break || sleep 0.3; done
}

release_tx(){
  local subcmd="$1"; shift
  local res hash
  res=$("$BIN" tx release "$subcmd" "$@" \
    --keyring-backend "$KEYRING" --home "$HOME_DIR" \
    --chain-id "$CHAIN_ID" --fees "$TX_FEES" -y -o json)
  echo "$res" | jq >&2
  hash=$(echo "$res" | jq -r .txhash)
  if [ -n "$hash" ] && [ "$hash" != "null" ]; then
    wait_tx_commit "$hash" >/dev/null || true
    RELEASE_LAST_CODE=${LAST_TX_CODE:-0}
  else
    RELEASE_LAST_CODE=$(echo "$res" | jq -r .code)
  fi
  printf '%s\n' "$RELEASE_LAST_CODE"
}

if [ "${1:-}" = "--skip-build" ]; then SKIP_BUILD=1; fi
build ${1:-}
init_chain
start_node
pqc_wait_ready "$RPC" "$API"
pqc_policy_must_be_required "$RPC"

P1=$("$BIN" keys show publisher1 -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
P2=$("$BIN" keys show publisher2 -a --keyring-backend "$KEYRING" --home "$HOME_DIR")

for signer in publisher1 publisher2; do
  setup_pqc_signer "$signer"
done

step "Publish v1"
BODY=$(jq -nc --arg c "stable" --arg v "1.0.0" --arg p "$P1" --arg pl1 "linux-amd64" --arg pl2 "darwin-arm64" '
  {creator:$p, release:{version:$v, channel:$c, artifacts:[
    {platform:$pl1, kind:"daemon", sha256_hex:"'"$(printf %064d 0 | tr 0 a)"'", size:1234, urls:["https://example.com/ex1"], signatures:[]},
    {platform:$pl2, kind:"daemon", sha256_hex:"'"$(printf %064d 0 | tr 0 b)"'", size:2345, urls:[], signatures:[]}
  ], notes:"first release", supersedes:[]}}')
CODE=$(release_tx publish --msg "$BODY" --from publisher1)
if [ "$CODE" = "0" ]; then echo "ok publish v1"; else echo "fail publish v1"; exit 1; fi

if [ "${SKIP_PQC_NEGATIVE:-0}" != "1" ]; then
  if "$BIN" tx bank send publisher1 "$P2" "1ulmn" \
     --pqc-enable=false --fees "$TX_FEES" --chain-id "$CHAIN_ID" \
     --keyring-backend "$KEYRING" --home "$HOME_DIR" \
     --broadcast-mode sync --yes >/tmp/pqc_neg.out 2>&1; then
    echo "error: PQC-disabled TX unexpectedly succeeded" >&2
    exit 1
  fi
  grep -qiE "pqc.*(missing|required|signature)" /tmp/pqc_neg.out || \
    echo "warning: PQC-disabled TX failed but no explicit PQC error found" >&2
fi

ID1=$(curl -s "$API/lumen/release/by_version/1.0.0" | jq -r .release.id)
test "$(curl -s "$API/lumen/release/$ID1" | jq -r .release.id)" = "$ID1"
test "$ID1" != "null"

if [ "$ID1" != "1" ]; then
  echo "expected first release id to be 1 (got $ID1)" >&2
  exit 1
fi

step "Assert v1 present via by_version"
RID=$(curl -s "$API/lumen/release/by_version/1.0.0" | jq -r .release.id)
test "$RID" = "$ID1"

step "Mirror: add URL to artifact[0]"
BODY=$(jq -nc --arg c "$P1" --argjson id "$ID1" '{creator:$c, id:$id, artifact_index:0, new_urls:["https://mirror1","https://mirror1"]}')
CODE=$(release_tx mirror --msg "$BODY" --from publisher1)
test "$CODE" = "0"
URLS=$(curl -s "$API/lumen/release/by_version/1.0.0" | jq -r '.release.artifacts[0].urls | length')
if [ "$URLS" -ge 2 ]; then echo "ok mirror urls=$URLS"; else echo "fail mirror urls=$URLS"; exit 1; fi

step "Publish v2 by publisher2"
BODY=$(jq -nc --arg c "stable" --arg v "1.0.1" --arg p "$P2" --arg pl1 "linux-amd64" --arg id "$ID1" '{creator:$p, release:{version:$v, channel:$c, artifacts:[{platform:$pl1, kind:"daemon", sha256_hex:"'"$(printf %064d 0 | tr 0 c)"'", size:3456, urls:[], signatures:[]}], notes:"v2", supersedes:[($id|tonumber)]}}')
CODE=$(release_tx publish --msg "$BODY" --from publisher2)
if [ "$CODE" = "0" ]; then echo "ok publish v2"; else echo "fail publish v2"; exit 1; fi
ID2=$(curl -s "$API/lumen/release/by_version/1.0.1" | jq -r .release.id)
test "$ID2" != "null"

step "Yank v1"
BODY=$(jq -nc --arg c "$P1" --argjson id "$ID1" '{creator:$c, id:$id}')
CODE=$(release_tx yank --msg "$BODY" --from publisher1)
if [ "$CODE" = "0" ]; then echo "ok yank v1"; else echo "fail yank v1"; exit 1; fi
YNK=$(curl -s "$API/lumen/release/by_version/1.0.0" | jq -r .release.yanked)
test "$YNK" = "true"

step "Negative: non-publisher cannot publish"
keys_add_quiet intruder
setup_pqc_signer intruder
IADDR=$("$BIN" keys show intruder -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
BODY=$(jq -nc --arg c "stable" --arg v "9.9.9" --arg p "$IADDR" --arg pl1 "linux-amd64" '{creator:$p, release:{version:$v, channel:$c, artifacts:[{platform:$pl1, kind:"daemon", sha256_hex:"'"$(printf %064d 0 | tr 0 d)"'", size:"1", urls:[], signatures:[]}], notes:"", supersedes:[]}}')
CODE=$(release_tx publish --msg "$BODY" --from intruder || true)
if [ "$CODE" != "0" ]; then echo "ok negative non-publisher"; else echo "fail negative non-publisher"; exit 1; fi

step "Negative: publish without sha -> fail"
BODY=$(jq -nc --arg c "stable" --arg v "1.2.3" --arg p "$P1" --arg pl1 "linux-amd64" '{creator:$p, release:{version:$v, channel:$c, artifacts:[{platform:$pl1, kind:"daemon", sha256_hex:"", size:"1", urls:[] }], notes:"", supersedes:[]}}')
CODE=$(release_tx publish --msg "$BODY" --from publisher1 || true)
if [ "$CODE" != "0" ]; then echo "ok negative missing sha"; else echo "fail negative missing sha"; exit 1; fi

step "Pagination: list >=2 releases, page 1 limit 1"
N=$(curl -s "$API/lumen/release/releases?page=1&limit=100" | jq -r '.releases | length')
if [ "$N" -ge 2 ]; then echo "ok list N=$N"; else echo "fail list N=$N"; exit 1; fi
P1CNT=$(curl -s "$API/lumen/release/releases?page=1&limit=1" | jq -r '.releases | length')
P2CNT=$(curl -s "$API/lumen/release/releases?page=2&limit=1" | jq -r '.releases | length')
echo "N=$N P1CNT=$P1CNT P2CNT=$P2CNT"
if [ "$P1CNT" = "1" ] && [ "$P2CNT" = "1" ]; then echo "ok pagination p1=$P1CNT p2=$P2CNT"; else echo "fail pagination p1=$P1CNT p2=$P2CNT"; exit 1; fi

echo
echo "All release tests passed."
kill_node
