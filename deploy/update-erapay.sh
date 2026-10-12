#!/usr/bin/env bash
# Satu perintah update: ambil kode -> build -> backup -> migrasi -> pasang -> restart -> cek kesehatan (API, lalu web).
# Salin ke ~/update-erapay.sh di server. Jalankan sebagai user biasa (sudo diminta di tahap pemasangan).
# Lihat docs/DEPLOY-AAPANEL.md. SKIP_WEB=1 untuk hanya memperbarui API.
set -euo pipefail
SRC_DIR="${SRC_DIR:-/www/wwwroot/erapay-src}"
APP_DIR="${APP_DIR:-/www/wwwroot/erapay-api.erayadigital.co.id}"
WEB_DIR="${WEB_DIR:-/www/wwwroot/erapay.erayadigital.co.id}"

cd "$SRC_DIR"
git pull --ff-only
echo "==> kode sekarang:"; git log --oneline -1

echo "==> API"
SRC_DIR="$SRC_DIR" bash "$SRC_DIR/deploy/build-release.sh"
sudo env APP_DIR="$APP_DIR" RESTART_CMD='systemctl restart erapay-api' \
  bash "$SRC_DIR/deploy/aapanel/update.sh" "$HOME/erapay-release.tar.gz"

if [ "${SKIP_WEB:-0}" != "1" ]; then
  echo "==> WEB"
  SRC_DIR="$SRC_DIR" bash "$SRC_DIR/deploy/web/build-release.sh"
  sudo env WEB_DIR="$WEB_DIR" RESTART_CMD='systemctl restart erapay-web' \
    bash "$SRC_DIR/deploy/web/update.sh" "$HOME/erapay-web-release.tar.gz"
fi
