# Midtrans (calon adapter `midtrans`)

Sumber: https://docs.midtrans.com (dibaca 19 Nov 2026: `quick-start`, `getting-started-with-snap`, `backend-integration`, `api-authorization-headers`, `api-headers`, `charge-transactions-1`, `qris`, `get-transaction-status`, `transaction-status-cycle`, `cancel-transaction`, `refund-transaction`, `https-notification-webhooks`, `testing-payment-on-sandbox`, `switching-to-production-mode`). **Adapter ada (QRIS: create, inquiry, uji koneksi, verifikasi webhook); sandbox Core API teruji create/inquiry/cancel (19 Nov 2026); webhook asli, bayar via simulator, expire, refund belum.** Hanya QRIS lewat Core API (bukan Snap: Snap butuh popup JS di browser, tidak cocok untuk adapter server-side). Kode nanti: `services/api/internal/gateway/midtrans`.

Item **BELUM JELAS** tidak ada di dokumentasi yang dibaca. Jangan ditebak; uji di sandbox atau tanya Midtrans.

## Kredensial

Panel DEWA, per environment: `server_key` (Dashboard > Settings > Access Keys) dan `signature_key_secret` (lihat "Teramati di sandbox"). Key sandbox dan produksi berbeda. `client_key` tidak dipakai (hanya untuk frontend/token kartu). Awalan key (`SB-Mid-server-...` untuk sandbox) hanya contoh di dokumen, **BELUM JELAS** apakah produksi punya awalan tetap; jangan jadikan satu-satunya penjaga environment, pilih host dari field environment.

**Core API di produksi harus diajukan aktivasi** ke Midtrans (sandbox aktif otomatis). Ini penghalang go-live.

## Host

| Environment | Core API | Snap (tidak dipakai) |
|---|---|---|
| Sandbox | `https://api.sandbox.midtrans.com` | `https://app.sandbox.midtrans.com` |
| Produksi | `https://api.midtrans.com` (tersirat dari URL QR dan halaman go-live; **tidak tertulis eksplisit untuk path `/v2`**, konfirmasi saat uji) | `https://app.midtrans.com` |

## Panggilan keluar

Header: `Authorization: Basic base64(server_key + ":")`, `Content-Type: application/json`, `Accept: application/json`. Opsional `Idempotency-Key` (lihat bawah).

| Panggilan | Dipakai untuk |
|---|---|
| `POST /v2/charge` | Create QRIS. Body: `payment_type=qris`, `transaction_details{order_id, gross_amount}` (integer IDR; `order_id` hanya `A-Za-z0-9 - _ ~ .`, unik seumur hidup merchant), `qris.acquirer` (opsional, mis. `gopay`), `custom_expiry{expiry_duration, unit}` (unit `minute`, QRIS maks 5 hari untuk acquirer AirPay/Shopee). Respons sukses: `status_code "201"`, `transaction_status pending`, `transaction_id` (disimpan sebagai `GatewayReference`), `qr_string`, `actions[]` (`generate-qr-code`, `generate-qr-code-v2` = URL GET gambar PNG), `acquirer`, `expire_time`. |
| `GET /v2/{order_id atau transaction_id}/status` | Inquiry. Bisa pakai `order_id` kita (= payment id EraPay), jadi create yang timeout tetap bisa diperiksa (beda dengan Xendit). |
| `POST /v2/{order_id atau transaction_id}/cancel` | Batalkan transaksi pending (QRIS didukung). 200 `transaction_status=cancel`; 412 tidak bisa diubah. |
| `POST /v2/{id}/refund` | Refund QRIS setelah `settlement`. Body opsional: `refund_key` (cegah double refund), `amount`, `reason` (maks 255). Batas: GoPay QRIS on-us 45 hari, off-us 7 hari. Error 412, 414 nominal salah, 406 refund_key duplikat. Retry `refund_key` sama hanya sampai 7 hari. |

Catatan: `qr_string` di respons contoh QRIS ada pada halaman charge tetapi tidak di halaman `qris`; pastikan di sandbox. `expire_time` vs `expiry_time` beda ejaan antar halaman: **BELUM JELAS**, baca keduanya.

Error: 400 validasi (`validation_messages[]`), 401 key salah, 406 `order_id` duplikat, 412 status tak bisa diubah, 429 rate limit, 500 internal. Usul klasifikasi adapter: 408/429/5xx/202 = ambigu, 406 = duplikat (inquiry dulu), 4xx lain = ditolak.

## Idempotency

