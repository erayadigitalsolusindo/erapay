#!/usr/bin/env bash
# Build binary Go (linux, statis) lalu bungkus ke ~/erapay-release.tar.gz. Butuh Go terpasang di server.
set -euo pipefail
SRC_DIR="${SRC_DIR:-$HOME/erapay-src}"
OUT="${OUT:-$HOME/erapay-release.tar.gz}"
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

mkdir -p "$STAGE/bin"
cd "$SRC_DIR/services/api"
export CGO_ENABLED=0
for cmd in api migrate bootstrap; do
  echo "==> build $cmd"
  go build -trimpath -ldflags "-s -w" -o "$STAGE/bin/erapay-$cmd" "./cmd/$cmd"
done
git -C "$SRC_DIR" rev-parse --short HEAD > "$STAGE/VERSION"

tar -C "$STAGE" -czf "$OUT" bin VERSION
echo "==> release: $OUT ($(cat "$STAGE/VERSION"))"
