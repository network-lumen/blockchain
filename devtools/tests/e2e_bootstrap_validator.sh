#!/usr/bin/env bash
set -euo pipefail

# Full validator bootstrap (devtools/scripts/bootstrap_validator.sh) + PQC sanity checks.
# Verifies: keystore encryption, genesis injection, and basic layout.

DIR=$(cd "$(dirname "$0")/../.." && pwd)
BOOTSTRAP_SCRIPT="$DIR/devtools/scripts/bootstrap_validator.sh"
BIN=${BIN:-"$DIR/build/lumend"}
CHAIN_ID=${CHAIN_ID:-e2e-bootstrap-1}
MONIKER=${MONIKER:-e2e-bootstrap}
PQC_NAME=${PQC_NAME:-val-pqc-e2e}

require() { command -v "$1" >/dev/null || { echo "error: missing dependency '$1'" >&2; exit 1; }; }
require jq
require head
require grep

TMPDIR=$(mktemp -d -t e2e-bootstrap-XXXXXX)
trap 'rm -rf "$TMPDIR"' EXIT
HOME_DIR="$TMPDIR/home"
PQC_PASS_FILE="$TMPDIR/pqc_passphrase"
echo "test-pqc-passphrase" > "$PQC_PASS_FILE"

"$BOOTSTRAP_SCRIPT" \
  --binary "$BIN" \
  --home "$HOME_DIR" \
  --moniker "$MONIKER" \
  --chain-id "$CHAIN_ID" \
  --balance "1000000000ulmn" \
  --stake "100000000ulmn" \
  --pqc-name "$PQC_NAME" \
  --pqc-passphrase-file "$PQC_PASS_FILE"

CONFIG_DIR="$HOME_DIR/config"
[[ -f "$CONFIG_DIR/config.toml" ]] || { echo "config.toml missing" >&2; exit 1; }
[[ -f "$CONFIG_DIR/genesis.json" ]] || { echo "genesis.json missing" >&2; exit 1; }
[[ -f "$CONFIG_DIR/priv_validator_key.json" ]] || { echo "priv_validator_key.json missing" >&2; exit 1; }

# Genesis validation
"$BIN" genesis validate-genesis --home "$HOME_DIR" >/dev/null

# PQC keystore encryption check
KEYSTORE_DIR="$HOME_DIR/pqc_keys"
[[ -d "$KEYSTORE_DIR" ]] || { echo "pqc_keys directory missing" >&2; exit 1; }
KEYSTORE_FILE="$KEYSTORE_DIR/keys.json"
[[ -f "$KEYSTORE_FILE" ]] || { echo "pqc keys.json missing" >&2; exit 1; }
MAGIC=$(head -c 7 "$KEYSTORE_FILE" | tr -d '\n')
if [[ "$MAGIC" != "PQCENC1" ]]; then
  echo "pqc keys not encrypted (magic $MAGIC)" >&2
  exit 1
fi

# Genesis contains PQC entry for validator account
VAL_ADDR=$("$BIN" keys show validator -a --keyring-backend test --home "$HOME_DIR")
MATCHES=$(jq --arg addr "$VAL_ADDR" '.app_state.pqc.accounts | map(select(.addr == $addr)) | length' "$CONFIG_DIR/genesis.json")
if [[ "$MATCHES" -lt 1 ]]; then
  echo "PQC account for $VAL_ADDR not found in genesis" >&2
  exit 1
fi

echo "Bootstrap validator e2e OK"
