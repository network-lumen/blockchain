#!/usr/bin/env bash
set -euo pipefail

# Environment variables:
# - LUMEN_USER (default lumen): system user that owns the service data directory.
# - LUMEN_HOME (default /var/lib/lumen): home/data directory passed to `lumend start`.
# - BIN_PATH (default /usr/local/bin/lumend): path to the binary used in ExecStart.
# - UNIT_PATH (default /etc/systemd/system/lumend.service): destination for the generated unit file.
# - RPC_HOST/PORT, API_HOST/PORT, GRPC_HOST/PORT: bind addresses for the node services.
# - GRPC_WEB_ENABLE (default 1): set to 0 to omit `--grpc-web.enable`.
# - MINIMUM_GAS_PRICES (default 0ulmn): propagated to the ExecStart command.
# - LUMEN_RL_PER_BLOCK, LUMEN_RL_PER_WINDOW, LUMEN_RL_WINDOW_SEC: exported ante rate-limit knobs.

usage() {
  cat <<'EOF'
Usage: install_service.sh [--print-unit]

Requires root unless --print-unit is used. The following environment variables customise the unit:
  LUMEN_USER               System user that runs the service (default: lumen)
  LUMEN_HOME               Home/data directory for the service (default: /var/lib/lumen)
  BIN_PATH                 Path to the lumend binary (default: /usr/local/bin/lumend)
  UNIT_PATH                Destination for the systemd unit (default: /etc/systemd/system/lumend.service)
  RPC_HOST / RPC_PORT      Tendermint RPC bind host/port (defaults: 127.0.0.1 / 26657)
  API_HOST / API_PORT      REST API bind host/port (defaults: 127.0.0.1 / 1317)
  GRPC_HOST / GRPC_PORT    gRPC bind host/port (defaults: 127.0.0.1 / 9090)
  GRPC_WEB_ENABLE          Set to 0 to disable --grpc-web.enable (default: 1)
  MINIMUM_GAS_PRICES       Minimum gas prices passed to lumend (default: 0ulmn)

Rate limiter environment exported to the service:
  LUMEN_RL_PER_BLOCK       Tokens refilled each block            (default: 5)
  LUMEN_RL_PER_WINDOW      Tokens refilled each sliding window   (default: 20)
  LUMEN_RL_WINDOW_SEC      Sliding window duration in seconds    (default: 10)
EOF
}

PRINT_ONLY=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --print-unit) PRINT_ONLY=1 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage >&2; exit 1 ;;
  esac
  shift
done

if [[ "$PRINT_ONLY" -ne 1 && "${EUID:-$(id -u)}" -ne 0 ]]; then
  echo "Please run as root (sudo) or use --print-unit for a dry run." >&2
  exit 1
fi

LUMEN_USER=${LUMEN_USER:-lumen}
LUMEN_HOME=${LUMEN_HOME:-/var/lib/lumen}
BIN_PATH=${BIN_PATH:-/usr/local/bin/lumend}
UNIT_PATH=${UNIT_PATH:-/etc/systemd/system/lumend.service}

RPC_HOST="${RPC_HOST:-127.0.0.1}"
RPC_PORT="${RPC_PORT:-26657}"
RPC_LADDR="${RPC_LADDR:-tcp://${RPC_HOST}:${RPC_PORT}}"
API_HOST="${API_HOST:-127.0.0.1}"
API_PORT="${API_PORT:-1317}"
API_ADDR="${API_ADDR:-tcp://${API_HOST}:${API_PORT}}"
GRPC_HOST="${GRPC_HOST:-127.0.0.1}"
GRPC_PORT="${GRPC_PORT:-9090}"
GRPC_ADDR="${GRPC_ADDR:-${GRPC_HOST}:${GRPC_PORT}}"
GRPC_WEB_ENABLE="${GRPC_WEB_ENABLE:-1}"
MINIMUM_GAS_PRICES=${MINIMUM_GAS_PRICES:-0ulmn}
if [[ -n "$MINIMUM_GAS_PRICES" && "$MINIMUM_GAS_PRICES" != "0ulmn" ]]; then
  echo "gasless chain: MINIMUM_GAS_PRICES must be 0ulmn or unset" >&2
  exit 1
fi

RL_PER_BLOCK=${LUMEN_RL_PER_BLOCK:-5}
RL_PER_WINDOW=${LUMEN_RL_PER_WINDOW:-20}
RL_WINDOW_SEC=${LUMEN_RL_WINDOW_SEC:-10}

EXEC_CMD="$BIN_PATH start --home $LUMEN_HOME --rpc.laddr $RPC_LADDR --api.enable --api.address $API_ADDR --grpc.address $GRPC_ADDR"
if [[ "$GRPC_WEB_ENABLE" = "1" ]]; then
  EXEC_CMD="$EXEC_CMD --grpc-web.enable"
fi
EXEC_CMD="$EXEC_CMD --minimum-gas-prices $MINIMUM_GAS_PRICES"

UNIT_CONTENT=$(cat <<EOF
[Unit]
Description=Lumen Node
After=network-online.target

[Service]
User=$LUMEN_USER
ExecStart=$EXEC_CMD
Restart=on-failure
RestartSec=5
LimitNOFILE=65536
Environment=LUMEN_RL_PER_BLOCK=$RL_PER_BLOCK LUMEN_RL_PER_WINDOW=$RL_PER_WINDOW LUMEN_RL_WINDOW_SEC=$RL_WINDOW_SEC

[Install]
WantedBy=multi-user.target
EOF
)

if [[ "$PRINT_ONLY" -eq 1 ]]; then
  echo "# lumend.service (dry run)"
  printf '%s\n' "$UNIT_CONTENT"
  exit 0
fi

if ! id -u "$LUMEN_USER" >/dev/null 2>&1; then
  useradd -r -m -d "$LUMEN_HOME" -s /sbin/nologin "$LUMEN_USER"
fi
install -d -o "$LUMEN_USER" -g "$LUMEN_USER" -m 0750 "$LUMEN_HOME"

tmp=$(mktemp)
printf '%s\n' "$UNIT_CONTENT" > "$tmp"
install -m 0644 "$tmp" "$UNIT_PATH"
rm -f "$tmp"

systemctl daemon-reload
echo "Service installed at $UNIT_PATH."
echo "Rate limiter env: LUMEN_RL_PER_BLOCK=$RL_PER_BLOCK tokens/block, LUMEN_RL_PER_WINDOW=$RL_PER_WINDOW tokens/window, LUMEN_RL_WINDOW_SEC=$RL_WINDOW_SEC s."
echo "Enable with: systemctl enable --now lumend"
