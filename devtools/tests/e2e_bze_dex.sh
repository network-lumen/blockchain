#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORKSPACE_ROOT_DEFAULT="$(cd "${SCRIPT_DIR}/.." && pwd)"
if [ -d "${SCRIPT_DIR}/chain_source" ]; then
	WORKSPACE_ROOT_DEFAULT="${SCRIPT_DIR}"
fi
WORKSPACE_ROOT="${WORKSPACE_ROOT:-${WORKSPACE_ROOT_DEFAULT}}"
CHAIN_SOURCE_DIR="${CHAIN_SOURCE_DIR:-${WORKSPACE_ROOT}/chain_source}"

LUMEN_BIN="${LUMEN_BIN:-${CHAIN_SOURCE_DIR}/build/lumend}"
LUMEN_BUILD_TAGS="${LUMEN_BUILD_TAGS:-dev}"
BZE_VERSION="${BZE_VERSION:-v8.0.2}"
BZE_VERSION_DIR="${BZE_VERSION#v}"
BZE_SOURCE_CACHE="${BZE_SOURCE_CACHE:-${WORKSPACE_ROOT}/artifacts/vendor-src/bze-${BZE_VERSION_DIR}}"
BZE_BIN="${BZE_BIN:-${WORKSPACE_ROOT}/artifacts/bin/bzed-${BZE_VERSION}}"
BZE_TARBALL_URL="${BZE_TARBALL_URL:-https://github.com/bze-alphateam/bze/archive/refs/tags/${BZE_VERSION}.tar.gz}"

RELAYER_IMAGE="${RELAYER_IMAGE:-ghcr.io/cosmos/relayer:v2.5.2}"
DOCKER_HOST_ALIAS="${DOCKER_HOST_ALIAS:-host.docker.internal}"
KEYRING="${KEYRING:-test}"

LUMEN_CHAIN_ID="${LUMEN_CHAIN_ID:-lumen-bze-e2e-1}"
BZE_CHAIN_ID="${BZE_CHAIN_ID:-bze-dex-local-1}"
RELAYER_PATH_NAME="${RELAYER_PATH_NAME:-lumen-bze-dex}"
RELAYER_LUMEN_CHAIN_NAME="${RELAYER_LUMEN_CHAIN_NAME:-${LUMEN_CHAIN_ID}}"
RELAYER_BZE_CHAIN_NAME="${RELAYER_BZE_CHAIN_NAME:-${BZE_CHAIN_ID}}"

LUMEN_DENOM="${LUMEN_DENOM:-ulmn}"
BZE_DENOM="${BZE_DENOM:-ubze}"
LUMEN_USER_TX_FEES="${LUMEN_USER_TX_FEES:-1000ulmn}"
LUMEN_RELAYER_GAS_PRICES="${LUMEN_RELAYER_GAS_PRICES:-0.01ulmn}"
BZE_RELAYER_GAS_PRICES="${BZE_RELAYER_GAS_PRICES:-0.01ubze}"
BZE_TX_GAS_PRICES="${BZE_TX_GAS_PRICES:-0.01ubze}"
IBC_TRANSFER_AMOUNT="${IBC_TRANSFER_AMOUNT:-200000}"

if [ -z "${E2E_BASE_PORT:-}" ]; then
	E2E_BASE_PORT=$(( (RANDOM % 1000) + 37000 ))
fi

LUMEN_RPC_PORT="${LUMEN_RPC_PORT:-${E2E_BASE_PORT}}"
LUMEN_API_PORT="${LUMEN_API_PORT:-$((E2E_BASE_PORT + 60))}"
LUMEN_GRPC_PORT="${LUMEN_GRPC_PORT:-$((E2E_BASE_PORT + 120))}"
LUMEN_P2P_PORT="${LUMEN_P2P_PORT:-$((E2E_BASE_PORT + 180))}"

BZE_RPC_PORT="${BZE_RPC_PORT:-$((E2E_BASE_PORT + 500))}"
BZE_API_PORT="${BZE_API_PORT:-$((E2E_BASE_PORT + 560))}"
BZE_GRPC_PORT="${BZE_GRPC_PORT:-$((E2E_BASE_PORT + 620))}"
BZE_P2P_PORT="${BZE_P2P_PORT:-$((E2E_BASE_PORT + 680))}"

LUMEN_RPC_URL="http://127.0.0.1:${LUMEN_RPC_PORT}"
LUMEN_API_URL="http://127.0.0.1:${LUMEN_API_PORT}"
LUMEN_API_READY_URL="${LUMEN_API_URL}/cosmos/base/tendermint/v1beta1/node_info"
LUMEN_RPC_LADDR="tcp://127.0.0.1:${LUMEN_RPC_PORT}"
LUMEN_API_ADDR="tcp://127.0.0.1:${LUMEN_API_PORT}"
LUMEN_GRPC_ADDR="127.0.0.1:${LUMEN_GRPC_PORT}"
LUMEN_P2P_LADDR="tcp://0.0.0.0:${LUMEN_P2P_PORT}"

BZE_RPC_URL="http://127.0.0.1:${BZE_RPC_PORT}"
BZE_API_URL="http://127.0.0.1:${BZE_API_PORT}"
BZE_API_READY_URL="${BZE_API_URL}/cosmos/base/tendermint/v1beta1/node_info"
BZE_RPC_LADDR="tcp://127.0.0.1:${BZE_RPC_PORT}"
BZE_API_ADDR="tcp://127.0.0.1:${BZE_API_PORT}"
BZE_GRPC_ADDR="127.0.0.1:${BZE_GRPC_PORT}"
BZE_P2P_LADDR="tcp://0.0.0.0:${BZE_P2P_PORT}"

WORK_ROOT="$(mktemp -d -t lumen-bze-dex-e2e-XXXXXX)"
ARTIFACT_DIR="${ARTIFACT_DIR:-${WORKSPACE_ROOT}/artifacts/dex-e2e-logs}"
mkdir -p "${ARTIFACT_DIR}" "$(dirname "${BZE_BIN}")" "$(dirname "${BZE_SOURCE_CACHE}")"

LUMEN_HOME="${WORK_ROOT}/lumen"
BZE_HOME="${WORK_ROOT}/beezee"
RELAYER_HOME="${WORK_ROOT}/relayer"

RUN_ID="$(date +%Y%m%d_%H%M%S)_${E2E_BASE_PORT}"
LUMEN_LOG="${ARTIFACT_DIR}/lumen_bze_dex_lumen_${RUN_ID}.log"
BZE_LOG="${ARTIFACT_DIR}/lumen_bze_dex_bze_${RUN_ID}.log"
RELAYER_LOG="${ARTIFACT_DIR}/lumen_bze_dex_relayer_${RUN_ID}.log"

LUMEN_NODE_PID=""
BZE_NODE_PID=""
RELAYER_CONTAINER="lumen-bze-relayer-${E2E_BASE_PORT}"

VALIDATOR_KEY="validator"
LUMEN_VALIDATOR_PQC_KEY="pqc-validator"
LUMEN_USER_KEY="user"
LUMEN_USER_PQC_KEY="pqc-user"
LUMEN_RELAYER_KEY="relayer"

BZE_MAKER_KEY="maker"
BZE_TAKER_KEY="taker"
BZE_RELAYER_KEY="relayer"

