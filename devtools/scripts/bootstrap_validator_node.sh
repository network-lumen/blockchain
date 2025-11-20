#!/usr/bin/env bash
set -euo pipefail

# Bootstrap a validator home + PQC entry on a fresh server.
# Expectations:
#   - Run as root on the validator host.
#   - The following files already exist in ~ (or override with env vars):
#       * ~/validator.pub         (hex-encoded Dilithium public key)
#       * ~/validator.priv        (hex-encoded Dilithium private key)
#       * ~/pqc_passphrase        (passphrase used to encrypt the PQC keystore)
#   - The script will create ~/mnemo (mnemonic backup) and ~/wallet (addresses).
#
# Environment overrides:
#   MONIKER, CHAIN_ID, KEY_NAME, PQC_KEY_NAME, LUMEN_HOME, MNEMO_FILE, WALLET_FILE,
#   PQC_PUB_FILE, PQC_PRIV_FILE, PQC_PASSPHRASE_FILE, GENESIS_BALANCE, GENTX_AMOUNT,
#   LUMEN_USER, MIN_GAS_PRICE

log() { printf '\e[32m[%s]\e[0m %s\n' "$(date +%H:%M:%S)" "$*"; }
err() { printf '\e[31m[%s] ERROR:\e[0m %s\n' "$(date +%H:%M:%S)" "$*" >&2; exit 1; }

require_cmd() { command -v "$1" >/dev/null 2>&1 || err "Missing command: $1"; }
require_file() { [[ -s "$1" ]] || err "Missing required file: $1"; }
require_user() { id "$1" >/dev/null 2>&1 || err "Missing system user: $1"; }

[[ "${EUID:-$(id -u)}" -eq 0 ]] || err "Run this script as root."
require_cmd lumend
require_cmd python3
require_cmd jq

MONIKER=${MONIKER:-node-1}
CHAIN_ID=${CHAIN_ID:-lumen}
KEY_NAME=${KEY_NAME:-validator}
PQC_KEY_NAME=${PQC_KEY_NAME:-validator-pqc}
DEFAULT_LUMEN_HOME="/var/lib/lumen"
DEFAULT_LUMEN_USER="lumen"

if [[ -z "${LUMEN_HOME:-}" ]]; then
  SERVICE_HOME=$(systemctl show -p ExecStart lumend 2>/dev/null | sed -n 's/.*--home[= ]\([^ ;]*\).*/\1/p' | head -n1 || true)
  if [[ -n "${SERVICE_HOME:-}" ]]; then
    LUMEN_HOME="$SERVICE_HOME"
  else
    LUMEN_HOME="$DEFAULT_LUMEN_HOME"
  fi
fi

MNEMO_FILE=${MNEMO_FILE:-$HOME/mnemo}
WALLET_FILE=${WALLET_FILE:-$HOME/wallet}
PQC_PUB_FILE=${PQC_PUB_FILE:-$HOME/validator.pub}
PQC_PRIV_FILE=${PQC_PRIV_FILE:-$HOME/validator.priv}
PQC_PASSPHRASE_FILE=${PQC_PASSPHRASE_FILE:-$HOME/pqc_passphrase}
GENESIS_BALANCE=${GENESIS_BALANCE:-1000000000000ulmn}
GENTX_AMOUNT=${GENTX_AMOUNT:-1000000000000ulmn}
if [[ -z "${LUMEN_USER:-}" ]]; then
  SERVICE_USER=$(systemctl show -p User lumend 2>/dev/null | awk -F= 'NR==1 {gsub(/^[[:space:]]+|[[:space:]]+$/, "", $2); print $2}' || true)
  if [[ -n "${SERVICE_USER:-}" && "${SERVICE_USER:-}" != "-" ]]; then
    LUMEN_USER="$SERVICE_USER"
  else
    LUMEN_USER="$DEFAULT_LUMEN_USER"
  fi
fi
MIN_GAS_PRICE=${MIN_GAS_PRICE:-0ulmn}

require_file "$PQC_PUB_FILE"
require_file "$PQC_PRIV_FILE"
require_file "$PQC_PASSPHRASE_FILE"
require_user "$LUMEN_USER"

log "Resetting $LUMEN_HOME"
systemctl stop lumend >/dev/null 2>&1 || true
rm -rf "$LUMEN_HOME"
mkdir -p "$LUMEN_HOME"

log "Running lumend init"
lumend init "$MONIKER" --chain-id "$CHAIN_ID" --home "$LUMEN_HOME" >/dev/null

APP_TOML="$LUMEN_HOME/config/app.toml"
if [[ -f "$APP_TOML" ]]; then
  log "Setting minimum gas price to $MIN_GAS_PRICE"
  python3 - "$APP_TOML" "$MIN_GAS_PRICE" <<'PY'
import io
import re
import sys

path = sys.argv[1]
value = sys.argv[2]
with open(path, "r", encoding="utf-8") as f:
    content = f.read()
new_content, count = re.subn(
    r"(?m)^minimum-gas-prices\s*=\s*\".*?\"",
    f'minimum-gas-prices = "{value}"',
    content,
    count=1,
)
if count != 1:
    raise SystemExit(f"failed to update minimum gas price in {path}")
