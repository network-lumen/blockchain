#!/usr/bin/env bash

set -euo pipefail

# Environment variables:
# - NETWORK_DIR: Optional absolute or repo-relative path where the resulting binaries are copied after the build.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SRC_DIR="$ROOT_DIR"
NETWORK_DIR="${NETWORK_DIR:-}"
if [[ -n "$NETWORK_DIR" && "$NETWORK_DIR" != /* ]]; then
  NETWORK_DIR="$ROOT_DIR/$NETWORK_DIR"
fi

printf '==> Native build for Lumen\n'
printf 'Root:     %s\n' "$ROOT_DIR"
if [[ -n "$NETWORK_DIR" ]]; then
  printf 'Network:  %s\n' "$NETWORK_DIR"
else
  printf 'Network:  (skip copy)\n'
fi

mkdir -p "$SRC_DIR/build"
if [[ -n "$NETWORK_DIR" ]]; then
  mkdir -p "$NETWORK_DIR"
fi
cd "$SRC_DIR"

command -v go >/dev/null 2>&1 || { echo "Error: go binary not found."; exit 1; }
FALLBACK=0
if command -v make >/dev/null 2>&1; then
  echo "-> make build"
  if ! make build; then
    echo "Info: make build failed, switching to go build fallback."
    FALLBACK=1
  fi
else
  echo "Info: make not found, using go build fallback."
  FALLBACK=1
fi

if [[ "$FALLBACK" -eq 1 ]]; then
  echo "-> fallback go build (linux/amd64)"
  if [[ -d "$SRC_DIR/cmd/lumend" ]]; then
    go build -o build/lumend ./cmd/lumend
  else
    echo "Error: directory cmd/lumend not found. Update build_native.sh accordingly."
    exit 1
  fi
fi

if [[ ! -f "$SRC_DIR/build/lumend" ]]; then
  echo "Error: build/lumend not found. Check the Makefile or fallback path."
  exit 1
fi

if [[ -d "$SRC_DIR/cmd/lumend" ]]; then
  echo "-> go build windows/amd64"
  if ! CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o build/lumend.exe ./cmd/lumend; then
    echo "Warning: failed to build lumend.exe. Continuing with Linux binary only." >&2
  fi
fi

if [[ -n "$NETWORK_DIR" ]]; then
  install -Dm755 "$SRC_DIR/build/lumend" "$NETWORK_DIR/lumend"
  if [[ -f "$SRC_DIR/build/lumend.exe" ]]; then
    install -Dm755 "$SRC_DIR/build/lumend.exe" "$NETWORK_DIR/lumend.exe"
  fi
  echo "OK: binaries copied under $NETWORK_DIR"
else
  echo "OK: binaries available under $SRC_DIR/build"
fi

set +e
"$SRC_DIR/build/lumend" version 2>/dev/null || true
if [[ -f "$SRC_DIR/build/lumend.exe" ]]; then
  "$SRC_DIR/build/lumend.exe" version 2>/dev/null || true
fi
if [[ -n "$NETWORK_DIR" ]]; then
  "$NETWORK_DIR/lumend" version 2>/dev/null || true
  if [[ -f "$NETWORK_DIR/lumend.exe" ]]; then
    "$NETWORK_DIR/lumend.exe" version 2>/dev/null || true
  fi
fi
