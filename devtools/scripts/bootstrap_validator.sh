#!/usr/bin/env bash
set -euo pipefail

# bootstrap_validator.sh provisions a validator home with PQC enabled,
# generates the necessary keys, produces a gentx, and optionally installs
# the systemd service.

usage() {
  cat <<'EOF'
Usage: bootstrap_validator.sh [options]

Required flags:
  --moniker <name>               Validator moniker
  --chain-id <id>                Target chain-id

Optional flags:
  --home <dir>                   Lumen home directory (default: $HOME/.lumen)
  --binary <path>                Path to the lumend binary (default: lumend in PATH)
  --keyring-backend <backend>    Keyring backend (default: test)
  --stake <amount>               Self-delegation amount for gentx (default: 1ulmn)
  --balance <amount>             Genesis balance for the validator account (default: 1000ulmn)
  --mnemonic-file <path>         File containing the validator mnemonic (if omitted, a new mnemonic is generated)
  --pqc-passphrase-file <path>   File containing the PQC keystore passphrase
  --pqc-name <name>              Local name for the PQC key (default: validator-pqc)
  --install-service              Install the systemd service after bootstrapping
  --force                        Remove existing home directory before bootstrapping

Environment:
  LUMEN_INSTALL_SERVICE_ARGS     Extra arguments passed to install_service.sh when --install-service is used.
EOF
}

MONIKER=""
CHAIN_ID=""
HOME_DIR="$HOME/.lumen"
BINARY="lumend"
KEYRING="test"
STAKE="1ulmn"
BALANCE="1000ulmn"
MNEMONIC_FILE=""
PQC_PASSPHRASE_FILE=""
PQC_NAME="validator-pqc"
INSTALL_SERVICE=0
FORCE=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --moniker) MONIKER="$2"; shift ;;
    --chain-id) CHAIN_ID="$2"; shift ;;
    --home) HOME_DIR="$2"; shift ;;
    --binary) BINARY="$2"; shift ;;
    --keyring-backend) KEYRING="$2"; shift ;;
    --stake) STAKE="$2"; shift ;;
    --balance) BALANCE="$2"; shift ;;
    --mnemonic-file) MNEMONIC_FILE="$2"; shift ;;
    --pqc-passphrase-file) PQC_PASSPHRASE_FILE="$2"; shift ;;
    --pqc-name) PQC_NAME="$2"; shift ;;
    --install-service) INSTALL_SERVICE=1 ;;
    --force) FORCE=1 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage >&2; exit 1 ;;
  esac
  shift
done

if [[ -z "$MONIKER" || -z "$CHAIN_ID" ]]; then
  echo "Error: --moniker and --chain-id are required." >&2
  usage
  exit 1
fi

command -v "$BINARY" >/dev/null 2>&1 || { echo "Binary $BINARY not found in PATH"; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "jq is required"; exit 1; }

if [[ -d "$HOME_DIR" ]]; then
  if [[ "$FORCE" -eq 1 ]]; then
    rm -rf "$HOME_DIR"
  else
    echo "Home directory $HOME_DIR already exists. Use --force to overwrite." >&2
    exit 1
  fi
fi

echo "[1/8] Initializing home at $HOME_DIR"
"$BINARY" init "$MONIKER" --chain-id "$CHAIN_ID" --home "$HOME_DIR" >/dev/null

echo "[2/8] Preparing validator key"
if [[ -n "$MNEMONIC_FILE" ]]; then
  "$BINARY" keys add validator --recover --source "$MNEMONIC_FILE" --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null
else
  KEY_JSON=$("$BINARY" keys add validator --keyring-backend "$KEYRING" --home "$HOME_DIR" --output json)
  MNEMONIC=$(printf '%s' "$KEY_JSON" | jq -r '.mnemonic')
  printf 'Generated mnemonic (store securely): %s\n' "$MNEMONIC"
fi
VAL_ADDR=$("$BINARY" keys show validator -a --keyring-backend "$KEYRING" --home "$HOME_DIR")

echo "[3/8] Funding validator account with $BALANCE"
"$BINARY" genesis add-genesis-account "$VAL_ADDR" "$BALANCE" --keyring-backend "$KEYRING" --home "$HOME_DIR"

PQC_ARGS=()
if [[ -n "$PQC_PASSPHRASE_FILE" ]]; then
  PQC_ARGS+=(--pqc-passphrase-file "$PQC_PASSPHRASE_FILE")
fi

echo "[4/8] Generating PQC key"
"$BINARY" keys pqc-generate --name "$PQC_NAME" --link-from validator --home "$HOME_DIR" --keyring-backend "$KEYRING" "${PQC_ARGS[@]}" >/dev/null

echo "[5/8] Injecting PQC entry into genesis"
"$BINARY" keys pqc-genesis-entry --from validator --pqc "$PQC_NAME" --home "$HOME_DIR" --keyring-backend "$KEYRING" --write-genesis "$HOME_DIR/config/genesis.json" "${PQC_ARGS[@]}" >/dev/null

echo "[6/8] Creating gentx with stake $STAKE"
"$BINARY" genesis gentx validator "$STAKE" --chain-id "$CHAIN_ID" --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null

echo "[7/8] Collecting gentxs"
"$BINARY" genesis collect-gentxs --home "$HOME_DIR" >/dev/null

CONFIG_DIR="$HOME_DIR/config"
APP_TOML="$CONFIG_DIR/app.toml"
CONFIG_TOML="$CONFIG_DIR/config.toml"

if [[ -f "$APP_TOML" ]]; then
  # Set pruning defaults (custom: keep 100 recent, prune frequently).
  sed -i.bak 's/^pruning *=.*/pruning = "custom"/' "$APP_TOML" || true
  sed -i.bak 's/^pruning-keep-recent *=.*/pruning-keep-recent = "100"/' "$APP_TOML" || true
  sed -i.bak 's/^pruning-keep-every *=.*/pruning-keep-every = "0"/' "$APP_TOML" || true
  sed -i.bak 's/^pruning-interval *=.*/pruning-interval = "1000"/' "$APP_TOML" || true
fi

if [[ -f "$CONFIG_TOML" ]]; then
  # Ensure goleveldb backend by default.
  sed -i.bak 's/^db_backend *=.*/db_backend = "goleveldb"/' "$CONFIG_TOML" || true

  # Add commented seed / peers recommendations without forcing values.
  if ! grep -q "Lumen testnet recommended seeds" "$CONFIG_TOML" 2>/dev/null; then
    cat <<'EOF' >>"$CONFIG_TOML"

# Lumen testnet recommended seeds / peers (example only, override as needed):
# seeds = "nodeid1@host1:26656,nodeid2@host2:26656"
# persistent_peers = "nodeid1@host1:26656,nodeid2@host2:26656"
EOF
  fi
fi

if [[ "$INSTALL_SERVICE" -eq 1 ]]; then
  echo "[8/8] Installing systemd service"
  LUMEN_HOME="$HOME_DIR" BIN_PATH="$BINARY" bash "$(dirname "$0")/install_service.sh" ${LUMEN_INSTALL_SERVICE_ARGS:-}
else
  echo "[8/8] Skipping systemd installation (use --install-service to enable)"
fi

cat <<EOF
Bootstrap completed.
  Home directory : $HOME_DIR
  Validator addr : $VAL_ADDR
  PQC key name   : $PQC_NAME

Use 'lumend start --home $HOME_DIR' or enable the systemd service to start the node.
EOF
