#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
ART_DIR="$ROOT_DIR/artifacts/test-logs"
REPORT_JSON="$ROOT_DIR/artifacts/test-report.json"
mkdir -p "$ART_DIR"

: "${DISABLE_PPROF:=1}"
: "${RPC_HOST:=127.0.0.1}"
: "${API_HOST:=127.0.0.1}"
: "${GRPC_HOST:=127.0.0.1}"

if [ -z "${E2E_BASE_PORT:-}" ]; then
	E2E_BASE_PORT=$(( (RANDOM % 1000) + 30000 ))
fi
: "${RPC_PORT:=$E2E_BASE_PORT}"
: "${API_PORT:=$((E2E_BASE_PORT + 60))}"
: "${GRPC_PORT:=$((E2E_BASE_PORT + 120))}"
: "${P2P_HOST:=0.0.0.0}"
: "${P2P_PORT:=$((E2E_BASE_PORT + 180))}"
export DISABLE_PPROF RPC_HOST API_HOST GRPC_HOST P2P_HOST
export RPC_PORT API_PORT GRPC_PORT P2P_PORT

: "${LUMEN_BUILD_TAGS:=dev}"
echo "==> Building lumend once"
build_cmd=(go build -trimpath -ldflags "-s -w")
if [ -n "$LUMEN_BUILD_TAGS" ]; then
  build_cmd+=(-tags "$LUMEN_BUILD_TAGS")
fi
build_cmd+=(-o build/lumend ./cmd/lumend)
(cd "$ROOT_DIR" && "${build_cmd[@]}")
export SKIP_BUILD=1

declare -a NAMES
declare -a CMDS

add_job() { NAMES+=("$1"); CMDS+=("$2"); }

add_job "unit (go test)" "bash '$ROOT_DIR/devtools/scripts/go_test.sh' -tags '!legacy'"
add_job "e2e_send_tax" "bash '$ROOT_DIR/devtools/tests/e2e_send_tax.sh' --skip-build"
add_job "e2e_dns_auction" "bash '$ROOT_DIR/devtools/tests/e2e_dns_auction.sh' --skip-build --mode prod"
add_job "e2e_release" "bash '$ROOT_DIR/devtools/tests/e2e_release.sh' --skip-build"
add_job "e2e_gov" "bash '$ROOT_DIR/devtools/tests/e2e_gov.sh' --skip-build"
add_job "e2e_upgrade" "bash '$ROOT_DIR/devtools/tests/e2e_upgrade.sh' --skip-build"
add_job "e2e_slashing" "bash '$ROOT_DIR/devtools/tests/e2e_slashing.sh' --skip-build"
add_job "e2e_gateways" "bash '$ROOT_DIR/devtools/tests/e2e_gateways.sh' --skip-build"
add_job "e2e_tokenomics" "bash '$ROOT_DIR/devtools/tests/e2e_tokenomics.sh' --skip-build"
add_job "e2e_pqc" "BIN='$ROOT_DIR/build/lumend' bash '$ROOT_DIR/devtools/tests/e2e_pqc.sh'"
add_job "e2e_pqc_cli" "BIN='$ROOT_DIR/build/lumend' bash '$ROOT_DIR/devtools/tests/e2e_pqc_cli.sh'"
add_job "e2e_bootstrap_validator" "BIN='$ROOT_DIR/build/lumend' bash '$ROOT_DIR/devtools/tests/e2e_bootstrap_validator.sh'"
add_job "e2e_pqc_tx_paths" "BIN='$ROOT_DIR/build/lumend' bash '$ROOT_DIR/devtools/tests/e2e_pqc_tx_paths.sh' --skip-build"

add_job "smoke REST" "bash '$ROOT_DIR/devtools/tests/smoke_rest.sh'"

RESULTS=()
PASSES=0
FAILS=0

for i in "${!NAMES[@]}"; do
  name="${NAMES[$i]}"; cmd="${CMDS[$i]}"; log="$ART_DIR/$(echo "$name" | tr ' /()' '____').log"
  echo "\n>>> Running: $name"
  set +e
  bash -lc "$cmd" >"$log" 2>&1
  code=$?
  set -e
  if [ "$code" -eq 0 ]; then
    echo "[PASS] $name"
    RESULTS+=("$name|PASS")
    PASSES=$((PASSES+1))
  else
    echo "[FAIL] $name (code=$code)"
    RESULTS+=("$name|FAIL")
    FAILS=$((FAILS+1))
  fi
done

TOTAL=${#NAMES[@]}
echo "\n======================="
echo "Test Summary"
for r in "${RESULTS[@]}"; do
  n=${r%|*}; s=${r#*|}
  echo "- $n : $s"
 done
echo "-----------------------"
echo "Total: $TOTAL  Passed: $PASSES  Failed: $FAILS"
echo "======================="

{
  printf '{"total":%d,"passed":%d,"failed":%d,"tests":[' "$TOTAL" "$PASSES" "$FAILS"
  comma=""
  for r in "${RESULTS[@]}"; do
    n=${r%|*}; s=${r#*|}
    printf "%s{\"name\":%s,\"status\":%s}" "$comma" "$(jq -Rn --arg x "$n" '$x')" "$(jq -Rn --arg x "$s" '$x')"
    comma=",";
  done
  printf ']}'
} > "$REPORT_JSON"

exit $([ "$FAILS" -eq 0 ] && echo 0 || echo 1)
