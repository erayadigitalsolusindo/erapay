# API [api] (PRD §5)

Baca juga: `../PRD.md`, `payment.md`, `data.md`.

**Prinsip:** base URL terpisah sandbox/prod; `/v1`; JSON UTF-8 HTTPS; ID opaque; idempotency wajib untuk create payment & operasi finansial yang bisa di-retry; format error/pagination (cursor)/timestamp/correlation ID konsisten; tanpa stack trace di respons publik. Endpoint admin/dashboard terpisah, auth user (bukan API key POS).

```
POST /v1/payments                        GET /v1/payments/{id}
GET  /v1/payments?merchant_reference=&status=&from=&to=&cursor=
POST /v1/payments/{id}/cancel   [cap]    POST /v1/payments/{id}/refund [cap]
GET  /v1/refunds/{id}
POST|GET /v1/webhook-endpoints           POST /v1/webhook-endpoints/{id}/rotate-secret
GET  /v1/webhook-deliveries              GET /v1/merchant/profile
GET  /v1/reports/transactions            GET /v1/reports/settlements
POST /webhooks/{gateway}                 # inbound dari gateway (bukan API publik POS)
GET  /v1/balance                         GET /v1/payouts[/{id}]
POST /v1/payouts                         # on-demand, jika diaktifkan
```
`[cap]` = hanya jika `Capabilities()` gateway aktif mendukung.

**Gateway di API publik:** POS tidak memilih gateway; EraPay memilih dari konfigurasi merchant. Respons boleh memuat `gateway` (kode, informasional). Payload QR tetap `provider_defined`. Webhook outbound & error EraPay tidak membocorkan format gateway.

**Create payment (konseptual, bukan kontrak gateway):** header `Authorization: Bearer <key>`, `Idempotency-Key`, opsional `X-Correlation-ID`.
Body: `{merchant_reference, amount:100000, currency:"IDR", payment_method:"QRIS", description, expires_at, metadata:{outlet_code}}`.
Respons: `{id, merchant_reference, amount (=gross_amount), fee:{bearer, mdr_fee, platform_fee, merchant_net}, currency, payment_method, gateway, status:"pending", qr_code:{type:"provider_defined", value:<dari gateway>}, expires_at, created_at}`.

**Error:** `{error:{code, message, request_id, retryable}}`.
Kode: `VALIDATION_ERROR, UNAUTHORIZED, FORBIDDEN, MERCHANT_NOT_ACTIVE, PAYMENT_METHOD_UNAVAILABLE, IDEMPOTENCY_CONFLICT, PAYMENT_NOT_FOUND, GATEWAY_TIMEOUT, GATEWAY_REJECTED, PAYMENT_STATUS_UNKNOWN, RATE_LIMITED, INTERNAL_ERROR`.

**Idempotency:** scope min. merchant + operasi; simpan key, hash request kanonik, resource ID, status proses, expiry. Key sama+request sama → hasil pertama; key sama+payload beda → `IDEMPOTENCY_CONFLICT`. Unique constraint di PostgreSQL. Crash setelah gateway terima tapi sebelum respons tersimpan → inquiry/recovery dulu sebelum ulang create.