LUMEN_VALIDATOR_MNEMONIC="alcohol hockey chair click sword crumble outside cash old example wealth ozone rice cash because friend holiday dinner endless poem dog royal tiny profit"
LUMEN_USER_MNEMONIC="exchange control olive wool aim seek double bamboo tell process sock door resist uncle grunt reform knock chair agent dad snake oven captain destroy"
LUMEN_RELAYER_MNEMONIC="village foil behind logic hand fitness bronze push turn undo chalk symbol elbow amazing kitten creek trip game intact square solid coach stock tomato"

BZE_VALIDATOR_MNEMONIC="cost describe scatter destroy above mistake evoke angle raw oil humble clip trophy ride pottery summer limb devote slice cat manual hen follow tired"
BZE_MAKER_MNEMONIC="green trial plate resource moral skull sample entire demise hollow device accuse marble club hospital creek category topple lens cabbage add clump frost ticket"
BZE_TAKER_MNEMONIC="rally mountain couple deputy mango man divorce sound giant initial vague seat attract shine upon rabbit sign excess vanish clarify dust cube hurry assault"
BZE_RELAYER_MNEMONIC="blouse false twelve destroy bring appear skill erase cinnamon feature oppose physical kitchen school master avocado rival unit security syrup reflect album enhance elephant"

LUMEN_VALIDATOR_ADDR=""
LUMEN_USER_ADDR=""
LUMEN_RELAYER_ADDR=""

BZE_VALIDATOR_ADDR=""
BZE_MAKER_ADDR=""
BZE_TAKER_ADDR=""
BZE_RELAYER_ADDR=""

LUMEN_CLIENT_ID=""
BZE_CLIENT_ID=""
LUMEN_CONNECTION_ID=""
BZE_CONNECTION_ID=""
LUMEN_CHANNEL_ID=""
BZE_CHANNEL_ID=""
BZE_IBC_DENOM=""
MARKET_ID=""
POOL_ID=""
POOL_LP_DENOM=""
SELL_ORDER_ONE=""
SELL_ORDER_TWO=""
BUY_ORDER_ONE=""

step() {
	printf '\n==== %s\n' "$*"
}

note() {
	printf -- '---- %s\n' "$*"
}

die() {
	echo "error: $*" >&2
	exit 1
}

require_bin() {
	if ! command -v "$1" >/dev/null 2>&1; then
		die "missing dependency: $1"
	fi
}

cleanup() {
	local code=$?
	set +e
	if [ -n "${LUMEN_NODE_PID}" ] && kill -0 "${LUMEN_NODE_PID}" >/dev/null 2>&1; then
		kill "${LUMEN_NODE_PID}" >/dev/null 2>&1 || true
		wait "${LUMEN_NODE_PID}" >/dev/null 2>&1 || true
	fi
	if [ -n "${BZE_NODE_PID}" ] && kill -0 "${BZE_NODE_PID}" >/dev/null 2>&1; then
		kill "${BZE_NODE_PID}" >/dev/null 2>&1 || true
		wait "${BZE_NODE_PID}" >/dev/null 2>&1 || true
	fi
	docker rm -f "${RELAYER_CONTAINER}" >/dev/null 2>&1 || true
	if [ "${DEBUG_KEEP:-0}" != "1" ]; then
		rm -rf "${WORK_ROOT}" >/dev/null 2>&1 || true
	else
		echo "DEBUG_KEEP=1 -> preserving ${WORK_ROOT}"
	fi
	exit "${code}"
}
trap cleanup EXIT INT TERM

expect_eq() {
	local actual="$1"
	local expected="$2"
	local label="$3"
	if [ "${actual}" != "${expected}" ]; then
		die "${label}: expected '${expected}', got '${actual}'"
	fi
}

expect_nonempty() {
	local value="$1"
	local label="$2"
	if [ -z "${value}" ] || [ "${value}" = "null" ]; then
		die "${label}: missing value"
	fi
}

expect_contains() {
	local haystack="$1"
	local needle="$2"
	local label="$3"
	if ! grep -qi -- "${needle}" <<<"${haystack}"; then
		printf '%s\n' "${haystack}" >&2
		die "${label}: missing '${needle}'"
	fi
}

expect_positive_int() {
	local value="$1"
	local label="$2"
	if ! [[ "${value}" =~ ^[0-9]+$ ]] || [ "${value}" -le 0 ]; then
		die "${label}: expected positive integer, got '${value}'"
	fi
}

expect_gt() {
	local left="$1"
	local right="$2"
	local label="$3"
	if [ "${left}" -le "${right}" ]; then
		die "${label}: expected ${left} > ${right}"
	fi
}

expect_lt() {
	local left="$1"
	local right="$2"
	local label="$3"
	if [ "${left}" -ge "${right}" ]; then
		die "${label}: expected ${left} < ${right}"
	fi
}

first_json_value() {
	local key="$1"
	jq -r --arg key "${key}" 'limit(1; .. | objects | .[$key]? // empty)'
}

wait_http() {
	local url="$1"
	local log_file="$2"
	for _ in $(seq 1 240); do
		if curl --connect-timeout 1 --max-time 2 -sSf "${url}" >/dev/null 2>&1; then
			return 0
		fi
		sleep 0.5
	done
	tail -n 120 "${log_file}" >&2 || true
	die "timeout waiting for ${url}"
}

current_height() {
	local rpc_url="$1"
	curl -s "${rpc_url}/status" | jq -r '.result.sync_info.latest_block_height // "0"'
}

wait_height() {
	local rpc_url="$1"
	local target="$2"
	local log_file="$3"
	for _ in $(seq 1 240); do
		local height
		height="$(current_height "${rpc_url}")"
		if [[ "${height}" =~ ^[0-9]+$ ]] && [ "${height}" -ge "${target}" ]; then
			return 0
		fi
		sleep 0.5
	done
	tail -n 120 "${log_file}" >&2 || true
	die "timeout waiting for height >= ${target} on ${rpc_url}"
}

wait_next_block() {
	local rpc_url="$1"
	local log_file="$2"
	local start
	start="$(current_height "${rpc_url}")"
	wait_height "${rpc_url}" "$((start + 1))" "${log_file}"
}

extract_txhash() {
	echo "$1" | jq -r '.txhash // .TxHash // empty'
}

wait_tx() {
	local rpc_url="$1"
	local txhash="$2"
	for _ in $(seq 1 240); do
		local resp code
		resp="$(curl -s "${rpc_url}/tx?hash=0x${txhash}" 2>/dev/null || true)"
		code="$(echo "${resp}" | jq -r '.result.tx_result.code // empty' 2>/dev/null || true)"
		if [ -n "${code}" ] && [ "${code}" != "null" ]; then
			echo "${code}"
			return 0
		fi
		sleep 0.5
	done
	die "timeout waiting for tx ${txhash}"
}

wait_balance_at_least() {
	local bin="$1"
	local node="$2"
	local address="$3"
	local denom="$4"
	local min_amount="$5"
	for _ in $(seq 1 240); do
		local amount
		amount="$(query_balance_amount "${bin}" "${node}" "${address}" "${denom}")"
		if [[ "${amount}" =~ ^[0-9]+$ ]] && [ "${amount}" -ge "${min_amount}" ]; then
			return 0
		fi
		sleep 0.5
	done
	die "timeout waiting for ${address} to hold at least ${min_amount}${denom}"
}

add_key_from_mnemonic() {
	local bin="$1"
	local home="$2"
	local name="$3"
	local mnemonic="$4"
	local mn_file
	mn_file="$(mktemp)"
	printf '%s' "${mnemonic}" >"${mn_file}"
	"${bin}" keys add "${name}" --recover --source "${mn_file}" --keyring-backend "${KEYRING}" --home "${home}" >/dev/null
	rm -f "${mn_file}"
}

