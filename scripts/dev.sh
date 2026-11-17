#!/usr/bin/env bash
# Jalankan semua lokal: Postgres + Redis (Docker), migrasi, API :8080, web :5173.
# Ctrl+C mematikan API dan web. `scripts/dev.sh down` juga mematikan Postgres + Redis.
set -euo pipefail
cd "$(dirname "$0")/.."
COMPOSE="docker compose -f infra/compose/docker-compose.yml"

if [ "${1:-}" = down ]; then
  fuser -k 8080/tcp 5173/tcp 2>/dev/null || true
  $COMPOSE down
  exit 0
fi

[ -f .env.local ] || { echo ".env.local tidak ada (salin dari .env.example)"; exit 1; }

if ! docker info >/dev/null 2>&1; then
  echo "Menyalakan Docker Desktop..."
  powershell.exe -NoProfile -Command "Start-Process \"\$env:LOCALAPPDATA\\Programs\\DockerDesktop\\Docker Desktop.exe\"" >/dev/null 2>&1 || true
  for _ in $(seq 1 60); do docker info >/dev/null 2>&1 && break; sleep 3; done
  docker info >/dev/null 2>&1 || { echo "Docker tidak menyala"; exit 1; }
fi

$COMPOSE up -d --wait postgres redis

set -a; . ./.env.local; set +a
mkdir -p "${ERAPAY_KYC_STORAGE_DIR:-.local-data/kyc}"

(cd services/api && go run ./cmd/migrate up)

trap 'kill 0' EXIT INT TERM
(cd services/api && go run ./cmd/api) &
(cd apps/web && bun --bun run dev) &
echo "API http://localhost:8080  |  Web http://localhost:5173  |  Ctrl+C untuk berhenti"
wait
