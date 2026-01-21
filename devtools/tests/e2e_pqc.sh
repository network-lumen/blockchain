#!/usr/bin/env bash
set -euo pipefail

# Environment variables:
# - BIN: Path to the lumend binary (default ./build/lumend relative to repo root).
# - CHAIN_ID: Chain ID used for the temporary network (default pqc-e2e).
# - RPC_HOST/PORT, API_HOST/PORT, GRPC_HOST/PORT: Bind addresses for the throwaway node.
# - SCHEME: PQC scheme to enforce (default dilithium3).
# - AMOUNT: Funding amount used for MsgSend assertions (default 1000ulmn).
# - KEYRING: Keyring backend (default test).
# - TX_FEES: Fees applied to transactions (default 1000ulmn).
# - LOG_FILE: Node log destination (default $HOME/lumend.log).
# - DEBUG_KEEP: Keep the temporary HOME directory on exit when set to 1.

DIR=$(cd "$(dirname "$0")/../.." && pwd)
BIN=${BIN:-"$DIR/build/lumend"}
: "${LUMEN_BUILD_TAGS:=dev}"
CHAIN_ID=${CHAIN_ID:-pqc-e2e}
RPC_HOST=${RPC_HOST:-127.0.0.1}
API_HOST=${API_HOST:-127.0.0.1}
GRPC_HOST=${GRPC_HOST:-127.0.0.1}

: "${DISABLE_PPROF:=1}"
: "${GRPC_WEB_ENABLE:=0}"

# Avoid port collisions when running locally by defaulting to a random base port.
if [ -z "${RANDOM_PORT_BASE:-}" ]; then
	RANDOM_PORT_BASE=$(( (RANDOM % 1000) + 30000 ))
fi
: "${RPC_PORT:=$RANDOM_PORT_BASE}"
: "${API_PORT:=$((RANDOM_PORT_BASE + 60))}"
: "${GRPC_PORT:=$((RANDOM_PORT_BASE + 120))}"
: "${P2P_PORT:=$((RANDOM_PORT_BASE + 180))}"

RPC="http://${RPC_HOST}:${RPC_PORT}"
API="http://${API_HOST}:${API_PORT}"
RPC_LADDR="${RPC_LADDR:-tcp://${RPC_HOST}:${RPC_PORT}}"
API_ADDR="${API_ADDR:-tcp://${API_HOST}:${API_PORT}}"
GRPC_ADDR="${GRPC_ADDR:-${GRPC_HOST}:${GRPC_PORT}}"
P2P_HOST="${P2P_HOST:-0.0.0.0}"
P2P_LADDR="${P2P_LADDR:-tcp://${P2P_HOST}:${P2P_PORT}}"
SCHEME=${SCHEME:-dilithium3}
AMOUNT=${AMOUNT:-1000ulmn}
KEYRING=${KEYRING:-test}
TX_FEES=${TX_FEES:-0ulmn}

require() { command -v "$1" >/dev/null || { echo "error: missing dependency '$1'" >&2; exit 1; }; }
require jq
require curl
require go

HOME_LUMEN=$(mktemp -d -t pqc-e2e-XXXXX)
export HOME="$HOME_LUMEN"
HOME_DIR="$HOME/.lumen"
LOG_FILE="${LOG_FILE:-$HOME/lumend.log}"
FAIL_LOG=""
PQC_GEN_DIR=$(mktemp -d -t pqc-gen-XXXXX)
cleanup() {
	pkill -f "lumend start" >/dev/null 2>&1 || true
	if [ "${DEBUG_KEEP:-0}" != "1" ]; then
		rm -rf "$HOME_LUMEN" "$PQC_GEN_DIR" >/dev/null 2>&1 || true
	else
		echo "DEBUG_KEEP=1: keeping $HOME_LUMEN"
	fi
	rm -f "${FAIL_LOG:-}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

cat <<'EOF' >"$PQC_GEN_DIR/main.go"
package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/cloudflare/circl/sign/dilithium"
)

func main() {
	mode := dilithium.Mode3
	pub, priv, err := mode.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	fmt.Println(hex.EncodeToString(pub.Bytes()))
	fmt.Println(hex.EncodeToString(priv.Bytes()))
}
EOF

cat <<'EOF' >"$PQC_GEN_DIR/go.mod"
module pqcgen

go 1.21

require github.com/cloudflare/circl v1.3.7
EOF
(cd "$PQC_GEN_DIR" && go mod tidy >/dev/null 2>&1)

