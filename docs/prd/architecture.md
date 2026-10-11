# Arsitektur [arch] (PRD §3)

Baca juga: `../PRD.md` (§0 Invarian).

- **Pola:** Go modular monolith (package+interface per modul: Identity, Merchant, Payments, Pricing, Ledger, Webhooks, Reconciliation, Audit); worker bisa proses terpisah. SvelteKit+TS → Go Public API → PostgreSQL (source of truth), Redis (cache/rate limit/koordinasi transient), outbox worker → Gateway adapter ⇄ gateway (inbound webhook, laporan settlement → rekonsiliasi). Saat ini adapter aktif: DOKU.
- **Alur dana (Model B):** pelanggan bayar QRIS (atas nama PT) → DOKU → settle ke rekening PT (lapis 1) → payout ke rekening merchant (lapis 2). Ledger mencatat kedua lapis; rekonsiliasi dua lapis.
- **PostgreSQL:** FK, unique, ACID, index; migration versioned & teruji; no migration destruktif tanpa persetujuan+rollback; pertimbangkan RLS.
- **Redis:** rate limit, cache non-finansial, lock sementara. Bukan satu-satunya salinan status/ledger/idempotency. Jika dipakai sebagai queue: retry, dead-letter, observability, recovery.
- **Outbox:** event ditulis di transaksi DB yang sama dengan perubahan bisnis, dikirim async.
- **Adapter gateway:** interface `Code()`, `Capabilities()`, `CreatePayment`, `GetPayment`, `RefundPayment` (jika didukung), `VerifyWebhook`, `ParseWebhook`, `MapStatus`. Adapter didaftarkan di `GatewayRegistry` (kode → adapter); service domain hanya memanggil registry/interface, tidak meng-import paket gateway konkret. `Capabilities()` = metode bayar, refund, cancel, inquiry, partial refund, settlement report, disbursement; UI/API membaca kapabilitas ini, bukan hardcode per gateway.
- **Pemilihan gateway:** MVP = satu gateway per (merchant, payment_method) lewat konfigurasi (`gateway_merchant_mappings`), default DOKU. Tanpa routing otomatis/failover.
- **Konvensi:** timestamp UTC (tampil Asia/Jakarta); ID eksternal UUID/ULID opaque; semua request punya request/correlation ID; REST `/v1` JSON; dashboard pakai session aman, POS pakai API key scoped.
- **Keputusan:** Docker Compose untuk dev; deployment prod hardened; pisah package/service hanya bila ada tanggung jawab/deploy nyata.
