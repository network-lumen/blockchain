#!/usr/bin/env bash
set -euo pipefail

DIR=$(cd "$(dirname "$0")/../.." && pwd)
BIN=${BIN:-"$DIR/build/lumend"}
: "${LUMEN_BUILD_TAGS:=dev}"
CHAIN_ID=${CHAIN_ID:-pqc-txpaths-e2e}
KEYRING=${KEYRING:-test}
SCHEME=${SCHEME:-dilithium3}
TX_FEES=${TX_FEES:-0ulmn}

RPC_HOST=${RPC_HOST:-127.0.0.1}
if [ -z "${RPC_PORT:-}" ]; then
	RPC_PORT=$(( (RANDOM % 1000) + 30000 ))
fi
RPC="http://${RPC_HOST}:${RPC_PORT}"

source "$DIR/devtools/tests/lib_pqc.sh"
pqc_require_bins

HOME_ROOT=$(mktemp -d -t pqc-txpaths-XXXXXX)
HOME_DIR="$HOME_ROOT/.lumen"
LOG_FILE="${LOG_FILE:-$HOME/lumend-pqc-txpaths.log}"

# Return the current ulmn balance (as an integer string) for a given address.
get_ulmn_balance() {
	local addr="$1"
	local amt
	amt=$("$BIN" q bank balances "$addr" --home "$HOME_DIR" -o json 2>/dev/null \
		| jq -r '.balances[] | select(.denom=="ulmn") | .amount' 2>/dev/null \
		| head -n1)
	if [ -z "$amt" ] || [ "$amt" = "null" ]; then
		echo "0"
	else
		echo "$amt"
	fi
}

echo "==> lumend binary diagnostics (e2e_pqc_tx_paths)"
echo "BIN: $BIN"
if command -v sha256sum >/dev/null 2>&1; then
	SHA=$(
		sha256sum "$BIN" 2>/dev/null | awk '{print $1}'
	)
	echo "BIN sha256: ${SHA:-unknown}"
fi
VER=$("$BIN" version 2>/dev/null || echo "unknown")
echo "BIN version: $VER"
echo "staking create-validator (staking --help):"
"$BIN" tx staking --help 2>&1 | grep -i 'create-validator' || true
echo "staking create-validator help (first lines):"
"$BIN" tx staking create-validator --help 2>&1 | sed -n '1,8p' || true
echo

# Basic helpers ---------------------------------------------------------------

run_tx_ok_nowait() {
	local label="$1"; shift
	echo "==> $label"
	local res code
	res=$("$BIN" "$@" --broadcast-mode sync -o json)
	echo "$res" | jq

	# Never allow PQC policy errors on these paths.
	if echo "$res" | jq -r '.raw_log // ""' | grep -qi "missing pqc signature extension"; then
		echo "error: PQC extension missing for $label" >&2
		exit 1
	fi

	code=$(echo "$res" | jq -r '.code // 0')
	if [ "$code" != "0" ]; then
		echo "$label failed with code=$code" >&2
		exit 1
	fi

	# Caller is responsible for any follow-up querying.
}

run_tx_ok() {
	local label="$1"; shift
	echo "==> $label"
	local res code hash tx_code
	res=$("$BIN" "$@" --broadcast-mode sync -o json)
	echo "$res" | jq

	# Never allow PQC policy errors on these paths.
	if echo "$res" | jq -r '.raw_log // ""' | grep -qi "missing pqc signature extension"; then
		echo "error: PQC extension missing for $label" >&2
		exit 1
	fi

	code=$(echo "$res" | jq -r '.code // 0')
	if [ "$code" != "0" ]; then
		echo "$label failed with code=$code" >&2
		exit 1
	fi

	hash=$(echo "$res" | jq -r '.txhash // empty')
	if [ -z "$hash" ] || [ "$hash" = "null" ]; then
		echo "$label: could not extract txhash for commit verification" >&2
		exit 1
	fi

	if ! tx_code=$(pqc_wait_tx "$hash" "$RPC"); then
		echo "$label: timeout waiting for commit (tx=$hash)" >&2
		exit 1
	fi
	if [ "$tx_code" != "0" ]; then
		echo "$label failed post-commit with code=$tx_code (tx=$hash)" >&2
		exit 1
	fi
}

