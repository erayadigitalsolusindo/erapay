# Refund / cancel / dispute [refund] (PRD §4.10)

Baca juga: `../PRD.md`, `ledger.md`, `payout.md`.

Hanya aktif bila `Capabilities()` gateway mendukung & dikontrak (DOKU: [KD]). Bedakan full/partial (bila didukung)/cancel sebelum bayar/reversal/dispute. Perlu permission, alasan, idempotency, audit. Refund adalah entitas & state machine terpisah (tidak mengubah pembayaran sukses jadi gagal); total refund ≤ sisa yang bisa dikembalikan; timeout refund → status tidak pasti, cek sebelum retry; tampilkan status gateway, jangan janji waktu dana kembali tanpa sumber resmi; catat dispute/chargeback bila relevan.

Model B: PT menanggung refund/chargeback ke pelanggan, lalu mengurangi saldo merchant (`refund_payable`); saldo tidak boleh negatif tanpa kebijakan eksplisit [OPEN]; payout ditahan bila ada dispute terbuka sesuai kebijakan.

## Implementasi (v1: refund manual, tanpa adapter gateway)
- **Entitas terpisah** `refunds` (`pending_approval → approved → refunded`; akhir lain `failed`, `cancelled`). Payment tetap `succeeded`; API payment menampilkan `refunded_amount` dan `refunding_amount` sebagai nilai turunan.
- **Request:** `POST /v1/payments/{id}/refund {amount, reason}` + `Idempotency-Key` (owner, sesi dashboard saja). Dalam satu transaksi (kunci merchant lalu baris payment): payment harus `succeeded`; total refund hidup ≤ nominal payment (parsial boleh); jumlah ≤ saldo `available` (dana sudah settle) sehingga saldo tak pernah negatif; reservasi ledger `Dr merchant_payable / Cr refund_payable` (tampil sebagai `on_hold` di `GET /v1/balance`).
- **Approve/eksekusi:** Finance `approve` (pemohon ≠ penyetuju, CHECK DB), mengembalikan dana ke pelanggan di luar sistem, lalu `mark-refunded {bank_reference}` → `Dr refund_payable / Cr cash_bank_pt` sekali. `reject`/`cancel`/`mark-failed` melepas reservasi dan memulihkan batas refund.
- **Notifikasi:** webhook `refund.refunded|failed|cancelled` (juga `payout.paid|failed|cancelled`) di transaksi yang sama dengan perubahan status.
- **Belum:** refund lewat gateway (DOKU [KD]), pengembalian fee, dispute/chargeback, refund atas payment yang belum settle.
