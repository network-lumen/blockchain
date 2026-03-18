#!/usr/bin/env bash
set -euo pipefail

SOURCE_DIR="$(cd "$(dirname "$0")" && pwd)"
. "${SOURCE_DIR}/lib_pqc.sh"

DIR="$(cd "${SOURCE_DIR}/../.." && pwd)"
BIN="${BIN:-"$DIR/build/lumend"}"
A_CHAIN_ID="${A_CHAIN_ID:-lumen-ibc-a-1}"
B_CHAIN_ID="${B_CHAIN_ID:-lumen-ibc-b-1}"
RELAYER_IMAGE="${RELAYER_IMAGE:-ghcr.io/cosmos/relayer:v2.5.2}"
DOCKER_HOST_ALIAS="${DOCKER_HOST_ALIAS:-host.docker.internal}"
KEYRING="${KEYRING:-test}"
USER_TX_FEES="${USER_TX_FEES:-1000ulmn}"
RELAYER_GAS_PRICES="${RELAYER_GAS_PRICES:-0.01ulmn}"
ZERO_FEE_GAS_PRICES="${ZERO_FEE_GAS_PRICES:-0ulmn}"
RELAYER_KEY_NAME_GOOD="${RELAYER_KEY_NAME_GOOD:-relayer-good}"
RELAYER_KEY_NAME_BAD="${RELAYER_KEY_NAME_BAD:-relayer-bad}"
RELAYER_PATH_NAME="${RELAYER_PATH_NAME:-lumen-ibc}"
RELAYER_CHAIN_A="${RELAYER_CHAIN_A:-$A_CHAIN_ID}"
RELAYER_CHAIN_B="${RELAYER_CHAIN_B:-$B_CHAIN_ID}"
TRANSFER_AMOUNT_A_TO_B="${TRANSFER_AMOUNT_A_TO_B:-25000}"
TRANSFER_AMOUNT_B_TO_A="${TRANSFER_AMOUNT_B_TO_A:-10000}"
TIMEOUT_TRANSFER_AMOUNT="${TIMEOUT_TRANSFER_AMOUNT:-7000}"

SKIP_BUILD="${SKIP_BUILD:-0}"
if [ "${1:-}" = "--skip-build" ]; then
	SKIP_BUILD=1
	shift
fi

require_bin() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "error: missing dependency '$1'" >&2
		exit 1
	fi
}

pqc_require_bins
require_bin docker
require_bin timeout

if [ -z "${E2E_BASE_PORT:-}" ]; then
	E2E_BASE_PORT=$(( (RANDOM % 1000) + 32000 ))
fi

declare -A HOME_DIR CHAIN_ID MONIKER RPC_PORT API_PORT GRPC_PORT P2P_PORT RPC_URL API_URL RPC_LADDR API_ADDR GRPC_ADDR P2P_LADDR LOG_FILE RELAYER_RPC_URL
declare -A ADDR_VALIDATOR ADDR_USER ADDR_RELAYER_GOOD ADDR_RELAYER_BAD

CHAIN_ID[a]="$A_CHAIN_ID"
CHAIN_ID[b]="$B_CHAIN_ID"
MONIKER[a]="ibc-a"
MONIKER[b]="ibc-b"
RPC_PORT[a]="$E2E_BASE_PORT"
API_PORT[a]="$((E2E_BASE_PORT + 60))"
GRPC_PORT[a]="$((E2E_BASE_PORT + 120))"
P2P_PORT[a]="$((E2E_BASE_PORT + 180))"
RPC_PORT[b]="$((E2E_BASE_PORT + 500))"
API_PORT[b]="$((E2E_BASE_PORT + 560))"
GRPC_PORT[b]="$((E2E_BASE_PORT + 620))"
P2P_PORT[b]="$((E2E_BASE_PORT + 680))"

for chain in a b; do
	RPC_URL[$chain]="http://127.0.0.1:${RPC_PORT[$chain]}"
	RELAYER_RPC_URL[$chain]="http://${DOCKER_HOST_ALIAS}:${RPC_PORT[$chain]}"
	API_URL[$chain]="http://127.0.0.1:${API_PORT[$chain]}"
	RPC_LADDR[$chain]="tcp://127.0.0.1:${RPC_PORT[$chain]}"
	API_ADDR[$chain]="tcp://127.0.0.1:${API_PORT[$chain]}"
	GRPC_ADDR[$chain]="127.0.0.1:${GRPC_PORT[$chain]}"
	P2P_LADDR[$chain]="tcp://0.0.0.0:${P2P_PORT[$chain]}"
done

WORK_ROOT="$(mktemp -d -t lumen-e2e-ibc-XXXXXX)"
ARTIFACT_DIR="${ARTIFACT_DIR:-$DIR/artifacts/test-logs}"
mkdir -p "$ARTIFACT_DIR"
HOME_DIR[a]="$WORK_ROOT/chain-a"
HOME_DIR[b]="$WORK_ROOT/chain-b"
LOG_FILE[a]="$ARTIFACT_DIR/e2e_ibc_chain_a.log"
LOG_FILE[b]="$ARTIFACT_DIR/e2e_ibc_chain_b.log"
RELAYER_GOOD_HOME="$WORK_ROOT/relayer-good"
RELAYER_BAD_HOME="$WORK_ROOT/relayer-bad"
RELAYER_ZERO_HOME="$WORK_ROOT/relayer-zero-fee"

