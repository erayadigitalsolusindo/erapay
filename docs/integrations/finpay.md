# FINPAY / Finnet (calon adapter `finpay`)

Sumber: https://docs.finpay.id (dibaca 18 Nov 2026) dan uji langsung ke sandbox pada hari yang sama, jalur Core API. Kode: `services/api/internal/gateway/finpay` (create QRIS, inquiry, uji koneksi). **Belum untuk uang nyata**: callback belum diverifikasi, status `PAID`/`EXPIRED` belum teramati, dan gate aktivasi di `docs/prd/gateway-new.md` belum lengkap. Hanya sandbox. Hanya QRIS.

Item bertanda **BELUM JELAS** tidak ada atau bertentangan di dokumentasi. Jangan ditebak di kode; uji di sandbox atau tanyakan ke Finpay.

## Kredensial

Disimpan dari panel DEWA, per environment. Diambil dari dasbor Finpay (Access Keys).

| Kunci | Isi |
|---|---|
| `merchant_id` | Username Basic auth |
| `merchant_key` | Password Basic auth, dan kunci HMAC callback. Secret. |
| `callback_url` | `https://erapay-api.erayadigital.co.id/webhooks/finpay` (host API, pola sama dengan DOKU di `docs/DEPLOY-AAPANEL.md`; bukan host web `erapay.`). FINPAY menolak create tanpa `url.callbackUrl` (400, "Invalid Mandatory Field url"). |

Jangan taruh nilai asli di repo, docs, atau `.env` yang di-commit.

## Host

| Environment | Base URL |
|---|---|
| Sandbox | `https://devo.finnet.co.id` |
| Produksi | `https://live.finnet.co.id` |

Host mengikuti environment (`finpayFromCreds`), jadi key sandbox tidak bisa menembak produksi.

## Panggilan keluar

Header: `Authorization: Basic base64(merchant_id:merchant_key)`, `Content-Type` dan `Accept: application/json`.

| Panggilan | Dipakai untuk |
|---|---|
| `POST /pg/payment/card/initiate` | Create QRIS. Body: `customer` (wajib; adapter mengirim nilai tetap tanpa data pribadi), `order.id` (= payment id EraPay, UUID 36 karakter diterima), `order.amount` (string), `order.description`, `order.timeout` (menit, **minimal 6**, bawaan 1440), `url.callbackUrl` (wajib), `sourceOfFunds.type="qris"`. |
| `GET /pg/payment/card/check/{orderId}` | Inquiry status. |
| `POST /pg/payment/card/refund` | Refund (`order.id`, `order.amount`). **BELUM JELAS** apakah berlaku untuk QRIS; adapter tidak boleh mengiklankan refund sebelum dikonfirmasi. |

Respons create sukses: `responseCode "2000000"`, `stringQr` (payload EMVCo, tidak ada di tabel field tetapi ada di contoh respons), `imageurl`, `expiryLink` (`YYYY-MM-DD HH:MM:SS`, WIB/UTC+7; mengikuti `order.timeout`, terbukti di sandbox), `processingTime`.

Idempotensi terbukti di sandbox: create ulang dengan `order.id` sama menjawab HTTP 409, `responseCode 4090001` ("Duplicate partnerReferenceNo") dan **mengembalikan `stringQr` yang sama**, juga bila amount berbeda. Adapter memperlakukannya sukses. Create yang timeout/5xx tetap tidak pasti: selesaikan dengan inquiry, jangan create ulang dengan id baru.

Inquiry id tak dikenal: HTTP 404, `4040001`. Key salah: HTTP 401, `4010000`. Uji koneksi memakai perbedaan ini (404 = key valid).

Format error dan daftar `responseCode` selain `2000000`: lihat `appendix/enumeration/response-code-list` (belum dibaca).

## Callback masuk

Finpay `POST` ke `url.callbackUrl`.

- Body: `order{id,reference,amount,currency}`, `customer`, `result.payment{amount,status,channel,datetime,userDesc,reference,statusDesc}`, `signature` (hex, 128 karakter).
- Signature: `HMAC-SHA512(merchant_key, json(body tanpa signature))`, hex. **BELUM JELAS**: urutan key dan whitespace JSON. Dokumen memberi contoh PHP `json_encode`. Tangkap callback sandbox nyata sebelum menulis verifikasi, dan verifikasi atas byte yang dikonfirmasi.
- Pengamanan tambahan yang disarankan dokumen: cocokkan dengan inquiry sebelum memproses. Wajib dilakukan (callback tidak mengubah status sendirian), sama seperti IDS.
- Balasan yang diharapkan: JSON `{"responseCode":"2000000","responseMessage":"Success"}`.
- Retry, timestamp anti-replay, dan batas waktu: **BELUM JELAS**.

