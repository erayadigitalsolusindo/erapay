# Semua perintah sehari-hari. Jalankan `make help`.
.DEFAULT_GOAL := help
COMPOSE := docker compose -f infra/compose/docker-compose.yml

help: ## Tampilkan daftar perintah
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  make %-10s %s\n", $$1, $$2}'

up: ## Nyalakan PostgreSQL, Redis, MinIO (Docker)
	$(COMPOSE) up -d

down: ## Matikan layanan Docker
	$(COMPOSE) down

migrate: ## Terapkan migration database (butuh .env)
	cd services/api && set -a && . ../../.env && set +a && go run ./cmd/migrate up

api: ## Jalankan Go API di :8080 (butuh `make up` dan file .env)
	cd services/api && set -a && . ../../.env && set +a && go run ./cmd/api

web: ## Jalankan SvelteKit dev server di :5173
	cd apps/web && npm run dev

install: ## Pasang dependensi frontend
	cd apps/web && npm install

test: ## Jalankan semua test (Go + frontend)
	cd services/api && go test ./...
	cd apps/web && npm test

check: ## Cek format/tipe (Go + frontend)
	cd services/api && test -z "$$(gofmt -l .)" && go vet ./...
	cd apps/web && npm run check
