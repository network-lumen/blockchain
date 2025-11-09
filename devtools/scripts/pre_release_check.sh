#!/usr/bin/env bash
set -euo pipefail

RED=$'\033[31m'
GREEN=$'\033[32m'
YELLOW=$'\033[33m'
BOLD=$'\033[1m'
NC=$'\033[0m'

ok()   { echo "${GREEN}✔${NC} $*"; }
warn() { echo "${YELLOW}⚠${NC} $*"; }
die()  { echo "${RED}✘${NC} $*"; exit 1; }
run()  { echo "${BOLD}→${NC} $*"; eval "$@"; }

FAST=0
NO_VULN=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --fast) FAST=1 ;;
    --no-vuln) NO_VULN=1 ;;
    *) die "unknown flag: $1" ;;
  esac
  shift
done

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

echo
echo "${BOLD}Lumen – Pre-release checks${NC}"
echo

if [[ $FAST -eq 0 ]]; then
  if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    if ! git diff --no-ext-diff --quiet; then
      die "Uncommitted changes; commit or stash first."
    fi
    if ! git diff --cached --quiet; then
      die "Staged but uncommitted changes; commit or unstage."
    fi
    ok "Git tree clean"
  else
    warn "Not a git repo (skipping dirty check)"
  fi
fi

run "go mod tidy"
if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  if ! git diff --quiet -- go.mod go.sum; then
    git --no-pager diff -- go.mod go.sum
    die "go mod tidy modified files; commit the changes."
  fi
fi
ok "go mod tidy OK"

run "go vet ./..."
ok "go vet OK"

run "go test ./... -count=1"
ok "go test ./... OK"

if grep -q '^preflight:' Makefile 2>/dev/null; then
  run "make preflight"
  ok "Preflight suite OK"
else
  warn "No Makefile preflight target (skipping)"
fi

if command -v make >/dev/null 2>&1; then
  run "make lint"
  ok "golangci-lint OK"
  run "make staticcheck"
  ok "staticcheck OK"
else
  warn "make not found; skipping lint/staticcheck"
fi

if [[ $NO_VULN -eq 1 ]]; then
  warn "Skipping vuln scan (--no-vuln)"
else
  if command -v make >/dev/null 2>&1; then
    run "make vuln-tools && make vulncheck"
    ok "govulncheck OK"
  else
    warn "make not found; skipping govulncheck"
  fi
fi

run "go build -trimpath -buildvcs=false -o ./build/lumend ./cmd/lumend"
ok "Build OK"

export LC_ALL=C
if strings ./build/lumend | grep -qiE '(pqc_testonly|\bnoop\b.*pqc)'; then
  die "Found test-only/noop PQC symbols in release binary"
fi
ok "PQC backend guard OK"

echo
ok "All pre-release checks passed."
