# Settlement & rekonsiliasi [recon] (PRD §4.9)

Baca juga: `../PRD.md`, `ledger.md`, `payout.md`.

- Status settlement: `not_expected, expected, processing, settled, delayed, failed, unknown, disputed`. Simpan batch/ref, gateway, tanggal, gross, fee, net, rekening tersamarkan, sumber laporan.
- Dua lapis: (1) gateway → PT (settlement), (2) PT → merchant (payout). Dana menjadi `available` untuk payout hanya setelah settlement gateway terkonfirmasi, bukan sekadar payment success (kecuali kebijakan float disetujui [LEGAL]).
- Cocokkan transaksi EraPay ↔ data gateway ↔ settlement. Selisih nominal/hilang/duplikat/telat → `reconciliation_exceptions`; Finance beri catatan/resolusi (diaudit).
- Parser laporan settlement per gateway (format berbeda), output ke model internal yang sama.
- Impor file: validasi kolom, preview, duplicate detection, laporan error. MVP: impor manual terkontrol bila laporan otomatis belum ada. Jadwal & hari libur dari konfigurasi (bukan hardcode). Jangan klaim dana diterima hanya dari payment success; ikuti konfirmasi gateway.
- **Jadwal settlement (lisan DOKU 14 Okt 2026, [KD] tertulis):** QRIS settle **T+1 hari kerja**, diproses **12:00–14:00 GMT+7** → job pencatatan batch/rekonsiliasi dijadwalkan **setelah 14:00 GMT+7** (bukan tengah malam); transisi `expected → settled` paling cepat T+1 sore. Dana tidak boleh dianggap diterima sebelum jendela itu lewat.

## Implementasi (v1, lapis 1: gateway → PT)
- **Tanpa parser laporan:** format laporan settlement DOKU belum terkonfirmasi [KD] (`integrations/doku.md` §6), jadi tidak ada parser. Finance mencatat batch manual dari laporan resmi: `POST /v1/admin/settlement-batches {gateway, batch_ref, currency, settlement_date, reported_net, payment_ids[], note}` (izin `settlement:manage`, hanya role `finance`).
- **Verifikasi server:** `expected_net = Σ(gross − mdr_fee)` dari data EraPay. Batch dibukukan hanya jika `reported_net == expected_net`; selisih → `422 SETTLEMENT_MISMATCH` + satu `reconciliation_exceptions` terbuka per `(gateway, batch_ref)`, tanpa perubahan saldo. Constraint DB `CHECK (reported_net = expected_net)` menjadi pagar kedua.
- **Ekspektasi exception MDR transaksi kecil (lisan DOKU 14 Okt 2026, [KD] tertulis):** MDR QRIS 0,7% dengan **kap Rp500 untuk Rp0–Rp100.000** (aturan BI, efektif 1 Nov 2026), sementara halaman harga publik menulis 0% untuk rentang itu → sampai kontrak tertulis final, `reported_net == expected_net` kemungkinan **gagal untuk transaksi ≤ Rp100.000** dan membuka `reconciliation_exceptions`. **Jangan ditutup manual** sebelum konfirmasi; selisih ini menunggu keputusan basis MDR (`pricing.md`, `integrations/doku.md` §6).
- **Pembukuan:** `Dr cash_bank_pt / Cr cash_doku_clearing` sebesar net, satu jurnal per batch (dedup key `settlement:<gateway>:<batch_ref>`), satu transaksi dengan `settlement_items` + audit.
- **Idempotensi:** `(gateway, batch_ref)` unik: kiriman identik → `200` replay; isi berbeda → `409`. `settlement_items.payment_id` unik: satu payment hanya bisa settle sekali, batch yang tumpang tindih tak bisa menang bersamaan. Item harus `succeeded`, gateway dan currency sama, dan belum settled; selain itu `422 SETTLEMENT_INELIGIBLE`.
- **Status terpisah (invarian #9):** `GET /v1/payments/{id}` memuat `settlement_status` = `not_expected | expected | settled`, terpisah dari `status`.
- **Saldo (`GET /v1/balance`, butuh `report:read`):** `payable` (ledger `merchant_payable`), `pending` (merchant_net payment sukses yang belum settle), `available = payable − pending`. Dana `available` hanya yang sudah settle (kebijakan float belum disetujui). `on_hold` / `in_payout` menyusul bersama payout dan refund.
- **Admin:** `GET /v1/admin/unsettled-payments`, `GET /v1/admin/settlement-batches`, `GET /v1/admin/reconciliation-exceptions`, `POST /v1/admin/reconciliation-exceptions/{id}/resolve {note}` (izin `settlement:read|manage`; `ops_support` hanya baca).