NODE_PIDS=()

VALIDATOR_KEY="validator"
USER_KEY="user"
USER_PQC_KEY="pqc-user"

VAL_MNEMONIC_A="alcohol hockey chair click sword crumble outside cash old example wealth ozone rice cash because friend holiday dinner endless poem dog royal tiny profit"
USER_MNEMONIC_A="exchange control olive wool aim seek double bamboo tell process sock door resist uncle grunt reform knock chair agent dad snake oven captain destroy"
GOOD_RELAYER_MNEMONIC_A="village foil behind logic hand fitness bronze push turn undo chalk symbol elbow amazing kitten creek trip game intact square solid coach stock tomato"
BAD_RELAYER_MNEMONIC_A="any record tool own rabbit same wage crazy auction dose relief ten winter post parrot isolate bundle opera drum uphold voice select great donkey"

VAL_MNEMONIC_B="cost describe scatter destroy above mistake evoke angle raw oil humble clip trophy ride pottery summer limb devote slice cat manual hen follow tired"
USER_MNEMONIC_B="green trial plate resource moral skull sample entire demise hollow device accuse marble club hospital creek category topple lens cabbage add clump frost ticket"
GOOD_RELAYER_MNEMONIC_B="rally mountain couple deputy mango man divorce sound giant initial vague seat attract shine upon rabbit sign excess vanish clarify dust cube hurry assault"
BAD_RELAYER_MNEMONIC_B="blouse false twelve destroy bring appear skill erase cinnamon feature oppose physical kitchen school master avocado rival unit security syrup reflect album enhance elephant"

step() { printf '\n==== %s\n' "$*"; }
note() { printf -- '---- %s\n' "$*"; }

cleanup() {
	local code=$?
	for pid in "${NODE_PIDS[@]:-}"; do
		if [ -n "${pid:-}" ] && kill -0 "$pid" >/dev/null 2>&1; then
			kill "$pid" >/dev/null 2>&1 || true
			wait "$pid" >/dev/null 2>&1 || true
		fi
	done
	if [ "${DEBUG_KEEP:-0}" != "1" ]; then
		rm -rf "$WORK_ROOT" >/dev/null 2>&1 || true
	else
		echo "DEBUG_KEEP=1 -> preserving $WORK_ROOT"
	fi
	exit "$code"
}
trap cleanup EXIT

build_binary_if_needed() {
	if [ "$SKIP_BUILD" = "1" ]; then
		note "Skip build (SKIP_BUILD=1)"
		return
	fi

	step "Build lumend"
	build_cmd=(go build -trimpath -ldflags "-s -w")
	if [ -n "${LUMEN_BUILD_TAGS:-dev}" ]; then
		build_cmd+=(-tags "${LUMEN_BUILD_TAGS:-dev}")
	fi
	build_cmd+=(-o "$BIN" ./cmd/lumend)
	(cd "$DIR" && "${build_cmd[@]}")
}

add_key_from_mnemonic() {
	local home="$1"
	local name="$2"
	local mnemonic="$3"
	local mn_file
	mn_file="$(mktemp)"
	printf '%s' "$mnemonic" >"$mn_file"
	"$BIN" keys add "$name" --recover --source "$mn_file" --keyring-backend "$KEYRING" --home "$home" >/dev/null
	rm -f "$mn_file"
}

query_addr() {
	local home="$1"
	local name="$2"
	"$BIN" keys show "$name" -a --keyring-backend "$KEYRING" --home "$home"
}

json_balance_amount() {
	local json="$1"
	local denom="$2"
	echo "$json" | jq -r --arg denom "$denom" 'limit(1; .balances[]? | select(.denom == $denom) | .amount)'
}

query_balance_amount() {
	local chain="$1"
	local addr="$2"
	local denom="$3"
	local json amount
	json=$("$BIN" query bank balances "$addr" --node "${RPC_LADDR[$chain]}" -o json)
	amount=$(json_balance_amount "$json" "$denom")
	if [ -z "$amount" ]; then
		echo 0
	else
		echo "$amount"
	fi
}

query_first_ibc_balance() {
	local chain="$1"
	local addr="$2"
	"$BIN" query bank balances "$addr" --node "${RPC_LADDR[$chain]}" -o json \
		| jq -r 'limit(1; .balances[]? | select(.denom | startswith("ibc/")) | [.denom, .amount] | @tsv)'
}

expect_eq() {
	local actual="$1"
	local expected="$2"
	local label="$3"
	if [ "$actual" != "$expected" ]; then
		echo "error: $label expected=$expected actual=$actual" >&2
		exit 1
	fi
}