Header `Idempotency-Key` (maks 46 karakter, UUID disarankan) berlaku untuk semua POST kecuali `/token` dan `/account`. Hanya respons sukses disimpan, masa berlaku **5 menit**; key sama dalam 5 menit = respons sama apa pun body-nya; retry saat request pertama masih jalan dibalas HTTP **202**. Setelah 5 menit, order_id sama bisa kena konflik 406. Pakai `Idempotency-Key` = payment id EraPay, dan tetap anggap lewat 5 menit sebagai ambigu: lakukan inquiry via `order_id`, bukan create ulang. Unik `order_id` adalah pengaman utama.

## Webhook

`POST` ke URL yang diset di dasbor (Settings > Configuration > Payment Notification URL), atau per transaksi lewat header charge `X-Override-Notification` / `X-Append-Notification` (maks 3 URL, pisah koma). Syarat endpoint: publik, port 80/443, TLS maks 1.2 diterima, **tanpa header kustom** (Midtrans tidak bisa mengirim Basic Auth/API key), balas **2xx** (dalam 5 detik, timeout 15 detik).

Retry: 2xx tidak retry; 500 sekali; 503 empat kali; 400/404 dua kali; 301/302/303 tidak; 307/308 ikuti maks 5; lainnya lima kali. Jeda sekitar 2, 10, 30, 90, 210 menit. IP Midtrans: halaman `docs.midtrans.com/docs/ip-address` tidak dibaca (**BELUM JELAS**).

Body (field utama): `transaction_time`, `transaction_status`, `transaction_id`, `status_message`, `status_code`, `signature_key`, `payment_type`, `order_id`, `merchant_id`, `gross_amount` (string, mis. `"10000.00"`), `fraud_status`, `currency`, `settlement_time`, `expiry_time`, untuk QRIS juga `issuer`, `acquirer`, `transaction_type` (`on-us`/`off-us`), `reference_id`, `shopeepay_reference_number`. Refund: dua notifikasi, pakai yang punya `bank_confirmed_at`.

**Verifikasi**: `signature_key == hex(SHA512(order_id + status_code + gross_amount + server_key))`, memakai `gross_amount` **persis string dari body** (jangan diformat ulang dari integer). Bandingkan constant-time. Karena signature tidak mengikat `transaction_status`, **tetap cocokkan dengan GET status sebelum memproses** (rekomendasi Midtrans juga), seperti IDS/FINPAY. Rekonsiliasi: GET status jika notifikasi belum datang 12/24 jam atau ada klaim dana terpotong.

## Status

| Midtrans | EraPay |
|---|---|
| `pending` | pending |
| `settlement` (`status_code 200`, `fraud_status accept` jika ada) | succeeded |
| `expire` | expired |
| `cancel` | cancelled |
| `deny`, `failure` | failed |
| `refund`, `partial_refund` | tidak dipetakan ke status payment (jalur refund) |
| `chargeback`, `partial_chargeback`, `capture`, `authorize` | tidak relevan QRIS / tidak dipetakan |

Transisi: `pending → settlement|expire|cancel|deny`; `settlement → refund|partial_refund|chargeback`. `settlement → deny` (reversal) hanya terdokumentasi untuk Permata VA/Mandiri Bill/Indomaret, bukan QRIS. `status_code` notifikasi: 200 sukses, 201 pending, 202 expire/cancel/deny (dari halaman QRIS dan cancel; daftar lengkap **BELUM JELAS**).

## Sandbox

Simulasi bayar: salin URL gambar QR (`generate-qr-code`) ke https://simulator.sandbox.midtrans.com/v2/qris/index (akun sebelum 4 Nov 2024: `/qris/index`). **Jangan bayar QR sandbox dengan uang nyata**; QR sandbox bisa menunjuk merchant produksi (order `QRIS-...`). Tidak ada API simulate seperti Xendit, jadi contract test otomatis butuh webhook buatan sendiri (hitung signature dari key uji).

## Teramati di sandbox Core API (19 Nov 2026, key `SB-Mid-server-...`)

Diuji: cek key, charge QRIS, status by `order_id`, charge ulang, cancel, status ulang. Pembayaran lewat simulator, webhook, expire, refund **belum diuji**.

