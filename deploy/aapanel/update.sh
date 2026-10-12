#!/usr/bin/env bash
# Pasang release: backup -> migrasi -> ganti binary -> restart -> cek kesehatan (rollback otomatis jika gagal).
# Jalankan sebagai root: sudo env APP_DIR=... RESTART_CMD='systemctl restart erapay-api' bash update.sh <release.tar.gz>
set -euo pipefail
TARBALL="${1:?usage: update.sh <release.tar.gz>}"
APP_DIR="${APP_DIR:-/www/wwwroot/erapay-api.erayadigital.co.id}"
RESTART_CMD="${RESTART_CMD:-systemctl restart erapay-api}"
ENV_FILE="${ENV_FILE:-$APP_DIR/erapay.env}"
RUN_USER="${RUN_USER:-www}"
HEALTH_URL="${HEALTH_URL:-}"

[ -f "$ENV_FILE" ] || { echo "env tidak ada: $ENV_FILE"; exit 1; }
if [ -z "$HEALTH_URL" ]; then
  ADDR="$(grep -E '^ERAPAY_HTTP_ADDR=' "$ENV_FILE" | tail -1 | cut -d= -f2- | tr -d "\"'")"
  HEALTH_URL="http://${ADDR:-127.0.0.1:8181}/readyz"
fi

STAMP="$(date +%Y%m%d-%H%M%S)"
STAGE="$(mktemp -d)"; trap 'rm -rf "$STAGE"' EXIT
tar -C "$STAGE" -xzf "$TARBALL"
mkdir -p "$APP_DIR/bin" "$APP_DIR/backups"

echo "==> backup binary lama -> backups/$STAMP"
if ls "$APP_DIR"/bin/erapay-* >/dev/null 2>&1; then
  mkdir -p "$APP_DIR/backups/$STAMP"
  cp -a "$APP_DIR"/bin/. "$APP_DIR/backups/$STAMP/"
fi
# simpan 5 backup terakhir
{ ls -1dt "$APP_DIR"/backups/*/ 2>/dev/null || true; } | tail -n +6 | xargs -r rm -rf

echo "==> migrasi database (versi $(cat "$STAGE/VERSION"))"
( set -a; . "$ENV_FILE"; set +a; "$STAGE/bin/erapay-migrate" up; "$STAGE/bin/erapay-migrate" status )

echo "==> pasang binary"
for f in "$STAGE"/bin/*; do
  install -m 0755 -o "$RUN_USER" -g "$RUN_USER" "$f" "$APP_DIR/bin/$(basename "$f").new"
  mv -f "$APP_DIR/bin/$(basename "$f").new" "$APP_DIR/bin/$(basename "$f")"
done
cp "$STAGE/VERSION" "$APP_DIR/VERSION"

echo "==> restart"
eval "$RESTART_CMD"

echo "==> cek kesehatan $HEALTH_URL"
for i in $(seq 1 15); do
  if curl -fsS -o /dev/null "$HEALTH_URL"; then
    echo "OK: erapay-api $(cat "$APP_DIR/VERSION") berjalan."
    exit 0
  fi
  sleep 1
done

echo "GAGAL: kesehatan tidak OK, rollback ke backups/$STAMP"
if [ -d "$APP_DIR/backups/$STAMP" ]; then
  cp -a "$APP_DIR/backups/$STAMP/." "$APP_DIR/bin/"
  eval "$RESTART_CMD" || true
fi
echo "Catatan: migrasi tidak di-rollback otomatis; pastikan migrasi kompatibel mundur."
exit 1