expect_lt() {
	local left="$1"
	local right="$2"
	local label="$3"
	if [ "$left" -ge "$right" ]; then
		echo "error: $label expected $left < $right" >&2
		exit 1
	fi
}

expect_contains() {
	local haystack="$1"
	local needle="$2"
	local label="$3"
	if ! grep -qi -- "$needle" <<<"$haystack"; then
		echo "error: $label missing '$needle'" >&2
		printf '%s\n' "$haystack" >&2
		exit 1
	fi
}

wait_http() {
	local url="$1"
	local log_file="$2"
	for _ in $(seq 1 240); do
		curl --connect-timeout 1 --max-time 2 -sSf "$url" >/dev/null 2>&1 && return 0
		sleep 0.5
	done
	echo "error: timeout waiting for $url" >&2
	tail -n 120 "$log_file" >&2 || true
	exit 1
}

current_height() {
	local chain="$1"
	curl -s "${RPC_URL[$chain]}/status" | jq -r '.result.sync_info.latest_block_height'
}

wait_height() {
	local chain="$1"
	local target="$2"
	for _ in $(seq 1 240); do
		local height
		height=$(current_height "$chain")
		if [ -n "$height" ] && [ "$height" != "null" ] && [ "$height" -ge "$target" ] 2>/dev/null; then
			return 0
		fi
		sleep 0.5
	done
	echo "error: timeout waiting for chain '$chain' height >= $target" >&2
	tail -n 120 "${LOG_FILE[$chain]}" >&2 || true
	exit 1
}

start_node() {
	local chain="$1"
	step "Start chain ${CHAIN_ID[$chain]}"
	"$BIN" start \
		--home "${HOME_DIR[$chain]}" \
		--rpc.laddr "${RPC_LADDR[$chain]}" \
		--rpc.pprof_laddr "" \
		--p2p.laddr "${P2P_LADDR[$chain]}" \
		--api.enable \
		--api.address "${API_ADDR[$chain]}" \
		--grpc.address "${GRPC_ADDR[$chain]}" \
		--minimum-gas-prices 0ulmn \
		>"${LOG_FILE[$chain]}" 2>&1 &
	NODE_PIDS+=("$!")
	wait_http "${RPC_URL[$chain]}/status" "${LOG_FILE[$chain]}"
	wait_http "${API_URL[$chain]}/" "${LOG_FILE[$chain]}"
	wait_height "$chain" 2
}

setup_chain() {
	local chain="$1"
	local val_mnemonic="$2"
	local user_mnemonic="$3"
	local good_relayer_mnemonic="$4"
	local bad_relayer_mnemonic="$5"

	step "Init ${CHAIN_ID[$chain]}"
	rm -rf "${HOME_DIR[$chain]}"
	"$BIN" init "${MONIKER[$chain]}" --chain-id "${CHAIN_ID[$chain]}" --home "${HOME_DIR[$chain]}" >/dev/null 2>&1
	if [ -f "${HOME_DIR[$chain]}/config/config.toml" ]; then
		sed -i.bak 's/^timeout_commit = .*/timeout_commit = "1s"/' "${HOME_DIR[$chain]}/config/config.toml" || true
	fi

	add_key_from_mnemonic "${HOME_DIR[$chain]}" "$VALIDATOR_KEY" "$val_mnemonic"
	add_key_from_mnemonic "${HOME_DIR[$chain]}" "$USER_KEY" "$user_mnemonic"
	add_key_from_mnemonic "${HOME_DIR[$chain]}" "$RELAYER_KEY_NAME_GOOD" "$good_relayer_mnemonic"
	add_key_from_mnemonic "${HOME_DIR[$chain]}" "$RELAYER_KEY_NAME_BAD" "$bad_relayer_mnemonic"

	ADDR_VALIDATOR[$chain]="$(query_addr "${HOME_DIR[$chain]}" "$VALIDATOR_KEY")"
	ADDR_USER[$chain]="$(query_addr "${HOME_DIR[$chain]}" "$USER_KEY")"
	ADDR_RELAYER_GOOD[$chain]="$(query_addr "${HOME_DIR[$chain]}" "$RELAYER_KEY_NAME_GOOD")"
	ADDR_RELAYER_BAD[$chain]="$(query_addr "${HOME_DIR[$chain]}" "$RELAYER_KEY_NAME_BAD")"

	"$BIN" genesis add-genesis-account "${ADDR_VALIDATOR[$chain]}" 250000000ulmn --keyring-backend "$KEYRING" --home "${HOME_DIR[$chain]}" >/dev/null 2>&1
	"$BIN" genesis add-genesis-account "${ADDR_USER[$chain]}" 100000000ulmn --keyring-backend "$KEYRING" --home "${HOME_DIR[$chain]}" >/dev/null 2>&1
	"$BIN" genesis add-genesis-account "${ADDR_RELAYER_GOOD[$chain]}" 50000000ulmn --keyring-backend "$KEYRING" --home "${HOME_DIR[$chain]}" >/dev/null 2>&1
	"$BIN" genesis add-genesis-account "${ADDR_RELAYER_BAD[$chain]}" 50000000ulmn --keyring-backend "$KEYRING" --home "${HOME_DIR[$chain]}" >/dev/null 2>&1

	"$BIN" keys pqc-generate --name "$USER_PQC_KEY" --link-from "$USER_KEY" --keyring-backend "$KEYRING" --home "${HOME_DIR[$chain]}" >/dev/null 2>&1
	"$BIN" keys pqc-genesis-entry --from "$USER_KEY" --pqc "$USER_PQC_KEY" --keyring-backend "$KEYRING" --home "${HOME_DIR[$chain]}" --write-genesis "${HOME_DIR[$chain]}/config/genesis.json" >/dev/null 2>&1

	"$BIN" genesis gentx "$VALIDATOR_KEY" 50000000ulmn --chain-id "${CHAIN_ID[$chain]}" --keyring-backend "$KEYRING" --home "${HOME_DIR[$chain]}" >/dev/null 2>&1
	"$BIN" genesis collect-gentxs --home "${HOME_DIR[$chain]}" >/dev/null 2>&1

	local tmp
	tmp="$(mktemp)"
	jq --arg allow "${ADDR_RELAYER_GOOD[$chain]}" '
		.app_state.pqc.params.ibc_relayer_allowlist = [$allow]
	' "${HOME_DIR[$chain]}/config/genesis.json" >"$tmp"
	mv "$tmp" "${HOME_DIR[$chain]}/config/genesis.json"

	"$BIN" genesis validate --home "${HOME_DIR[$chain]}" >/dev/null 2>&1
	pqc_set_client_config "${HOME_DIR[$chain]}" "${RPC_LADDR[$chain]}" "${CHAIN_ID[$chain]}"
}