## Status

Daftar umum: `PAID`, `PENDING`, `EXPIRED`, `FAILURE`, `CANCELLED`, `REFUNDED`, `PARTIALLY_REFUNDED`, plus status kartu (`AUTHORIZED`, `CAPTURED`, dst., tidak relevan untuk QRIS). Contoh respons inquiry hanya memperlihatkan `CAPTURED` (kartu). Yang sudah teramati di sandbox: hanya `PENDING` (amount 0 sampai dibayar). `PAID`, `EXPIRED`, `FAILURE`, `CANCELLED` dipetakan dari dokumentasi dan **belum teramati**; konfirmasi lewat simulator pembayaran sebelum menerima uang nyata.

Pemetaan di adapter: `PAID` → succeeded; `PENDING` → pending; `EXPIRED` → expired; `FAILURE` → failed; `CANCELLED` → cancelled. Status lain (kartu, `REFUNDED`) tidak dipetakan, pembayaran dibiarkan.

## Jalur alternatif

SNAP QRIS (`/finpay-pg/snap/qris`): token dengan tanda tangan RSA, transaksi dengan `HMAC-SHA512` atas `METHOD:path:token:sha256(body):timestamp`. Butuh pasangan kunci RSA (public key dikirim ke Finnet), jadi tidak cukup dengan `merchant_id`/`merchant_key`. Hanya dipakai bila Finpay mewajibkannya.

## Fee, settlement, uji

- MDR QRIS: **0,7% per transaksi** (harga publik di https://www.finpay.id/en/home-en bagian Subscription Fees, dibaca 18 Nov 2026; VA Rp3.500, e-money Rp500-3.500 atau 1,5-2%). Konfirmasi dengan kontrak sebelum paket harga dibuat. Jadwal dan format laporan settlement, batas nominal: **BELUM JELAS** (dokumentasi tidak memuatnya); syarat gate poin 3 dan 5.
- Simulator pembayaran sandbox: https://simulator.finpay.id/qris (langkah dan input tidak dijelaskan di dokumentasi; dipakai manual). Postman collection ada di `developer-tools/postman-collection`.
- IP whitelist: https://docs.finpay.id/documentation/security/ip-whitelist (belum dibaca; mungkin berlaku untuk IP keluar EraPay).

## Sisa gate (`gateway-new.md`)

1. DOKU lulus pilot produksi. 2. Kontrak, SLA, rate limit, review legal. 4. Contract test adapter (success, pending, failure, timeout, invalid signature, duplicate, out-of-order, inquiry). 5. Fee/ledger/rekonsiliasi teruji. 6. Feature flag per gateway dan per merchant, default OFF.

## Status kode dan uji

- Adapter: create, inquiry, uji koneksi. Tanpa `WebhookVerifier` sampai serialisasi signature callback jelas, jadi status berubah hanya lewat inquiry.
- Tes unit: `cd services/api && go test ./internal/gateway/finpay/...`.
- Tes sandbox nyata (dilewati bila env kosong): `FINPAY_SANDBOX_ID=... FINPAY_SANDBOX_KEY=... go test ./internal/gateway/finpay/ -run Live -v`. Ia membuat QR Rp10.000 yang tidak dibayar.
- Untuk menangkap callback asli: set `callback_url` ke URL publik (tunnel) `/webhooks/finpay` dengan `ERAPAY_WEBHOOK_CAPTURE` aktif, bayar lewat simulator, lalu bandingkan signature.

## Sebelum menerima uang nyata

1. ~~Akun clearing sendiri~~ selesai: `cash_finpay_clearing` (`ledger.ClearingAccount`, migrasi `0039_finpay_gateway_ledger_pricing.sql`).
1b. ~~Paket harga~~ selesai (keputusan pemilik, 18 Nov 2026: sama dengan DOKU). Migrasi 0039 menyalin paket QRIS DOKU ke `finpay-qris-<merchant>`, dan merchant yang disetujui kemudian mendapat paket itu lewat `onboarding.PlanGateways`. MDR asli FINPAY (0,7%) berbeda dari MDR paket; selisihnya muncul sebagai selisih settlement, bukan dibukukan.
2. ~~Tambahkan `finpay` ke `routing.Candidates` dan `onboarding.PlanGateways`~~ selesai. Saklar "ON" sekarang: kredensial FINPAY disimpan di panel DEWA (tanpa itu adapter tidak terdaftar) lalu DEWA memilihnya di Gateway → Jalur pembayaran. Bawaan tetap tanpa pilihan, jadi tidak ada transaksi yang berpindah sendiri.
3. Verifikasi callback, konfirmasi status `PAID`/`EXPIRED`, kontrak dan MDR, laporan settlement.
