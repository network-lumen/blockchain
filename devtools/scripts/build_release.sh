#!/usr/bin/env bash
set -euo pipefail

DIR=$(cd "$(dirname "$0")/../.." && pwd)
DIST="$DIR/dist"
VERSION=$(git -C "$DIR" describe --tags --dirty --always 2>/dev/null || echo dev)
COMMIT=$(git -C "$DIR" rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS="-s -w -X github.com/cosmos/cosmos-sdk/version.Version=$VERSION -X github.com/cosmos/cosmos-sdk/version.Commit=$COMMIT"

targets=("linux/amd64" "linux/arm64" "darwin/arm64" "windows/amd64")

echo "==> Building lumend $VERSION ($COMMIT)"

build_root="$DIST/$VERSION"
mkdir -p "$build_root"

for t in "${targets[@]}"; do
  IFS=/ read -r GOOS GOARCH <<< "$t"
  artifact_stem="${GOOS}-${GOARCH}-${VERSION}"
  bin_name="$artifact_stem"
  if [[ "$GOOS" == "windows" ]]; then
    bin_name="${artifact_stem}.exe"
  fi
  bin_path="$build_root/$bin_name"
  echo "-- $t"
  (cd "$DIR" && CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build -trimpath -ldflags "$LDFLAGS" -o "$bin_path" ./cmd/lumend)

  if [[ "$GOOS" == "windows" ]]; then
    archive_name="${artifact_stem}.zip"
    python3 - "$build_root" "$bin_name" "$archive_name" <<'PY'
import os, sys, zipfile
root, binary_name, archive_name = sys.argv[1:]
src = os.path.join(root, binary_name)
dst = os.path.join(root, archive_name)
if os.path.exists(dst):
    os.remove(dst)
with zipfile.ZipFile(dst, "w", compression=zipfile.ZIP_DEFLATED) as zf:
    zf.write(src, arcname=binary_name)
PY
  else
    archive_name="${artifact_stem}.tar.gz"
    (
      cd "$build_root"
      tar -czf "$archive_name" "$bin_name"
    )
  fi
done

echo "==> Generating SHA256SUMS"
(
  cd "$build_root"
  find . -maxdepth 1 -type f \( -name "*-${VERSION}" -o -name "*-${VERSION}.exe" \) -exec sha256sum {} \; | sed 's|\./||' > SHA256SUMS.bin
  find . -maxdepth 1 -type f \( -name "*-${VERSION}.tar.gz" -o -name "*-${VERSION}.zip" \) -exec sha256sum {} \; | sed 's|\./||' > SHA256SUMS.archive
  cat SHA256SUMS.bin SHA256SUMS.archive > SHA256SUMS
)

echo "Done. Artifacts under $build_root"
