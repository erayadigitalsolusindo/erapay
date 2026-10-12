# Payout & saldo merchant (Model B) [payout] (PRD §4.13)

Baca juga: `../PRD.md` (§0 #14, #14a, #14b), `ledger.md`, `reconciliation.md`, `security.md`, `merchant.md`.

- **Payout:** entitas `payouts` + `payout_items` (transaksi yang dibayarkan). State: `requested → pending_approval → approved → submitted → processing → paid | failed | returned | cancelled`; `unknown` bila timeout (inquiry dulu sebelum retry).
- **Jadwal:** terjadwal (T+N sesuai kebijakan) atau on-demand bila diaktifkan; min/max payout, limit harian; fee payout belum ada dalam rencana (OPEN; jangan dihardcode).
- **Kontrol:** maker-checker (pembuat ≠ penyetuju) di atas ambang; MFA saat approve; hanya rekening terverifikasi (allowlist) + cooling period setelah ganti rekening; idempotency key per payout; payout ≤ `available` dengan lock baris/constraint DB dalam transaksi; limit & alert anomali; semua aksi diaudit; developer tanpa akses approve payout produksi.
- **Eksekusi:** via DOKU disbursement bila ada (`Capabilities().disbursement`, [KD]); MVP: batch file/transfer manual + konfirmasi (adapter bank opsional kemudian). Default kode payout = dry-run di non-produksi.
- **Pemisahan dana:** rekening penampung dana merchant terpisah dari operasional [LEGAL]; alert bila `sum(merchant_payable)` > dana di rekening penampung.
- **Rekonsiliasi harian:** ledger ↔ laporan DOKU ↔ mutasi bank PT; selisih → exception.
- **Notifikasi:** merchant diberi tahu payout dibuat/dibayar/gagal + rincian per transaksi (gross, fee, net).

## Implementasi (v1: dry-run, transfer dicatat manual)
- **State:** `pending_approval → approved → paid`; akhir lain: `failed`, `cancelled` (cancel/reject). Belum ada `submitted/processing/returned/unknown`: tak ada adapter eksekusi, transfer dilakukan Finance lalu dicatat.
- **Rekening:** `POST /v1/bank-accounts` (owner, sesi saja) → `pending_verification`; Finance `POST /v1/admin/bank-accounts/{id}/verify` (pembuat ≠ verifikator, juga CHECK DB). Nomor rekening dienkripsi AES-256-GCM (terikat ke id baris), API hanya mengembalikan `••••last4`. Cooling period setelah verifikasi (`ERAPAY_PAYOUT_COOLING`, default 24h). Rekening tak bisa dinonaktifkan selama ada payout terbuka.
- **Request:** `POST /v1/payouts` + `Idempotency-Key` (sesi dashboard, bukan API key). Satu transaksi: kunci baris merchant (`FOR NO KEY UPDATE`) → cek limit (min/max/harian rolling 24h) → cek `available` → insert → **reservasi ledger** `Dr merchant_payable / Cr payout_in_transit`. Maka dua request paralel tak bisa melampaui saldo, dan `available` turun seketika.
- **Approve (checker):** `POST /v1/admin/payouts/{id}/approve` (role `finance`), pemohon tak boleh approve (kode + CHECK DB). Reject/cancel/mark-failed melepas reservasi (`Dr payout_in_transit / Cr merchant_payable`).
- **Eksekusi:** `GET /v1/admin/payouts/{id}/destination` (hanya payout `approved`, diaudit, `no-store`) memberi nomor rekening ke Finance; setelah transfer: `POST .../mark-paid {bank_reference}` → `Dr payout_in_transit / Cr cash_bank_pt`, sekali (dedup key + status). Ulangi dengan referensi sama = no-op; referensi lain = 409.
- **Gate produksi:** `dry_run` = `ERAPAY_ENV != production`. Di produksi payout ditolak (`503 PAYOUT_DISABLED`) sampai `ERAPAY_PAYOUT_LIVE=1` (go/no-go). Seluruh fitur butuh `ERAPAY_ENCRYPTION_KEY`.
- **Saldo:** `GET /v1/balance` kini memuat `in_payout` (ledger `payout_in_transit`); `payable` tidak lagi memuat dana yang sedang direservasi.
- **Belum:** `payout_items` per transaksi, jadwal otomatis, MFA saat approve, alert anomali, notifikasi ke merchant (webhook outbound untuk event payout), rekonsiliasi lapis 2 (mutasi bank).
