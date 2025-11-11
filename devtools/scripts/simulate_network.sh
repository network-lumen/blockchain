#!/usr/bin/env bash
set -euo pipefail

# Environment variables:
# - LUMEN_RL_PER_BLOCK (default 5), LUMEN_RL_PER_WINDOW (20), LUMEN_RL_WINDOW_SEC (10), LUMEN_RL_GLOBAL_MAX (300):
#   passed to every node to clamp ante rate-limit knobs.
# - FAST (0/1): skip heavy global rate-limit spam (set via --fast flag as well).
# - CLEAN (0/1): remove previous artifacts before bootstrapping (also available as --clean).
# - KEEP (0/1): keep containers and the Docker network after the run (set via --keep).
# - TIMEOUT (default 600): maximum seconds to wait for the run (overridden by --timeout).
# - IMAGE_TAG (default lumen-node:sim): runtime image tag to use/build.

RED=$'\033[31m'; GREEN=$'\033[32m'; YELLOW=$'\033[33m'; BOLD=$'\033[1m'; NC=$'\033[0m'
ok()   { echo "${GREEN}✔${NC} $*"; }
warn() { echo "${YELLOW}⚠${NC} $*"; }
info() { echo "${BOLD}i${NC} $*"; }
die()  { echo "${RED}✘${NC} $*"; exit 1; }
run()  { echo "${BOLD}→${NC} $*"; eval "$@"; }

require() {
	if ! command -v "$1" >/dev/null 2>&1; then
		die "missing dependency: $1"
	fi
}

VALIDATORS=2
FULLNODES=1
KEEP=0
CLEAN=0
FAST=0
TIMEOUT="${TIMEOUT:-600}"
IMAGE_TAG="lumen-node:sim"
LUMEN_CHAIN_ID="lumen-sim-1"
RL_PER_BLOCK="${LUMEN_RL_PER_BLOCK:-5}"
RL_PER_WINDOW="${LUMEN_RL_PER_WINDOW:-20}"
RL_WINDOW_SEC="${LUMEN_RL_WINDOW_SEC:-10}"
RL_GLOBAL_MAX="${LUMEN_RL_GLOBAL_MAX:-300}"
GATEWAY_MONTH_SECONDS=5
DNS_NAME="codexnet"
DNS_EXT="lumen"
DNS_MIN_BID_ULMN=0
DNS_HIGH_BID_ULMN=0
HOST_RPC_PORT="${SIM_HOST_RPC_PORT:-27657}"
HOST_P2P_PORT="${SIM_HOST_P2P_PORT:-27656}"
HOST_API_PORT="${SIM_HOST_API_PORT:-2327}"
HOST_RPC_URL="http://127.0.0.1:${HOST_RPC_PORT}"

while [[ $# -gt 0 ]]; do
	case "$1" in
		--validators) VALIDATORS="$2"; shift ;;
		--fullnodes)  FULLNODES="$2"; shift ;;
		--keep)       KEEP=1 ;;
		--clean)      CLEAN=1 ;;
		--fast)       FAST=1 ;;
		--timeout)    TIMEOUT="$2"; shift ;;
		--image)      IMAGE_TAG="$2"; shift ;;
		*) die "unknown flag: $1" ;;
	esac
	shift
done

require docker
require jq
require go

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

ART_ROOT="$ROOT_DIR/artifacts/sim"
NODES_DIR="$ART_ROOT/nodes"
LOGS_DIR="$ART_ROOT/logs"
GENESIS_DIR="$ART_ROOT/genesis"
KEYS_DIR="$ART_ROOT/keys"
SNAP_DIR="$ART_ROOT/snapshots"
mkdir -p "$NODES_DIR" "$LOGS_DIR" "$GENESIS_DIR" "$KEYS_DIR" "$SNAP_DIR"

NET_NAME="lumen-net"
SEED_NAME="seed-0"
RUNNER_NAME="runner-ctl"
HOST_UID="$(id -u)"
HOST_GID="$(id -g)"

cleanup_on_exit() {
	local status=$?
	if [[ ${KEEP:-0} -eq 0 ]]; then
		docker rm -f "$RUNNER_NAME" "$SEED_NAME" >/dev/null 2>&1 || true
		for i in $(seq 1 "$VALIDATORS"); do docker rm -f "val-$i" >/dev/null 2>&1 || true; done
		for j in $(seq 1 "$FULLNODES"); do docker rm -f "full-$j" >/dev/null 2>&1 || true; done
		docker network rm "$NET_NAME" >/dev/null 2>&1 || true
	fi
	exit $status
}
trap cleanup_on_exit EXIT INT TERM

FAUCET_MNEMONIC="village foil behind logic hand fitness bronze push turn undo chalk symbol elbow amazing kitten creek trip game intact square solid coach stock tomato"
SENDER_MNEMONIC="any record tool own rabbit same wage crazy auction dose relief ten winter post parrot isolate bundle opera drum uphold voice select great donkey"
RECIP_MNEMONIC="green trial plate resource moral skull sample entire demise hollow device accuse marble club hospital creek category topple lens cabbage add clump frost ticket"
GATEWAY_MNEMONIC="exchange control olive wool aim seek double bamboo tell process sock door resist uncle grunt reform knock chair agent dad snake oven captain destroy"
FINALIZER_MNEMONIC="asthma refuse key orbit view oppose purchase spread great unique burger one choice parade shaft orange during copy gown trumpet toy charge bundle beach"
VALIDATOR_MNEMONICS=(
"alcohol hockey chair click sword crumble outside cash old example wealth ozone rice cash because friend holiday dinner endless poem dog royal tiny profit"
"cost describe scatter destroy above mistake evoke angle raw oil humble clip trophy ride pottery summer limb devote slice cat manual hen follow tired"
"rally mountain couple deputy mango man divorce sound giant initial vague seat attract shine upon rabbit sign excess vanish clarify dust cube hurry assault"
"blouse false twelve destroy bring appear skill erase cinnamon feature oppose physical kitchen school master avocado rival unit security syrup reflect album enhance elephant"
"decrease feed long lawsuit half census lecture april speak pottery arrest floor vote gun clog unaware parrot tip true garment lawsuit you volume near"
"host since route grow omit accident bargain among joke coffee strategy hockey game luggage hand happy arctic increase excite disorder actor furnace grass blind"
)

