# EraPay API: panduan integrator

Untuk developer POS / merchant yang memanggil EraPay. Spesifikasi sumber: `docs/prd/api.md`. Contoh memakai `curl`; nilai uang selalu **integer satuan terkecil** (IDR: rupiah, tanpa desimal).

- Base URL: sandbox `https://<host-sandbox>/v1`, produksi `https://<host-produksi>/v1` (terpisah, key tidak lintas lingkungan).
- Format: JSON UTF-8, `Content-Type: application/json`. Waktu RFC 3339 (UTC).
- Setiap respons memuat header `X-Request-Id`; sebutkan saat menghubungi support.

## 1. Autentikasi

| Cara | Dipakai untuk | Header |
|---|---|---|
| **API key** (`epk_sbx_…` / `epk_live_…`) | server POS: buat & baca pembayaran, baca saldo | `Authorization: Bearer <key>` |
| **Sesi dashboard** (cookie) | manusia: kelola API key, webhook, rekening, payout, refund | login di dashboard |

Key dibuat di dashboard → **API Keys**, dengan *scope*: `payment:create`, `payment:read`, `report:read`. Secret key tampil **sekali**. Rotasi dan pencabutan tersedia di dashboard. Key yang bocor: cabut segera.

Operasi uang keluar (payout, refund, rekening bank) **tidak bisa** dilakukan dengan API key, hanya lewat sesi dashboard.

## 2. Idempotency

Wajib untuk operasi yang mengubah uang. Kirim `Idempotency-Key` (8–128 karakter: huruf, angka, `. _ : -`), unik per operasi.

- Key sama + body sama → hasil pertama diulang (`200`, atau status asli).
- Key sama + body beda → `409 IDEMPOTENCY_CONFLICT`.
- Request sama masih berjalan → `409` dengan `retryable: true`; ulangi beberapa detik lagi.
- Timeout/jaringan putus: **ulangi dengan key yang sama**, jangan buat key baru.

## 3. Error

```json
{ "error": { "code": "VALIDATION_ERROR", "message": "…", "request_id": "…", "retryable": false } }
```

| HTTP | `code` | Arti |
|---|---|---|
| 400 | `VALIDATION_ERROR` | body/parameter salah; field tak dikenal ditolak |
| 401 | `UNAUTHORIZED` | key/sesi tidak ada atau salah |
| 403 | `FORBIDDEN`, `MERCHANT_NOT_ACTIVE` | tidak punya izin / merchant belum aktif |
| 404 | `NOT_FOUND`, `PAYMENT_NOT_FOUND` | tidak ada (atau milik merchant lain) |
| 409 | `IDEMPOTENCY_CONFLICT`, `INVALID_STATE` | konflik key / status tidak mengizinkan |
| 422 | `GATEWAY_REJECTED`, `INSUFFICIENT_BALANCE`, `REFUND_EXCEEDS_PAYMENT`, … | aturan bisnis menolak |
| 429 | `RATE_LIMITED` | terlalu banyak percobaan |
| 5xx | `INTERNAL_ERROR`, `GATEWAY_TIMEOUT` | `retryable: true` → ulangi dengan key yang sama |

## 4. Pembayaran (QRIS)

### Buat pembayaran
```bash
curl -X POST $BASE/payments \
  -H "Authorization: Bearer $KEY" -H "Idempotency-Key: ord-1001-try1" \
  -H "Content-Type: application/json" \
  -d '{"merchant_reference":"ORD-1001","amount":100000,"currency":"IDR","payment_method":"QRIS","description":"Kopi"}'
```
Body hanya boleh berisi `merchant_reference`, `amount`, `currency`, `payment_method`, `description`, `outlet_id`, `expires_at`. **Fee, status, gateway, dan merchant tidak bisa dikirim**: ditentukan server.

Respons `201`:
```json
{ "id":"…", "merchant_reference":"ORD-1001", "amount":100000, "currency":"IDR", "payment_method":"QRIS",
  "status":"pending", "settlement_status":"not_expected",
  "fee":{"bearer":"merchant","mdr_fee":700,"platform_fee":1000,"merchant_net":98300},
  "qr_code":{"type":"provider_defined","value":"…"}, "expires_at":"…", "refunded_amount":0, "refunding_amount":0 }
```
Tampilkan `qr_code.value` sebagai QR. Pelanggan membayar **nominal transaksi tanpa biaya tambahan**; `fee` adalah potongan atas merchant.

`202` + `status:"unknown"` = gateway tidak menjawab pasti. **Bukan gagal**: jangan buat pembayaran baru; tunggu webhook atau baca ulang statusnya.

### Status
`GET /payments/{id}` (scope `payment:read`). Tiga status **terpisah**:

| Field | Nilai | Catatan |
|---|---|---|
| `status` | `created, pending, succeeded, failed, expired, cancelled, unknown` | final: `succeeded, failed, expired, cancelled`. Hanya berubah dari gateway tervalidasi. |
| `settlement_status` | `not_expected, expected, settled` | `settled` = dana sudah diterima EraPay dari gateway |
| `refunded_amount` / `refunding_amount` | integer | refund selesai / sedang diproses; **tidak** mengubah `status` |

Jangan menandai pesanan lunas dari layar/redirect pelanggan; gunakan `status == "succeeded"` dari API atau webhook.