query_addr() {
	local bin="$1"
	local home="$2"
	local name="$3"
	"${bin}" keys show "${name}" -a --keyring-backend "${KEYRING}" --home "${home}"
}

query_balance_amount() {
	local bin="$1"
	local node="$2"
	local address="$3"
	local denom="$4"
	local amount
	amount="$("${bin}" query bank balances "${address}" --node "${node}" -o json | jq -r --arg denom "${denom}" 'limit(1; .balances[]? | select(.denom == $denom) | .amount)')"
	if [ -z "${amount}" ] || [ "${amount}" = "null" ]; then
		echo 0
	else
		echo "${amount}"
	fi
}

query_first_ibc_balance() {
	local bin="$1"
	local node="$2"
	local address="$3"
	"${bin}" query bank balances "${address}" --node "${node}" -o json \
		| jq -r 'limit(1; .balances[]? | select(.denom | startswith("ibc/")) | [.denom, .amount] | @tsv)'
}

ensure_lumend_binary() {
	if [ -x "${LUMEN_BIN}" ]; then
		return
	fi

	step "Build lumend"
	(
		cd "${CHAIN_SOURCE_DIR}"
		go build -trimpath -buildvcs=false -tags "${LUMEN_BUILD_TAGS}" -o "${LUMEN_BIN}" ./cmd/lumend
	)
}

fetch_bze_source() {
	if [ -d "${BZE_SOURCE_CACHE}" ]; then
		return
	fi

	step "Fetch BeeZee source ${BZE_VERSION}"
	local tmp_root tarball
	tmp_root="$(mktemp -d -t bze-src-XXXXXX)"
	tarball="${tmp_root}/bze.tar.gz"
	curl -L -f -o "${tarball}" "${BZE_TARBALL_URL}"
	tar -xzf "${tarball}" -C "${tmp_root}"
	mv "${tmp_root}/bze-${BZE_VERSION_DIR}" "${BZE_SOURCE_CACHE}"
	rm -rf "${tmp_root}"
}

ensure_bzed_binary() {
	if [ -x "${BZE_BIN}" ]; then
		return
	fi

	fetch_bze_source

	step "Build bzed ${BZE_VERSION}"
	(
		cd "${BZE_SOURCE_CACHE}"
		go build -trimpath -buildvcs=false -o "${BZE_BIN}" ./cmd/bzed
	)
}

pqc_set_client_config() {
	local home="$1"
	local rpc="$2"
	local chain_id="$3"
	local cfg="${home}/config/client.toml"
	[ -f "${cfg}" ] || return 0
	local tmp
	tmp="$(mktemp)"
	awk -v rpc="${rpc}" -v chain="${chain_id}" '
BEGIN { set_node=0; set_chain=0 }
/^node = / { print "node = \"" rpc "\""; set_node=1; next }
/^chain-id = / { print "chain-id = \"" chain "\""; set_chain=1; next }
{ print }
END {
	if (!set_node) print "node = \"" rpc "\""
	if (!set_chain) print "chain-id = \"" chain "\""
}
' "${cfg}" >"${tmp}"
	mv "${tmp}" "${cfg}"
}

set_fast_consensus() {
	local home="$1"
	local cfg="${home}/config/config.toml"
	[ -f "${cfg}" ] || return 0
	sed -i.bak 's/^timeout_commit = .*/timeout_commit = "1s"/' "${cfg}" || true
	sed -i.bak 's/^timeout_propose = .*/timeout_propose = "1s"/' "${cfg}" || true
}

configure_lumen_genesis() {
	local tmp
	tmp="$(mktemp)"
	jq '
		.app_state.gov.params = {
			min_deposit:[{denom:"ulmn",amount:"10000000"}],
			expedited_min_deposit:[{denom:"ulmn",amount:"50000000"}],
			max_deposit_period:"8s",
			voting_period:"8s",
			expedited_voting_period:"4s",
			quorum:"0.334000000000000000",
			threshold:"0.500000000000000000",
			expedited_threshold:"0.670000000000000000",
			veto_threshold:"0.334000000000000000",
			min_initial_deposit_ratio:"0.000000000000000000",
			proposal_cancel_ratio:"0.000000000000000000",
			proposal_cancel_dest:"",
			burn_proposal_deposit_prevote:false,
			burn_vote_quorum:false,
			burn_vote_veto:false,
			min_deposit_ratio:"0.010000000000000000"
		}
		| .app_state.gov.constitution = "Local e2e: approve IBC relayer allowlist proposals quickly."
		| .app_state.distribution.params.community_tax = "0.0"
	' "${LUMEN_HOME}/config/genesis.json" >"${tmp}"
	mv "${tmp}" "${LUMEN_HOME}/config/genesis.json"
}

init_lumen_chain() {
	step "Init Lumen chain"
	rm -rf "${LUMEN_HOME}"
	"${LUMEN_BIN}" init lumen-dex --chain-id "${LUMEN_CHAIN_ID}" --home "${LUMEN_HOME}" >/dev/null 2>&1
	set_fast_consensus "${LUMEN_HOME}"

	add_key_from_mnemonic "${LUMEN_BIN}" "${LUMEN_HOME}" "${VALIDATOR_KEY}" "${LUMEN_VALIDATOR_MNEMONIC}"
	add_key_from_mnemonic "${LUMEN_BIN}" "${LUMEN_HOME}" "${LUMEN_USER_KEY}" "${LUMEN_USER_MNEMONIC}"
	add_key_from_mnemonic "${LUMEN_BIN}" "${LUMEN_HOME}" "${LUMEN_RELAYER_KEY}" "${LUMEN_RELAYER_MNEMONIC}"

	LUMEN_VALIDATOR_ADDR="$(query_addr "${LUMEN_BIN}" "${LUMEN_HOME}" "${VALIDATOR_KEY}")"
	LUMEN_USER_ADDR="$(query_addr "${LUMEN_BIN}" "${LUMEN_HOME}" "${LUMEN_USER_KEY}")"
	LUMEN_RELAYER_ADDR="$(query_addr "${LUMEN_BIN}" "${LUMEN_HOME}" "${LUMEN_RELAYER_KEY}")"

	"${LUMEN_BIN}" genesis add-genesis-account "${LUMEN_VALIDATOR_ADDR}" 250000000ulmn --keyring-backend "${KEYRING}" --home "${LUMEN_HOME}" >/dev/null 2>&1
	"${LUMEN_BIN}" genesis add-genesis-account "${LUMEN_USER_ADDR}" 150000000ulmn --keyring-backend "${KEYRING}" --home "${LUMEN_HOME}" >/dev/null 2>&1
	"${LUMEN_BIN}" genesis add-genesis-account "${LUMEN_RELAYER_ADDR}" 50000000ulmn --keyring-backend "${KEYRING}" --home "${LUMEN_HOME}" >/dev/null 2>&1

	"${LUMEN_BIN}" keys pqc-generate --name "${LUMEN_VALIDATOR_PQC_KEY}" --link-from "${VALIDATOR_KEY}" --keyring-backend "${KEYRING}" --home "${LUMEN_HOME}" >/dev/null 2>&1
	"${LUMEN_BIN}" keys pqc-genesis-entry --from "${VALIDATOR_KEY}" --pqc "${LUMEN_VALIDATOR_PQC_KEY}" --keyring-backend "${KEYRING}" --home "${LUMEN_HOME}" --write-genesis "${LUMEN_HOME}/config/genesis.json" >/dev/null 2>&1
	"${LUMEN_BIN}" keys pqc-generate --name "${LUMEN_USER_PQC_KEY}" --link-from "${LUMEN_USER_KEY}" --keyring-backend "${KEYRING}" --home "${LUMEN_HOME}" >/dev/null 2>&1
	"${LUMEN_BIN}" keys pqc-genesis-entry --from "${LUMEN_USER_KEY}" --pqc "${LUMEN_USER_PQC_KEY}" --keyring-backend "${KEYRING}" --home "${LUMEN_HOME}" --write-genesis "${LUMEN_HOME}/config/genesis.json" >/dev/null 2>&1

	"${LUMEN_BIN}" genesis gentx "${VALIDATOR_KEY}" 50000000ulmn --chain-id "${LUMEN_CHAIN_ID}" --keyring-backend "${KEYRING}" --home "${LUMEN_HOME}" >/dev/null 2>&1
	"${LUMEN_BIN}" genesis collect-gentxs --home "${LUMEN_HOME}" >/dev/null 2>&1
	configure_lumen_genesis
	"${LUMEN_BIN}" genesis validate --home "${LUMEN_HOME}" >/dev/null 2>&1
	pqc_set_client_config "${LUMEN_HOME}" "${LUMEN_RPC_LADDR}" "${LUMEN_CHAIN_ID}"
}

