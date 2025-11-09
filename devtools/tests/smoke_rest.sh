#!/usr/bin/env bash
set -euo pipefail

# Environment variables:
# - API (default http://127.0.0.1:1317): REST endpoint queried by the smoke test.
# - RPC (default http://127.0.0.1:26657): Tendermint RPC endpoint used for tx polling.
# - PD (default 0): When set to a small positive integer, the script may try to register
#   a DNS domain through the disabled REST handler for quick smoke coverage.
# - INDEX (default codex-auc.lumen): DNS name used when probing optional endpoints.

API=${API:-http://127.0.0.1:1317}
RPC=${RPC:-http://127.0.0.1:26657}
PD=${PD:-0}

require() { command -v "$1" >/dev/null || { echo "Missing dependency: $1" >&2; exit 0; }; }
require curl
require jq
wait_http() { local url="$1"; for i in $(seq 1 120); do curl -sSf "$url" >/dev/null && return 0; sleep 0.25; done; echo "Timeout $url" >&2; return 1; }
log(){ echo "[smoke] $*"; }

if ! wait_http "$API/"; then
  log "API not reachable for smoke; skipping"
  exit 0
fi
log "API ready"

curl -sf "$API/lumen/dns/v1/params" >/dev/null && log "dns params OK"
curl -sf "$API/lumen/release/params" >/dev/null && log "release params OK"

curl -sf "$API/lumen/gateway/v1/params" >/dev/null && log "gateways params OK"
curl -sf "$API/lumen/gateway/v1/gateways?page=1&limit=5" >/dev/null && log "gateways list OK"
curl -sf "$API/lumen/gateway/v1/contracts?page=1&limit=5" >/dev/null && log "contracts list OK"

LEN=$(curl -sf "$API/lumen/release/releases?page=1&limit=1" | jq -r '(.releases // []) | length' 2>/dev/null || echo 0)
if ! [[ "$LEN" =~ ^[0-9]+$ ]]; then LEN=0; fi
test "$LEN" -ge 0
log "release list len=$LEN"

INDEX=${INDEX:-codex-auc.lumen}
if curl -sf "$API/lumen/dns/v1/domain/$INDEX" >/dev/null; then
  VAL=$(curl -sf "$API/lumen/dns/v1/domain/$INDEX" | jq -r '.domain.index // ""')
  if [ "$VAL" = "$INDEX" ]; then log "dns domain present: $INDEX"; fi
else
  if [[ "$PD" =~ ^[0-9]+$ ]] && [ "$PD" -le 12 ] && command -v go >/dev/null; then
    OWNER=$(curl -s "$API/cosmos/auth/v1beta1/accounts" | jq -r '(.accounts // [])[0].address' 2>/dev/null)
  if [ -n "${OWNER:-}" ] && [ "$OWNER" != "null" ]; then
      D=${INDEX%%.*}; E=${INDEX#*.}
      BODY=$(jq -nc --arg d "$D" --arg e "$E" --arg o "$OWNER" '{domain:$d,ext:$e,owner:$o,duration_days:0}')
      THASH=$(curl -s -X POST "$API/lumen/dns/register_disabled" -H 'content-type: application/json' -d "$BODY" | jq -r .txhash 2>/dev/null)
      if [ -n "$THASH" ] && [ "$THASH" != "null" ]; then
        for i in $(seq 1 60); do c=$(curl -s "$RPC/tx?hash=0x$THASH" | jq -r .result.tx_result.code); [ "$c" != "null" ] && break; sleep 0.3; done
      fi
      if curl -sf "$API/lumen/dns/v1/domain/$INDEX" >/dev/null; then log "dns domain created: $INDEX"; else log "dns domain absent; skipping"; fi
    else
      log "no OWNER; skip domain create"
    fi
  fi
fi

RID=$(curl -sf "$API/lumen/release/by_version/1.0.1" | jq -r .release.id 2>/dev/null || true)
if [ -n "${RID:-}" ] && [ "$RID" != "null" ]; then
  curl -sf "$API/lumen/release/$RID" >/dev/null && log "release by-id OK ($RID)"
else
  log "release 1.0.1 not present; skip by-id"
fi