### Daftar transaksi
`GET /payments?status=&merchant_reference=&from=&to=&limit=&cursor=` (scope `payment:read`). Terbaru lebih dulu, maks 100 per halaman (default 25). `from` dan `to` berupa tanggal `YYYY-MM-DD` (hari WIB, `to` inklusif) atau RFC 3339 (`to` eksklusif). `merchant_reference` dicocokkan persis. Bila ada halaman berikutnya, respons memuat `next_cursor`: kirim apa adanya sebagai `cursor` (nilai opak; tetap stabil walau ada transaksi baru masuk). Kode QR tidak pernah ada di daftar; ambil lewat `GET /payments/{id}` selama status masih `pending`.

### Ringkasan periode
`GET /reports/summary?from=YYYY-MM-DD&to=YYYY-MM-DD` (scope `report:read`; default 30 hari terakhir, maks 366 hari). Berisi `totals` (jumlah per status; `gross`, `mdr_fee`, `platform_fee`, `merchant_net` untuk transaksi berhasil; dan `refunded`) serta deret harian `daily` (dihitung pada hari transaksi berhasil, zona `Asia/Jakarta`). Selalu berlaku `gross = mdr_fee + platform_fee + merchant_net`.

## 5. Saldo

`GET /balance` (scope `report:read`, atau sesi dashboard):
```json
{ "currency":"IDR", "payable":983000, "pending":0, "available":983000, "in_payout":0, "on_hold":0 }
```
`payable` = kewajiban EraPay ke merchant; `pending` = belum settle dari gateway; `available` = `payable − pending` (boleh di-payout); `in_payout` = sedang diproses payout; `on_hold` = tertahan untuk refund.

## 6. Webhook ke server Anda

Daftarkan endpoint di dashboard → **Webhook** (HTTPS publik; IP privat/loopback, redirect, dan kredensial di URL ditolak). Secret `whsec_…` tampil sekali.

### Event
`payment.succeeded`, `payment.failed`, `payment.expired`, `payment.cancelled`, `refund.refunded`, `refund.failed`, `refund.cancelled`, `payout.paid`, `payout.failed`, `payout.cancelled`.

```json
{ "id":"<event id>", "type":"payment.succeeded", "created_at":"…",
  "data":{ "payment":{ "id":"…","merchant_reference":"ORD-1001","amount":100000,"currency":"IDR",
           "payment_method":"QRIS","status":"succeeded","final_at":"…",
           "fee":{"bearer":"merchant","mdr_fee":700,"platform_fee":1000,"merchant_net":98300} } } }
```
Header: `X-EraPay-Signature`, `X-EraPay-Event-Id`, `X-EraPay-Delivery-Attempt`.

### Verifikasi tanda tangan (wajib)
`X-EraPay-Signature: t=<unix>,v1=<hex>` dengan `v1 = HMAC-SHA256(secret, "<t>." + raw_body)`. Hitung ulang atas **raw body** (sebelum di-parse), bandingkan *constant-time*, dan tolak jika `t` lebih dari 5 menit dari waktu Anda.

```js
// Node.js
import crypto from "node:crypto";
export function verify(secret, header, rawBody, toleranceSec = 300) {
  const p = Object.fromEntries(header.split(",").map(s => s.trim().split("=")));
  if (Math.abs(Date.now() / 1000 - Number(p.t)) > toleranceSec) return false;
  const mac = crypto.createHmac("sha256", secret).update(`${p.t}.`).update(rawBody).digest("hex");
  return p.v1 && p.v1.length === mac.length && crypto.timingSafeEqual(Buffer.from(mac), Buffer.from(p.v1));
}
```
```php
// PHP
function verify(string $secret, string $header, string $raw, int $tol = 300): bool {
  parse_str(str_replace(',', '&', $header), $p);
  if (!isset($p['t'], $p['v1']) || abs(time() - (int)$p['t']) > $tol) return false;
  return hash_equals(hash_hmac('sha256', $p['t'] . '.' . $raw, $secret), $p['v1']);
}
```
Go: lihat `Verify` di `services/api/internal/outbound/sign.go`.

### Pengiriman
- Balas **HTTP 2xx** secepatnya (proses berat secara asinkron). Selain 2xx, redirect, atau timeout (10 dtk) dianggap gagal.
- Gagal → diulang dengan jeda 30 dtk, 1, 2, 4, … hingga maks 6 jam (±20%), total 8 percobaan, lalu *dead-letter*. Kirim ulang manual dari dashboard (riwayat pengiriman).
- **At-least-once**: event yang sama bisa datang lebih dari sekali. Simpan `id` event dan abaikan duplikat.
- Urutan tidak dijamin; gunakan `data.*.status` dan `final_at`, bukan urutan kedatangan.

## 7. Payout dan refund (dashboard)

Dikelola owner merchant di dashboard; disetujui dan dieksekusi **Finance EraPay** (maker ≠ checker). Dana payout hanya dari saldo `available` (sudah settle). Status payout: `pending_approval → approved → paid`, atau `failed / cancelled`. Status refund: `pending_approval → approved → refunded`, atau `failed / cancelled`. Pantau lewat webhook `payout.*` dan `refund.*`.

API (sesi dashboard, `Idempotency-Key` wajib): `POST /bank-accounts`, `POST /payouts`, `POST /payouts/{id}/cancel`, `GET /payouts[/{id}]`, `POST /payments/{id}/refund`, `POST /refunds/{id}/cancel`, `GET /refunds[/{id}]`. Rekening baru harus diverifikasi Finance dan melewati masa tunggu sebelum bisa dipakai.

## 8. Sandbox

Gunakan key `epk_sbx_…`. Payout di luar produksi adalah *dry run*: seluruh alur dan pembukuan berjalan, tetapi tidak ada transfer nyata.
