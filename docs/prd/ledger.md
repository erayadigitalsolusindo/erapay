# Ledger [ledger] (PRD §4.8)

Baca juga: `../PRD.md` (§0 #6, #14, #14b), `pricing.md`, `payout.md`, `reconciliation.md`.

Setiap peristiwa finansial relevan → catatan immutable; double-entry bila cocok dengan skema akuntansi; debit=kredit per journal; entri punya currency, ref transaksi, event sumber, timestamp, dedup key; pisahkan catatan operasional dari jurnal akuntansi; saldo merchant di ledger = kewajiban (payable) yang hanya keluar lewat payout (`payout.md`), bukan wallet bebas; laporan gross/fee/expected net/settlement observed/selisih/status, semua traceable ke transaksi & gateway reference. Ledger netral-gateway.

**Akun minimal Model B:** `cash_doku_clearing`, `cash_bank_pt`, `merchant_payable`, `payout_in_transit`, `refund_payable`, `platform_fee_revenue`, `gateway_fee_expense`, `tax_payable`.

**Saldo merchant dibagi:** `pending` (belum settle DOKU), `available`, `on_hold`, `in_payout`.
