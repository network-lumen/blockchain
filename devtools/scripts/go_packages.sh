#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
cd "$ROOT_DIR"

if ! command -v git >/dev/null 2>&1; then
  exit 1
fi

declare -A dirs=()

while IFS= read -r file; do
  dir=${file%/*}
  if [[ "$dir" == "$file" ]]; then
    dir="."
  fi
  dirs["$dir"]=1
done < <(git ls-files -- '*.go')

if ((${#dirs[@]} == 0)); then
  exit 0
fi

while IFS= read -r d; do
  pkg="."
  if [[ "$d" != "." ]]; then
    pkg="./$d"
  fi
  if go list "$pkg" >/dev/null 2>&1; then
    printf '%s\n' "$pkg"
  fi
done < <(printf '%s\n' "${!dirs[@]}" | LC_ALL=C sort)
