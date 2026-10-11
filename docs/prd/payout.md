# Payout & saldo merchant (Model B) [payout] (PRD §4.13)

Baca juga: `../PRD.md` (§0 #14, #14a, #14b), `ledger.md`, `reconciliation.md`, `security.md`, `merchant.md`.

- **Payout:** entitas `payouts` + `payout_items` (transaksi yang dibayarkan). State: `requested → pending_approval → approved → submitted → processing → paid | failed | returned | cancelled`; `unknown` bila timeout (inquiry dulu sebelum retry).
- **Jadwal:** terjadwal (T+N sesuai kebijakan) atau on-demand bila diaktifkan; min/max payout, limit harian; fee payout belum ada dalam rencana (OPEN; jangan dihardcode).
- **Kontrol:** maker-checker (pembuat ≠ penyetuju) di atas ambang; MFA saat approve; hanya rekening terverifikasi (allowlist) + cooling period setelah ganti rekening; idempotency key per payout; payout ≤ `available` dengan lock baris/constraint DB dalam transaksi; limit & alert anomali; semua aksi diaudit; developer tanpa akses approve payout produksi.
- **Eksekusi:** via DOKU disbursement bila ada (`Capabilities().disbursement`, [KD]); MVP: batch file/transfer manual + konfirmasi (adapter bank opsional kemudian). Default kode payout = dry-run di non-produksi.
- **Pemisahan dana:** rekening penampung dana merchant terpisah dari operasional [LEGAL]; alert bila `sum(merchant_payable)` > dana di rekening penampung.
- **Rekonsiliasi harian:** ledger ↔ laporan DOKU ↔ mutasi bank PT; selisih → exception.
- **Notifikasi:** merchant diberi tahu payout dibuat/dibayar/gagal + rincian per transaksi (gross, fee, net).