init_bze_chain() {
	step "Init BeeZee chain"
	rm -rf "${BZE_HOME}"
	"${BZE_BIN}" init beezee-dex --chain-id "${BZE_CHAIN_ID}" --home "${BZE_HOME}" --default-denom "${BZE_DENOM}" >/dev/null 2>&1
	set_fast_consensus "${BZE_HOME}"

	add_key_from_mnemonic "${BZE_BIN}" "${BZE_HOME}" "${VALIDATOR_KEY}" "${BZE_VALIDATOR_MNEMONIC}"
	add_key_from_mnemonic "${BZE_BIN}" "${BZE_HOME}" "${BZE_MAKER_KEY}" "${BZE_MAKER_MNEMONIC}"
	add_key_from_mnemonic "${BZE_BIN}" "${BZE_HOME}" "${BZE_TAKER_KEY}" "${BZE_TAKER_MNEMONIC}"
	add_key_from_mnemonic "${BZE_BIN}" "${BZE_HOME}" "${BZE_RELAYER_KEY}" "${BZE_RELAYER_MNEMONIC}"

	BZE_VALIDATOR_ADDR="$(query_addr "${BZE_BIN}" "${BZE_HOME}" "${VALIDATOR_KEY}")"
	BZE_MAKER_ADDR="$(query_addr "${BZE_BIN}" "${BZE_HOME}" "${BZE_MAKER_KEY}")"
	BZE_TAKER_ADDR="$(query_addr "${BZE_BIN}" "${BZE_HOME}" "${BZE_TAKER_KEY}")"
	BZE_RELAYER_ADDR="$(query_addr "${BZE_BIN}" "${BZE_HOME}" "${BZE_RELAYER_KEY}")"

	"${BZE_BIN}" genesis add-genesis-account "${BZE_VALIDATOR_ADDR}" 250000000000ubze --keyring-backend "${KEYRING}" --home "${BZE_HOME}" >/dev/null 2>&1
	"${BZE_BIN}" genesis add-genesis-account "${BZE_MAKER_ADDR}" 220000000000ubze --keyring-backend "${KEYRING}" --home "${BZE_HOME}" >/dev/null 2>&1
	"${BZE_BIN}" genesis add-genesis-account "${BZE_TAKER_ADDR}" 80000000000ubze --keyring-backend "${KEYRING}" --home "${BZE_HOME}" >/dev/null 2>&1
	"${BZE_BIN}" genesis add-genesis-account "${BZE_RELAYER_ADDR}" 50000000ubze --keyring-backend "${KEYRING}" --home "${BZE_HOME}" >/dev/null 2>&1

	"${BZE_BIN}" genesis gentx "${VALIDATOR_KEY}" 50000000ubze --chain-id "${BZE_CHAIN_ID}" --keyring-backend "${KEYRING}" --home "${BZE_HOME}" >/dev/null 2>&1
	"${BZE_BIN}" genesis collect-gentxs --home "${BZE_HOME}" >/dev/null 2>&1
	"${BZE_BIN}" genesis validate --home "${BZE_HOME}" >/dev/null 2>&1
}

start_lumen_node() {
	step "Start Lumen node"
	"${LUMEN_BIN}" start \
		--home "${LUMEN_HOME}" \
		--rpc.laddr "${LUMEN_RPC_LADDR}" \
		--rpc.pprof_laddr "" \
		--p2p.laddr "${LUMEN_P2P_LADDR}" \
		--api.enable \
		--api.address "${LUMEN_API_ADDR}" \
		--grpc.address "${LUMEN_GRPC_ADDR}" \
		--minimum-gas-prices 0ulmn \
		>"${LUMEN_LOG}" 2>&1 &
	LUMEN_NODE_PID="$!"
	wait_http "${LUMEN_RPC_URL}/status" "${LUMEN_LOG}"
	wait_http "${LUMEN_API_READY_URL}" "${LUMEN_LOG}"
	wait_height "${LUMEN_RPC_URL}" 2 "${LUMEN_LOG}"
}

start_bze_node() {
	step "Start BeeZee node"
	"${BZE_BIN}" start \
		--home "${BZE_HOME}" \
		--rpc.laddr "${BZE_RPC_LADDR}" \
		--rpc.pprof_laddr "" \
		--p2p.laddr "${BZE_P2P_LADDR}" \
		--api.enable \
		--api.address "${BZE_API_ADDR}" \
		--grpc.address "${BZE_GRPC_ADDR}" \
		--minimum-gas-prices 0ubze \
		--x-crisis-skip-assert-invariants \
		>"${BZE_LOG}" 2>&1 &
	BZE_NODE_PID="$!"
	wait_http "${BZE_RPC_URL}/status" "${BZE_LOG}"
	wait_http "${BZE_API_READY_URL}" "${BZE_LOG}"
	wait_height "${BZE_RPC_URL}" 2 "${BZE_LOG}"
}

relayer_cmd() {
	docker run --rm \
		--add-host "${DOCKER_HOST_ALIAS}:host-gateway" \
		-u "$(id -u):$(id -g)" \
		-v "${RELAYER_HOME}:/home/relayer/.relayer" \
		"${RELAYER_IMAGE}" \
		rly --home /home/relayer/.relayer "$@"
}

relayer_cmd_timeout() {
	local seconds="$1"
	shift
	timeout "${seconds}" docker run --rm \
		--add-host "${DOCKER_HOST_ALIAS}:host-gateway" \
		-u "$(id -u):$(id -g)" \
		-v "${RELAYER_HOME}:/home/relayer/.relayer" \
		"${RELAYER_IMAGE}" \
		rly --home /home/relayer/.relayer "$@"
}

expect_failure_output() {
	set +e
	local output
	output="$("$@" 2>&1)"
	local code=$?
	set -e
	if [ "${code}" -eq 0 ]; then
		printf '%s\n' "${output}" >&2
		die "command unexpectedly succeeded: $*"
	fi
	printf '%s\n' "${output}"
}