with open(path, "w", encoding="utf-8") as f:
    f.write(new_content)
PY
fi

log "Generating validator key ($KEY_NAME)"
KEY_JSON=$(lumend keys add "$KEY_NAME" --home "$LUMEN_HOME" --keyring-backend test --output json)
MNEMONIC=$(printf '%s' "$KEY_JSON" | jq -r '.mnemonic // empty')
[[ -n "$MNEMONIC" ]] || err "could not extract mnemonic from: $KEY_JSON"
ADDRESS=$(lumend keys show "$KEY_NAME" -a --home "$LUMEN_HOME" --keyring-backend test)
VALOPER=$(lumend keys show "$KEY_NAME" -a --bech val --home "$LUMEN_HOME" --keyring-backend test)

umask 077
printf '%s\n' "$MNEMONIC" > "$MNEMO_FILE"
printf 'address=%s\nvaloper=%s\n' "$ADDRESS" "$VALOPER" > "$WALLET_FILE"
log "Mnemonic written to $MNEMO_FILE"
log "Wallet info written to $WALLET_FILE"

log "Adding genesis account with $GENESIS_BALANCE"
lumend genesis add-genesis-account "$ADDRESS" "$GENESIS_BALANCE" --home "$LUMEN_HOME"

log "Creating validator gentx ($GENTX_AMOUNT)"
lumend genesis gentx "$KEY_NAME" "$GENTX_AMOUNT" \
  --chain-id "$CHAIN_ID" \
  --from "$KEY_NAME" \
  --home "$LUMEN_HOME" \
  --keyring-backend test >/dev/null

GENTX_FILE=""
for _ in $(seq 1 20); do
  GENTX_FILE=$(find "$LUMEN_HOME/config/gentx" -maxdepth 1 -name 'gentx-*.json' -print -quit)
  [[ -n "$GENTX_FILE" && -f "$GENTX_FILE" ]] && break
  sleep 0.2
done
[[ -n "$GENTX_FILE" && -f "$GENTX_FILE" ]] || err "gentx file not found under $LUMEN_HOME/config/gentx"

CURRENT_DELEGATOR=""
jq_ok=1
for _ in $(seq 1 10); do
  if CURRENT_DELEGATOR=$(jq -r '.body.messages[0].delegator_address // empty' "$GENTX_FILE" 2>/dev/null); then
    jq_ok=0
    break
  fi
  sleep 0.2
done
if [[ $jq_ok -ne 0 ]]; then
  err "failed to read delegator address from $GENTX_FILE"
fi
if [[ "$CURRENT_DELEGATOR" != "$ADDRESS" ]]; then
  log "Patching delegator address in gentx (expected $ADDRESS, found $CURRENT_DELEGATOR)"
  tmp=$(mktemp)
  jq --arg addr "$ADDRESS" '.body.messages[0].delegator_address=$addr' "$GENTX_FILE" >"$tmp"
  mv "$tmp" "$GENTX_FILE"

  log "Re-signing gentx for updated delegator"
  lumend tx sign "$GENTX_FILE" \
    --home "$LUMEN_HOME" \
    --keyring-backend test \
    --from "$KEY_NAME" \
    --chain-id "$CHAIN_ID" \
    --offline \
    --account-number 0 \
    --sequence 0 \
    --sign-mode direct \
    --overwrite \
    --pqc-enable=false \
    --output-document "$GENTX_FILE" >/dev/null
fi

log "Collecting gentxs"
lumend genesis collect-gentxs --home "$LUMEN_HOME" >/dev/null

log "Importing PQC key ($PQC_KEY_NAME)"
lumend keys pqc-import \
  --home "$LUMEN_HOME" \
  --keyring-backend test \
  --name "$PQC_KEY_NAME" \
  --scheme dilithium3 \
  --pubkey "$(cat "$PQC_PUB_FILE")" \
  --privkey "$(cat "$PQC_PRIV_FILE")" \
  --pqc-passphrase-file "$PQC_PASSPHRASE_FILE" >/dev/null

log "Linking PQC key to $ADDRESS"
lumend keys pqc-link \
  --home "$LUMEN_HOME" \
  --keyring-backend test \
  --from "$KEY_NAME" \
  --pqc "$PQC_KEY_NAME" \
  --pqc-passphrase-file "$PQC_PASSPHRASE_FILE" >/dev/null

log "Injecting PQC genesis entry"
lumend keys pqc-genesis-entry \
  --home "$LUMEN_HOME" \
  --keyring-backend test \
  --from "$KEY_NAME" \
  --pqc "$PQC_KEY_NAME" \
  --write-genesis "$LUMEN_HOME/config/genesis.json" \
  --force \
  --pqc-passphrase-file "$PQC_PASSPHRASE_FILE" >/dev/null

log "Validating genesis"
lumend genesis validate-genesis --home "$LUMEN_HOME" >/dev/null

log "Fixing ownership ($LUMEN_USER)"
chown -R "$LUMEN_USER:$LUMEN_USER" "$LUMEN_HOME"

log "Restarting lumend service"
systemctl restart lumend

log "Bootstrap complete. Tail logs with: journalctl -fu lumend"
