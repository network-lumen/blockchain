#!/usr/bin/env bash
set -euo pipefail

DIR=$(cd "$(dirname "$0")/../.." && pwd)
BIN=${BIN:-"$DIR/build/lumend"}
CHAIN_ID=${CHAIN_ID:-pqc-cli-e2e}
HOME_DIR=$(mktemp -d -t pqc-cli-XXXXXX)
PQC_PASS_FILE="$HOME_DIR/pqc_pass.txt"
GENESIS_OUT="$HOME_DIR/genesis-pqc.json"

trap 'rm -rf "$HOME_DIR"' EXIT

echo "s3cret-passphrase" > "$PQC_PASS_FILE"

require() { command -v "$1" >/dev/null || { echo "error: missing $1" >&2; exit 1; }; }
require jq

# init
"$BIN" init cli-tester --chain-id "$CHAIN_ID" --home "$HOME_DIR" >/dev/null

# create validator key
"$BIN" keys add validator --keyring-backend test --home "$HOME_DIR" --output json >/dev/null
VAL_ADDR=$("$BIN" keys show validator -a --keyring-backend test --home "$HOME_DIR")

# PQC generate + link
"$BIN" keys pqc-generate --home "$HOME_DIR" \
  --name validator-cli \
  --link-from validator \
  --keyring-backend test \
  --pqc-passphrase-file "$PQC_PASS_FILE" >/dev/null

# ensured keystore encrypted
if grep -q '"public_key"' "$HOME_DIR/pqc_keys/keys.json" 2>/dev/null; then
  echo "PQC keystore not encrypted" >&2
  exit 1
fi

# genesis entry + inject
"$BIN" keys pqc-genesis-entry --home "$HOME_DIR" \
  --from validator \
  --pqc validator-cli \
  --keyring-backend test \
  --pqc-passphrase-file "$PQC_PASS_FILE" \
  --output "$GENESIS_OUT" \
  --write-genesis "$HOME_DIR/config/genesis.json" >/dev/null

echo "entry="
cat "$GENESIS_OUT"

jq '.app_state.pqc.accounts | length == 1' "$HOME_DIR/config/genesis.json" | grep -q true

rm -f "$GENESIS_OUT"
echo "PQC CLI e2e OK"
