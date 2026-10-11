# Refund / cancel / dispute [refund] (PRD §4.10)

Baca juga: `../PRD.md`, `ledger.md`, `payout.md`.

Hanya aktif bila `Capabilities()` gateway mendukung & dikontrak (DOKU: [KD]). Bedakan full/partial (bila didukung)/cancel sebelum bayar/reversal/dispute. Perlu permission, alasan, idempotency, audit. Refund adalah entitas & state machine terpisah (tidak mengubah pembayaran sukses jadi gagal); total refund ≤ sisa yang bisa dikembalikan; timeout refund → status tidak pasti, cek sebelum retry; tampilkan status gateway, jangan janji waktu dana kembali tanpa sumber resmi; catat dispute/chargeback bila relevan.

Model B: PT menanggung refund/chargeback ke pelanggan, lalu mengurangi saldo merchant (`refund_payable`); saldo tidak boleh negatif tanpa kebijakan eksplisit [OPEN]; payout ditahan bila ada dispute terbuka sesuai kebijakan.