init_relayer_home() {
	step "Prepare relayer home"
	rm -rf "${RELAYER_HOME}"
	mkdir -p "${RELAYER_HOME}"

	cat >"${RELAYER_HOME}/lumen.json" <<EOF
{
  "type": "cosmos",
  "value": {
    "key": "${LUMEN_RELAYER_KEY}",
    "chain-id": "${LUMEN_CHAIN_ID}",
    "rpc-addr": "http://${DOCKER_HOST_ALIAS}:${LUMEN_RPC_PORT}",
    "account-prefix": "lmn",
    "keyring-backend": "${KEYRING}",
    "gas-adjustment": 1.5,
    "gas-prices": "${LUMEN_RELAYER_GAS_PRICES}",
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

	cat >"${RELAYER_HOME}/bze.json" <<EOF
{
  "type": "cosmos",
  "value": {
    "key": "${BZE_RELAYER_KEY}",
    "chain-id": "${BZE_CHAIN_ID}",
    "rpc-addr": "http://${DOCKER_HOST_ALIAS}:${BZE_RPC_PORT}",
    "account-prefix": "bze",
    "keyring-backend": "${KEYRING}",
    "gas-adjustment": 1.5,
    "gas-prices": "${BZE_RELAYER_GAS_PRICES}",
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

	relayer_cmd config init >/dev/null
	relayer_cmd chains add --file /home/relayer/.relayer/lumen.json "${RELAYER_LUMEN_CHAIN_NAME}" >/dev/null
	relayer_cmd chains add --file /home/relayer/.relayer/bze.json "${RELAYER_BZE_CHAIN_NAME}" >/dev/null
	relayer_cmd paths new "${RELAYER_LUMEN_CHAIN_NAME}" "${RELAYER_BZE_CHAIN_NAME}" "${RELAYER_PATH_NAME}" >/dev/null
	relayer_cmd keys restore "${RELAYER_LUMEN_CHAIN_NAME}" "${LUMEN_RELAYER_KEY}" "${LUMEN_RELAYER_MNEMONIC}" >/dev/null
	relayer_cmd keys restore "${RELAYER_BZE_CHAIN_NAME}" "${BZE_RELAYER_KEY}" "${BZE_RELAYER_MNEMONIC}" >/dev/null
}

relayer_query_client_id() {
	local chain_name="$1"
	relayer_cmd query clients "${chain_name}" -o json | first_json_value client_id
}

relayer_query_connection_id() {
	local chain_name="$1"
	relayer_cmd query connections "${chain_name}" -o json \
		| jq -r 'limit(1; .. | objects | (.id? // .connection_id? // empty) | select(test("^connection-")) )'
}

relayer_query_channel_id() {
	local chain_name="$1"
	relayer_cmd query channels "${chain_name}" -o json \
		| jq -r 'limit(1; .. | objects | .channel_id? // empty | select(test("^channel-")) )'
}

relayer_query_state() {
	local kind="$1"
	local chain_name="$2"
	local object_id="$3"
	local port_id="${4:-}"
	if [ "${kind}" = "channel" ]; then
		relayer_cmd query channels "${chain_name}" -o json \
			| jq -r --arg id "${object_id}" --arg port "${port_id}" '
				limit(1; .. | objects
					| select((.channel_id? // "") == $id and (($port == "") or ((.port_id? // "") == $port)))
					| .state? // empty)'
	else
		relayer_cmd query connections "${chain_name}" -o json \
			| jq -r --arg id "${object_id}" '
				limit(1; .. | objects
					| select(((.id? // .connection_id? // "") == $id))
					| .state? // empty)'
	fi
}

assert_open_state() {
	local value="$1"
	local label="$2"
	case "${value}" in
		OPEN|STATE_OPEN|3) ;;
		*) die "${label} is not open (state=${value})" ;;
	esac
}

start_relayer_daemon() {
	step "Start background relayer"
	docker rm -f "${RELAYER_CONTAINER}" >/dev/null 2>&1 || true
	docker run -d \
		--name "${RELAYER_CONTAINER}" \
		--add-host "${DOCKER_HOST_ALIAS}:host-gateway" \
		-u "$(id -u):$(id -g)" \
		-v "${RELAYER_HOME}:/home/relayer/.relayer" \
		"${RELAYER_IMAGE}" \
		rly --home /home/relayer/.relayer start "${RELAYER_PATH_NAME}" >"${RELAYER_LOG}"
	sleep 3
	local running
	running="$(docker inspect -f '{{.State.Running}}' "${RELAYER_CONTAINER}" 2>/dev/null || true)"
	expect_eq "${running}" "true" "relayer daemon state"
}

gov_resolve_authority() {
	"${LUMEN_BIN}" query auth module-account gov --node "${LUMEN_RPC_LADDR}" -o json \
		| jq -r '.account.base_account.address // .account.value.address // .account.address // empty'
}

gov_extract_proposal_id() {
	local tx_json="$1"
	echo "${tx_json}" | jq -r '.result.tx_result.events[]? | select(.type=="submit_proposal") | .attributes[]? | select(.key=="proposal_id") | .value' | tail -n1
}

gov_wait_status() {
	local proposal_id="$1"
	local target="$2"
	for _ in $(seq 1 180); do
		local status
		status="$("${LUMEN_BIN}" query gov proposal "${proposal_id}" --node "${LUMEN_RPC_LADDR}" -o json | jq -r '.proposal.status // empty')"
		if [ "${status}" = "${target}" ]; then
			return 0
		fi
		sleep 1
	done
	die "proposal ${proposal_id} did not reach ${target}"
}

submit_lumen_proposal_file() {
	local file="$1"
	local deposit="$2"
	local res hash code tx_json pid
	res="$("${LUMEN_BIN}" tx gov submit-proposal "${file}" \
		--from "${VALIDATOR_KEY}" \
		--home "${LUMEN_HOME}" \
		--keyring-backend "${KEYRING}" \
		--chain-id "${LUMEN_CHAIN_ID}" \
		--node "${LUMEN_RPC_LADDR}" \
		--pqc-from "${LUMEN_VALIDATOR_ADDR}" \
		--pqc-key "${LUMEN_VALIDATOR_PQC_KEY}" \
		--gas 400000 \
		--fees 0ulmn \
		--yes \
		-o json)"
	hash="$(extract_txhash "${res}")"
	expect_nonempty "${hash}" "gov proposal txhash"
	code="$(wait_tx "${LUMEN_RPC_URL}" "${hash}")"
	expect_eq "${code}" "0" "gov proposal submit code"
	tx_json="$(curl -s "${LUMEN_RPC_URL}/tx?hash=0x${hash}")"
	pid="$(gov_extract_proposal_id "${tx_json}")"
	expect_nonempty "${pid}" "proposal id"
	echo "${pid}"
}

vote_lumen_yes() {
	local proposal_id="$1"
	local res hash code
	res="$("${LUMEN_BIN}" tx gov vote "${proposal_id}" yes \
		--from "${VALIDATOR_KEY}" \
		--home "${LUMEN_HOME}" \
		--keyring-backend "${KEYRING}" \
		--chain-id "${LUMEN_CHAIN_ID}" \
		--node "${LUMEN_RPC_LADDR}" \
		--pqc-from "${LUMEN_VALIDATOR_ADDR}" \
		--pqc-key "${LUMEN_VALIDATOR_PQC_KEY}" \
		--fees 0ulmn \
		--yes \
		-o json)"
	hash="$(extract_txhash "${res}")"
	expect_nonempty "${hash}" "gov vote txhash"
	code="$(wait_tx "${LUMEN_RPC_URL}" "${hash}")"
	expect_eq "${code}" "0" "gov vote code"
}

lumen_allowlist_relayer_via_gov() {
	local relayer_addr="$1"
	local gov_authority proposal_file proposal_id
	gov_authority="$(gov_resolve_authority)"
	expect_nonempty "${gov_authority}" "gov authority"

	proposal_file="$(mktemp -t lumen-pqc-relayer-XXXXXX.json)"
	jq -n --arg title "Allow IBC relayer" \
		--arg summary "Allow the local IBC relayer for Lumen/BeeZee e2e." \
		--arg relayer "${relayer_addr}" \
		--arg auth "${gov_authority}" \
		--arg deposit "10000000ulmn" \
		'{
			title: $title,
			summary: $summary,
			deposit: $deposit,
			metadata: "",
			messages: [
				{
					"@type": "/lumen.pqc.v1.MsgAddIBCRelayer",
					authority: $auth,
					relayer: $relayer
				}
			]
		}' >"${proposal_file}"

	proposal_id="$(submit_lumen_proposal_file "${proposal_file}" "10000000ulmn")"
	vote_lumen_yes "${proposal_id}"
	gov_wait_status "${proposal_id}" "PROPOSAL_STATUS_PASSED"
	rm -f "${proposal_file}"
}

run_lumen_ibc_transfer() {
	local channel_id="$1"
	local receiver="$2"
	local amount="$3"
	shift 3
	local res hash code
	res="$("${LUMEN_BIN}" tx ibc-transfer transfer "${channel_id}" "${receiver}" "${amount}" \
		--from "${LUMEN_USER_KEY}" \
		--home "${LUMEN_HOME}" \
		--keyring-backend "${KEYRING}" \
		--chain-id "${LUMEN_CHAIN_ID}" \
		--node "${LUMEN_RPC_LADDR}" \
		--pqc-from "${LUMEN_USER_ADDR}" \
		--pqc-key "${LUMEN_USER_PQC_KEY}" \
		--yes \
		--broadcast-mode sync \
		--output json \
		"$@")"
	hash="$(extract_txhash "${res}")"
	expect_nonempty "${hash}" "lumen ibc transfer txhash"
	code="$(wait_tx "${LUMEN_RPC_URL}" "${hash}")"
	expect_eq "${code}" "0" "lumen ibc transfer code"
	echo "${res}"
}

run_bze_tx() {
	local from="$1"
	shift
	local res hash code
	res="$("${BZE_BIN}" tx "$@" \
		--from "${from}" \
		--home "${BZE_HOME}" \
		--keyring-backend "${KEYRING}" \
		--chain-id "${BZE_CHAIN_ID}" \
		--node "${BZE_RPC_LADDR}" \
		--gas auto \
		--gas-adjustment 1.6 \
		--gas-prices "${BZE_TX_GAS_PRICES}" \
		--yes \
		--broadcast-mode sync \
		--output json)"
	hash="$(extract_txhash "${res}")"
	expect_nonempty "${hash}" "BeeZee txhash"
	code="$(wait_tx "${BZE_RPC_URL}" "${hash}")"
	expect_eq "${code}" "0" "BeeZee tx code"
	echo "${res}"
}

query_bze_tradebin() {
	"${BZE_BIN}" query tradebin "$@" --node "${BZE_RPC_LADDR}" -o json
}

rest_get_json() {
	local url="$1"
	curl -sSf "${url}"
}

query_user_order_ids() {
	local address="$1"
	query_bze_tradebin user-market-orders "${address}" "${MARKET_ID}" \
		| jq -r '.list[]?.id'
}

query_user_order_id_count() {
	local address="$1"
	query_bze_tradebin user-market-orders "${address}" "${MARKET_ID}" \
		| jq -r '.list | length'
}

rest_assert_market_queries() {
	step "Verify BeeZee DEX market endpoints"

	local params_json market_json all_markets_json asset_markets_json
	params_json="$(rest_get_json "${BZE_API_URL}/bze/tradebin/params")"
	market_json="$(rest_get_json "${BZE_API_URL}/bze/tradebin/market?base=$(printf '%s' "${BZE_IBC_DENOM}" | jq -sRr @uri)&quote=${BZE_DENOM}")"
	all_markets_json="$(rest_get_json "${BZE_API_URL}/bze/tradebin/all_markets")"
	asset_markets_json="$(rest_get_json "${BZE_API_URL}/bze/tradebin/asset_markets?asset=$(printf '%s' "${BZE_IBC_DENOM}" | jq -sRr @uri)")"

	expect_contains "${params_json}" "createMarketFee" "tradebin params endpoint"
	expect_contains "${market_json}" "\"base\":\"${BZE_IBC_DENOM}\"" "tradebin market endpoint"
	expect_contains "${all_markets_json}" "\"quote\":\"${BZE_DENOM}\"" "tradebin all-markets endpoint"
	expect_contains "${asset_markets_json}" "\"base\":\"${BZE_IBC_DENOM}\"" "tradebin asset-markets endpoint"
}

rest_assert_orderbook_queries() {
	step "Verify BeeZee DEX orderbook endpoints"

	local user_orders_json aggregated_sell_json market_order_json all_user_dust_json
	user_orders_json="$(rest_get_json "${BZE_API_URL}/bze/tradebin/user_market_orders/${BZE_MAKER_ADDR}?market=$(printf '%s' "${MARKET_ID}" | jq -sRr @uri)")"
	aggregated_sell_json="$(rest_get_json "${BZE_API_URL}/bze/tradebin/market_aggregated_orders?market=$(printf '%s' "${MARKET_ID}" | jq -sRr @uri)&order_type=sell")"
	market_order_json="$(rest_get_json "${BZE_API_URL}/bze/tradebin/market_order?market=$(printf '%s' "${MARKET_ID}" | jq -sRr @uri)&order_type=sell&order_id=${SELL_ORDER_ONE}")"
	all_user_dust_json="$(rest_get_json "${BZE_API_URL}/bze/tradebin/all_user_dust?address=${BZE_MAKER_ADDR}")"

	expect_contains "${user_orders_json}" "\"id\":\"${SELL_ORDER_ONE}\"" "tradebin user-market-orders endpoint"
	expect_contains "${aggregated_sell_json}" "\"price\":\"5\"" "tradebin market-aggregated-orders endpoint"
	expect_contains "${market_order_json}" "\"id\":\"${SELL_ORDER_ONE}\"" "tradebin market-order endpoint"
	expect_contains "${all_user_dust_json}" "\"list\"" "tradebin all-user-dust endpoint"
}

rest_assert_history_queries() {
	step "Verify BeeZee DEX history endpoint"

	local history_json buy_orders_json
	history_json="$(rest_get_json "${BZE_API_URL}/bze/tradebin/market_history?market=$(printf '%s' "${MARKET_ID}" | jq -sRr @uri)")"
	buy_orders_json="$(rest_get_json "${BZE_API_URL}/bze/tradebin/user_market_orders/${BZE_TAKER_ADDR}?market=$(printf '%s' "${MARKET_ID}" | jq -sRr @uri)")"

	expect_contains "${history_json}" "\"market_id\":\"${MARKET_ID}\"" "tradebin market-history endpoint"
	expect_contains "${buy_orders_json}" "\"list\"" "tradebin taker orders endpoint"
}

rest_assert_pool_queries() {
	step "Verify BeeZee DEX pool endpoints"

	local all_pools_json pool_json
	all_pools_json="$(rest_get_json "${BZE_API_URL}/bze/tradebin/all_liquidity_pools")"
	pool_json="$(rest_get_json "${BZE_API_URL}/bze/tradebin/liquidity_pool?pool_id=$(printf '%s' "${POOL_ID}" | jq -sRr @uri)")"

	expect_contains "${all_pools_json}" "\"id\":\"${POOL_ID}\"" "tradebin all-liquidity-pools endpoint"
	expect_contains "${pool_json}" "\"id\":\"${POOL_ID}\"" "tradebin liquidity-pool endpoint"
}

compute_pool_id() {
	local a="$1"
	local b="$2"
	if [[ "${a}" > "${b}" ]]; then
		echo "${b}_${a}"
	else
		echo "${a}_${b}"
	fi
}

main() {
	require_bin jq
	require_bin curl
	require_bin docker
	require_bin go
	require_bin timeout
	require_bin tar

	ensure_lumend_binary
	ensure_bzed_binary
	init_lumen_chain
	init_bze_chain
	start_lumen_node
	start_bze_node

	wait_balance_at_least "${LUMEN_BIN}" "${LUMEN_RPC_LADDR}" "${LUMEN_VALIDATOR_ADDR}" "${LUMEN_DENOM}" 1000000
	wait_balance_at_least "${BZE_BIN}" "${BZE_RPC_LADDR}" "${BZE_MAKER_ADDR}" "${BZE_DENOM}" 1000000
	init_relayer_home

	step "Verify relayer is blocked before governance allowlist"
	local bad_output
	bad_output="$(expect_failure_output relayer_cmd_timeout 25 transact client "${RELAYER_LUMEN_CHAIN_NAME}" "${RELAYER_BZE_CHAIN_NAME}" "${RELAYER_PATH_NAME}")"
	expect_contains "${bad_output}" "pqc" "relayer rejection before allowlist"

	step "Allowlist the Lumen relayer via governance"
	lumen_allowlist_relayer_via_gov "${LUMEN_RELAYER_ADDR}"
	local allowlist_json
	allowlist_json="$("${LUMEN_BIN}" query pqc params --node "${LUMEN_RPC_LADDR}" -o json)"
	expect_contains "${allowlist_json}" "${LUMEN_RELAYER_ADDR}" "pqc relayer allowlist"

	step "Create IBC clients, connection and channel"
	relayer_cmd transact clients "${RELAYER_PATH_NAME}"
	wait_next_block "${LUMEN_RPC_URL}" "${LUMEN_LOG}"
	wait_next_block "${BZE_RPC_URL}" "${BZE_LOG}"
	relayer_cmd transact update-clients "${RELAYER_PATH_NAME}"
	relayer_cmd transact connection "${RELAYER_PATH_NAME}"
	relayer_cmd transact channel "${RELAYER_PATH_NAME}"

	LUMEN_CLIENT_ID="$(relayer_query_client_id "${RELAYER_LUMEN_CHAIN_NAME}")"
	BZE_CLIENT_ID="$(relayer_query_client_id "${RELAYER_BZE_CHAIN_NAME}")"
	LUMEN_CONNECTION_ID="$(relayer_query_connection_id "${RELAYER_LUMEN_CHAIN_NAME}")"
	BZE_CONNECTION_ID="$(relayer_query_connection_id "${RELAYER_BZE_CHAIN_NAME}")"
	LUMEN_CHANNEL_ID="$(relayer_query_channel_id "${RELAYER_LUMEN_CHAIN_NAME}")"
	BZE_CHANNEL_ID="$(relayer_query_channel_id "${RELAYER_BZE_CHAIN_NAME}")"

	expect_nonempty "${LUMEN_CLIENT_ID}" "Lumen client id"
	expect_nonempty "${BZE_CLIENT_ID}" "BeeZee client id"
	expect_nonempty "${LUMEN_CONNECTION_ID}" "Lumen connection id"
	expect_nonempty "${BZE_CONNECTION_ID}" "BeeZee connection id"
	expect_nonempty "${LUMEN_CHANNEL_ID}" "Lumen channel id"
	expect_nonempty "${BZE_CHANNEL_ID}" "BeeZee channel id"

	start_relayer_daemon

	step "Send ULMN over IBC to BeeZee"
	local lumen_user_start transfer_tx denom_hash_json denom_trace_json
	lumen_user_start="$(query_balance_amount "${LUMEN_BIN}" "${LUMEN_RPC_LADDR}" "${LUMEN_USER_ADDR}" "${LUMEN_DENOM}")"
	transfer_tx="$(run_lumen_ibc_transfer "${LUMEN_CHANNEL_ID}" "${BZE_MAKER_ADDR}" "${IBC_TRANSFER_AMOUNT}${LUMEN_DENOM}" --fees "${LUMEN_USER_TX_FEES}")"
	note "IBC transfer txhash: $(extract_txhash "${transfer_tx}")"

	for _ in $(seq 1 30); do
		local balance_tsv
		balance_tsv="$(query_first_ibc_balance "${BZE_BIN}" "${BZE_RPC_LADDR}" "${BZE_MAKER_ADDR}")"
		if [ -n "${balance_tsv}" ]; then
			BZE_IBC_DENOM="$(printf '%s\n' "${balance_tsv}" | cut -f1)"
			break
		fi
		sleep 1
	done
	if [ -z "${BZE_IBC_DENOM}" ]; then
		relayer_cmd transact flush "${RELAYER_PATH_NAME}" "${LUMEN_CHANNEL_ID}" >/dev/null
		local balance_tsv
		balance_tsv="$(query_first_ibc_balance "${BZE_BIN}" "${BZE_RPC_LADDR}" "${BZE_MAKER_ADDR}")"
		BZE_IBC_DENOM="$(printf '%s\n' "${balance_tsv}" | cut -f1)"
	fi
	expect_nonempty "${BZE_IBC_DENOM}" "BeeZee IBC voucher denom"
	case "${BZE_IBC_DENOM}" in
		ibc/*) ;;
		*) die "expected BeeZee IBC voucher denom, got ${BZE_IBC_DENOM}" ;;
	esac

	denom_hash_json="$("${BZE_BIN}" query ibc-transfer denom-hash "transfer/${LUMEN_CHANNEL_ID}/${LUMEN_DENOM}" --node "${BZE_RPC_LADDR}" -o json)"
	expect_eq "${BZE_IBC_DENOM#ibc/}" "$(echo "${denom_hash_json}" | jq -r '.hash // .denom_hash // empty')" "IBC denom hash"

	denom_trace_json="$("${BZE_BIN}" query ibc-transfer denom-trace "${BZE_IBC_DENOM}" --node "${BZE_RPC_LADDR}" -o json)"
	expect_eq "$(echo "${denom_trace_json}" | jq -r '.denom_trace.base_denom // .denom.base // .base_denom // empty')" "${LUMEN_DENOM}" "IBC base denom"
	expect_eq "$(echo "${denom_trace_json}" | jq -r '.denom_trace.path // .denom.path // empty')" "transfer/${LUMEN_CHANNEL_ID}" "IBC denom trace path"

	expect_eq \
		"$(query_balance_amount "${LUMEN_BIN}" "${LUMEN_RPC_LADDR}" "${LUMEN_USER_ADDR}" "${LUMEN_DENOM}")" \
		"$((lumen_user_start - IBC_TRANSFER_AMOUNT - 1000))" \
		"Lumen source balance after IBC transfer"

	MARKET_ID="${BZE_IBC_DENOM}/${BZE_DENOM}"

	step "Create market on BeeZee"
	run_bze_tx "${BZE_MAKER_KEY}" tradebin create-market "${BZE_IBC_DENOM}" "${BZE_DENOM}" >/dev/null
	rest_assert_market_queries

	step "Create the first resting sell order"
	run_bze_tx "${BZE_MAKER_KEY}" tradebin create-order sell 12000 5 "${MARKET_ID}" >/dev/null
	wait_next_block "${BZE_RPC_URL}" "${BZE_LOG}"
	SELL_ORDER_ONE="$(query_user_order_ids "${BZE_MAKER_ADDR}" | head -n1)"
	expect_nonempty "${SELL_ORDER_ONE}" "sell order one id"
	rest_assert_orderbook_queries

	step "Create and cancel a second sell order"
	run_bze_tx "${BZE_MAKER_KEY}" tradebin create-order sell 6000 7 "${MARKET_ID}" >/dev/null
	wait_next_block "${BZE_RPC_URL}" "${BZE_LOG}"
	SELL_ORDER_TWO="$(query_user_order_ids "${BZE_MAKER_ADDR}" | grep -vx "${SELL_ORDER_ONE}" | head -n1)"
	expect_nonempty "${SELL_ORDER_TWO}" "sell order two id"
	run_bze_tx "${BZE_MAKER_KEY}" tradebin cancel-order "${MARKET_ID}" "${SELL_ORDER_TWO}" sell >/dev/null
	wait_next_block "${BZE_RPC_URL}" "${BZE_LOG}"
	expect_eq "$(query_user_order_id_count "${BZE_MAKER_ADDR}")" "1" "maker open order count after cancel"

	step "Create a resting taker buy order"
	run_bze_tx "${BZE_TAKER_KEY}" tradebin create-order buy 4000 4 "${MARKET_ID}" >/dev/null
	wait_next_block "${BZE_RPC_URL}" "${BZE_LOG}"
	BUY_ORDER_ONE="$(query_user_order_ids "${BZE_TAKER_ADDR}" | head -n1)"
	expect_nonempty "${BUY_ORDER_ONE}" "buy order id"

	step "Fill the taker buy order"
	local taker_ibc_before maker_ibc_before
	taker_ibc_before="$(query_balance_amount "${BZE_BIN}" "${BZE_RPC_LADDR}" "${BZE_TAKER_ADDR}" "${BZE_IBC_DENOM}")"
	maker_ibc_before="$(query_balance_amount "${BZE_BIN}" "${BZE_RPC_LADDR}" "${BZE_MAKER_ADDR}" "${BZE_IBC_DENOM}")"
	# BeeZee exposes `fill-orders` through AutoCLI, but today its repeated field only accepts one JSON object positional.
	run_bze_tx "${BZE_MAKER_KEY}" tradebin fill-orders "${MARKET_ID}" buy '{"price":"4","amount":"4000"}' >/dev/null
	wait_next_block "${BZE_RPC_URL}" "${BZE_LOG}"
	expect_gt "$(query_balance_amount "${BZE_BIN}" "${BZE_RPC_LADDR}" "${BZE_TAKER_ADDR}" "${BZE_IBC_DENOM}")" "${taker_ibc_before}" "taker IBC balance after fill"
	expect_lt "$(query_balance_amount "${BZE_BIN}" "${BZE_RPC_LADDR}" "${BZE_MAKER_ADDR}" "${BZE_IBC_DENOM}")" "${maker_ibc_before}" "maker IBC balance after fill"
	expect_eq "$(query_user_order_id_count "${BZE_TAKER_ADDR}")" "0" "taker open order count after full fill"
	rest_assert_history_queries

	step "Create a liquidity pool"
	run_bze_tx "${BZE_MAKER_KEY}" tradebin create-liquidity-pool "${BZE_IBC_DENOM}" "${BZE_DENOM}" 0.003 '{"treasury":"0","burner":"0.5","providers":"0.5"}' false 10000 10000 >/dev/null
	wait_next_block "${BZE_RPC_URL}" "${BZE_LOG}"
	POOL_ID="$(compute_pool_id "${BZE_IBC_DENOM}" "${BZE_DENOM}")"
	POOL_LP_DENOM="ulp_${POOL_ID}"
	rest_assert_pool_queries

	step "Add liquidity"
	run_bze_tx "${BZE_MAKER_KEY}" tradebin add-liquidity "${POOL_ID}" 2000 2000 1 >/dev/null
	wait_next_block "${BZE_RPC_URL}" "${BZE_LOG}"
	local lp_balance
	lp_balance="$(query_balance_amount "${BZE_BIN}" "${BZE_RPC_LADDR}" "${BZE_MAKER_ADDR}" "${POOL_LP_DENOM}")"
	expect_positive_int "${lp_balance}" "maker LP balance after add-liquidity"

	step "Remove liquidity"
	local remove_lp
	remove_lp="$((lp_balance / 2))"
	if [ "${remove_lp}" -le 0 ]; then
		remove_lp="${lp_balance}"
	fi
	run_bze_tx "${BZE_MAKER_KEY}" tradebin remove-liquidity "${POOL_ID}" "${remove_lp}" 1 1 >/dev/null
	wait_next_block "${BZE_RPC_URL}" "${BZE_LOG}"

	step "Swap through the new pool"
	taker_ibc_before="$(query_balance_amount "${BZE_BIN}" "${BZE_RPC_LADDR}" "${BZE_TAKER_ADDR}" "${BZE_IBC_DENOM}")"
	taker_ubze_before="$(query_balance_amount "${BZE_BIN}" "${BZE_RPC_LADDR}" "${BZE_TAKER_ADDR}" "${BZE_DENOM}")"
	run_bze_tx "${BZE_TAKER_KEY}" tradebin multi-swap "${POOL_ID}" 1000ubze 500"${BZE_IBC_DENOM}" >/dev/null
	wait_next_block "${BZE_RPC_URL}" "${BZE_LOG}"
	expect_gt "$(query_balance_amount "${BZE_BIN}" "${BZE_RPC_LADDR}" "${BZE_TAKER_ADDR}" "${BZE_IBC_DENOM}")" "${taker_ibc_before}" "taker IBC balance after swap"
	expect_lt "$(query_balance_amount "${BZE_BIN}" "${BZE_RPC_LADDR}" "${BZE_TAKER_ADDR}" "${BZE_DENOM}")" "${taker_ubze_before}" "taker ubze balance after swap"

	step "Final IBC sanity checks"
	expect_contains "$("${LUMEN_BIN}" query ibc channel channels --node "${LUMEN_RPC_LADDR}" -o json)" "\"channel_id\":\"${LUMEN_CHANNEL_ID}\"" "Lumen IBC channel query"
	expect_contains "$("${BZE_BIN}" query ibc channel channels --node "${BZE_RPC_LADDR}" -o json)" "\"channel_id\":\"${BZE_CHANNEL_ID}\"" "BeeZee IBC channel query"

	printf '\nDEX / IBC e2e completed successfully.\n'
	note "Lumen logs: ${LUMEN_LOG}"
	note "BeeZee logs: ${BZE_LOG}"
	note "Relayer logs: ${RELAYER_LOG}"
}

main "$@"
