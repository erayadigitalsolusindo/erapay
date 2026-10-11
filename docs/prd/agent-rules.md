# Aturan kerja agent [agent] (PRD §12)

Baca juga: `../PRD.md`, `testing.md`.

- Baca `AGENTS.md` + `docs/PRD.md` (dan file modul terkait) sebelum mengubah. Implementasi hanya adapter DOKU; rancang kode agar gateway baru cukup menambah adapter+mapping+docs (tanpa mengubah domain/ledger/API publik). Pecah ke issue kecil; satu PR/commit = satu area. Test sebelum/bersama implementasi. Jalankan formatter, linter, unit/integration test relevan, validasi migration.
- Payout produksi/dana nyata dilarang sebelum go/no-go (`roadmap.md`); kode payout default dry-run.
- Tanpa migration destruktif tanpa persetujuan+rollback; tanpa perubahan API publik/skema rilis tanpa rencana migration/versioning; tanpa production secret di test/prompt.
- **Urutan:** 1 repo+config+logging+health+CI → 2 schema PG+migration → 3 identity/RBAC/tenancy → 4 API key+idempotency → 5 payment domain+state machine → 6 gateway interface+registry+adapter DOKU sandbox → 7 webhook inbound → 8 outbox+webhook outbound → 9 pricing snapshot+perhitungan fee+ledger Model B+saldo → 9b payout+maker-checker → 10 dashboard → 11 rekonsiliasi → 12 observability/hardening/runbook → 13 pilot readiness.
- **Definition of Done:** requirement+acceptance terpenuhi; test baru & terkait lulus; tanpa secret di code/fixture/log/docs; error handling & observability memadai; migration aman/backward-compatible; API/docs diperbarui; ringkasan + risiko + langkah verifikasi; tak ada klaim selesai untuk mock/stub; perubahan finansial punya uji idempotency & concurrency.
- **Struktur repo (awal):** `AGENTS.md, README.md, docs/{PRD,ARCHITECTURE,API_CONVENTIONS,SECURITY,RUNBOOK}.md, docs/prd/*.md, docs/decisions/ADR-*.md, docs/integrations/DOKU.md, apps/web (SvelteKit), services/api (Go API+worker; internal/gateway/{registry,doku} — paket gateway baru hanya via gateway-new.md), packages/api-client (opsional), infra/{docker,compose,monitoring}, scripts/, tests/`. Jangan buat package/service terpisah hanya demi struktur.