# Generic helper when we expect a specific business code (including non-zero),
# while still enforcing that PQC is injected. If expected_code is 0 we also
# wait for the tx to commit successfully.
run_tx_expect_code() {
	local label="$1"; shift
	local expected_code="$1"; shift

	echo "==> $label (expect code=$expected_code)"
	local res code
	res=$("$BIN" "$@" -o json || true)
	echo "$res" | jq

	if echo "$res" | jq -r '.raw_log // ""' | grep -qi "missing pqc signature extension"; then
		echo "error: PQC extension missing for $label" >&2
		exit 1
	fi

	code=$(echo "$res" | jq -r '.code // 0')
	if [ "$code" != "$expected_code" ]; then
		echo "$label: expected code=$expected_code, got code=$code" >&2
		exit 1
	fi
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

# Optional: reuse existing build
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

HELP_OUT=$("$BIN" tx --help 2>&1 || true)
if echo "$HELP_OUT" | grep -q "authz"; then
	echo "[error] authz command unexpectedly present in 'lumend tx --help'" >&2
	exit 1
fi
if echo "$HELP_OUT" | grep -q "feegrant"; then
	echo "[error] feegrant command unexpectedly present in 'lumend tx --help'" >&2
	exit 1
fi

echo "==> Init chain for PQC tx-path coverage"
"$BIN" init pqc-txpaths --chain-id "$CHAIN_ID" --home "$HOME_DIR" >/dev/null

BOOTSTRAP="bootstrap"
NEWVAL="validator2"
GRANTEE="grantee"
RECIPIENT="recipient"

printf '\n' | "$BIN" keys add "$BOOTSTRAP" --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null 2>&1 || true
printf '\n' | "$BIN" keys add "$NEWVAL"    --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null 2>&1 || true
printf '\n' | "$BIN" keys add "$GRANTEE"   --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null 2>&1 || true
printf '\n' | "$BIN" keys add "$RECIPIENT" --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null 2>&1 || true

ADDR_BOOTSTRAP=$("$BIN" keys show "$BOOTSTRAP" -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
ADDR_NEWVAL=$("$BIN" keys show "$NEWVAL" -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
VALOPER_NEWVAL=$("$BIN" keys show "$NEWVAL" --bech val --address --keyring-backend "$KEYRING" --home "$HOME_DIR")
VALOPER_BOOTSTRAP=$("$BIN" keys show "$BOOTSTRAP" --bech val --address --keyring-backend "$KEYRING" --home "$HOME_DIR")
ADDR_GRANTEE=$("$BIN" keys show "$GRANTEE" -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
ADDR_RECIPIENT=$("$BIN" keys show "$RECIPIENT" -a --keyring-backend "$KEYRING" --home "$HOME_DIR")

"$BIN" genesis add-genesis-account "$ADDR_BOOTSTRAP" 200000000ulmn --keyring-backend "$KEYRING" --home "$HOME_DIR"
"$BIN" genesis add-genesis-account "$ADDR_NEWVAL"    200000000ulmn --keyring-backend "$KEYRING" --home "$HOME_DIR"
"$BIN" genesis add-genesis-account "$ADDR_GRANTEE"   200000000ulmn --keyring-backend "$KEYRING" --home "$HOME_DIR"
"$BIN" genesis add-genesis-account "$ADDR_RECIPIENT"  10000000ulmn --keyring-backend "$KEYRING" --home "$HOME_DIR"

echo "==> Genesis gentx for bootstrap validator (non-PQC path, height <= 1)"
"$BIN" genesis gentx "$BOOTSTRAP" 1000000ulmn \
	--chain-id "$CHAIN_ID" \
	--keyring-backend "$KEYRING" \
	--home "$HOME_DIR" >/dev/null
"$BIN" genesis collect-gentxs --home "$HOME_DIR" >/dev/null
"$BIN" genesis validate --home "$HOME_DIR" >/dev/null

# Ensure client.toml points to our throwaway node.
pqc_set_client_config "$HOME_DIR" "tcp://${RPC_HOST}:${RPC_PORT}" "$CHAIN_ID"

echo "==> Starting node"
(
	"$BIN" start \
		--home "$HOME_DIR" \
		--rpc.laddr "tcp://${RPC_HOST}:${RPC_PORT}" \
		--p2p.laddr "tcp://0.0.0.0:0" \
		--rpc.pprof_laddr "" \
		--minimum-gas-prices 0ulmn >"$LOG_FILE" 2>&1
) &
sleep 1

wait_ready 2

export NODE="tcp://${RPC_HOST}:${RPC_PORT}"
export RPC
pqc_policy_must_be_required "$RPC"
export BIN KEYRING HOME_DIR SCHEME TX_FEES NODE RPC CHAIN_ID

echo "==> Linking PQC accounts on-chain (BOOTSTRAP, NEWVAL, GRANTEE)"
setup_pqc_signer "$BOOTSTRAP"
setup_pqc_signer "$NEWVAL"
setup_pqc_signer "$GRANTEE"

echo "==> tx staking create-validator (NEWVAL) should pass under PQC_REQUIRED (PQC-injected)"
# Generate a fresh consensus pubkey for NEWVAL so that the
# create-validator tx is fully valid (no duplicate pubkey) and
# committed with code=0.
PUBKEY_JSON=$(python3 - <<'PY'
import base64, os, json
pub = base64.b64encode(os.urandom(32)).decode()
print(json.dumps({"@type":"/cosmos.crypto.ed25519.PubKey","key":pub}))
PY
)

TMP_JSON=$(mktemp)
cat >"$TMP_JSON" <<EOF
{
  "pubkey": $PUBKEY_JSON,
  "amount": "1000000ulmn",
  "moniker": "pqc-txpaths-newval",
  "identity": "",
  "website": "",
  "security": "",
  "details": "",
  "commission-rate": "0.1",
  "commission-max-rate": "0.2",
  "commission-max-change-rate": "0.01",
  "min-self-delegation": "1"
}
EOF

# Use broadcast-mode=sync and then wait for the tx to commit so that we
# validate both PQC injection and the actual on-chain result.
CREATE_OK=$("$BIN" tx staking create-validator "$TMP_JSON" \
	--from "$NEWVAL" \
	--chain-id "$CHAIN_ID" \
	--keyring-backend "$KEYRING" \
	--home "$HOME_DIR" \
	--gas auto \
	--gas-adjustment 1.5 \
	--yes \
	--fees "$TX_FEES" \
	--broadcast-mode sync \
	-o json)
echo "$CREATE_OK" | jq

if echo "$CREATE_OK" | jq -r '.raw_log // ""' | grep -qi "missing pqc signature extension"; then
	echo "error: PQC extension missing for staking create-validator" >&2
	exit 1
fi

CREATE_CODE=$(echo "$CREATE_OK" | jq -r '.code // 0')
if [ "$CREATE_CODE" != "0" ]; then
	echo "staking create-validator (NEWVAL) failed with code=$CREATE_CODE" >&2
	exit 1
fi

CREATE_HASH=$(echo "$CREATE_OK" | jq -r '.txhash // empty')
if [ -z "$CREATE_HASH" ] || [ "$CREATE_HASH" = "null" ]; then
	echo "error: could not extract txhash for staking create-validator" >&2
	exit 1
fi

CREATE_COMMIT_CODE=$(pqc_wait_tx "$CREATE_HASH" "$RPC") || {
	echo "staking create-validator (NEWVAL): timeout waiting for commit (tx=$CREATE_HASH)" >&2
	exit 1
}
if [ "$CREATE_COMMIT_CODE" != "0" ]; then
	echo "staking create-validator (NEWVAL) failed post-commit with code=$CREATE_COMMIT_CODE" >&2
	exit 1
fi

echo "staking create-validator (NEWVAL) passed (business+commit code=0)"
echo "==> PQC-enabled staking delegate should pass"
run_tx_ok "staking delegate (self)" \
	tx staking delegate "$VALOPER_BOOTSTRAP" 1000000ulmn \
	--from "$NEWVAL" \
	--chain-id "$CHAIN_ID" \
	--keyring-backend "$KEYRING" \
	--home "$HOME_DIR" \
	--yes \
	--fees "$TX_FEES"

echo "==> PQC-enabled bank send should pass"
run_tx_ok_nowait "bank send BOOTSTRAP -> RECIPIENT" \
	tx bank send "$BOOTSTRAP" "$ADDR_RECIPIENT" 5000000ulmn \
	--chain-id "$CHAIN_ID" \
	--keyring-backend "$KEYRING" \
	--home "$HOME_DIR" \
	--yes \
	--fees "$TX_FEES"

echo "==> PQC-enabled bank multi-send should pass"
# Track recipient balance before and after to verify on-chain effect,
# instead of relying on the tx index for this path.
BAL_BEFORE_MS=$(get_ulmn_balance "$ADDR_RECIPIENT")
run_tx_ok_nowait "bank multi-send GRANTEE -> RECIPIENT,GRANTEE" \
	tx bank multi-send "$GRANTEE" "$ADDR_RECIPIENT" "$ADDR_GRANTEE" 1000000ulmn \
	--chain-id "$CHAIN_ID" \
	--keyring-backend "$KEYRING" \
	--home "$HOME_DIR" \
	--yes \
	--fees "$TX_FEES"

TARGET_MS=$((BAL_BEFORE_MS + 1000000))
for _ in $(seq 1 120); do
	BAL_AFTER_MS=$(get_ulmn_balance "$ADDR_RECIPIENT")
	if [ "$BAL_AFTER_MS" -ge "$TARGET_MS" ] 2>/dev/null; then
		echo "bank multi-send recipient balance OK (before=$BAL_BEFORE_MS after=$BAL_AFTER_MS)"
		break
	fi
	sleep 0.5
done
if [ "$BAL_AFTER_MS" -lt "$TARGET_MS" ] 2>/dev/null; then
	echo "bank multi-send did not update recipient balance as expected (before=$BAL_BEFORE_MS after=$BAL_AFTER_MS target>=$TARGET_MS)" >&2
	exit 1
fi

echo "==> Distribution withdraw-rewards / withdraw-all-rewards should be PQC-safe"
run_tx_ok "distribution withdraw-rewards" \
	tx distribution withdraw-rewards "$VALOPER_NEWVAL" \
	--from "$NEWVAL" \
	--chain-id "$CHAIN_ID" \
	--keyring-backend "$KEYRING" \
	--home "$HOME_DIR" \
	--yes \
	--fees "$TX_FEES"

run_tx_ok "distribution withdraw-all-rewards (NEWVAL)" \
	tx distribution withdraw-all-rewards \
	--from "$NEWVAL" \
	--chain-id "$CHAIN_ID" \
	--keyring-backend "$KEYRING" \
	--home "$HOME_DIR" \
	--yes \
	--fees "$TX_FEES"

echo "==> Distribution set-withdraw-addr and withdraw-validator-commission should be PQC-safe"
run_tx_ok "distribution set-withdraw-addr" \
	tx distribution set-withdraw-addr "$ADDR_RECIPIENT" \
	--from "$NEWVAL" \
	--chain-id "$CHAIN_ID" \
	--keyring-backend "$KEYRING" \
	--home "$HOME_DIR" \
	--yes \
	--fees "$TX_FEES"

run_tx_ok "distribution withdraw-validator-commission" \
	tx distribution withdraw-validator-commission "$VALOPER_NEWVAL" \
	--from "$NEWVAL" \
	--chain-id "$CHAIN_ID" \
	--keyring-backend "$KEYRING" \
	--home "$HOME_DIR" \
	--yes \
	--fees "$TX_FEES"

echo "==> Governance legacy text proposal + deposit + vote should be PQC-safe"
PROPOSAL_DEPOSIT=10000000ulmn
PROPOSAL_ID=1

run_tx_ok "gov submit-legacy-proposal" \
	tx gov submit-legacy-proposal \
	--from "$NEWVAL" \
	--title "PQC TX-paths test" \
	--description "text proposal for PQC tx-path coverage" \
	--type "text" \
	--deposit "$PROPOSAL_DEPOSIT" \
	--chain-id "$CHAIN_ID" \
	--keyring-backend "$KEYRING" \
	--home "$HOME_DIR" \
	--yes \
	--fees "$TX_FEES"

run_tx_ok "gov deposit" \
	tx gov deposit "$PROPOSAL_ID" "$PROPOSAL_DEPOSIT" \
	--from "$NEWVAL" \
	--chain-id "$CHAIN_ID" \
	--keyring-backend "$KEYRING" \
	--home "$HOME_DIR" \
	--yes \
	--fees "$TX_FEES"

run_tx_ok "gov vote yes" \
	tx gov vote "$PROPOSAL_ID" yes \
	--from "$NEWVAL" \
	--chain-id "$CHAIN_ID" \
	--keyring-backend "$KEYRING" \
	--home "$HOME_DIR" \
	--yes \
	--fees "$TX_FEES"

echo "==> Additional staking flows (edit-validator / unbond / redelegate / cancel-unbond) should be PQC-safe"
run_tx_ok "staking edit-validator" \
	tx staking edit-validator \
	--from "$NEWVAL" \
	--moniker "pqc-txpaths-edited" \
	--chain-id "$CHAIN_ID" \
	--keyring-backend "$KEYRING" \
	--home "$HOME_DIR" \
	--yes \
	--fees "$TX_FEES"

run_tx_ok "staking unbond" \
	tx staking unbond "$VALOPER_BOOTSTRAP" 500000ulmn \
	--from "$NEWVAL" \
	--chain-id "$CHAIN_ID" \
	--keyring-backend "$KEYRING" \
	--home "$HOME_DIR" \
	--yes \
	--fees "$TX_FEES"

run_tx_ok "staking redelegate" \
	tx staking redelegate "$VALOPER_BOOTSTRAP" "$VALOPER_NEWVAL" 100000ulmn \
	--from "$NEWVAL" \
	--chain-id "$CHAIN_ID" \
	--keyring-backend "$KEYRING" \
	--home "$HOME_DIR" \
	--yes \
	--fees "$TX_FEES"

#
# For cancel-unbond we first fetch the actual unbonding entry for NEWVAL so
# that we pass the correct creation height instead of a hard-coded value.
#
UNBOND_JSON=$("$BIN" q staking unbonding-delegation "$ADDR_NEWVAL" "$VALOPER_BOOTSTRAP" --home "$HOME_DIR" -o json || echo "")
UNBOND_HEIGHT=$(echo "$UNBOND_JSON" | jq -r '.unbonding_responses[0].entries[0].creation_height // empty')
if [ -z "$UNBOND_HEIGHT" ] || [ "$UNBOND_HEIGHT" = "null" ]; then
	echo "error: could not discover unbonding creation height for cancel-unbond" >&2
	exit 1
fi

run_tx_ok "staking cancel-unbond" \
	tx staking cancel-unbond "$VALOPER_BOOTSTRAP" 100000ulmn "$UNBOND_HEIGHT" \
	--from "$NEWVAL" \
	--chain-id "$CHAIN_ID" \
	--keyring-backend "$KEYRING" \
	--home "$HOME_DIR" \
	--yes \
	--fees "$TX_FEES"

echo "==> Slashing unjail CLI should return \"validator not jailed\" but still include PQC"
run_tx_expect_code "slashing unjail (not jailed)" 2 \
	tx slashing unjail \
	--from "$NEWVAL" \
	--chain-id "$CHAIN_ID" \
	--keyring-backend "$KEYRING" \
	--home "$HOME_DIR" \
	--yes \
	--fees "$TX_FEES"

echo "e2e_pqc_tx_paths completed successfully"
