# Merchant [merchant] (PRD §4.2)

Baca juga: `../PRD.md`, `payout.md` (rekening payout).

- Admin: buat/review/aktifkan/suspend/nonaktifkan. Merchant isi profil legal/bisnis/kontak/alamat/kategori/data onboarding.
- **Data diri (verifikasi, DOKU-style — 14 Okt 2026):** pendaftaran mandiri wajib mengisi data diri pemilik: NIK (16 digit), tempat & tanggal lahir (usia ≥ 17 tahun), jenis kelamin, alamat sesuai KTP; NPWP pemilik opsional (15–16 digit); akun `business` wajib NPWP badan usaha + NIB (13 digit). Validasi hanya di Go. NIK & NPWP dienkripsi at-rest (AES-256-GCM via `secretbox`, AAD = id merchant, skema seperti `merchant_bank_accounts.account_enc`) dan tampil **ter-masking** (`••••1234`) di semua view; **plaintext hanya di view detail reviewer** (`GET /v1/admin/merchants/{id}` → `GetForReview`, izin `merchant:review`) karena review manual membutuhkan pembacaan data isian (data.md). Audit hanya mencatat `id_last4` + kelengkapan, tidak pernah plaintext. Upload dokumen KYC mentah tetap dihindari (lihat `open-questions.md`).
- Pelengkapan/perbaikan data diri: `POST /v1/merchant/identity` (sesi merchant, izin `merchant:write`, hanya saat status `submitted`/`under_review`/`action_required`; terkunci setelah diputuskan). Aturan wajib mengikuti `account_type` merchant, bukan permintaan.
- **Gate approval:** `merchant.approve` ditolak (`IDENTITY_INCOMPLETE`) sampai data diri lengkap (cek dari DB), seperti penolakan `EMAIL_NOT_VERIFIED`. Merchant yang dibuat sebelum fitur ini (termasuk dari ARUS) melengkapi datanya via `Status akun`.
- Status onboarding: `draft, submitted, under_review, action_required, approved, rejected, suspended`. Status EraPay & status per gateway (mis. DOKU) disimpan **terpisah**; simpan referensi merchant di gateway + status aktivasi setelah dikonfirmasi.
- Rekening payout merchant: diverifikasi (nama rekening sesuai merchant) sebelum payout; perubahan dibatasi + diaudit + cooling period + notifikasi, dan menahan payout sampai terverifikasi.
- Satu merchant: banyak outlet, terminal, user.