- **HTTP selalu 200**, termasuk untuk 404 dan 406. Kode sebenarnya ada di body `status_code` (string). Adapter wajib membaca `status_code`, bukan HTTP status. Kegagalan jaringan/5xx/timeout tetap ambigu.
- Cek key: `GET /v2/{order_id_asal}/status` → body `404 "Transaction doesn't exist."` = key valid (key salah → `401`). Pola sama dengan uji koneksi xendit.
- Create QRIS (`acquirer=gopay`, `custom_expiry 15 menit`): body `status_code "201"`, `transaction_status pending`, `fraud_status accept`, `gross_amount "10000.00"` (string dua desimal), `transaction_id` UUID, `qr_string` (ada), `expiry_time` (**bukan** `expire_time`), `actions[]` = `generate-qr-code` (`api.sandbox.../v2/qris/{id}/qr-code`) dan `generate-qr-code-v2` (`merchants-app.sbx.../v4/qris/gopay/...`). `merchant_id` ikut di respons.
- Status by `order_id` kita berfungsi untuk transaksi pending: `status_code "201"`, `transaction_status pending`.
- **Charge ulang `order_id` sama** (tanpa Idempotency-Key): body `406` "conflict with the current state". Jadi `order_id` unik = pengaman duplikat; saat 406 lakukan inquiry, jangan buat ulang.
- Cancel pending: body `status_code "200"`, `transaction_status cancel`; status sesudahnya `cancel`.
- **`signature_key` terkonfirmasi (respons GET status, pending dan cancel)**: `hex(SHA512(order_id + status_code + gross_amount + KEY))` dengan `gross_amount` string persis (`"10000.00"`) dan `status_code` dari body (`201` pending, `200` cancel). **KEY-nya bukan server key sandbox `SB-Mid-server-...`**, melainkan key terpisah berawalan `Mid-server-...` (pemilik menyebutnya "signature secret key"; apakah itu server key produksi atau key khusus signature: **BELUM JELAS**, tanyakan ke Midtrans/dasbor). Konsekuensi: kredensial adapter butuh **dua** secret, `server_key` (Basic auth) dan `signature_key_secret` (verifikasi webhook). Jangan pakai satu untuk yang lain. Webhook nyata (notifikasi) belum diuji; asumsi: memakai KEY yang sama.

### Uji end-to-end lokal lewat API EraPay (19 Nov 2026, sandbox)

