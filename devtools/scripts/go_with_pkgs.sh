#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <go-subcommand> [flags...]" >&2
  exit 1
fi

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
cmd=$1
shift

mapfile -t pkgs < <("$ROOT_DIR/devtools/scripts/go_packages.sh")
if ((${#pkgs[@]} == 0)); then
  echo "no Go packages detected" >&2
  exit 1
fi

cd "$ROOT_DIR"
exec go "$cmd" "$@" "${pkgs[@]}"