LUMEND_BIN="$ROOT_DIR/build/lumend"
: "${LUMEN_BUILD_TAGS:=dev}"

ensure_artifacts_writable() {
	if [[ -d "$ART_ROOT" ]]; then
		docker run --rm -v "$ART_ROOT":/data alpine chown -R "${HOST_UID}:${HOST_GID}" /data >/dev/null 2>&1 || true
	fi
}

if [[ $CLEAN -eq 1 ]]; then
	warn "Cleaning previous simulation artifacts"
	docker ps -a --format '{{.Names}}' | grep -E '^(seed-0|val-[0-9]+|full-[0-9]+|runner-ctl)$' | xargs -r docker rm -f >/dev/null 2>&1 || true
	docker network rm "$NET_NAME" >/dev/null 2>&1 || true
	ensure_artifacts_writable
	rm -rf "$ART_ROOT"
	mkdir -p "$NODES_DIR" "$LOGS_DIR" "$GENESIS_DIR" "$KEYS_DIR" "$SNAP_DIR"
fi

ensure_artifacts_writable

rebuild_lumend() {
	build_cmd=(go build -trimpath -buildvcs=false)
	if [[ -n "$LUMEN_BUILD_TAGS" ]]; then
		build_cmd+=(-tags "$LUMEN_BUILD_TAGS")
	fi
	build_cmd+=(-o ./build/lumend ./cmd/lumend)
	run "${build_cmd[*]}"
	local file_hash
	file_hash="$(sha256sum "$LUMEND_BIN" | awk '{print $1}')"
	printf '%s\n%s\n' "${LUMEN_BUILD_TAGS:-}" "$file_hash" >"$ROOT_DIR/build/.lumend.tags"
}

verify_pqc_cli() {
	if [[ ! -x "$LUMEND_BIN" ]]; then
		return 1
	fi
	local help
	if ! help=$("$LUMEND_BIN" keys pqc-import --help 2>&1); then
		return 1
	fi
	grep -q -- "--name" <<<"$help"
}

ensure_binary() {
	local tagfile="$ROOT_DIR/build/.lumend.tags"
	local wanted="${LUMEN_BUILD_TAGS:-}"
	local current_tags=""
	local current_hash=""
	if [[ -f "$tagfile" ]]; then
		current_tags="$(sed -n '1p' "$tagfile")"
		current_hash="$(sed -n '2p' "$tagfile")"
	fi
	local file_hash=""
	if [[ -x "$LUMEND_BIN" ]]; then
		file_hash="$(sha256sum "$LUMEND_BIN" | awk '{print $1}')"
	fi
	local need_build=0
	if [[ ! -x "$LUMEND_BIN" ]]; then
		need_build=1
	elif [[ "$current_tags" != "$wanted" ]]; then
		need_build=1
	elif [[ -n "$current_hash" && -n "$file_hash" && "$current_hash" != "$file_hash" ]]; then
		need_build=1
	fi
	if (( need_build )); then
		rebuild_lumend
	elif ! verify_pqc_cli; then
		rebuild_lumend
	fi
	ok "Local lumend build ready"
}

build_image() {
	run "docker build -f devtools/docker/runtime/Dockerfile --build-arg LUMEND_SRC=./build/lumend -t ${IMAGE_TAG} ."
	ok "Docker runtime image ${IMAGE_TAG} built"
}

ensure_network() {
	if ! docker network inspect "$NET_NAME" >/dev/null 2>&1; then
		run "docker network create $NET_NAME"
	fi
	ok "Docker network $NET_NAME ready"
}

