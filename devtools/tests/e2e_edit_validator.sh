#!/usr/bin/env bash
set -euo pipefail

DIR=$(cd "$(dirname "$0")/../.." && pwd)
BIN=${BIN:-"$DIR/build/lumend"}
: "${LUMEN_BUILD_TAGS:=dev}"
CHAIN_ID=${CHAIN_ID:-e2e-edit-validator}
KEYRING=${KEYRING:-test}
SCHEME=${SCHEME:-dilithium3}
TX_FEES=${TX_FEES:-0ulmn}

RPC_HOST=${RPC_HOST:-127.0.0.1}
if [ -z "${RPC_PORT:-}" ]; then
	RPC_PORT=$(( (RANDOM % 1000) + 30000 ))
fi
RPC="http://${RPC_HOST}:${RPC_PORT}"
NODE="tcp://${RPC_HOST}:${RPC_PORT}"

source "$DIR/devtools/tests/lib_pqc.sh"
pqc_require_bins

HOME_ROOT=$(mktemp -d -t e2e-edit-validator-XXXXXX)
HOME_DIR="$HOME_ROOT/.lumen"
LOG_FILE="${LOG_FILE:-$HOME_ROOT/lumend.log}"

cleanup() {
	pkill -f "lumend start" >/dev/null 2>&1 || true
	if [ "${DEBUG_KEEP:-0}" != "1" ]; then
		rm -rf "$HOME_ROOT" >/dev/null 2>&1 || true
	else
		echo "DEBUG_KEEP=1: keeping $HOME_ROOT"
		echo "  HOME_DIR=$HOME_DIR"
	fi
}
trap cleanup EXIT

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

if [ "${1:-}" = "--skip-build" ]; then
	SKIP_BUILD=1
	shift
else
	SKIP_BUILD=0
fi

if [ "$SKIP_BUILD" != "1" ]; then
	build_cmd=(go build -trimpath -ldflags "-s -w")
	if [ -n "$LUMEN_BUILD_TAGS" ]; then
		build_cmd+=(-tags "$LUMEN_BUILD_TAGS")
	fi
	build_cmd+=(-o "$BIN" ./cmd/lumend)
	(cd "$DIR" && "${build_cmd[@]}")
fi

BOOTSTRAP="bootstrap"

echo "==> Init single-node chain (edit-validator e2e)"
"$BIN" init edit-validator --chain-id "$CHAIN_ID" --home "$HOME_DIR" >/dev/null

printf '\n' | "$BIN" keys add "$BOOTSTRAP" --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null 2>&1 || true
ADDR_BOOTSTRAP=$("$BIN" keys show "$BOOTSTRAP" -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
VALOPER_BOOTSTRAP=$("$BIN" keys show "$BOOTSTRAP" --bech val --address --keyring-backend "$KEYRING" --home "$HOME_DIR")

"$BIN" genesis add-genesis-account "$ADDR_BOOTSTRAP" 200000000ulmn --keyring-backend "$KEYRING" --home "$HOME_DIR"
"$BIN" genesis gentx "$BOOTSTRAP" 1000000ulmn \
	--chain-id "$CHAIN_ID" \
	--keyring-backend "$KEYRING" \
	--home "$HOME_DIR" >/dev/null
"$BIN" genesis collect-gentxs --home "$HOME_DIR" >/dev/null
"$BIN" genesis validate --home "$HOME_DIR" >/dev/null

# Ensure client.toml points to our throwaway node.
pqc_set_client_config "$HOME_DIR" "$NODE" "$CHAIN_ID"

echo "==> Starting node"
(
	"$BIN" start \
		--home "$HOME_DIR" \
		--rpc.laddr "tcp://${RPC_HOST}:${RPC_PORT}" \
		--p2p.laddr "tcp://0.0.0.0:0" \
		--rpc.pprof_laddr "" \
		--api.enable=false \
		--grpc.enable=false \
		--grpc-web.enable=false \
		--minimum-gas-prices 0ulmn >"$LOG_FILE" 2>&1
) &
sleep 1

wait_ready 2

export BIN KEYRING HOME_DIR SCHEME TX_FEES NODE RPC CHAIN_ID

echo "==> Linking PQC signer on-chain (bootstrap)"
setup_pqc_signer "$BOOTSTRAP"

echo "==> tx staking edit-validator should update moniker on-chain"
EDIT_MONIKER="e2e-edited-moniker"
RES=$("$BIN" tx staking edit-validator \
	--from "$BOOTSTRAP" \
	--new-moniker "$EDIT_MONIKER" \
	--chain-id "$CHAIN_ID" \
	--keyring-backend "$KEYRING" \
	--home "$HOME_DIR" \
	--yes \
	--fees "$TX_FEES" \
	--broadcast-mode sync \
	-o json)
echo "$RES" | jq

if echo "$RES" | jq -r '.raw_log // ""' | grep -qi "missing pqc signature extension"; then
	echo "error: PQC extension missing for staking edit-validator" >&2
	exit 1
fi

CODE=$(echo "$RES" | jq -r '.code // 0')
if [ "$CODE" != "0" ]; then
	echo "staking edit-validator failed with code=$CODE" >&2
	exit 1
fi

HASH=$(echo "$RES" | jq -r '.txhash // empty')
if [ -z "$HASH" ] || [ "$HASH" = "null" ]; then
	echo "error: could not extract txhash for edit-validator" >&2
	exit 1
fi

TX_CODE=$(pqc_wait_tx "$HASH" "$RPC") || exit 1
if [ "$TX_CODE" != "0" ]; then
	echo "staking edit-validator failed post-commit with code=$TX_CODE (tx=$HASH)" >&2
	exit 1
fi

VAL_JSON=$("$BIN" q staking validator "$VALOPER_BOOTSTRAP" --node "$NODE" --home "$HOME_DIR" -o json)
ONCHAIN_MONIKER=$(echo "$VAL_JSON" | jq -r '.validator.description.moniker // .description.moniker // empty')
if [ "$ONCHAIN_MONIKER" != "$EDIT_MONIKER" ]; then
	echo "error: edit-validator did not update moniker (got=$ONCHAIN_MONIKER expected=$EDIT_MONIKER)" >&2
	exit 1
fi

echo "e2e_edit_validator OK"
