# Auth [auth] (PRD §4.1)

Baca juga: `../PRD.md`, `security.md`.

- Login/logout admin & merchant; password hashing modern; MFA admin wajib, merchant owner direncanakan; RBAC granular; session/token bisa dicabut + expiry jelas; rate limit + audit di login & aksi sensitif; sandbox & production terpisah.
- **Daftar merchant mandiri (12 Okt 2026):** `POST /v1/auth/register` (publik, tanpa sesi) membuat user + merchant `submitted` + outlet + keanggotaan `merchant_owner` dalam satu transaksi. Pembayaran dan payout sudah menolak merchant yang bukan `approved`. Review lewat `GET/POST /v1/admin/merchants…` (izin `merchant:review` untuk melihat, `merchant:approve` hanya `super_admin` karena persetujuan menetapkan tarif; sesi merchant selalu ditolak). Kebijakan kata sandi: 12–128 karakter. Rate limit 5 percobaan/IP/jam (`register_attempts`).
- API key: prefix publik, secret tampil sekali, disimpan hash, revoke/rotate, scoped.