relayer_cmd() {
	local home="$1"
	shift
	docker run --rm \
		--add-host "${DOCKER_HOST_ALIAS}:host-gateway" \
		-u "$(id -u):$(id -g)" \
		-v "$home":/home/relayer/.relayer \
		"$RELAYER_IMAGE" \
		rly --home /home/relayer/.relayer "$@"
}

relayer_cmd_timeout() {
	local seconds="$1"
	local home="$2"
	shift 2
	timeout "$seconds" docker run --rm \
		--add-host "${DOCKER_HOST_ALIAS}:host-gateway" \
		-u "$(id -u):$(id -g)" \
		-v "$home":/home/relayer/.relayer \
		"$RELAYER_IMAGE" \
		rly --home /home/relayer/.relayer "$@"
}

init_relayer_home() {
	local home="$1"
	local key_name="$2"
	local gas_prices="$3"
	local mnemonic_a="$4"
	local mnemonic_b="$5"

	rm -rf "$home"
	mkdir -p "$home"

	cat >"$home/chain-a.json" <<EOF
{
  "type": "cosmos",
  "value": {
    "key": "$key_name",
    "chain-id": "${CHAIN_ID[a]}",
    "rpc-addr": "${RELAYER_RPC_URL[a]}",
    "account-prefix": "lmn",
    "keyring-backend": "$KEYRING",
    "gas-adjustment": 1.5,
    "gas-prices": "$gas_prices",
    "min-gas-amount": 200000,
    "max-gas-amount": 1500000,
    "debug": true,
    "timeout": "20s",
    "output-format": "json",
    "sign-mode": "direct",
    "broadcast-mode": "single"
  }
}
EOF

	cat >"$home/chain-b.json" <<EOF
{
  "type": "cosmos",
  "value": {
    "key": "$key_name",
    "chain-id": "${CHAIN_ID[b]}",
    "rpc-addr": "${RELAYER_RPC_URL[b]}",
    "account-prefix": "lmn",
    "keyring-backend": "$KEYRING",
    "gas-adjustment": 1.5,
    "gas-prices": "$gas_prices",
    "min-gas-amount": 200000,
    "max-gas-amount": 1500000,
    "debug": true,
    "timeout": "20s",
    "output-format": "json",
    "sign-mode": "direct",
    "broadcast-mode": "single"
  }
}
EOF

	relayer_cmd "$home" config init >/dev/null
	relayer_cmd "$home" chains add --file /home/relayer/.relayer/chain-a.json "$RELAYER_CHAIN_A" >/dev/null
	relayer_cmd "$home" chains add --file /home/relayer/.relayer/chain-b.json "$RELAYER_CHAIN_B" >/dev/null
	relayer_cmd "$home" paths new "$RELAYER_CHAIN_A" "$RELAYER_CHAIN_B" "$RELAYER_PATH_NAME" >/dev/null
	relayer_cmd "$home" keys restore "$RELAYER_CHAIN_A" "$key_name" "$mnemonic_a" >/dev/null
	relayer_cmd "$home" keys restore "$RELAYER_CHAIN_B" "$key_name" "$mnemonic_b" >/dev/null
}