`POST /v1/payments` (merchant uji) → gateway `midtrans`, QR terbentuk, fee 70 + 30, net 9.900. Bayar lewat simulator (https://simulator.sandbox.midtrans.com/v2/qris/index): tempel `https://api.sandbox.midtrans.com/v2/qris/{transaction_id}/qr-code`, lalu di halaman berikut **pilih `issuer=gopay`** (tanpa issuer: "QOA-101 issuer not supported") dan kirim `journeyId`, `referenceId`, `exploreData`, `issuer`. Midtrans → `settlement` (`settlement_time` WIB, `fraud_status accept`); `POST /v1/payments/{id}/inquiry` mengubah payment ke `succeeded`; jurnal `Dr cash_midtrans_clearing 9930 / Cr merchant_payable 9900 / Cr platform_fee_revenue 30`, seimbang. `GET /v1/payments/{id}` hanya membaca, tidak memicu inquiry. Order di Midtrans = payment id EraPay (bukan `merchant_reference`).

Webhook buatan sendiri (signature dihitung dengan `signature_key` tersimpan): signature salah → 400; `status_code 201` + `settlement` → 200 tanpa efek; nominal beda dari payment → 200 tanpa efek; sah → `succeeded`; kirim ulang → 200 tanpa jurnal ganda. Ini membuktikan kode, **bukan** bahwa notifikasi asli Midtrans memakai key dan body yang sama: itu menunggu satu notifikasi asli di URL publik.

## Alternatif: BI-SNAP Core API (standar Bank Indonesia, HMAC/RSA)

Dibaca 19 Nov 2026: `core-api-snap-open-api-overview`, `getting-started-1`, `credential-exchange`, `security-specification`, `signature-generation`, `bi-snap-security-architecture`, `access-token-api`, `mpm-api-qris`, `get-transaction-status-api`, `cancel-api`, `refund-api`, `payment-notification-api`, `testing-bi-snap-on-sandbox-environment`, `ip-address`, `code-4xx`. Bukan "Snap" hosted checkout. **Belum teruji; belum dibaca: `register-ip-address`, `setting-up-notification-url`, `merchants-migration`.** Aktivasi manual lewat sales PIC Midtrans.

### Onboarding
Merchant membuat RSA 2048+ PKCS#8 PEM, mendaftarkan public key di Dashboard > Settings > Access Keys > Payment BI SNAP (**sandbox dulu**, baru produksi), memberi IP keluar (hanya IP terdaftar boleh memanggil), notification URL, dan MID. Midtrans memberi: `ClientID` (= `X-CLIENT-KEY`), `ClientSecret` (HMAC), `PartnerID` (`X-PARTNER-ID`), `ChannelID` (5 digit), public key Midtrans (verifikasi notifikasi; cocokkan fingerprint lewat saluran terpisah). Alternatif kirim: ZIP berpassword lewat email.

### Host
Sandbox: `https://merchants.sbx.midtrans.com` (Get Auth Code: `merchants-app.sbx.midtrans.com`). Produksi: domain `merchants.midtrans.com` (daftar domain di halaman IP), **tersirat, tidak tertulis eksplisit untuk path API**. Whitelist keluar berbasis domain, bukan IP (IP API sering berubah).

### Token
`POST /v1.0/access-token/b2b`; header `X-TIMESTAMP` (ISO-8601 `+07:00`), `X-CLIENT-KEY`, `X-SIGNATURE` = Base64(SHA256withRSA(privkey, `clientKey|timestamp`)); body `{"grantType":"client_credentials"}`. Respons `accessToken`, `tokenType Bearer`, `expiresIn "900"`. Error 401 `4017300`. Cache token, perbarui sebelum 900 detik.

### QRIS (GoPay / `AIRPAY SHOPEE`)
Semua POST, header: `Content-Type`, `X-TIMESTAMP`, `X-SIGNATURE`, `Authorization: Bearer`, `X-PARTNER-ID`, `X-EXTERNAL-ID`, `CHANNEL-ID`.
`X-SIGNATURE` = Base64(HMAC-SHA512(clientSecret, `METHOD:path:accessToken:lower(hex(SHA256(minify(body)))):timestamp`)), token tanpa prefix "Bearer". Aturan minify: **BELUM JELAS** (kirim body yang sudah compact dan hash byte yang persis dikirim).

| Operasi | Path (service code) | Catatan |
|---|---|---|
| Generate | `/v1.0/qr/qr-mpm-generate` (47) | Body: `partnerReferenceNo` (maks 36, unik 3 bulan, **harus = `X-EXTERNAL-ID`**), `amount{value:"12345.00",currency:"IDR"}`, `merchantId`, `validityPeriod` (ISO-8601; min 20 detik, maks 180 hari, default 15 menit), `additionalInfo.acquirer` (`GOPAY` default / `AIRPAY SHOPEE`). Respons sukses `2004700`: `referenceNo` (id Midtrans), `qrContent`, `qrUrl`, `qrImage` (base64). |
| Query | `/v1.0/qr/qr-mpm-query` (51) | Body: salah satu `originalReferenceNo` / `originalPartnerReferenceNo` / `originalExternalId`, plus `serviceCode "47"`, `merchantId`. Respons `2005100`, `latestTransactionStatus`: 00 sukses, 03 pending, 04 refunded, 05 cancel, 06 gagal, 07 not found, 08 expiry, 09 rejected. |
| Cancel | `/v1.0/qr/qr-mpm-cancel` (77) | `merchantId` dan `reason` wajib. Tabel kode tidak ada; sukses contoh `2007700`. |
| Refund | `/v1.0/qr/qr-mpm-refund` (78) | `merchantId`, `originalPartnerReferenceNo`, `partnerRefundNo` (= `X-EXTERNAL-ID`) wajib. Airpay Shopee: hanya refund penuh, jam 06:00-23:55 WIB. Window refund: error `4057801`, durasi **BELUM JELAS**. |

Kode respons = HTTP(3) + service(2) + kasus(2). Generate: `2004700` 200, `2024700` 202 masih diproses (ambigu), `4094700` 409 `X-EXTERNAL-ID` sudah dipakai (duplikat: query, jangan buat ulang), `4034702` melebihi limit, `4034703` fraud, `4044708` merchant tak valid, `5004701/5004702` 500 (ambigu), `5044700` 504 timeout issuer (ambigu), 401x/400x = ditolak. Untuk timeout: ulang dengan `X-EXTERNAL-ID` dan body **sama**, atau query. Idempotensi lebih kuat daripada Core API v2.

### Notifikasi
Midtrans `POST /v1.0/qr/qr-mpm-notify` (service 52) ke URL kita. Header: `X-TIMESTAMP`, `X-SIGNATURE` (SHA256withRSA oleh private key Midtrans), `X-PARTNER-ID`, `X-EXTERNAL-ID` (numerik, idempotency 1 hari). `stringToSign` = `METHOD:EndpointUrl:lower(hex(SHA256(minify(body)))):timestamp`; format `EndpointUrl` (path relatif/penuh): **BELUM JELAS**. Verifikasi atas seluruh payload (Midtrans bisa menambah field di `additionalInfo`). Body: `originalReferenceNo`, `originalPartnerReferenceNo`, `latestTransactionStatus`, `amount`, `additionalInfo{refundHistory,qris}`. Balas HTTP 200 body `{"responseCode":"2005200","responseMessage":"..."}`; `2025200` (202) = masih diproses; selain HTTP 200 dianggap error. Retry 2/10/30/90/210 menit, ada circuit breaker. Whitelist masuk: daftar IP notifikasi di `docs/ip-address` (30 IP produksi baru + legacy, 30 sandbox); jangan jadikan satu-satunya bukti keaslian.

### Sandbox
Simulator QRIS: https://simulator.sandbox.midtrans.com/openapi/qris/index (tempel `qrUrl`, "Scan QR"). Kredensial BI-SNAP produksi berbeda dari sandbox; sandbox memakai MID yang sama dengan produksi.

### Perbandingan singkat dengan Core API v2
Lebih kuat: signature RSA dua arah, HMAC di request, idempotensi `X-EXTERNAL-ID`, query tanpa ambigu id, cancel/refund QRIS terdokumentasi. Lebih berat: key pair + credential exchange manual, token 15 menit, IP whitelist, aktivasi sales, hanya GoPay/Shopee QRIS yang terdokumentasi, aturan minify belum jelas. **Rekomendasi: Core API v2 untuk adapter pertama (sandbox bisa dicoba hari ini dengan server key); BI-SNAP bila Midtrans mewajibkan, atau setelah sales mengaktifkan dan aturan minify/path terkonfirmasi di sandbox.**

## Batas yang belum diketahui

MDR QRIS, nominal min/maks QRIS, settlement (T+?), rate limit angka, SLA, kontrak/legal (gate 2). Expiry default QRIS bila `custom_expiry` kosong. Daftar IP webhook. Apakah `qris.acquirer` wajib di produksi.

## Kode, ledger, harga, routing

Selesai 19 Nov 2026 dengan pola Xendit/FINPAY: `internal/gateway/midtrans` (create, inquiry by `order_id` = payment id, `TestConnection`, `VerifyWebhook`; cancel/refund tidak diiklankan), skema kredensial `midtrans` (`server_key`, `signature_key`), akun `cash_midtrans_clearing` (`ledger.ClearingAccount`), migrasi `0042_midtrans_gateway_ledger_pricing.sql` (paket QRIS = salinan paket DOKU; MDR asli Midtrans belum dibaca), `routing.Candidates`, `onboarding.PlanGateways`. Bawaan tetap tanpa pilihan: kredensial disimpan di panel DEWA, lalu DEWA memilihnya di Gateway > Jalur pembayaran.

Keputusan desain: webhook hanya dipercaya sukses bila `status_code` bertanda tangan = `200`, `transaction_status = settlement`, dan `fraud_status` kosong/`accept` (signature tidak mengikat `transaction_status`). Create memakai `Idempotency-Key` = payment id (replay dalam 5 menit teramati mengembalikan jawaban pertama); lewat itu, `order_id` ganda = body 406 = Ambiguous, selesaikan lewat inquiry. Inquiry 404 = Ambiguous (create bisa masih berjalan). `server_key` harus berawalan `SB-` di sandbox dan tanpa `SB-` di produksi (ditegakkan `midtransFromCreds`); `signature_key` tidak dicek awalannya.

## Uji

`cd services/api && go test ./internal/gateway/midtrans/...`; live: `MIDTRANS_SANDBOX_KEY=SB-Mid-server-... go test -run TestLiveSandbox ./internal/gateway/midtrans/` (menolak key non-`SB-`).

## Sisa gate (`gateway-new.md`)

Poin 1 (dokumen ini; webhook asli/simulator belum), 2 (kontrak, SLA, MDR QRIS: belum dibaca), 4 (contract test: unit test untuk signature salah/ubah isi/status tak dikenal ada; duplikat dan out-of-order ikut mesin status payment, belum ada tes end-to-end dengan webhook asli), 5 (settlement/ledger: akun dan harga selesai, laporan settlement belum), 6 (flag per merchant: bawaan tanpa pilihan, DEWA yang menyalakan).
