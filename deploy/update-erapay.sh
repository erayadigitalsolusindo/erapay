#!/usr/bin/env bash
# Satu perintah update: ambil kode -> build -> backup -> migrasi -> pasang -> restart -> cek kesehatan.
# Salin ke ~/update-erapay.sh di server. Jalankan sebagai user biasa (sudo diminta di tahap pemasangan).
# Lihat docs/DEPLOY-AAPANEL.md.
set -euo pipefail
SRC_DIR="${SRC_DIR:-$HOME/erapay-src}"
APP_DIR="${APP_DIR:-/www/wwwroot/erapay-api.erayadigital.co.id}"

cd "$SRC_DIR"
git pull --ff-only
echo "==> kode sekarang:"; git log --oneline -1
SRC_DIR="$SRC_DIR" bash "$SRC_DIR/deploy/build-release.sh"
sudo env APP_DIR="$APP_DIR" RESTART_CMD='systemctl restart erapay-api' \
  bash "$SRC_DIR/deploy/aapanel/update.sh" "$HOME/erapay-release.tar.gz"
