# AGENTS.md — EraPay

Sebelum mengubah apa pun, baca `docs/PRD.md` (§0 Invarian) dan file modul terkait di `docs/prd/`.

- Stack: Go (`services/api`, chi + pgx), PostgreSQL, Redis, SvelteKit (`apps/web`).
- Hanya adapter DOKU yang boleh diimplementasikan; domain tidak boleh meng-import paket gateway konkret.
- UI (`apps/web`) wajib mengikuti tema di `docs/prd/ui-theme.md` (referensi: `docs/refrence/*.html`, Tailwind).
- Uang = integer. Secret hanya dari env; jangan commit `.env`.
- Payout default dry-run; tidak ada dana nyata sebelum go/no-go (`docs/prd/roadmap.md`).
- Perubahan finansial wajib uji idempotency + concurrency.
- Dev lokal: `docker compose -f infra/compose/docker-compose.yml up -d`, lalu `cd services/api && go test ./...`.