expect_failure_output() {
	set +e
	local output
	output=$("$@" 2>&1)
	local code=$?
	set -e
	if [ "$code" -eq 0 ]; then
		echo "error: command unexpectedly succeeded: $*" >&2
		printf '%s\n' "$output" >&2
		exit 1
	fi
	printf '%s\n' "$output"
}

run_ibc_transfer() {
	local chain="$1"
	local channel="$2"
	local receiver="$3"
	local amount="$4"
	shift 4
	"$BIN" tx ibc-transfer transfer "$channel" "$receiver" "$amount" \
		--from "$USER_KEY" \
		--home "${HOME_DIR[$chain]}" \
		--keyring-backend "$KEYRING" \
		--chain-id "${CHAIN_ID[$chain]}" \
		--node "${RPC_LADDR[$chain]}" \
		--pqc-from "${ADDR_USER[$chain]}" \
		--pqc-key "$USER_PQC_KEY" \
		--yes \
		--broadcast-mode sync \
		--output json \
		"$@"
}

first_json_value() {
	local key="$1"
	jq -r --arg key "$key" 'limit(1; .. | objects | .[$key]? // empty)'
}

relayer_query_client_id() {
	local chain_name="$1"
	relayer_cmd "$RELAYER_GOOD_HOME" query clients "$chain_name" -o json | first_json_value client_id
}

relayer_query_connection_id() {
	local chain_name="$1"
	relayer_cmd "$RELAYER_GOOD_HOME" query connections "$chain_name" -o json \
		| jq -r 'limit(1; .. | objects | (.id? // .connection_id? // empty) | select(test("^connection-")) )'
}

relayer_query_channel_id() {
	local chain_name="$1"
	relayer_cmd "$RELAYER_GOOD_HOME" query channels "$chain_name" -o json \
		| jq -r 'limit(1; .. | objects | .channel_id? // empty | select(test("^channel-")) )'
}

relayer_query_state() {
	local kind="$1"
	local chain_name="$2"
	local object_id="$3"
	local port_id="${4:-}"
	if [ "$kind" = "channel" ]; then
		relayer_cmd "$RELAYER_GOOD_HOME" query channel "$chain_name" "$object_id" "$port_id" -o json | first_json_value state
	else
		relayer_cmd "$RELAYER_GOOD_HOME" query connection "$chain_name" "$object_id" -o json | first_json_value state
	fi
}

assert_open_state() {
	local value="$1"
	local label="$2"
	case "$value" in
		OPEN|STATE_OPEN|3) ;;
		*)
			echo "error: $label not open (state=$value)" >&2
			exit 1
			;;
	esac
}

extract_txhash() {
	echo "$1" | jq -r '.txhash // .TxHash // empty'
}

build_binary_if_needed

step "Verify IBC transfer CLI wrapper"
"$BIN" tx ibc-transfer transfer --help >/dev/null

step "Verify IBC query CLI wrappers"
"$BIN" query ibc --help >/dev/null
"$BIN" query ibc channelv2 --help >/dev/null
"$BIN" query ibc-transfer --help >/dev/null

step "Verify IBC tx CLI wrappers"
"$BIN" tx ibc --help >/dev/null
"$BIN" tx ibc client --help >/dev/null
"$BIN" tx ibc channelv2 --help >/dev/null

setup_chain a "$VAL_MNEMONIC_A" "$USER_MNEMONIC_A" "$GOOD_RELAYER_MNEMONIC_A" "$BAD_RELAYER_MNEMONIC_A"
setup_chain b "$VAL_MNEMONIC_B" "$USER_MNEMONIC_B" "$GOOD_RELAYER_MNEMONIC_B" "$BAD_RELAYER_MNEMONIC_B"

start_node a
start_node b

step "Prepare relayer homes"
init_relayer_home "$RELAYER_BAD_HOME" "$RELAYER_KEY_NAME_BAD" "$RELAYER_GAS_PRICES" "$BAD_RELAYER_MNEMONIC_A" "$BAD_RELAYER_MNEMONIC_B"
init_relayer_home "$RELAYER_ZERO_HOME" "$RELAYER_KEY_NAME_GOOD" "$ZERO_FEE_GAS_PRICES" "$GOOD_RELAYER_MNEMONIC_A" "$GOOD_RELAYER_MNEMONIC_B"
init_relayer_home "$RELAYER_GOOD_HOME" "$RELAYER_KEY_NAME_GOOD" "$RELAYER_GAS_PRICES" "$GOOD_RELAYER_MNEMONIC_A" "$GOOD_RELAYER_MNEMONIC_B"

step "Reject relayer tx when signer is not PQC-allowlisted"
BAD_OUTPUT=$(expect_failure_output relayer_cmd_timeout 25 "$RELAYER_BAD_HOME" transact client "$RELAYER_CHAIN_A" "$RELAYER_CHAIN_B" "$RELAYER_PATH_NAME")
expect_contains "$BAD_OUTPUT" "pqc" "bad relayer rejection"

