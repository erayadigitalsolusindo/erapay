# Panduan Development

## Mulai cepat
```bash
cp .env.example .env     # isi nilai lokal; jangan commit
make install             # dependensi frontend
make up                  # PostgreSQL + Redis + MinIO via Docker
make api                 # terminal 1: Go API  → http://localhost:8080/readyz
make web                 # terminal 2: SvelteKit → http://localhost:5173
make test                # semua test
```

## Peta folder
```
apps/web/                 SvelteKit (UI saja, tanpa logika keuangan)
  src/lib/components      komponen UI bersama
  src/lib/server          kode server-only (klien Go API); tak bisa di-import browser
  src/lib/state           reactive state (runes)
  src/lib/utils           fungsi pembantu (format rupiah, dll.)
  src/routes              halaman berbasis folder; _components = komponen lokal rute
  src/hooks.server.ts     middleware per-request (sesi/auth)
  tests/                  unit test (vitest)
services/api/             Go API (semua validasi & logika uang ada di sini)
  cmd/api/main.go         titik masuk: baca config, sambung DB/Redis, jalankan server
  internal/config         baca environment variable
  internal/httpapi        router & handler HTTP
  migrations/             skema database (SQL berversi)
infra/compose/            Docker Compose lokal
docs/PRD.md, docs/prd/    spesifikasi produk
```

## Go untuk yang belum paham Go
- Kamu **tidak perlu menulis Go** untuk menjalankan sistem; cukup `make api`.
- `internal/` = kode privat proyek. Setiap folder = satu "package" (satu tanggung jawab).
- Satu fitur baru biasanya = satu folder baru di `internal/` + rute di `internal/httpapi/router.go`.
- `go test ./...` menjalankan semua test; `gofmt` merapikan format otomatis.
- Pola: `handler (HTTP) → service (aturan bisnis) → repository (SQL)`. Aturan uang hanya di service.

## Aturan penting (ringkas dari PRD)
Uang = integer; secret hanya di `.env`; status bayar hanya dari webhook/inquiry tervalidasi; frontend tidak menghitung fee.
