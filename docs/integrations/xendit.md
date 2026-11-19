# Xendit (calon adapter `xendit`)

Sumber: https://docs.xendit.co (dibaca 19 Nov 2026: `apidocs`, `docs/qris.md`, Payments API v3 `payment_requests`). **Sandbox: create, inquiry, dan uji koneksi teruji pada 19 Nov 2026 (key `xnd_development_`). Belum untuk uang nyata.** Hanya QRIS. Kode: `services/api/internal/gateway/xendit`.

Item **BELUM JELAS** tidak ada di dokumentasi yang dibaca. Jangan ditebak; uji di sandbox atau tanya Xendit.

## Kredensial

Panel DEWA, per environment: `secret_key` (`xnd_development_...` untuk sandbox, `xnd_production_...` untuk produksi). Host sama (`https://api.xendit.co`), jadi environment dijaga lewat awalan key (`xenditFromCreds` menolak key yang tidak cocok). Satu akun untuk seluruh platform (`PlatformAccount`).

## Panggilan keluar

Header: Basic auth (secret key sebagai username, password kosong), `api-version: 2024-11-11`.

| Panggilan | Dipakai untuk |
|---|---|
| `POST /v3/payment_requests` | Create. Body: `reference_id` (= payment id EraPay), `type=PAY`, `country=ID`, `currency=IDR`, `request_amount` (integer), `channel_code=QRIS`, `channel_properties.expires_at` (RFC3339 UTC). QR ada di `actions[]` dengan `descriptor=QR_STRING`. Id Xendit: `payment_request_id`, disimpan sebagai `GatewayReference`. |
| `GET /v3/payment_requests/{payment_request_id}` | Inquiry. Butuh id Xendit (39 karakter, `pr-...`), bukan `reference_id`. |
| `POST /v3/payment_requests/{id}/simulate` | Simulasi bayar (test mode). Teramati 19 Nov 2026: QRIS didukung (HTTP 200, `status PENDING`), dan inquiry berikutnya menunjukkan `SUCCEEDED` dalam 3 detik. Nominal `amount` opsional. |

Error: 400 validasi/`INVALID_*`, 401 key salah, 403 `INVALID_MERCHANT_SETTINGS`, 404 `DATA_NOT_FOUND` (id tak dikenal; dipakai uji koneksi), 409 `DUPLICATE_ERROR`, 429, 5xx/503 `CHANNEL_UNAVAILABLE`. Adapter: 408/409/429/5xx = ambigu, 4xx lain = ditolak.

## Batas yang diketahui

- QRIS: nominal 1 sampai 10.000.000; kedaluwarsa dapat diatur, maksimum 48 jam; settlement T+1 hari kerja.
- **Create TIDAK idempoten (teramati di sandbox)**: `reference_id` yang sama membuat payment request kedua dengan id baru, tanpa 409. Dokumen tidak menyebut `idempotency-key` untuk v3. Jadi create tidak boleh diulang setelah hasil ambigu. Create yang timeout tidak menghasilkan id Xendit, sehingga inquiry tidak bisa dilakukan; pembayaran itu tetap ambigu sampai kedaluwarsa di EraPay. Endpoint list per `reference_id` belum terkonfirmasi (halaman `list-payment-requests` 404). Jangan create ulang dengan id baru.
- Refund QRIS penuh/parsial didukung Xendit (7 hari, tergantung issuer), tetapi **tidak diiklankan** adapter. Cancel juga tidak.

## Webhook

`POST` ke URL yang diset di dasbor. Body: `event` (`payment.capture`, `payment.authorization`, `payment.failure`), `business_id`, `created`, `data{payment_id, reference_id, payment_request_id, status, request_amount, ...}`. Header `x-callback-token` (token statis dasbor) hanya membuktikan asal. Retry dan apakah `EXPIRED`/`CANCELED` memicu webhook: **BELUM JELAS**. Belum ada `WebhookVerifier`; status berubah lewat inquiry. Bila nanti dipakai: cocokkan dengan inquiry sebelum memproses, seperti IDS/FINPAY.

## Status

`REQUIRES_ACTION`, `ACCEPTING_PAYMENTS` → pending; `SUCCEEDED` → succeeded; `EXPIRED` → expired; `FAILED` → failed; `CANCELED` → cancelled. `AUTHORIZED` (kartu) tidak dipetakan. Teramati di sandbox: `REQUIRES_ACTION` (setelah create), amount 10000 terbaca benar. `SUCCEEDED` teramati lewat simulate. `EXPIRED`/`FAILED`/`CANCELED` dari dokumentasi, **belum teramati**.

## Sisa gate (`gateway-new.md`)

Poin 1, 2 (kontrak, SLA, rate limit, legal, MDR QRIS: belum dibaca), 4 (contract test: baru unit test; webhook/duplicate/out-of-order menunggu verifier), 5 (settlement/ledger), 6 (flag per merchant).

## Ledger, harga, routing

Selesai 19 Nov 2026 dengan pola FINPAY: akun `cash_xendit_clearing` (`ledger.ClearingAccount`), migrasi `0041_xendit_gateway_ledger_pricing.sql` (paket QRIS = salinan paket DOKU, asumsi aturan yang sama dengan FINPAY; MDR asli Xendit belum dibaca, selisihnya muncul sebagai selisih settlement), `routing.Candidates` dan `onboarding.PlanGateways`. Saklar "ON": kredensial disimpan di panel DEWA, lalu DEWA memilihnya di Gateway → Jalur pembayaran. Bawaan tetap tanpa pilihan.

## Uji

`cd services/api && go test ./internal/gateway/xendit/...`
