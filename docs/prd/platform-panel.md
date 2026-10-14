# Panel Platform [platform-panel] (PRD §2.1)

Baca juga: `../PRD.md`, `auth.md`, `ui.md`, `security.md`.

Pola panel khusus mengikuti aciraba_newgen (`/platform/setup`, auth terpisah, 2FA wajib),
tapi MENERAPKAN keputusan kubus EraPay: **DEWA = Platform Admin**, dipisahkan **hanya di
lapisan UI/auth** — bukan tabel `platform_admins` terpisah. Role tetap `dewa` di tabel
`roles` (scope `platform`), RBAC tetap satu sumber; yang dipisah adalah domain login dan
jalur panel.

## Keputusan (14 Okt 2026)
- `dewa` = **Platform Admin**, pengelola RBAC: `rbac:manage`, `menu:manage`, plus semua
  permission via migrasi `0016` (pola Super Admin++). Bukan role dalam dashboard merchant.
- Hanya `dewa` yang pindah ke panel. `super_admin`, `finance`, `ops_support` TETAP di
  dashboard (`/dashboard/finance/…`) memakai sesi `erapay_session` biasa.
- Initiate impersonasi "masuk sebagai tenant" (aciraba-style) = fase berikut, TIDAK bagian
  dari panel ini.
- Bootstrap DEWA pertama via `/platform/setup` + env `ERAPAY_PLATFORM_SETUP_TOKEN`
  (≥ 32 karakter), menggantikan/sekali-sekali di samping `cmd/bootstrap platform-admin`.

## Alur
1. **Bootstrap (sekali saja):** `ERAPAY_PLATFORM_SETUP_TOKEN` dipasang di env.
   `GET /v1/platform/setup-status` → `{done, enabled}` (`enabled = token ≠ "" && !done`).
   `POST /v1/platform/setup` `{token, email, password}` membuat user DEWA pertama yang
   langsung terverifikasi (operator menjamin via token), atomic + advisory lock, memakai
   `auth.CreatePlatformAdmin`. Setelah DEWA pertama ada, `done=true` dan setup menolak
   (`409 PLATFORM_SETUP_DONE`) selamanya.
2. **Login:** `POST /v1/platform/auth/login` memakai `auth.Login`; akun non-DEWA (merchant,
   super_admin/finance/ops) ditolak `401` identik tanpa bocor siapa DEWA, dan sesi yang
   sempat dibuat langsung dicabut. Bila MFA dibutuhkan: `{mfa_required}` → `POST
   /v1/platform/auth/mfa/verify`. Sukses → cookie `erapay_platform_session`
   (HttpOnly, Path=`/platform`).
3. **Panel:** rute `/v1/platform/*` di balik `requirePlatformAdmin` (DEWA saja),
   termasuk `admin/*` untuk RBAC dan `auth/mfa/*`. Semua izin tetap `rbac:manage`
   (dan di mfa.go, DEWA sudah `AdminMFARequired`).
4. **Logout:** `POST /v1/platform/auth/logout` cabut sesi + hapus cookie.

## Batas (migrasi 0019)
- Menu `/dashboard/admin/access` dihapus dari `menu_items`/`menu_role_access`: RBAC tidak
  lagi bagian sidebar dashboard. sidebar `nav.ts` juga tidak lagi memuat entri `access`
  (DEWA memakai panel, bukan dashboard).

## Referensi
- Backend: `services/api/internal/httpapi/platform.go` (auth terpisah + setup), service di
  `internal/auth/service.go` (`PlatformAdminExists`, `CreatePlatformAdmin`).
- Migrasi: `0019_platform_panel.sql`.
- Web: `apps/web/src/routes/platform/` → `(tamu)` (login/setup) & `(panel)` (layout+overview+access),
  helper `apps/web/src/lib/server/platform.ts`, `hooks.server.ts` mengisi
  `locals.platformUser` dari `/v1/platform/auth/me`.

## Terbuka (fase berikut, bukan bagian panel ini)
- Impersonasi "masuk sebagai" tenant dari panel.
- Penonaktifan/rotasi DEWA dari dalam panel (saat ini satu DEWA di bootstrap; DEWA lain
  bisa ditambah/turun di halaman Hak akses).
