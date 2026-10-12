#!/usr/bin/env bash
# Pasang release web: backup -> ekstrak -> npm ci --omit=dev -> restart -> cek kesehatan (rollback jika gagal).
# Jalankan sebagai root: sudo env WEB_DIR=... RESTART_CMD='systemctl restart erapay-web' bash update.sh <release.tar.gz>
set -euo pipefail
TARBALL="${1:?usage: update.sh <release.tar.gz>}"
WEB_DIR="${WEB_DIR:-/www/wwwroot/erapay.erayadigital.co.id}"
RESTART_CMD="${RESTART_CMD:-systemctl restart erapay-web}"
ENV_FILE="${ENV_FILE:-$WEB_DIR/erapay-web.env}"
RUN_USER="${RUN_USER:-www}"

[ -f "$ENV_FILE" ] || { echo "env tidak ada: $ENV_FILE"; exit 1; }
PORT="$(grep -E '^PORT=' "$ENV_FILE" | tail -1 | cut -d= -f2- | tr -d "\"'")"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:${PORT:-3000}/login}"

STAMP="$(date +%Y%m%d-%H%M%S)"
STAGE="$(mktemp -d)"; trap 'rm -rf "$STAGE"' EXIT
tar -C "$STAGE" -xzf "$TARBALL"
mkdir -p "$WEB_DIR/backups"

echo "==> backup build lama -> backups/$STAMP"
if [ -d "$WEB_DIR/build" ]; then
  mkdir -p "$WEB_DIR/backups/$STAMP"
  cp -a "$WEB_DIR/build" "$WEB_DIR/package.json" "$WEB_DIR/package-lock.json" "$WEB_DIR/backups/$STAMP/" 2>/dev/null || true
fi
{ ls -1dt "$WEB_DIR"/backups/*/ 2>/dev/null || true; } | tail -n +6 | xargs -r rm -rf

echo "==> pasang build (versi $(cat "$STAGE/VERSION"))"
rm -rf "$WEB_DIR/build"
cp -a "$STAGE/build" "$STAGE/package.json" "$STAGE/package-lock.json" "$STAGE/VERSION" "$WEB_DIR/"
( cd "$WEB_DIR" && npm ci --omit=dev --no-audit --no-fund )
chown -R "$RUN_USER:$RUN_USER" "$WEB_DIR/build" "$WEB_DIR/node_modules" "$WEB_DIR/package.json" "$WEB_DIR/package-lock.json" "$WEB_DIR/VERSION"

echo "==> restart"
eval "$RESTART_CMD"

echo "==> cek kesehatan $HEALTH_URL"
for i in $(seq 1 20); do
  code="$(curl -s -o /dev/null -w '%{http_code}' "$HEALTH_URL" || true)"
  case "$code" in 200|302|303) echo "OK: erapay-web $(cat "$WEB_DIR/VERSION") berjalan (HTTP $code)."; exit 0;; esac
  sleep 1
done

echo "GAGAL: kesehatan tidak OK, rollback ke backups/$STAMP"
if [ -d "$WEB_DIR/backups/$STAMP/build" ]; then
  rm -rf "$WEB_DIR/build"
  cp -a "$WEB_DIR/backups/$STAMP/build" "$WEB_DIR/build"
  eval "$RESTART_CMD" || true
fi
exit 1
