# IDS Payment Service (adapter `ids`)

Sumber: tutorial integrasi di Operator Dashboard IDS (disimpan 17 Nov 2026). Kontrak yang berlaku adalah `/v1/api/*` dan callback bertanda tangan. Kode: `services/api/internal/gateway/ids`.

EraPay adalah satu `income_channel` IDS. IDS membungkus DOKU. Hanya QRIS yang dipakai; VA, cancel, refund, dan payout tidak diiklankan oleh adapter. Keputusan pemilik: 28 Okt 2026 (lihat `docs/prd/gateway-new.md`).

## Kredensial

Disimpan dari panel DEWA (tanpa fallback env), per environment:

| Kunci | Isi |
|---|---|
| `base_url` | Host API IDS (`https://<host>`), bukan host dashboard |
| `api_key` | `ids_<env>_<income_channel_id>_<secret>`, tampil sekali saat dibuat atau dirotasi |
| `callback_secret` | Kunci HMAC callback (min 8 karakter), sama dengan yang diisi di Channels IDS |

`income_channel_id` diambil dari `api_key`. `callback_url` channel di IDS harus HTTPS dan terjangkau dari internet.

## Panggilan keluar

Semua memakai `Authorization: Bearer <api_key>`.

| Panggilan | Dipakai untuk |
|---|---|
| `POST /v1/api/payments` | Create QRIS. Idempoten pada `(income_channel_id, income_channel_transaction_id)`; transaction id = payment id EraPay. Body: `payment_method=qris`, `currency=IDR`, `amount` (integer), `expiry_seconds`. |
| `POST /v1/api/payments/status?income_channel_transaction_id=` | Meminta IDS menanyakan DOKU, lalu menerapkan PAID/EXPIRED. Cooldown 429 (`Retry-After`, default 15 dtk). Dipanggil best-effort sebelum GET. |
| `GET /v1/api/payments?income_channel_transaction_id=` | Membaca state IDS (tidak menanyakan DOKU). |
| `GET /v1/api/payment-methods` | Dipakai uji koneksi. |

Respons create berisi `payment.qr_string` (payload EMVCo mentah, bukan gambar), `total_payment` (= `amount`, tanpa biaya tambahan untuk pembayar), dan `expires_at`. EraPay merender `qr_string` menjadi QR di UI.

### Klasifikasi hasil

- 2xx: sukses.
- 409, 408, 429, 5xx (termasuk 502 saat `CREATING`): hasil tidak pasti. Selesaikan dengan inquiry, jangan create ulang dengan id baru.
- 4xx lain: ditolak. 502 dengan status `FAILED`: gunakan transaction id baru.

## Callback masuk

IDS mengirim `POST` ke `callback_url` saat status benar-benar berubah (outbox, retry dengan backoff, lalu DLQ).

- Header: `X-IDS-Timestamp` (detik unix, per percobaan kirim) dan `X-IDS-Signature` = hex `HMAC-SHA256(callback_secret, "{ts}.{raw body}")`.
- Tolak bila `|now - ts| > 300` detik. Verifikasi atas raw body.
- Dedup per `(payment_id, status)`; status tidak boleh mundur.
- Saat `PAID`, jumlah yang dibandingkan adalah `paid_amount`. Jika `amount_mismatch` true, rekonsiliasi memakai `paid_amount`.
- `net_amount` = dibayar dikurangi `merchant_fee` (estimasi MDR DOKU). Adapter belum mencatat kedua nilai ini.
- Balas 2xx cepat setelah diproses durable; non-2xx memicu backoff.

## Status

`CREATING` (internal) → `PENDING` → `PAID`, atau `EXPIRED` / `FAILED`. `SETTLED` dicadangkan dan tidak dikirim untuk QRIS. `CREATING` dan `SETTLED` sengaja tidak dipetakan, sehingga pembayaran dibiarkan apa adanya.

`PAID` hanya berarti IDS sudah mengonfirmasi ke DOKU; notifikasi saja tidak mengubah status.

## Uji

Tidak ada endpoint simulasi untuk channel; pembayaran uji harus scan QRIS sungguhan dengan nominal kecil. Beberapa detik `PENDING` setelah scan itu normal, bukan kegagalan.

Tes unit adapter: `cd services/api && go test ./internal/gateway/ids/...`.
