#!/usr/bin/env bash
set -euo pipefail

DIR=$(cd "$(dirname "$0")/../.." && pwd)
DIST="$DIR/dist"
VERSION=$(git -C "$DIR" describe --tags --dirty --always 2>/dev/null || echo dev)
COMMIT=$(git -C "$DIR" rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS="-s -w -X github.com/cosmos/cosmos-sdk/version.Version=$VERSION -X github.com/cosmos/cosmos-sdk/version.Commit=$COMMIT"

targets=("linux/amd64" "linux/arm64" "darwin/arm64")

echo "==> Building lumend $VERSION ($COMMIT)"

for t in "${targets[@]}"; do
  IFS=/ read -r GOOS GOARCH <<< "$t"
  outdir="$DIST/$VERSION/${GOOS}-${GOARCH}"
  mkdir -p "$outdir"
  echo "-- $t"
  (cd "$DIR" && CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build -trimpath -ldflags "$LDFLAGS" -o "$outdir/lumend" ./cmd/lumend)
done

echo "==> Generating SHA256SUMS"
(
  cd "$DIST/$VERSION"
  find . -type f -name lumend -exec sha256sum {} \; | sed 's|\./||' > SHA256SUMS
)

echo "Done. Artifacts under $DIST/$VERSION"