generate_pqc_pair() {
	local out
	out=$(cd "$PQC_GEN_DIR" && go run .)
	PQC_PUB=$(echo "$out" | sed -n '1p')
	PQC_PRIV=$(echo "$out" | sed -n '2p')
	if [ -z "$PQC_PUB" ] || [ -z "$PQC_PRIV" ]; then
		echo "error: failed to generate Dilithium keypair" >&2
		exit 1
	fi
}

keys_add_quiet() {
	local name="$1"
	printf '\n' | "$BIN" keys add "$name" --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null 2>&1 || true
}

wait_http() {
	local url="$1"
	local tries="${WAIT_HTTP_TRIES:-240}"
	for _ in $(seq 1 "$tries"); do
		# Any 2xx response means the service is up and responding.
		curl --connect-timeout 1 --max-time 2 -sSf "$url" >/dev/null 2>&1 && return 0
		sleep 0.5
	done
	echo "error: timeout waiting for $url" >&2
	tail -n 120 "$LOG_FILE" 2>/dev/null >&2 || true
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
	echo "error: timeout waiting for block height >= $target" >&2
	tail -n 120 "$LOG_FILE" 2>/dev/null >&2 || true
	return 1
}

wait_ready() {
	local target=${1:-1}
	for _ in $(seq 1 240); do
		local status height catching_up
		status=$(curl -s "$RPC/status" 2>/dev/null || echo "")
		catching_up=$(echo "$status" | jq -r '.result.sync_info.catching_up' 2>/dev/null || echo "")
		height=$(echo "$status" | jq -r '.result.sync_info.latest_block_height' 2>/dev/null || echo "")
		if [ "$catching_up" = "false" ] && [ -n "$height" ] && [ "$height" != "null" ]; then
			if [ "$height" -ge "$target" ] 2>/dev/null; then
				return 0
			fi
		fi
		sleep 0.5
	done
	echo "error: timeout waiting for chain ready state (height >= $target)" >&2
	tail -n 120 "$LOG_FILE" 2>/dev/null >&2 || true
	return 1
}

wait_tx() {
	local hash="$1"
	for _ in $(seq 1 120); do
		local code
		code=$(curl -s "$RPC/tx?hash=0x$hash" | jq -r '.result.tx_result.code' 2>/dev/null || echo "")
		if [ "$code" != "" ] && [ "$code" != "null" ]; then
			echo "$code"
			return 0
		fi
		sleep 0.5
	done
	echo "error: timeout waiting for tx $hash" >&2
	return 1
}

import_and_link_local() {
	local keyname="$1" alias="$2" pub="$3" priv="$4"
	"$BIN" keys pqc-import \
		--name "$alias" \
		--scheme "$SCHEME" \
		--pubkey "$pub" \
		--privkey "$priv" \
		--keyring-backend "$KEYRING" \
		--home "$HOME_DIR" >/dev/null

	"$BIN" keys pqc-link \
		--from "$keyname" \
		--pqc "$alias" \
		--keyring-backend "$KEYRING" \
		--home "$HOME_DIR" >/dev/null
}

SKIP_BUILD=${SKIP_BUILD:-0}
if [ "${1:-}" = "--skip-build" ]; then
	SKIP_BUILD=1
	shift
fi
if [ "$SKIP_BUILD" != "1" ]; then
	build_cmd=(go build -trimpath -ldflags "-s -w")
	if [ -n "$LUMEN_BUILD_TAGS" ]; then
		build_cmd+=(-tags "$LUMEN_BUILD_TAGS")
	fi
	build_cmd+=(-o "$BIN" ./cmd/lumend)
	(cd "$DIR" && "${build_cmd[@]}")
fi

echo "==> Init single-node chain"
"$BIN" init local --chain-id "$CHAIN_ID" --home "$HOME_DIR" >/dev/null

VALIDATOR=validator
SENDER=sender
RECIPIENT=recipient

keys_add_quiet "$VALIDATOR"
keys_add_quiet "$SENDER"
keys_add_quiet "$RECIPIENT"

