# AGENTS.md — EraPay

Sebelum mengubah apa pun, baca `docs/PRD.md` (§0 Invarian) dan file modul terkait di `docs/prd/`.

- Stack: Go (`services/api`, chi + pgx), PostgreSQL, Redis, SvelteKit (`apps/web`).
- Adapter yang boleh: DOKU dan IDS (`internal/gateway/ids`, IDS Payment Service sebagai upstream QRIS; keputusan pemilik, 28 Okt 2026). Gateway lain tetap dilarang tanpa gate `docs/prd/gateway-new.md`. Domain tidak boleh meng-import paket gateway konkret.
- UI (`apps/web`) wajib mengikuti tema di `docs/prd/ui-theme.md` (referensi: `docs/refrence/*.html`, Tailwind).
- Uang = integer. Secret hanya dari env; jangan commit `.env`.
- Payout default dry-run; tidak ada dana nyata sebelum go/no-go (`docs/prd/roadmap.md`).
- Perubahan finansial wajib uji idempotency + concurrency.
- Dev lokal: `docker compose -f infra/compose/docker-compose.yml up -d`, lalu `cd services/api && go test ./...`.
