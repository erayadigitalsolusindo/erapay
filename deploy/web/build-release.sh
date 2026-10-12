#!/usr/bin/env bash
# Build web SvelteKit (adapter-node) lalu bungkus ke ~/erapay-web-release.tar.gz. Butuh Node >= 22.17 + npm di server.
set -euo pipefail
SRC_DIR="${SRC_DIR:-/www/wwwroot/erapay-src}"
OUT="${OUT:-$HOME/erapay-web-release.tar.gz}"
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

cd "$SRC_DIR/apps/web"
echo "==> npm ci"
npm ci --no-audit --no-fund
echo "==> build"
npm run build

# adapter-node membiarkan dependency runtime tetap eksternal: kirim package*.json agar server menjalankan `npm ci --omit=dev`.
cp -r build "$STAGE/build"
cp package.json package-lock.json "$STAGE/"
git -C "$SRC_DIR" rev-parse --short HEAD > "$STAGE/VERSION"
tar -C "$STAGE" -czf "$OUT" build package.json package-lock.json VERSION
echo "==> release web: $OUT ($(cat "$STAGE/VERSION"))"