step "Reject relayer tx when IBC fee is zero"
ZERO_OUTPUT=$(expect_failure_output relayer_cmd_timeout 25 "$RELAYER_ZERO_HOME" transact client "$RELAYER_CHAIN_A" "$RELAYER_CHAIN_B" "$RELAYER_PATH_NAME")
expect_contains "$ZERO_OUTPUT" "positive ulmn fee" "zero-fee relayer rejection"

RELAYER_A_START=$(query_balance_amount a "${ADDR_RELAYER_GOOD[a]}" ulmn)
RELAYER_B_START=$(query_balance_amount b "${ADDR_RELAYER_GOOD[b]}" ulmn)

step "Create IBC clients"
relayer_cmd "$RELAYER_GOOD_HOME" transact clients "$RELAYER_PATH_NAME"
sleep 1
A_CLIENT_ID="07-tendermint-0"
B_CLIENT_ID="07-tendermint-0"
note "A client: $A_CLIENT_ID"
note "B client: $B_CLIENT_ID"

step "Force explicit MsgUpdateClient on both chains"
TARGET_A_UPDATE=$(( $(current_height a) + 2 ))
TARGET_B_UPDATE=$(( $(current_height b) + 2 ))
wait_height a "$TARGET_A_UPDATE"
wait_height b "$TARGET_B_UPDATE"
relayer_cmd "$RELAYER_GOOD_HOME" transact update-clients "$RELAYER_PATH_NAME"

step "Open connection"
relayer_cmd "$RELAYER_GOOD_HOME" transact connection "$RELAYER_PATH_NAME"
A_CONNECTION_ID="connection-0"
B_CONNECTION_ID="connection-0"
assert_open_state "$(relayer_query_state connection "$RELAYER_CHAIN_A" "$A_CONNECTION_ID")" "source connection"
assert_open_state "$(relayer_query_state connection "$RELAYER_CHAIN_B" "$B_CONNECTION_ID")" "destination connection"
note "A connection: $A_CONNECTION_ID"
note "B connection: $B_CONNECTION_ID"

step "Open transfer channel"
relayer_cmd "$RELAYER_GOOD_HOME" transact channel "$RELAYER_PATH_NAME"
A_CHANNEL_ID="channel-0"
B_CHANNEL_ID="channel-0"
assert_open_state "$(relayer_query_state channel "$RELAYER_CHAIN_A" "$A_CHANNEL_ID" transfer)" "source transfer channel"
assert_open_state "$(relayer_query_state channel "$RELAYER_CHAIN_B" "$B_CHANNEL_ID" transfer)" "destination transfer channel"
note "A channel: $A_CHANNEL_ID"
note "B channel: $B_CHANNEL_ID"

step "Verify IBC query commands against the live chains"
CLIENT_STATES_JSON=$("$BIN" query ibc client states --node "${RPC_LADDR[a]}" -o json)
expect_contains "$CLIENT_STATES_JSON" "\"client_id\":\"$A_CLIENT_ID\"" "ibc client states query"

CONNECTIONS_JSON=$("$BIN" query ibc connection connections --node "${RPC_LADDR[a]}" -o json)
expect_contains "$CONNECTIONS_JSON" "\"id\":\"$A_CONNECTION_ID\"" "ibc connection query"

CHANNELS_JSON=$("$BIN" query ibc channel channels --node "${RPC_LADDR[a]}" -o json)
expect_contains "$CHANNELS_JSON" "\"channel_id\":\"$A_CHANNEL_ID\"" "ibc channel query"
expect_contains "$CHANNELS_JSON" "\"port_id\":\"transfer\"" "ibc transfer channel query"

TRANSFER_PARAMS_JSON=$("$BIN" query ibc-transfer params --node "${RPC_LADDR[a]}" -o json)
expect_contains "$TRANSFER_PARAMS_JSON" "send_enabled" "ibc-transfer params query"

ESCROW_ADDR=$("$BIN" query ibc-transfer escrow-address transfer "$A_CHANNEL_ID" --node "${RPC_LADDR[a]}" | tr -d '\r')
case "$ESCROW_ADDR" in
	lmn*) ;;
	*)
		echo "error: expected lmn escrow address, got $ESCROW_ADDR" >&2
		exit 1
		;;
esac

step "Smoke test tx ibc client generate-only flow"
IBC_CLIENT_TX_GEN_JSON=$("$BIN" tx ibc client delete-client-creator "$A_CLIENT_ID" \
	--from "$RELAYER_KEY_NAME_GOOD" \
	--home "${HOME_DIR[a]}" \
	--keyring-backend "$KEYRING" \
	--chain-id "${CHAIN_ID[a]}" \
	--node "${RPC_LADDR[a]}" \
	--fees "$USER_TX_FEES" \
	--generate-only \
	--output json)
expect_contains "$IBC_CLIENT_TX_GEN_JSON" "/ibc.core.client.v1.MsgDeleteClientCreator" "tx ibc client generate-only"