ADDR_VALIDATOR=$("$BIN" keys show "$VALIDATOR" -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
VALOPER_ADDR=$("$BIN" keys show "$VALIDATOR" --bech val --address --keyring-backend "$KEYRING" --home "$HOME_DIR")
ADDR_SENDER=$("$BIN" keys show "$SENDER" -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
ADDR_RECIPIENT=$("$BIN" keys show "$RECIPIENT" -a --keyring-backend "$KEYRING" --home "$HOME_DIR")

"$BIN" genesis add-genesis-account "$ADDR_VALIDATOR" 200000000ulmn --keyring-backend "$KEYRING" --home "$HOME_DIR"
"$BIN" genesis add-genesis-account "$ADDR_SENDER"    100000000ulmn --keyring-backend "$KEYRING" --home "$HOME_DIR"
"$BIN" genesis add-genesis-account "$ADDR_RECIPIENT"  10000000ulmn --keyring-backend "$KEYRING" --home "$HOME_DIR"

generate_pqc_pair
PQC_PUB_VALIDATOR=$PQC_PUB
PQC_PRIV_VALIDATOR=$PQC_PRIV
import_and_link_local "$VALIDATOR" "pqc-$VALIDATOR" "$PQC_PUB_VALIDATOR" "$PQC_PRIV_VALIDATOR"

generate_pqc_pair
PQC_PUB_SENDER=$PQC_PUB
PQC_PRIV_SENDER=$PQC_PRIV
import_and_link_local "$SENDER" "pqc-$SENDER" "$PQC_PUB_SENDER" "$PQC_PRIV_SENDER"

echo "==> gentx with PQC signing"
"$BIN" genesis gentx "$VALIDATOR" 1000000ulmn \
--chain-id "$CHAIN_ID" \
--keyring-backend "$KEYRING" \
--home "$HOME_DIR" >/dev/null
"$BIN" genesis collect-gentxs --home "$HOME_DIR" >/dev/null
"$BIN" genesis validate --home "$HOME_DIR" >/dev/null
CLIENT_TOML="$HOME_DIR/config/client.toml"
if [ -f "$CLIENT_TOML" ]; then
	tmpcfg=$(mktemp)
	awk -v rpc="tcp://${RPC_HOST}:${RPC_PORT}" -v chain="$CHAIN_ID" '
BEGIN { replaced_node=0; replaced_chain=0 }
/^node = / { print "node = \"" rpc "\""; replaced_node=1; next }
/^chain-id = / { print "chain-id = \"" chain "\""; replaced_chain=1; next }
{ print }
END {
	if (!replaced_node) { print "node = \"" rpc "\"" }
	if (!replaced_chain) { print "chain-id = \"" chain "\"" }
}
' "$CLIENT_TOML" >"$tmpcfg" && mv "$tmpcfg" "$CLIENT_TOML"
fi

echo "==> Starting node"
pkill -f "lumend start" >/dev/null 2>&1 || true
grpc_web_flag=(--grpc-web.enable=false)
if [ "${GRPC_WEB_ENABLE:-0}" = "1" ]; then
	grpc_web_flag=(--grpc-web.enable)
fi
extra_flags=()
if [ "${DISABLE_PPROF:-0}" = "1" ]; then
	extra_flags+=(--rpc.pprof_laddr "")
fi
(
	"$BIN" start \
		--home "$HOME_DIR" \
		--rpc.laddr "$RPC_LADDR" \
		--p2p.laddr "$P2P_LADDR" \
		--api.enable \
		--api.address "$API_ADDR" \
		--grpc.address "$GRPC_ADDR" \
		"${grpc_web_flag[@]}" \
		"${extra_flags[@]}" \
		--minimum-gas-prices 0ulmn >"$LOG_FILE" 2>&1
) &
sleep 1
echo "Node log (head):"
head -n 40 "$LOG_FILE" 2>/dev/null || true
wait_http "$RPC/status"
wait_http "$API/"
wait_ready 2

echo "==> Register PQC key on-chain for $SENDER"
LINK_RES=$("$BIN" tx pqc link-account \
	--from "$SENDER" \
	--scheme "$SCHEME" \
	--pubkey "$PQC_PUB_SENDER" \
	--chain-id "$CHAIN_ID" \
	--keyring-backend "$KEYRING" \
	--home "$HOME_DIR" \
	--yes \
	--fees "$TX_FEES" \
	--broadcast-mode sync \
	-o json)
echo "$LINK_RES" | jq
LINK_HASH=$(echo "$LINK_RES" | jq -r '.txhash')
CODE=$(wait_tx "$LINK_HASH")
[ "$CODE" = "0" ] || { echo "link-account failed (code=$CODE)"; exit 1; }

echo "==> PQC second link must fail (immutable)"
FAIL_LOG=$(mktemp)
if "$BIN" tx pqc link-account \
	--from "$SENDER" \
	--scheme "$SCHEME" \
	--pubkey "$PQC_PUB_SENDER" \
	--chain-id "$CHAIN_ID" \
	--keyring-backend "$KEYRING" \
	--home "$HOME_DIR" \
	--yes \
	--fees "$TX_FEES" \
	--broadcast-mode sync \
	-o json >"$FAIL_LOG" 2>&1; then
	echo "error: second link-account unexpectedly succeeded" >&2
	cat "$FAIL_LOG" >&2
	exit 1
fi

if grep -qiE "pqc.*(already linked|immutable)" "$FAIL_LOG"; then
	echo "[OK] second link-account rejected as immutable"
else
	echo "warning: second link-account failed without explicit immutability error" >&2
fi

ONCHAIN_SCHEME=$("$BIN" q pqc account "$ADDR_SENDER" --node "$RPC" --output json | jq -r '.account.scheme // .info.scheme // empty')
if [ "$ONCHAIN_SCHEME" != "$SCHEME" ]; then
	echo "error: expected on-chain scheme '$SCHEME', got '$ONCHAIN_SCHEME'" >&2
	exit 1
fi

echo "==> PQC-enabled bank transfer should pass"
SEND_OK=$("$BIN" tx bank send "$SENDER" "$ADDR_RECIPIENT" "$AMOUNT" \
	--chain-id "$CHAIN_ID" \
	--keyring-backend "$KEYRING" \
	--home "$HOME_DIR" \
	--yes \
	--fees "$TX_FEES" \
	--broadcast-mode sync \
	-o json)
echo "$SEND_OK" | jq
HASH=$(echo "$SEND_OK" | jq -r '.txhash')
CODE=$(wait_tx "$HASH")
[ "$CODE" = "0" ] || { echo "PQC-enabled transfer failed: code=$CODE" >&2; exit 1; }

echo "==> PQC-disabled bank transfer should fail"
FAIL_LOG=$(mktemp)
if "$BIN" tx bank send "$SENDER" "$ADDR_RECIPIENT" "$AMOUNT" \
	--pqc-enable=false \
	--chain-id "$CHAIN_ID" \
	--keyring-backend "$KEYRING" \
	--home "$HOME_DIR" \
	--yes \
	--fees "$TX_FEES" \
	--broadcast-mode sync >"$FAIL_LOG" 2>&1; then
	echo "error: PQC-disabled transfer unexpectedly succeeded" >&2
	cat "$FAIL_LOG" >&2
	exit 1
fi

if grep -qiE "pqc.*(missing|required|signature)" "$FAIL_LOG"; then
	echo "[OK] PQC-disabled transfer rejected with PQC error"
else
	echo "warning: PQC-disabled transfer failed without explicit PQC error" >&2
fi

echo "==> PQC-enabled staking delegation should pass"
DELEGATE_OK=$("$BIN" tx staking delegate "$VALOPER_ADDR" "$AMOUNT" \
	--from "$SENDER" \
	--chain-id "$CHAIN_ID" \
	--keyring-backend "$KEYRING" \
	--home "$HOME_DIR" \
	--yes \
	--fees "$TX_FEES" \
	--broadcast-mode sync \
	-o json)
echo "$DELEGATE_OK" | jq
DELEGATE_HASH=$(echo "$DELEGATE_OK" | jq -r '.txhash')
DELEGATE_CODE=$(wait_tx "$DELEGATE_HASH")
[ "$DELEGATE_CODE" = "0" ] || { echo "PQC-enabled delegation failed: code=$DELEGATE_CODE" >&2; exit 1; }

echo "==> PQC-disabled staking delegation should fail"
FAIL_LOG=$(mktemp)
if "$BIN" tx staking delegate "$VALOPER_ADDR" "$AMOUNT" \
	--from "$SENDER" \
	--pqc-enable=false \
	--chain-id "$CHAIN_ID" \
	--keyring-backend "$KEYRING" \
	--home "$HOME_DIR" \
	--yes \
	--fees "$TX_FEES" \
	--broadcast-mode sync >"$FAIL_LOG" 2>&1; then
	echo "error: PQC-disabled delegation unexpectedly succeeded" >&2
	cat "$FAIL_LOG" >&2
	exit 1
fi

if grep -qiE "pqc.*(missing|required|signature)" "$FAIL_LOG"; then
	echo "[OK] PQC-disabled delegation rejected with PQC error"
else
	echo "warning: PQC-disabled delegation failed without explicit PQC error" >&2
fi

echo "==> e2e_pqc succeeded"
echo "Summary:"
echo "  - Validator : $ADDR_VALIDATOR"
echo "  - Sender    : $ADDR_SENDER"
echo "  - Recipient : $ADDR_RECIPIENT"