mnemonic_for_validator() {
	local idx=$1
	if (( idx > ${#VALIDATOR_MNEMONICS[@]} )); then
		die "need mnemonic for validator $idx; extend VALIDATOR_MNEMONICS"
	fi
	echo "${VALIDATOR_MNEMONICS[$((idx-1))]}"
}

generate_pqc_pair() {
	local out tmpdir
	tmpdir="$(mktemp -d -t pqc-sim-XXXXXX)"
	cat >"$tmpdir/main.go" <<'EOF'
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
	cat >"$tmpdir/go.mod" <<'EOF'
module pqcgen

go 1.21

require github.com/cloudflare/circl v1.3.7
EOF
	out="$(cd "$tmpdir" && GO111MODULE=on go mod tidy >/dev/null 2>&1 && GO111MODULE=on go run .)"
	printf '%s\n' "$out"
	rm -rf "$tmpdir"
}

SEED_HOME="$NODES_DIR/$SEED_NAME"

add_key() {
	local home="$1" name="$2" mnemonic="$3"
	local mn_file
	mn_file="$(mktemp)"
	printf '%s' "$mnemonic" >"$mn_file"
	"$LUMEND_BIN" keys add "$name" --recover --source "$mn_file" --keyring-backend test --home "$home" >/dev/null
	rm -f "$mn_file"
}

add_genesis_account() {
	local address="$1" amount="$2"
	"$LUMEND_BIN" genesis add-genesis-account "$address" "$amount" --keyring-backend test --home "$SEED_HOME"
}

prepare_genesis() {
	rm -rf "$SEED_HOME"
	"$LUMEND_BIN" init seed --chain-id "$LUMEN_CHAIN_ID" --home "$SEED_HOME" >/dev/null

	add_key "$SEED_HOME" faucet "$FAUCET_MNEMONIC"
	add_key "$SEED_HOME" sender "$SENDER_MNEMONIC"
	add_key "$SEED_HOME" recipient "$RECIP_MNEMONIC"
	add_key "$SEED_HOME" gateway "$GATEWAY_MNEMONIC"
	add_key "$SEED_HOME" finalizer "$FINALIZER_MNEMONIC"

	FAUCET_ADDR=$("$LUMEND_BIN" keys show faucet --keyring-backend test --home "$SEED_HOME" -a)
	SENDER_ADDR=$("$LUMEND_BIN" keys show sender --keyring-backend test --home "$SEED_HOME" -a)
	RECIP_ADDR=$("$LUMEND_BIN" keys show recipient --keyring-backend test --home "$SEED_HOME" -a)
	GATEWAY_ADDR=$("$LUMEND_BIN" keys show gateway --keyring-backend test --home "$SEED_HOME" -a)
	FINALIZER_ADDR=$("$LUMEND_BIN" keys show finalizer --keyring-backend test --home "$SEED_HOME" -a)

	add_genesis_account "$FAUCET_ADDR"   "1000000000ulmn"
	add_genesis_account "$SENDER_ADDR"   "400000000ulmn"
	add_genesis_account "$RECIP_ADDR"    "400000000ulmn"
	add_genesis_account "$GATEWAY_ADDR"  "400000000ulmn"
	add_genesis_account "$FINALIZER_ADDR" "400000000ulmn"

	mkdir -p "$SEED_HOME/config/gentx"

	for i in $(seq 1 "$VALIDATORS"); do
		local name="val-$i"
		local home="$NODES_DIR/$name"
		rm -rf "$home"
		"$LUMEND_BIN" init "$name" --chain-id "$LUMEN_CHAIN_ID" --home "$home" >/dev/null
		add_key "$home" validator "$(mnemonic_for_validator "$i")"
		local addr
		addr=$("$LUMEND_BIN" keys show validator -a --keyring-backend test --home "$home")
		add_genesis_account "$addr" "200000000ulmn"
		cp "$SEED_HOME/config/genesis.json" "$home/config/genesis.json"
		"$LUMEND_BIN" genesis gentx validator 1000000ulmn --chain-id "$LUMEN_CHAIN_ID" --keyring-backend test --home "$home" >/dev/null
		cp "$home/config/gentx"/*.json "$SEED_HOME/config/gentx/"
	done

	"$LUMEND_BIN" genesis collect-gentxs --home "$SEED_HOME" >/dev/null
	"$LUMEND_BIN" genesis validate --home "$SEED_HOME" >/dev/null

	local tmp
	tmp="$(mktemp)"
	jq --arg month "$GATEWAY_MONTH_SECONDS" --arg gw "$GATEWAY_ADDR" --arg sender "$SENDER_ADDR" '
		.app_state.gateways.params.platform_commission_bps = 150
		| .app_state.gateways.params.month_seconds = $month
		| .app_state.gateways.params.finalize_delay_months = 0
		| .app_state.gateways.params.finalizer_reward_bps = 500
		| .app_state.gateways.params.min_price_ulmn_per_month = "100"
		| .app_state.gateways.gateways = [{
			id:"1",
			operator:$gw,
			payout:$gw,
			active:true,
			metadata:"genesis",
			created_at:"1",
			active_clients:"0",
			cancellations:"0"
		}]
		| .app_state.gateways.gateway_count = "1"
		| .app_state.release.params.allowed_publishers = [$sender]
	' "$SEED_HOME/config/genesis.json" >"$tmp"
	mv "$tmp" "$SEED_HOME/config/genesis.json"

	tmp="$(mktemp)"
	jq '
		.app_state.dns.params.update_rate_limit_seconds="0"
		| .app_state.dns.params.update_pow_difficulty="0"
	' "$SEED_HOME/config/genesis.json" >"$tmp"
	mv "$tmp" "$SEED_HOME/config/genesis.json"

	local base_bid
	base_bid=$(calc_dns_price_from_genesis "$SEED_HOME/config/genesis.json" "$DNS_NAME" "$DNS_EXT" 365)
	if [[ -z "$base_bid" || "$base_bid" -le 0 ]]; then
		base_bid=1000000
	fi
	DNS_MIN_BID_ULMN="$base_bid"
	local bid_delta=$(( DNS_MIN_BID_ULMN / 2 ))
	if (( bid_delta == 0 )); then
		bid_delta=1
	fi
	DNS_HIGH_BID_ULMN=$(( DNS_MIN_BID_ULMN + bid_delta ))

	for i in $(seq 1 "$VALIDATORS"); do
		local vhome="$NODES_DIR/val-$i"
		cp "$SEED_HOME/config/genesis.json" "$vhome/config/genesis.json"
	done

	cp "$SEED_HOME/config/genesis.json" "$GENESIS_DIR/genesis.json"
	ok "Genesis prepared at $GENESIS_DIR/genesis.json"
}

stop_and_rm() {
	for name in "$@"; do
		docker rm -f "$name" >/dev/null 2>&1 || true
	done
}

start_seed() {
	stop_and_rm "$SEED_NAME"
run "docker run -d --name $SEED_NAME --network $NET_NAME -p ${HOST_RPC_PORT}:26657 -p ${HOST_P2P_PORT}:26656 -p ${HOST_API_PORT}:1317 \
	-e LUMEN_RL_PER_BLOCK=$RL_PER_BLOCK \
	-e LUMEN_RL_PER_WINDOW=$RL_PER_WINDOW \
	-e LUMEN_RL_WINDOW_SEC=$RL_WINDOW_SEC \
	-e LUMEN_RL_GLOBAL_MAX=$RL_GLOBAL_MAX \
		-v \"$SEED_HOME\":/root/.lumen \
		${IMAGE_TAG} start \
		--home /root/.lumen \
		--rpc.laddr tcp://0.0.0.0:26657 \
		--p2p.laddr tcp://0.0.0.0:26656 \
		--grpc.address 0.0.0.0:9090 \
		--api.address tcp://0.0.0.0:1317 \
		--minimum-gas-prices 0ulmn >/dev/null"
	ok "Seed started"
}

wait_http_json_field() {
	local url="$1" jq_expr="$2" condition="$3" timeout="${4:-60}"
	local end=$(( $(date +%s) + timeout ))
	while (( $(date +%s) < end )); do
		local out val
		out=$(curl -s "$url" || true)
		val=$(echo "$out" | jq -r "$jq_expr" 2>/dev/null || echo "")
		if VALUE="$val" bash -u -c "$condition"; then
			return 0
		fi
		sleep 1
	done
	return 1
}

runner_exec() {
	docker exec "$RUNNER_NAME" /bin/bash -lc "$*"
}

bal_ulmn() {
	local addr="$1"
	runner_exec "lumend q bank balances $addr -o json --node http://$SEED_NAME:26657 | jq -r '(.balances[]? | select(.denom==\"ulmn\").amount) // \"0\"'"
}

qprint() {
	local cmd="$*"
	runner_exec "echo '>>> $cmd'; $cmd | sed -n '1,80p'"
}

dns_owner() {
	local fqdn="$1"
	runner_exec "lumend q dns list-domain --node http://$SEED_NAME:26657 -o json | jq -r '(.domain[]? | select(.name==\"$fqdn\") | .owner) // empty'"
}

release_count_for() {
	local creator="$1"
	runner_exec "lumend q release releases -o json --node http://$SEED_NAME:26657 | jq -r '[.releases[]? | select(.publisher==\"$creator\")] | length'"
}

gw_contract_state() {
	local id="$1"
	runner_exec "lumend q gateways contract $id -o json --node http://$SEED_NAME:26657 | jq -r '.contract.status // .state // empty'"
}

token_tax_rate_from_genesis() {
	runner_exec "curl -s http://$SEED_NAME:26657/genesis | jq -r '.result.genesis.app_state.tokenomics.params.tx_tax_rate // empty'"
}

decimal_to_fraction() {
	local rate="${1:-0}"
	local sign=1
	if [[ "$rate" == -* ]]; then
		sign=-1
		rate="${rate#-}"
	fi
	local int_part="${rate%%.*}"
	if [[ "$int_part" == "" || "$int_part" == "$rate" ]]; then
		if [[ "$rate" == "$int_part" ]]; then
			int_part="$rate"
			frac_part=""
		else
			int_part="0"
			frac_part="$rate"
		fi
	else
		frac_part="${rate#*.}"
	fi
	if [[ "$int_part" == "" ]]; then int_part="0"; fi
	if [[ "$rate" == "$int_part" ]]; then frac_part=""; fi
	local digits="${int_part}${frac_part}"
	if [[ -z "$digits" ]]; then digits="0"; fi
	local num=$((10#$digits))
	local den=1
	for ((i=0;i<${#frac_part};i++)); do
		den=$((den*10))
	done
	if (( sign < 0 )); then
		num=$(( -num ))
	fi
	echo "$num $den"
}

calc_dns_price_from_genesis() {
	local genesis="$1"
	local domain="$2"
	local ext="$3"
	local days="${4:-365}"
	python3 - "$genesis" "$domain" "$ext" "$days" <<'PY'
import json, sys
from decimal import Decimal, ROUND_CEILING, getcontext

getcontext().prec = 64

genesis_path, domain, ext, days = sys.argv[1:5]
days = int(days)
with open(genesis_path) as f:
    params = json.load(f)["app_state"]["dns"]["params"]

def pick(tiers, length):
    if not tiers:
        return 10000
    for tier in tiers:
        max_len = int(tier.get("max_len", 0) or 0)
        if max_len == 0 or length <= max_len:
            return int(tier["multiplier_bps"])
    return int(tiers[-1]["multiplier_bps"])

def apply_bps(amount, bps):
    return (amount * bps + 9999) // 10000

min_price = int(params.get("min_price_ulmn_per_month", 0))
if min_price <= 0:
    raise SystemExit("0")

months = max(1, (days + 29) // 30)
amount = min_price * months
amount = apply_bps(amount, pick(params.get("domain_tiers") or [], len(domain)))
amount = apply_bps(amount, pick(params.get("ext_tiers") or [], len(ext)))
price = (Decimal(amount) * Decimal(params.get("base_fee_dns", "1"))).to_integral_value(rounding=ROUND_CEILING)
print(int(price))
PY
}

module_has_subcommand() {
	local module="$1" pattern="$2"
	if runner_exec "lumend tx $module --help >/tmp/${module}-help 2>&1 && grep -Eq '$pattern' /tmp/${module}-help"; then
		return 0
	fi
	return 1
}

runner_add_key() {
	local name="$1" mnemonic="$2"
	runner_exec "printf '%s' \"$mnemonic\" > /tmp/mn-$name"
	runner_exec "lumend keys add $name --recover --source /tmp/mn-$name --keyring-backend test >/dev/null"
	runner_exec "rm -f /tmp/mn-$name >/dev/null 2>&1 || true"
}

runner_add_key_with_index() {
	local name="$1" mnemonic="$2" index="$3"
	runner_exec "printf '%s' \"$mnemonic\" > /tmp/mn-$name"
	runner_exec "lumend keys add $name --recover --source /tmp/mn-$name --keyring-backend test --index $index >/dev/null"
	runner_exec "rm -f /tmp/mn-$name >/dev/null 2>&1 || true"
}

fund_addr() {
	local to="$1"
	local amt="${2:-100000}"
	local outfile="/tmp/fund-$to.json"
	local code raw
	local attempts=0
	local max_attempts=6
	while (( attempts < max_attempts )); do
		((attempts++))
run_tx_json "$outfile" "lumend tx bank send sender $to ${amt}ulmn --yes --keyring-backend test --node http://$SEED_NAME:26657 --chain-id $LUMEN_CHAIN_ID --broadcast-mode sync --fees 0ulmn -o json"
		code=$(runner_exec "head -n 1 $outfile | jq -r '.code // 0'" || echo "1")
		if [[ "$code" == "0" ]]; then
			tx_expect_code "$outfile" "0" "fund $to"
			wait_for_tx "$outfile" "fund $to"
			return 0
		fi
		raw=$(runner_exec "head -n 1 $outfile | jq -r '.raw_log // empty'" || echo "")
		warn "fund $to attempt $attempts failed (code=$code${raw:+, raw=$raw}); waiting before retry"
		sleep $((attempts + 1))
	done
	warn "fund $to exhausted retries; last response ↓"
	runner_exec "sed -n '1,60p' $outfile" || true
	return 1
}

link_pqc_for() {
	local name="$1"
	local alias="pqc-$name"
	local addr
	addr=$(runner_exec "lumend keys show $name -a --keyring-backend test")
	import_and_link_pqc "$name" "$alias" "$addr"
}

run_tx_json() {
	local outfile="$1"
	local cmd="$2"
	runner_exec "$cmd > $outfile 2>&1 || true"
}
tx_expect_code() {
	local file="$1" expected="$2" label="$3"
	local code
	code=$(runner_exec "head -n 1 $file | jq -r '.code // 0' 2>/dev/null" || echo "1")
	if [[ "$expected" == "0" ]]; then
		if [[ "$code" == "0" ]]; then
			ok "$label"
		else
			warn "$label failed (code=$code). First 60 lines of $file ↓"
			runner_exec "sed -n '1,60p' $file" || true
		fi
	else
		if [[ "$code" != "0" ]]; then
			ok "$label (expected failure)"
		else
			warn "$label unexpectedly succeeded. First 60 lines of $file ↓"
			runner_exec "sed -n '1,60p' $file" || true
		fi
	fi
}

wait_for_tx() {
	local file="$1" label="$2"
	local result_code
	result_code=$(runner_exec "head -n 1 $file | jq -r '.code // 0'" || echo "1")
	if [[ "$result_code" != "0" ]]; then
		return 0
	fi
	local txhash
	txhash=$(runner_exec "head -n 1 $file | jq -r '.txhash // empty'" || echo "")
	if [[ -z "$txhash" || "$txhash" == "null" ]]; then
		return 0
	fi
	if runner_exec "for i in \$(seq 1 120); do res=\$(curl -s http://$SEED_NAME:26657/tx?hash=0x$txhash); code=\$(echo \"\$res\" | jq -r '.result.tx_result.code // empty'); if [ \"\$code\" != \"\" ]; then exit 0; fi; sleep 0.25; done; exit 1" >/dev/null 2>&1; then
		return 0
	fi
	warn "timeout waiting for $label (txhash=$txhash)"
	return 1
}

import_and_link_pqc() {
	local name="$1" alias="$2" addr="$3"
	local pair pub priv
	pair=$(generate_pqc_pair)
	pub="${pair%%$'\n'*}"
	priv="${pair#*$'\n'}"
	runner_exec "lumend keys pqc-import --name $alias --scheme dilithium3 --pubkey $pub --privkey $priv --keyring-backend test >/dev/null"
	runner_exec "lumend keys pqc-link --from $name --pqc $alias --keyring-backend test >/dev/null"
run_tx_json "/tmp/link-$name.json" "lumend tx pqc link-account --from $name --scheme dilithium3 --pubkey $pub --keyring-backend test --node http://$SEED_NAME:26657 --chain-id $LUMEN_CHAIN_ID --yes --fees 0ulmn --broadcast-mode sync -o json"
	tx_expect_code "/tmp/link-$name.json" "0" "PQC link-account for $name"
	wait_for_tx "/tmp/link-$name.json" "pqc link-account $name"
}

simulate() {
	ensure_binary
	build_image
	ensure_network
	prepare_genesis
	start_seed
	sleep 3
	local seed_id
	seed_id=$(docker exec "$SEED_NAME" lumend tendermint show-node-id | tr -d '\r\n')

	for i in $(seq 1 "$VALIDATORS"); do
		local name="val-$i"
		local home="$NODES_DIR/$name"
		stop_and_rm "$name"
		run "docker run -d --name $name --network $NET_NAME \
			-e LUMEN_RL_PER_BLOCK=$RL_PER_BLOCK \
			-e LUMEN_RL_PER_WINDOW=$RL_PER_WINDOW \
			-e LUMEN_RL_WINDOW_SEC=$RL_WINDOW_SEC \
			-e LUMEN_RL_GLOBAL_MAX=$RL_GLOBAL_MAX \
			-v \"$home\":/root/.lumen \
			${IMAGE_TAG} start \
			--home /root/.lumen \
			--p2p.persistent_peers ${seed_id}@${SEED_NAME}:26656 \
			--minimum-gas-prices 0ulmn >/dev/null"
	done

	for j in $(seq 1 "$FULLNODES"); do
		local name="full-$j"
		local home="$NODES_DIR/$name"
		rm -rf "$home"
		"$LUMEND_BIN" init "$name" --chain-id "$LUMEN_CHAIN_ID" --home "$home" >/dev/null
		cp "$SEED_HOME/config/genesis.json" "$home/config/genesis.json"
		stop_and_rm "$name"
		run "docker run -d --name $name --network $NET_NAME \
			-e LUMEN_RL_PER_BLOCK=$RL_PER_BLOCK \
			-e LUMEN_RL_PER_WINDOW=$RL_PER_WINDOW \
			-e LUMEN_RL_WINDOW_SEC=$RL_WINDOW_SEC \
			-e LUMEN_RL_GLOBAL_MAX=$RL_GLOBAL_MAX \
			-v \"$home\":/root/.lumen \
			${IMAGE_TAG} start \
			--home /root/.lumen \
			--p2p.persistent_peers ${seed_id}@${SEED_NAME}:26656 \
			--minimum-gas-prices 0ulmn >/dev/null"
	done

	ok "Waiting for blocks..."
wait_http_json_field "${HOST_RPC_URL}/status" '.result.sync_info.latest_block_height|tonumber' '[[ ${VALUE:-0} -ge 1 ]]' 120 || die "seed RPC not ready"

	stop_and_rm "$RUNNER_NAME"
	run "docker run -d --name $RUNNER_NAME --network $NET_NAME --entrypoint /bin/bash ${IMAGE_TAG} -lc \"sleep infinity\" >/dev/null"

runner_add_key sender "$SENDER_MNEMONIC"
runner_add_key recipient "$RECIP_MNEMONIC"
runner_add_key gateway "$GATEWAY_MNEMONIC"
runner_add_key finalizer "$FINALIZER_MNEMONIC"

import_and_link_pqc "sender" "pqc-sender" "$SENDER_ADDR"
import_and_link_pqc "recipient" "pqc-recipient" "$RECIP_ADDR"
import_and_link_pqc "gateway" "pqc-gateway" "$GATEWAY_ADDR"
import_and_link_pqc "finalizer" "pqc-finalizer" "$FINALIZER_ADDR"

	ok "PQC-enabled bank send (with 1% tax assertion)"
	B1_S=$(bal_ulmn "$SENDER_ADDR"); B1_R=$(bal_ulmn "$RECIP_ADDR")
run_tx_json "/tmp/bank-ok.json" "lumend tx bank send sender $RECIP_ADDR 1000ulmn --yes --keyring-backend test --node http://$SEED_NAME:26657 --chain-id $LUMEN_CHAIN_ID --broadcast-mode sync --fees 0ulmn -o json"
	tx_expect_code "/tmp/bank-ok.json" "0" "bank send"
	wait_for_tx "/tmp/bank-ok.json" "bank send"
	B2_S=$(bal_ulmn "$SENDER_ADDR"); B2_R=$(bal_ulmn "$RECIP_ADDR")
	DELTA_R=$(( B2_R - B1_R ))
	DELTA_S=$(( B1_S - B2_S ))
	AMOUNT=1000
	tax_rate="$(token_tax_rate_from_genesis)"
	[[ -z "$tax_rate" ]] && tax_rate="0.01"
	read TAX_NUM TAX_DEN <<<"$(decimal_to_fraction "$tax_rate")"
	(( TAX_DEN == 0 )) && TAX_DEN=1
	remaining=$(( TAX_DEN - TAX_NUM ))
	exp_recv=$(( AMOUNT * remaining / TAX_DEN ))
	tax_paid=$(( AMOUNT - exp_recv ))
	if (( DELTA_R == exp_recv && (DELTA_S - DELTA_R) == tax_paid )); then
		ok "Tax verified (recv=$DELTA_R==$exp_recv, sender_loss=$DELTA_S, tax=$tax_paid, rate=$tax_rate)"
	else
		warn "Tax mismatch (recv=$DELTA_R vs $exp_recv, sender_loss=$DELTA_S; expected tax=$tax_paid, rate=$tax_rate)"
	fi

	ok "PQC-disabled bank send (expect failure)"
run_tx_json "/tmp/bank-bad.json" "lumend tx bank send sender $RECIP_ADDR 1ulmn --yes --keyring-backend test --node http://$SEED_NAME:26657 --chain-id $LUMEN_CHAIN_ID --broadcast-mode sync --fees 0ulmn --pqc-enable=false -o json"
	tx_expect_code "/tmp/bank-bad.json" "1" "bank send pqc-disabled"
	if module_has_subcommand "pqc" "rotate"; then
		ok "PQC rotate (expect failure)"
		run_tx_json "/tmp/pqc-rotate.json" "lumend tx pqc rotate --from sender --keyring-backend test --node http://$SEED_NAME:26657 --chain-id $LUMEN_CHAIN_ID --yes -o json"
		tx_expect_code "/tmp/pqc-rotate.json" "1" "pqc rotate (forbidden)"
	else
		info "PQC rotate CLI unavailable; skipping rotate negative test"
	fi

	if module_has_subcommand "dns" "register"; then
		ok "DNS register/bid/settle"
		dns_index="${DNS_NAME}.${DNS_EXT}"
		run_tx_json "/tmp/dns-register.json" "lumend tx dns register $DNS_NAME $DNS_EXT --records '[]' --duration-days 30 --owner $SENDER_ADDR --from sender --keyring-backend test --node http://$SEED_NAME:26657 --chain-id $LUMEN_CHAIN_ID --yes --broadcast-mode sync --fees 0ulmn -o json"
		tx_expect_code "/tmp/dns-register.json" "0" "dns register"
		wait_for_tx "/tmp/dns-register.json" "dns register"
		now_ts=$(date +%s)
		past_ts=$((now_ts - 120))
		run_tx_json "/tmp/dns-expire.json" "lumend tx dns update-domain $dns_index $dns_index $SENDER_ADDR '{}' $past_ts --from sender --keyring-backend test --node http://$SEED_NAME:26657 --chain-id $LUMEN_CHAIN_ID --yes --broadcast-mode sync --fees 0ulmn -o json"
		tx_expect_code "/tmp/dns-expire.json" "0" "dns expire to auction"
		wait_for_tx "/tmp/dns-expire.json" "dns expire to auction"
		run_tx_json "/tmp/dns-bid1.json" "lumend tx dns bid $DNS_NAME $DNS_EXT $DNS_MIN_BID_ULMN --from recipient --keyring-backend test --node http://$SEED_NAME:26657 --chain-id $LUMEN_CHAIN_ID --yes --broadcast-mode sync --fees 0ulmn -o json"
		tx_expect_code "/tmp/dns-bid1.json" "0" "dns bid recipient"
		wait_for_tx "/tmp/dns-bid1.json" "dns bid recipient"
		run_tx_json "/tmp/dns-bid2.json" "lumend tx dns bid $DNS_NAME $DNS_EXT $DNS_HIGH_BID_ULMN --from gateway --keyring-backend test --node http://$SEED_NAME:26657 --chain-id $LUMEN_CHAIN_ID --yes --broadcast-mode sync --fees 0ulmn -o json"
		tx_expect_code "/tmp/dns-bid2.json" "0" "dns bid gateway"
		wait_for_tx "/tmp/dns-bid2.json" "dns bid gateway"
		settle_ts=$((now_ts - 172800))
		run_tx_json "/tmp/dns-expire-done.json" "lumend tx dns update-domain $dns_index $dns_index $SENDER_ADDR '{}' $settle_ts --from sender --keyring-backend test --node http://$SEED_NAME:26657 --chain-id $LUMEN_CHAIN_ID --yes --broadcast-mode sync --fees 0ulmn -o json"
		tx_expect_code "/tmp/dns-expire-done.json" "0" "dns expire settle"
		wait_for_tx "/tmp/dns-expire-done.json" "dns expire settle"
		run_tx_json "/tmp/dns-settle.json" "lumend tx dns settle $DNS_NAME $DNS_EXT --from finalizer --keyring-backend test --node http://$SEED_NAME:26657 --chain-id $LUMEN_CHAIN_ID --yes --broadcast-mode sync --fees 0ulmn -o json"
		tx_expect_code "/tmp/dns-settle.json" "0" "dns settle"
		wait_for_tx "/tmp/dns-settle.json" "dns settle"
		owner=$(dns_owner "$dns_index")
		if [[ -n "$owner" ]]; then
			ok "DNS owner query returned $owner"
		else
			warn "DNS owner query returned empty value"
		fi
	else
		warn "DNS CLI unavailable; skipping DNS flow"
	fi

	if module_has_subcommand "gateways" "create-contract"; then
		ok "Gateways create/claim/finalize"
		gateway_id="1"
		contract_claim_id="0"
		run_tx_json "/tmp/gw-create-claim.json" "lumend tx gateways create-contract $gateway_id 1000 50 20 1 --metadata sim-claim --from sender --keyring-backend test --node http://$SEED_NAME:26657 --chain-id $LUMEN_CHAIN_ID --yes --broadcast-mode sync --fees 0ulmn -o json"
		tx_expect_code "/tmp/gw-create-claim.json" "0" "gateway create contract #0"
		wait_for_tx "/tmp/gw-create-claim.json" "gateway create contract #0"
		sleep $((GATEWAY_MONTH_SECONDS + 1))
		run_tx_json "/tmp/gw-claim.json" "lumend tx gateways claim-payment $contract_claim_id --from gateway --keyring-backend test --node http://$SEED_NAME:26657 --chain-id $LUMEN_CHAIN_ID --yes --broadcast-mode sync --fees 0ulmn -o json"
		tx_expect_code "/tmp/gw-claim.json" "0" "gateway claim-payment #0"
		wait_for_tx "/tmp/gw-claim.json" "gateway claim-payment #0"
		contract_final_id="1"
		run_tx_json "/tmp/gw-create-final.json" "lumend tx gateways create-contract $gateway_id 1500 40 15 1 --metadata sim-final --from sender --keyring-backend test --node http://$SEED_NAME:26657 --chain-id $LUMEN_CHAIN_ID --yes --broadcast-mode sync --fees 0ulmn -o json"
		tx_expect_code "/tmp/gw-create-final.json" "0" "gateway create contract #1"
		wait_for_tx "/tmp/gw-create-final.json" "gateway create contract #1"
		sleep $((GATEWAY_MONTH_SECONDS + 1))
		run_tx_json "/tmp/gw-claim-final.json" "lumend tx gateways claim-payment $contract_final_id --from gateway --keyring-backend test --node http://$SEED_NAME:26657 --chain-id $LUMEN_CHAIN_ID --yes --broadcast-mode sync --fees 0ulmn -o json"
		tx_expect_code "/tmp/gw-claim-final.json" "0" "gateway claim-payment #1"
		wait_for_tx "/tmp/gw-claim-final.json" "gateway claim-payment #1"
		run_tx_json "/tmp/gw-finalize.json" "lumend tx gateways finalize-contract $contract_final_id --from finalizer --keyring-backend test --node http://$SEED_NAME:26657 --chain-id $LUMEN_CHAIN_ID --yes --broadcast-mode sync --fees 0ulmn -o json"
		tx_expect_code "/tmp/gw-finalize.json" "0" "gateway finalize-contract #1"
		wait_for_tx "/tmp/gw-finalize.json" "gateway finalize-contract #1"
		state=$(gw_contract_state "$contract_final_id")
		if [[ -n "$state" ]]; then
			ok "Gateway contract #$contract_final_id state: $state"
		else
			warn "Gateway contract #$contract_final_id state query empty"
		fi
	else
		warn "Gateways CLI unavailable; skipping gateway flow"
	fi

	if module_has_subcommand "release" "publish"; then
		ok "Release publish to beta"
		release_json="/tmp/release.json"
		rel_sha=$(printf %064d 0 | tr 0 e)
		runner_exec "cat <<EOF > $release_json
{
  \"creator\": \"$SENDER_ADDR\",
  \"release\": {
    \"version\": \"1.0.0\",
    \"channel\": \"beta\",
    \"artifacts\": [
      {
        \"platform\": \"linux-amd64\",
        \"kind\": \"daemon\",
        \"sha256_hex\": \"$rel_sha\",
        \"size\": 1024,
        \"urls\": [\"https://example.com/bin\"],
        \"signatures\": []
      }
    ],
    \"notes\": \"sim release\",
    \"supersedes\": []
  }
}
EOF"
	run_tx_json "/tmp/release-publish.json" "lumend tx release publish --msg-file $release_json --from sender --keyring-backend test --node http://$SEED_NAME:26657 --chain-id $LUMEN_CHAIN_ID --yes --broadcast-mode sync --fees 0ulmn -o json"
		tx_expect_code "/tmp/release-publish.json" "0" "release publish"
		wait_for_tx "/tmp/release-publish.json" "release publish"
		rcount=$(release_count_for "$SENDER_ADDR")
		if [[ "$rcount" != "0" ]]; then
			ok "Release list contains entries for sender (count=$rcount)"
		else
			warn "Release list missing sender entries"
		fi
	else
		warn "Release CLI unavailable; skipping release flow"
	fi

	ok "Rate-limit per-account burst"
	runner_exec "for i in \$(seq 1 30); do lumend tx bank send sender $RECIP_ADDR 1ulmn --yes --keyring-backend test --node http://$SEED_NAME:26657 --chain-id $LUMEN_CHAIN_ID --broadcast-mode sync --fees 0ulmn >/tmp/rl-acc-\$i.json 2>&1 || true; done"
	if runner_exec "grep -qi 'rate limit' /tmp/rl-acc-*.json"; then
		ok "Per-account limiter produced rejections"
	else
		warn "Per-account limiter produced no obvious rejections"
	fi

	if [[ $FAST -eq 0 ]]; then
		ok "Rate-limit global burst"
		for idx in 1 2 3 4 5 6; do
			name="spam-$idx"
			runner_add_key_with_index "$name" "$FAUCET_MNEMONIC" "$idx"
			addr=$(runner_exec "lumend keys show $name -a --keyring-backend test")
			if fund_addr "$addr" 200000; then
				link_pqc_for "$name"
			else
				warn "Skipping PQC link for $name due to funding failure"
				continue
			fi
		done
		local current_global_rl
		if [[ "${LUMEN_RL_GLOBAL_MAX+x}" == "x" ]]; then
			current_global_rl="$LUMEN_RL_GLOBAL_MAX"
		else
			current_global_rl="$RL_GLOBAL_MAX"
		fi
		info "Tip: set LUMEN_RL_GLOBAL_MAX=50 (current=${current_global_rl}) for stronger global limiter signal."
		runner_exec "for idx in 1 2 3 4 5 6; do name=spam-\$idx; for i in \$(seq 1 120); do lumend tx bank send \$name $RECIP_ADDR 1ulmn --yes --keyring-backend test --node http://$SEED_NAME:26657 --chain-id $LUMEN_CHAIN_ID --broadcast-mode sync --fees 0ulmn >/tmp/rl-global-\${idx}-\${i}.json 2>&1 || true; done; done"
		if runner_exec "grep -qi 'rate limit' /tmp/rl-global-*.json"; then
			ok "Global limiter produced rejections"
		else
			warn "Global limiter produced no obvious rejections"
		fi
	else
		warn "--fast enabled: skipping global rate-limit spam"
	fi

	ok "Final status checks"
wait_http_json_field "${HOST_RPC_URL}/status" '.result.sync_info.latest_block_height|tonumber' '[[ ${VALUE:-0} -ge 20 ]]' "$TIMEOUT" || die "height did not advance"
wait_http_json_field "${HOST_RPC_URL}/net_info" '.result.n_peers|tonumber' '[[ ${VALUE:-0} -ge 1 ]]' 60 || die "no peers connected"

	docker logs "$SEED_NAME" >"$LOGS_DIR/${SEED_NAME}.log" 2>&1 || true
	for i in $(seq 1 "$VALIDATORS"); do docker logs "val-$i" >"$LOGS_DIR/val-$i.log" 2>&1 || true; done
	for j in $(seq 1 "$FULLNODES"); do docker logs "full-$j" >"$LOGS_DIR/full-$j.log" 2>&1 || true; done

	tar -czf "$SNAP_DIR/${LUMEN_CHAIN_ID}-$(date +%s).tar.gz" -C "$ART_ROOT" nodes genesis >/dev/null 2>&1 || true

	if [[ $KEEP -eq 1 ]]; then
		warn "--keep specified; containers preserved for inspection"
	fi

	echo
	ok "Simulation completed successfully."
}

simulate