RELAYER_A_AFTER_HANDSHAKE=$(query_balance_amount a "${ADDR_RELAYER_GOOD[a]}" ulmn)
RELAYER_B_AFTER_HANDSHAKE=$(query_balance_amount b "${ADDR_RELAYER_GOOD[b]}" ulmn)
expect_lt "$RELAYER_A_AFTER_HANDSHAKE" "$RELAYER_A_START" "relayer A balance after handshake"
expect_lt "$RELAYER_B_AFTER_HANDSHAKE" "$RELAYER_B_START" "relayer B balance after handshake"

step "Reject MsgTransfer without fee"
TRANSFER_NO_FEE_OUTPUT=$(expect_failure_output run_ibc_transfer a "$A_CHANNEL_ID" "${ADDR_USER[b]}" "${TRANSFER_AMOUNT_A_TO_B}ulmn" --fees 0ulmn)
expect_contains "$TRANSFER_NO_FEE_OUTPUT" "positive ulmn fee" "zero-fee ibc transfer rejection"

step "Reject MsgTransfer without PQC signature"
TRANSFER_NO_PQC_OUTPUT=$(expect_failure_output "$BIN" tx ibc-transfer transfer "$A_CHANNEL_ID" "${ADDR_USER[b]}" "${TRANSFER_AMOUNT_A_TO_B}ulmn" \
	--from "$USER_KEY" \
	--home "${HOME_DIR[a]}" \
	--keyring-backend "$KEYRING" \
	--chain-id "${CHAIN_ID[a]}" \
	--node "${RPC_LADDR[a]}" \
	--fees "$USER_TX_FEES" \
	--pqc-enable=false \
	--yes \
	--broadcast-mode sync \
	--output json)
expect_contains "$TRANSFER_NO_PQC_OUTPUT" "pqc" "no-pqc ibc transfer rejection"

A_USER_START=$(query_balance_amount a "${ADDR_USER[a]}" ulmn)

step "Relay A->B ICS-20 transfer"
A_TO_B_TX=$(run_ibc_transfer a "$A_CHANNEL_ID" "${ADDR_USER[b]}" "${TRANSFER_AMOUNT_A_TO_B}ulmn" --fees "$USER_TX_FEES")
note "A->B txhash: $(extract_txhash "$A_TO_B_TX")"
relayer_cmd "$RELAYER_GOOD_HOME" transact flush "$RELAYER_PATH_NAME" "$A_CHANNEL_ID"

EXPECTED_A_AFTER_FIRST=$(( A_USER_START - TRANSFER_AMOUNT_A_TO_B - 1000 ))
ACTUAL_A_AFTER_FIRST=$(query_balance_amount a "${ADDR_USER[a]}" ulmn)
expect_eq "$ACTUAL_A_AFTER_FIRST" "$EXPECTED_A_AFTER_FIRST" "source native balance after A->B transfer"

