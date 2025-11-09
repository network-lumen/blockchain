#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

: "${SCHEME:=dilithium3}"

pqc_require_bins() {
	for bin in jq curl go; do
		if ! command -v "$bin" >/dev/null 2>&1; then
			echo "error: missing dependency '$bin'" >&2
			exit 1
		fi
	done
}

__pqc_home_dir() {
	if [ -n "${HOME_DIR:-}" ]; then
		echo "$HOME_DIR"
	else
		echo "${HOME}/.lumen"
	fi
}

pqc_generate_pair() {
	local tmpdir out pub sk
	tmpdir="$(mktemp -d -t pqcgen-XXXXXX)"
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
	pushd "$tmpdir" >/dev/null
	GO111MODULE=on go mod tidy >/dev/null 2>&1
	out="$(GO111MODULE=on go run .)"
	popd >/dev/null
	rm -rf "$tmpdir"

	pub="${out%%$'\n'*}"
	sk="${out#*$'\n'}"

	if [ -z "${pub:-}" ] || [ -z "${sk:-}" ] || [ "$pub" = "$sk" ]; then
		echo "pqc_generate_pair: empty key material" >&2
		return 1
	fi

	PQC_PUB_HEX="$pub"
	PQC_SK_HEX="$sk"
	export PQC_PUB_HEX PQC_SK_HEX
}

pqc_import_and_link_local() {
	local keyname="$1"
	local alias="${2:-pqc-$keyname}"
	local keyring="${KEYRING:-test}"
	local home
	home="$(__pqc_home_dir)"

	: "${BIN:?BIN must be set before calling pqc_import_and_link_local}"
	: "${PQC_PUB_HEX:?pqc_import_and_link_local requires PQC_PUB_HEX}"
	: "${PQC_SK_HEX:?pqc_import_and_link_local requires PQC_SK_HEX}"

	"$BIN" keys pqc-import \
		--name "$alias" \
		--scheme "$SCHEME" \
		--pubkey "$PQC_PUB_HEX" \
		--privkey "$PQC_SK_HEX" \
		--keyring-backend "$keyring" \
		--home "$home" >/dev/null

	"$BIN" keys pqc-link \
		--from "$keyname" \
		--pqc "$alias" \
		--keyring-backend "$keyring" \
		--home "$home" >/dev/null
}

pqc_wait_tx() {
	local hash="$1"
	local rpc_base="${2:-http://127.0.0.1:26657}"
	for _ in $(seq 1 120); do
		local resp code
		resp=$(curl -s "$rpc_base/tx?hash=0x$hash" 2>/dev/null || true)
		code=$(echo "$resp" | jq -r '.result.tx_result.code' 2>/dev/null || echo "")
		if [ -n "$code" ] && [ "$code" != "null" ]; then
			echo "$code"
			return 0
		fi
		sleep 0.5
	done
	echo "error: timeout waiting for tx $hash" >&2
	return 1
}

pqc_link_onchain() {
	local from="$1"
	local home keyring fees cli_node rpc_base chain_id
	home="$(__pqc_home_dir)"
	keyring="${KEYRING:-test}"
	fees="${TX_FEES:-0ulmn}"
	cli_node="${NODE:-tcp://127.0.0.1:26657}"
	rpc_base="${RPC:-http://127.0.0.1:26657}"
	chain_id="${CHAIN_ID:-lumen}"

	: "${BIN:?BIN must be set before calling pqc_link_onchain}"
	: "${PQC_PUB_HEX:?pqc_link_onchain requires PQC_PUB_HEX}"

	local res hash code
	res=$("$BIN" tx pqc link-account \
		--from "$from" \
		--scheme "$SCHEME" \
		--pubkey "$PQC_PUB_HEX" \
		--keyring-backend "$keyring" \
		--home "$home" \
		--chain-id "$chain_id" \
		--node "$cli_node" \
		--yes \
		--fees "$fees" \
		--broadcast-mode sync \
		-o json)
	echo "$res" | jq
	hash=$(echo "$res" | jq -r '.txhash')
	code=$(pqc_wait_tx "$hash" "$rpc_base") || exit 1
	if [ "$code" != "0" ]; then
		echo "link-account failed (code=$code)" >&2
		exit 1
	fi
}

setup_pqc_signer() {
	local signer="$1"
	local alias="pqc-${signer}"
	pqc_generate_pair
	pqc_import_and_link_local "$signer" "$alias"
	pqc_link_onchain "$signer"
}

pqc_policy_must_be_required() {
	local rpc="$1"
	local params
	params=$("$BIN" q pqc params --node "${NODE:-tcp://127.0.0.1:26657}" -o json 2>/dev/null || true)
	local policy
	policy=$(echo "$params" | jq -r '.params.policy // .policy // empty')
	if [ "$policy" != "PQC_POLICY_REQUIRED" ]; then
		echo "PQC policy is not REQUIRED (found: $policy)" >&2
		exit 1
	fi
}

pqc_wait_ready() {
	local rpc="$1" api="$2"
	for url in "$rpc/status" "$api/"; do
		local ok=0
		for _ in $(seq 1 120); do
			if curl -sSf "$url" >/dev/null 2>&1; then
				ok=1
				break
			fi
			sleep 0.5
		done
		if [ "$ok" = "0" ]; then
			echo "timeout waiting for $url" >&2
			exit 1
		fi
	done
}