FIRST_IBC_BALANCE=$(query_first_ibc_balance b "${ADDR_USER[b]}")
[ -n "$FIRST_IBC_BALANCE" ] || { echo "error: destination missing IBC voucher after A->B transfer" >&2; exit 1; }
IBC_DENOM_ON_B=$(printf '%s\n' "$FIRST_IBC_BALANCE" | cut -f1)
IBC_AMOUNT_ON_B=$(printf '%s\n' "$FIRST_IBC_BALANCE" | cut -f2)
expect_eq "$IBC_AMOUNT_ON_B" "$TRANSFER_AMOUNT_A_TO_B" "destination voucher amount after A->B transfer"
case "$IBC_DENOM_ON_B" in
	ibc/*) ;;
	*)
		echo "error: expected IBC voucher denom after A->B transfer, got $IBC_DENOM_ON_B" >&2
		exit 1
		;;
	esac

DENOM_HASH_JSON=$("$BIN" query ibc-transfer denom-hash "transfer/$A_CHANNEL_ID/ulmn" --node "${RPC_LADDR[b]}" -o json)
DENOM_HASH_ON_B=$(echo "$DENOM_HASH_JSON" | jq -r '.hash // .denom_hash // empty')
[ -n "$DENOM_HASH_ON_B" ] || { echo "error: missing denom hash from ibc-transfer denom-hash query" >&2; exit 1; }

expect_eq "${IBC_DENOM_ON_B#ibc/}" "$DENOM_HASH_ON_B" "destination voucher hash matches ibc-transfer denom-hash"

DENOM_TRACE_JSON=$("$BIN" query ibc-transfer denom "$IBC_DENOM_ON_B" --node "${RPC_LADDR[b]}" -o json)
DENOM_TRACE_BASE=$(echo "$DENOM_TRACE_JSON" | jq -r '.denom.base // .denom.base_denom // .base_denom // empty')
DENOM_TRACE_PORT=$(echo "$DENOM_TRACE_JSON" | jq -r '.denom.trace[0].port_id // empty')
DENOM_TRACE_CHANNEL=$(echo "$DENOM_TRACE_JSON" | jq -r '.denom.trace[0].channel_id // empty')
expect_eq "$DENOM_TRACE_BASE" "ulmn" "ibc-transfer denom trace base denom"
expect_eq "$DENOM_TRACE_PORT" "transfer" "ibc-transfer denom trace port"
expect_eq "$DENOM_TRACE_CHANNEL" "$A_CHANNEL_ID" "ibc-transfer denom trace channel"

step "Relay B->A return transfer to unescrow native funds"
B_USER_START=$(query_balance_amount b "${ADDR_USER[b]}" ulmn)
B_TO_A_TX=$(run_ibc_transfer b "$B_CHANNEL_ID" "${ADDR_USER[a]}" "${TRANSFER_AMOUNT_B_TO_A}${IBC_DENOM_ON_B}" --fees "$USER_TX_FEES")
note "B->A txhash: $(extract_txhash "$B_TO_A_TX")"
relayer_cmd "$RELAYER_GOOD_HOME" transact flush "$RELAYER_PATH_NAME" "$B_CHANNEL_ID"

EXPECTED_B_AFTER_RETURN=$(( B_USER_START - 1000 ))
ACTUAL_B_AFTER_RETURN=$(query_balance_amount b "${ADDR_USER[b]}" ulmn)
expect_eq "$ACTUAL_B_AFTER_RETURN" "$EXPECTED_B_AFTER_RETURN" "destination native fee balance after return transfer"

EXPECTED_VOUCHER_AFTER_RETURN=$(( TRANSFER_AMOUNT_A_TO_B - TRANSFER_AMOUNT_B_TO_A ))
ACTUAL_VOUCHER_AFTER_RETURN=$(query_balance_amount b "${ADDR_USER[b]}" "$IBC_DENOM_ON_B")
expect_eq "$ACTUAL_VOUCHER_AFTER_RETURN" "$EXPECTED_VOUCHER_AFTER_RETURN" "remaining voucher amount after return transfer"

EXPECTED_A_AFTER_RETURN=$(( EXPECTED_A_AFTER_FIRST + TRANSFER_AMOUNT_B_TO_A ))
ACTUAL_A_AFTER_RETURN=$(query_balance_amount a "${ADDR_USER[a]}" ulmn)
expect_eq "$ACTUAL_A_AFTER_RETURN" "$EXPECTED_A_AFTER_RETURN" "source native balance after voucher return"

step "Trigger timeout on an open transfer channel"
DEST_TIMEOUT_HEIGHT=$(( $(current_height b) + 3 ))
TIMEOUT_HEIGHT="1-${DEST_TIMEOUT_HEIGHT}"
A_BEFORE_TIMEOUT=$(query_balance_amount a "${ADDR_USER[a]}" ulmn)
TIMEOUT_TX=$(run_ibc_transfer a "$A_CHANNEL_ID" "${ADDR_USER[b]}" "${TIMEOUT_TRANSFER_AMOUNT}ulmn" --fees "$USER_TX_FEES" --packet-timeout-height "$TIMEOUT_HEIGHT" --packet-timeout-seconds 0)
note "timeout txhash: $(extract_txhash "$TIMEOUT_TX")"
wait_height b "$((DEST_TIMEOUT_HEIGHT + 1))"
relayer_cmd "$RELAYER_GOOD_HOME" transact flush "$RELAYER_PATH_NAME" "$A_CHANNEL_ID"

EXPECTED_A_AFTER_TIMEOUT=$(( A_BEFORE_TIMEOUT - 1000 ))
ACTUAL_A_AFTER_TIMEOUT=$(query_balance_amount a "${ADDR_USER[a]}" ulmn)
expect_eq "$ACTUAL_A_AFTER_TIMEOUT" "$EXPECTED_A_AFTER_TIMEOUT" "source balance after MsgTimeout refund"

step "Confirm transfer channel close is rejected by ICS-20 application logic"
set +e
CHANNEL_CLOSE_OUTPUT=$(relayer_cmd "$RELAYER_GOOD_HOME" transact channel-close "$RELAYER_PATH_NAME" "$A_CHANNEL_ID" transfer 2>&1)
set -e
expect_contains "$CHANNEL_CLOSE_OUTPUT" "unable to be closed" "channel close rejection"

RELAYER_A_FINAL=$(query_balance_amount a "${ADDR_RELAYER_GOOD[a]}" ulmn)
RELAYER_B_FINAL=$(query_balance_amount b "${ADDR_RELAYER_GOOD[b]}" ulmn)
expect_lt "$RELAYER_A_FINAL" "$RELAYER_A_AFTER_HANDSHAKE" "relayer A balance after packet relay"
expect_lt "$RELAYER_B_FINAL" "$RELAYER_B_AFTER_HANDSHAKE" "relayer B balance after packet relay"

step "IBC e2e completed"
note "Covered operational txs: MsgCreateClient, MsgUpdateClient, connection open, channel open, MsgTransfer, MsgRecvPacket, MsgAcknowledgement, MsgTimeout"
note "Covered CLI surface: query ibc, query ibc-transfer, tx ibc client (generate-only), tx ibc-transfer transfer"
note "Covered rejection paths: relayer without PQC allowlist, relayer without fee, MsgTransfer without fee, MsgTransfer without PQC, MsgChannelCloseInit on transfer channel"
